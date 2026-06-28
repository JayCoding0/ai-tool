// Package application 应用服务层
// eval_service.go — Agent 评估体系：数据集/用例管理 + 批量运行 + LLM-as-judge 评分
package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiProject/internal/domain/eval"
	"aiProject/internal/domain/knowledge"
	"aiProject/internal/domain/model"
	"aiProject/internal/domain/session"
	"aiProject/internal/domain/tool"
	infra_knowledge "aiProject/internal/infrastructure/knowledge"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// EvalService Agent 评估服务
type EvalService struct {
	repo         eval.Repository
	modelFactory func(string) model.Generator
	defaultModel string
	embedder     knowledge.Embedder // 语义相似度评分用（可为 nil）
}

// NewEvalService 创建评估服务
func NewEvalService(repo eval.Repository, modelFactory func(string) model.Generator, defaultModel string) *EvalService {
	return &EvalService{
		repo:         repo,
		modelFactory: modelFactory,
		defaultModel: defaultModel,
	}
}

// SetEmbedder 注入向量化器（启用语义相似度评分）
func (s *EvalService) SetEmbedder(e knowledge.Embedder) {
	s.embedder = e
}

// ─── 数据集 / 用例管理 ──────────────────────────────────────────────────────────

// CreateDataset 创建数据集
func (s *EvalService) CreateDataset(ctx context.Context, userID int64, name, description string) (*eval.Dataset, error) {
	if name == "" {
		return nil, fmt.Errorf("数据集名称不能为空")
	}
	d := &eval.Dataset{Name: name, Description: description, UserID: userID}
	if err := s.repo.CreateDataset(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// ListDatasets 列出用户数据集
func (s *EvalService) ListDatasets(ctx context.Context, userID int64) ([]*eval.Dataset, error) {
	return s.repo.ListDatasets(ctx, userID)
}

// DeleteDataset 删除数据集
func (s *EvalService) DeleteDataset(ctx context.Context, id, userID int64) error {
	return s.repo.DeleteDataset(ctx, id, userID)
}

// ListCases 列出用例
func (s *EvalService) ListCases(ctx context.Context, datasetID int64) ([]*eval.Case, error) {
	return s.repo.ListCases(ctx, datasetID)
}

// AddCase 新增单条用例
func (s *EvalService) AddCase(ctx context.Context, datasetID int64, input, expected string) (*eval.Case, error) {
	if input == "" {
		return nil, fmt.Errorf("用例输入不能为空")
	}
	c := &eval.Case{DatasetID: datasetID, Input: input, Expected: expected}
	if err := s.repo.CreateCase(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// ImportCases 批量导入用例（JSON 数组：[{input, expected}, ...]）
func (s *EvalService) ImportCases(ctx context.Context, datasetID int64, cases []struct {
	Input    string `json:"input"`
	Expected string `json:"expected"`
}) (int, error) {
	count := 0
	for _, c := range cases {
		if c.Input == "" {
			continue
		}
		if err := s.repo.CreateCase(ctx, &eval.Case{DatasetID: datasetID, Input: c.Input, Expected: c.Expected}); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// DeleteCase 删除用例
func (s *EvalService) DeleteCase(ctx context.Context, id int64) error {
	return s.repo.DeleteCase(ctx, id)
}

// ─── 评测运行 ──────────────────────────────────────────────────────────────────

// RunConfig 评测运行配置
type RunConfig struct {
	DatasetID    int64
	Name         string
	ModelName    string
	SystemPrompt string
	Tools        []string
	Scorer       string // "judge"(默认) | "exact" | "semantic"
	JudgeModel   string
	Threshold    float64
	UserID       int64
}

// StartRun 启动一次评测运行（异步执行，立即返回运行记录）
func (s *EvalService) StartRun(ctx context.Context, cfg RunConfig) (*eval.Run, error) {
	cases, err := s.repo.ListCases(ctx, cfg.DatasetID)
	if err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("数据集没有用例，无法评测")
	}

	if cfg.ModelName == "" {
		cfg.ModelName = s.defaultModel
	}
	if cfg.JudgeModel == "" {
		cfg.JudgeModel = s.defaultModel
	}
	if cfg.Threshold <= 0 {
		cfg.Threshold = 0.6
	}
	if cfg.Name == "" {
		cfg.Name = "运行 " + time.Now().Format("01-02 15:04")
	}
	// 评分器类型校验与降级
	scorer := eval.ScorerType(cfg.Scorer)
	switch scorer {
	case eval.ScorerExact, eval.ScorerSemantic, eval.ScorerJudge:
	default:
		scorer = eval.ScorerJudge
	}
	// 语义评分需要 embedder，缺失则降级为裁判
	if scorer == eval.ScorerSemantic && s.embedder == nil {
		shared.GetLogger().Warn("[Eval] 未配置 Embedder，语义评分降级为 LLM 裁判")
		scorer = eval.ScorerJudge
	}

	run := &eval.Run{
		DatasetID:    cfg.DatasetID,
		Name:         cfg.Name,
		ModelName:    cfg.ModelName,
		SystemPrompt: cfg.SystemPrompt,
		Tools:        cfg.Tools,
		Scorer:       scorer,
		JudgeModel:   cfg.JudgeModel,
		Threshold:    cfg.Threshold,
		Status:       eval.RunStatusRunning,
		TotalCases:   len(cases),
		UserID:       cfg.UserID,
	}
	if err := s.repo.CreateRun(ctx, run); err != nil {
		return nil, err
	}

	// 异步执行评测（不阻塞 HTTP 请求；用独立 context 避免请求结束被取消）
	go func() {
		defer shared.Recover("eval.executeRun")
		s.executeRun(context.Background(), run, cases, cfg)
	}()

	return run, nil
}

// executeRun 执行评测：逐条跑 Agent + 裁判评分，持久化结果并更新聚合指标
func (s *EvalService) executeRun(ctx context.Context, run *eval.Run, cases []*eval.Case, cfg RunConfig) {
	logger := shared.GetLogger()
	logger.Info("[Eval] 评测运行开始",
		zap.Int64("run_id", run.ID),
		zap.Int("total_cases", len(cases)),
		zap.String("model", cfg.ModelName),
		zap.String("judge", cfg.JudgeModel),
	)

	var totalScore float64
	passed := 0

	for idx, c := range cases {
		start := time.Now()
		actual, usage, runErr := s.runAgentOnce(ctx, cfg.ModelName, cfg.SystemPrompt, c.Input, cfg.Tools)
		latency := time.Since(start).Milliseconds()

		var score float64
		var reason string
		if runErr != nil {
			actual = fmt.Sprintf("[Agent 执行失败] %v", runErr)
			score = 0
			reason = "Agent 执行失败"
		} else {
			score, reason = s.score(ctx, run.Scorer, cfg.JudgeModel, c.Input, c.Expected, actual)
		}

		isPass := score >= cfg.Threshold
		if isPass {
			passed++
		}
		totalScore += score

		result := &eval.Result{
			RunID:     run.ID,
			CaseID:    c.ID,
			Input:     c.Input,
			Expected:  c.Expected,
			Actual:    actual,
			Score:     score,
			Passed:    isPass,
			Reason:    reason,
			LatencyMs: latency,
			Tokens:    usage.TotalTokens,
		}
		if err := s.repo.CreateResult(ctx, result); err != nil {
			logger.Error("[Eval] 保存结果失败", zap.Int64("run_id", run.ID), zap.Int64("case_id", c.ID), zap.Error(err))
		}

		// 增量更新运行进度（便于前端轮询展示）
		run.PassedCases = passed
		if idx+1 > 0 {
			run.AvgScore = totalScore / float64(idx+1)
		}
		_ = s.repo.UpdateRun(ctx, run)

		logger.Info("[Eval] 用例完成",
			zap.Int64("run_id", run.ID),
			zap.Int("progress", idx+1),
			zap.Int("total", len(cases)),
			zap.Float64("score", score),
			zap.Bool("passed", isPass),
		)
	}

	// 最终化运行
	run.Status = eval.RunStatusCompleted
	run.PassedCases = passed
	if len(cases) > 0 {
		run.AvgScore = totalScore / float64(len(cases))
	}
	now := time.Now()
	run.FinishedAt = &now
	if err := s.repo.UpdateRun(ctx, run); err != nil {
		logger.Error("[Eval] 更新运行状态失败", zap.Int64("run_id", run.ID), zap.Error(err))
	}

	logger.Info("[Eval] 评测运行完成",
		zap.Int64("run_id", run.ID),
		zap.Int("passed", passed),
		zap.Int("total", len(cases)),
		zap.Float64("avg_score", run.AvgScore),
	)
}

// runAgentOnce 用指定配置跑一次 Agent，复用 ReAct 循环（含工具调用），返回完整回复
func (s *EvalService) runAgentOnce(ctx context.Context, modelName, systemPrompt, input string, toolNames []string) (string, model.TokenUsage, error) {
	var usage model.TokenUsage
	modelGen := s.modelFactory(modelName)
	if modelGen == nil {
		return "", usage, fmt.Errorf("无法创建模型生成器: %s", modelName)
	}

	var messages []model.Message
	if systemPrompt != "" {
		messages = append(messages, model.Message{Role: model.RoleSystem, Content: systemPrompt})
	}
	messages = append(messages, model.Message{Role: model.RoleUser, Content: input})

	toolDefs := tool.GetDefinitions(toolNames)

	// ReAct 循环会向 outCh 推送中间事件；评测场景不展示过程，开后台 goroutine 丢弃
	outCh := make(chan StreamChatResponse, 64)
	drained := make(chan struct{})
	go func() {
		defer shared.Recover("eval.drainOutCh")
		for range outCh {
		}
		close(drained)
	}()

	runner := newAgentRunner(modelGen, modelName, session.SessionID("eval"), outCh)
	content, u, err := runner.runReActLoop(ctx, messages, toolDefs)
	close(outCh)
	<-drained
	return content, u, err
}

// score 根据评分器类型对实际回答打分，返回 score(0-1) 与理由
func (s *EvalService) score(ctx context.Context, scorer eval.ScorerType, judgeModel, input, expected, actual string) (float64, string) {
	switch scorer {
	case eval.ScorerExact:
		return exactScore(expected, actual)
	case eval.ScorerSemantic:
		return s.semanticScore(ctx, expected, actual)
	default:
		return s.judge(ctx, judgeModel, input, expected, actual)
	}
}

// exactScore 精确/包含匹配评分：归一化后完全相等或包含则 1 分，否则 0 分
func exactScore(expected, actual string) (float64, string) {
	exp := strings.TrimSpace(strings.ToLower(expected))
	act := strings.TrimSpace(strings.ToLower(actual))
	if exp == "" {
		return 0, "精确匹配需要期望答案，但期望为空"
	}
	if act == exp {
		return 1, "完全匹配"
	}
	if strings.Contains(act, exp) {
		return 1, "包含期望答案"
	}
	return 0, "未匹配期望答案"
}

// semanticScore 语义相似度评分：对期望与实际向量化后计算余弦相似度（截断到 [0,1]）
func (s *EvalService) semanticScore(ctx context.Context, expected, actual string) (float64, string) {
	if s.embedder == nil {
		return 0, "未配置向量化器，无法语义评分"
	}
	if strings.TrimSpace(expected) == "" {
		return 0, "语义评分需要期望答案，但期望为空"
	}
	vecs, err := s.embedder.EmbedBatch(ctx, []string{expected, actual})
	if err != nil || len(vecs) < 2 {
		shared.GetLogger().Warn("[Eval] 语义评分向量化失败", zap.Error(err))
		return 0, "向量化失败"
	}
	sim := infra_knowledge.CosineSimilarity(vecs[0], vecs[1])
	score := float64(sim)
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score, fmt.Sprintf("语义相似度 %.3f", sim)
}

// judgeSchema LLM-as-judge 的结构化输出 JSON Schema（复用 #24 结构化输出能力）
var judgeSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"score":  map[string]interface{}{"type": "number", "description": "0到1之间的评分，1为完全正确，0为完全错误"},
		"reason": map[string]interface{}{"type": "string", "description": "简短的中文评分理由"},
	},
	"required":             []string{"score", "reason"},
	"additionalProperties": false,
}

// judge 用 LLM-as-judge 对实际回答打分，返回 score(0-1) 和理由
func (s *EvalService) judge(ctx context.Context, judgeModel, input, expected, actual string) (float64, string) {
	gen := s.modelFactory(judgeModel)
	if gen == nil {
		return 0, "无法创建裁判模型"
	}

	sysPrompt := "你是一个严格、公正的 AI 回答评测裁判。请根据【期望答案】评估【实际回答】的质量，" +
		"综合考虑正确性、完整性与相关性，给出 0 到 1 之间的分数（1=完全正确，0=完全错误）以及简短的中文理由。" +
		"若期望答案为空，则按回答是否合理、是否切题来评分。"
	userPrompt := fmt.Sprintf("【问题】\n%s\n\n【期望答案】\n%s\n\n【实际回答】\n%s\n\n请客观评分。", input, expected, actual)

	messages := []model.Message{
		{Role: model.RoleSystem, Content: sysPrompt},
		{Role: model.RoleUser, Content: userPrompt},
	}

	opts := model.GenerateOptions{
		ResponseFormat: &model.ResponseFormat{
			Type:       model.ResponseFormatJSONSchema,
			SchemaName: "eval_judge",
			Schema:     judgeSchema,
			Strict:     true,
		},
	}

	var content string
	if sg, ok := gen.(model.StructuredGenerator); ok {
		res, err := sg.GenerateWithToolsOpts(ctx, messages, nil, opts)
		if err != nil {
			shared.GetLogger().Warn("[Eval] 裁判调用失败", zap.Error(err))
			return 0, "裁判调用失败: " + err.Error()
		}
		content = res.Content
	} else {
		// 降级：不支持结构化输出的模型，在 Prompt 中要求返回 JSON
		messages[0].Content += "\n请只返回 JSON，格式：{\"score\": 数字, \"reason\": \"理由\"}，不要包含其他文字。"
		res, err := gen.GenerateWithTools(ctx, messages, nil)
		if err != nil {
			return 0, "裁判调用失败: " + err.Error()
		}
		content = res.Content
	}

	// 解析评分（复用 workflow_engine_nodes.go 的 extractJSONContent 剥离 markdown 围栏）
	cleaned := extractJSONContent(content)
	var parsed struct {
		Score  float64 `json:"score"`
		Reason string  `json:"reason"`
	}
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		shared.GetLogger().Warn("[Eval] 解析裁判评分失败", zap.String("content", msgPreview(content, 200)), zap.Error(err))
		return 0, "解析评分失败"
	}
	// 约束分数范围
	if parsed.Score < 0 {
		parsed.Score = 0
	}
	if parsed.Score > 1 {
		parsed.Score = 1
	}
	return parsed.Score, parsed.Reason
}

// ─── 查询 ──────────────────────────────────────────────────────────────────────

// ListRuns 列出运行记录
func (s *EvalService) ListRuns(ctx context.Context, datasetID, userID int64) ([]*eval.Run, error) {
	return s.repo.ListRuns(ctx, datasetID, userID)
}

// GetRunDetail 获取运行详情（含每条用例结果）
func (s *EvalService) GetRunDetail(ctx context.Context, runID int64) (*eval.Run, []*eval.Result, error) {
	run, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	results, err := s.repo.ListResults(ctx, runID)
	if err != nil {
		return run, nil, err
	}
	return run, results, nil
}

// ─── 运行回归对比 ──────────────────────────────────────────────────────────────

// CompareCaseRow 单条用例在两次运行中的对比
type CompareCaseRow struct {
	CaseID      int64   `json:"case_id"`
	Input       string  `json:"input"`
	Expected    string  `json:"expected"`
	BaseScore   float64 `json:"base_score"`
	TargetScore float64 `json:"target_score"`
	Delta       float64 `json:"delta"` // target - base
	BaseActual  string  `json:"base_actual"`
	TargetActual string `json:"target_actual"`
}

// CompareResult 两次运行的对比结果
type CompareResult struct {
	Base       *eval.Run        `json:"base"`
	Target     *eval.Run        `json:"target"`
	AvgDelta   float64          `json:"avg_delta"`   // 平均分变化（target - base）
	Improved   int              `json:"improved"`    // 变好的用例数
	Regressed  int              `json:"regressed"`   // 变差的用例数
	Unchanged  int              `json:"unchanged"`   // 持平的用例数
	Cases      []CompareCaseRow `json:"cases"`
}

// CompareRuns 对比两次评测运行（按 case_id 匹配，计算逐用例与整体分数变化）
func (s *EvalService) CompareRuns(ctx context.Context, baseID, targetID int64) (*CompareResult, error) {
	baseRun, err := s.repo.GetRun(ctx, baseID)
	if err != nil {
		return nil, fmt.Errorf("基线运行不存在: %w", err)
	}
	targetRun, err := s.repo.GetRun(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("对比运行不存在: %w", err)
	}

	baseResults, err := s.repo.ListResults(ctx, baseID)
	if err != nil {
		return nil, err
	}
	targetResults, err := s.repo.ListResults(ctx, targetID)
	if err != nil {
		return nil, err
	}

	baseMap := make(map[int64]*eval.Result, len(baseResults))
	for _, r := range baseResults {
		baseMap[r.CaseID] = r
	}
	targetMap := make(map[int64]*eval.Result, len(targetResults))
	for _, r := range targetResults {
		targetMap[r.CaseID] = r
	}

	cmp := &CompareResult{Base: baseRun, Target: targetRun}
	// 以 base 用例为主序，叠加 target 中独有的用例
	seen := make(map[int64]bool)
	appendRow := func(caseID int64) {
		if seen[caseID] {
			return
		}
		seen[caseID] = true
		b := baseMap[caseID]
		t := targetMap[caseID]
		row := CompareCaseRow{CaseID: caseID}
		if b != nil {
			row.Input = b.Input
			row.Expected = b.Expected
			row.BaseScore = b.Score
			row.BaseActual = b.Actual
		}
		if t != nil {
			row.Input = t.Input
			row.Expected = t.Expected
			row.TargetScore = t.Score
			row.TargetActual = t.Actual
		}
		row.Delta = row.TargetScore - row.BaseScore
		switch {
		case row.Delta > 0.001:
			cmp.Improved++
		case row.Delta < -0.001:
			cmp.Regressed++
		default:
			cmp.Unchanged++
		}
		cmp.Cases = append(cmp.Cases, row)
	}
	for _, r := range baseResults {
		appendRow(r.CaseID)
	}
	for _, r := range targetResults {
		appendRow(r.CaseID)
	}

	cmp.AvgDelta = targetRun.AvgScore - baseRun.AvgScore
	return cmp, nil
}
