// Package application 应用服务层，编排领域对象完成业务用例
package application

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"aiProject/internal/domain/knowledge"
	"aiProject/internal/domain/memory"
	"aiProject/internal/domain/model"
	"aiProject/internal/domain/session"
	infra_knowledge "aiProject/internal/infrastructure/knowledge"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// ─── 记忆服务 ─────────────────────────────────────────────────────────────────

// MemoryService 记忆应用服务
// 负责记忆的提取、更新、检索、衰减等全生命周期管理
// 参考 Mem0 论文的双阶段架构：Extraction Phase → Update Phase
type MemoryService struct {
	memoryRepo   memory.Repository
	embedder     knowledge.Embedder    // 复用 RAG 的 embedding 模型
	modelGen     model.Generator       // 用于记忆提取和更新的 LLM
	modelFactory func(string) model.Generator
	defaultModel string
}

// NewMemoryService 创建记忆服务
func NewMemoryService(memoryRepo memory.Repository, embedder knowledge.Embedder, modelGen model.Generator) *MemoryService {
	return &MemoryService{
		memoryRepo: memoryRepo,
		embedder:   embedder,
		modelGen:   modelGen,
	}
}

// SetModelFactory 设置模型工厂（支持动态选择模型）
func (s *MemoryService) SetModelFactory(factory func(string) model.Generator, defaultModel string) {
	s.modelFactory = factory
	s.defaultModel = defaultModel
}

// getModelGen 获取模型生成器
func (s *MemoryService) getModelGen(modelName string) model.Generator {
	if modelName == "" || s.modelFactory == nil {
		return s.modelGen
	}
	return s.modelFactory(modelName)
}

// ─── Phase 1: 记忆提取（Mem0 Extraction Phase）─────────────────────────────────

// extractionPrompt 记忆提取 Prompt 模板
const extractionPrompt = `请从以下对话中提取用户的关键信息、偏好和事实。
仅提取值得长期记忆的信息，忽略临时性的对话内容（如问候、闲聊）。

提取规则：
1. fact（事实）：用户的身份、职业、技术栈、项目信息等客观事实
2. preference（偏好）：用户的沟通风格、回答格式、语言偏好等
3. episode（情景）：重要的决策、结论、约定等值得记住的对话片段

请以 JSON 数组格式返回，每条记忆包含 content（内容）、type（类型）、importance（重要性 0-1）。
如果没有值得记忆的信息，返回空数组 []。

示例输出：
[
  {"content": "用户是一名 Go 后端开发者，主要使用 DDD 架构", "type": "fact", "importance": 0.8},
  {"content": "用户偏好简洁的代码注释风格", "type": "preference", "importance": 0.6}
]

## 对话内容
%s

请提取记忆（仅返回 JSON 数组，不要其他内容）：`

// ExtractMemories 从对话中提取候选记忆（Mem0 Extraction Phase）
// 异步调用，在每轮对话结束后触发
func (s *MemoryService) ExtractMemories(ctx context.Context, userID int64, sessionID string, messages []session.Message, modelName string) {
	logger := shared.GetLogger()

	if len(messages) < 2 {
		return // 至少需要一轮对话（user + ai）
	}

	// 只取最近的对话（最多 10 条消息）
	recentMsgs := messages
	if len(recentMsgs) > 10 {
		recentMsgs = recentMsgs[len(recentMsgs)-10:]
	}

	// 构建对话文本
	var dialogText strings.Builder
	for _, msg := range recentMsgs {
		role := "用户"
		if msg.Role == "ai" {
			role = "AI"
		}
		dialogText.WriteString(fmt.Sprintf("%s: %s\n", role, msg.Content))
	}

	// 调用 LLM 提取记忆
	prompt := fmt.Sprintf(extractionPrompt, dialogText.String())
	gen := s.getModelGen(modelName)
	result, err := gen.Generate(ctx, model.Prompt(prompt))
	if err != nil {
		logger.Warn("记忆提取 LLM 调用失败", zap.Error(err))
		return
	}

	// 解析 LLM 返回的 JSON
	responseText := strings.TrimSpace(string(result.Response))
	// 去除可能的 markdown 代码块包裹
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	var extracted []memory.ExtractedMemory
	if err := json.Unmarshal([]byte(responseText), &extracted); err != nil {
		logger.Warn("记忆提取结果解析失败",
			zap.String("response", responseText[:min(len(responseText), 200)]),
			zap.Error(err),
		)
		return
	}

	if len(extracted) == 0 {
		logger.Debug("本轮对话无需提取记忆", zap.String("session_id", sessionID))
		return
	}

	logger.Info("记忆提取完成",
		zap.String("session_id", sessionID),
		zap.Int("extracted_count", len(extracted)),
	)

	// Phase 2: 对每条候选记忆执行 Mem0 式更新
	for _, em := range extracted {
		if em.Importance < memory.MinImportanceThreshold {
			continue // 过滤低重要性记忆
		}
		if !em.Type.IsValid() {
			em.Type = memory.MemoryTypeFact // 默认为事实类型
		}
		s.processExtractedMemory(ctx, userID, sessionID, em)
	}

	// 记忆容量管理：检查是否超限
	s.enforceMemoryLimit(ctx, userID)
}

// ─── Phase 2: 记忆更新（Mem0 Update Phase）──────────────────────────────────────

// processExtractedMemory 处理单条提取的记忆
// 与用户现有记忆库做向量相似度比对，决定 ADD/UPDATE/DELETE/NOOP
func (s *MemoryService) processExtractedMemory(ctx context.Context, userID int64, sessionID string, em memory.ExtractedMemory) {
	logger := shared.GetLogger()

	// 1. 将候选记忆向量化
	embedding, err := s.embedder.Embed(ctx, em.Content)
	if err != nil {
		logger.Warn("记忆向量化失败", zap.String("content", em.Content[:min(len(em.Content), 50)]), zap.Error(err))
		return
	}
	embeddingBytes := infra_knowledge.Float32SliceToBytes(embedding)

	// 2. 加载用户现有记忆的向量
	existingMemories, err := s.memoryRepo.ListAllEmbeddings(ctx, userID)
	if err != nil {
		logger.Warn("加载用户记忆失败", zap.Error(err))
		// 如果加载失败，直接新增
		s.addNewMemory(ctx, userID, sessionID, em, embeddingBytes)
		return
	}

	// 3. 计算与所有现有记忆的相似度
	var bestMatch *memory.Memory
	var bestScore float32

	for _, existing := range existingMemories {
		if existing.Embedding == nil {
			continue
		}
		existingVec := infra_knowledge.BytesToFloat32Slice(existing.Embedding)
		score := infra_knowledge.CosineSimilarity(embedding, existingVec)
		if score > bestScore {
			bestScore = score
			bestMatch = existing
		}
	}

	// 4. 根据相似度决定操作
	switch {
	case bestScore > memory.SimilarityThresholdUpdate:
		// 高度相似 → UPDATE（合并已有记忆）
		logger.Info("记忆更新（合并）",
			zap.Int64("target_id", bestMatch.ID),
			zap.Float32("similarity", bestScore),
			zap.String("old_content", bestMatch.Content[:min(len(bestMatch.Content), 50)]),
			zap.String("new_content", em.Content[:min(len(em.Content), 50)]),
		)
		// 合并内容：如果新内容更详细，替换旧内容；否则保留旧内容
		mergedContent := em.Content
		if len(em.Content) < len(bestMatch.Content) {
			mergedContent = bestMatch.Content // 保留更详细的版本
		}
		bestMatch.Content = mergedContent
		bestMatch.Embedding = embeddingBytes
		bestMatch.Importance = maxFloat64(bestMatch.Importance, em.Importance)
		if err := s.memoryRepo.UpdateMemory(ctx, bestMatch); err != nil {
			logger.Warn("更新记忆失败", zap.Error(err))
		}

	case bestScore < memory.SimilarityThresholdAdd:
		// 低相似度 → ADD（全新记忆）
		s.addNewMemory(ctx, userID, sessionID, em, embeddingBytes)

	default:
		// 中等相似度 → NOOP（可能是相关但不完全相同的信息，暂不处理）
		logger.Debug("记忆跳过（中等相似度）",
			zap.Float32("similarity", bestScore),
			zap.String("content", em.Content[:min(len(em.Content), 50)]),
		)
	}
}

// addNewMemory 新增一条记忆
func (s *MemoryService) addNewMemory(ctx context.Context, userID int64, sessionID string, em memory.ExtractedMemory, embeddingBytes []byte) {
	logger := shared.GetLogger()
	m := &memory.Memory{
		UserID:          userID,
		Content:         em.Content,
		Embedding:       embeddingBytes,
		MemoryType:      em.Type,
		SourceSessionID: sessionID,
		Importance:      em.Importance,
		AccessCount:     0,
	}
	if err := s.memoryRepo.CreateMemory(ctx, m); err != nil {
		logger.Warn("新增记忆失败", zap.Error(err))
		return
	}
	logger.Info("新增记忆",
		zap.Int64("memory_id", m.ID),
		zap.String("type", string(em.Type)),
		zap.Float64("importance", em.Importance),
		zap.String("content", em.Content[:min(len(em.Content), 80)]),
	)
}

// ─── 记忆检索（注入对话上下文）────────────────────────────────────────────────

// RetrieveRelevantMemories 检索与当前消息相关的记忆（Top-K 向量检索）
// 返回格式化的记忆文本，可直接注入 System Prompt
func (s *MemoryService) RetrieveRelevantMemories(ctx context.Context, userID int64, query string, topK int) (string, error) {
	if userID == 0 || query == "" {
		return "", nil
	}
	if topK <= 0 {
		topK = 5
	}

	// 1. 将查询向量化
	queryVec, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return "", fmt.Errorf("查询向量化失败: %w", err)
	}

	// 2. 加载用户所有记忆的向量
	memories, err := s.memoryRepo.ListAllEmbeddings(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("加载记忆失败: %w", err)
	}
	if len(memories) == 0 {
		return "", nil
	}

	// 3. 计算余弦相似度并排序
	type scored struct {
		mem   *memory.Memory
		score float32
	}
	results := make([]scored, 0, len(memories))
	for _, m := range memories {
		if m.Embedding == nil {
			continue
		}
		memVec := infra_knowledge.BytesToFloat32Slice(m.Embedding)
		score := infra_knowledge.CosineSimilarity(queryVec, memVec)
		results = append(results, scored{mem: m, score: score})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// 4. 取 TopK，过滤低相似度（阈值 0.3）
	const minScore float32 = 0.3
	if topK > len(results) {
		topK = len(results)
	}

	var memoryTexts []string
	for i := 0; i < topK; i++ {
		if results[i].score < minScore {
			break
		}
		m := results[i].mem
		memoryTexts = append(memoryTexts, fmt.Sprintf("- [%s] %s", m.MemoryType, m.Content))

		// 异步增加访问计数
		go func(id int64) {
			_ = s.memoryRepo.IncrementAccessCount(context.Background(), id)
		}(m.ID)
	}

	if len(memoryTexts) == 0 {
		return "", nil
	}

	return strings.Join(memoryTexts, "\n"), nil
}

// ─── 记忆衰减机制 ─────────────────────────────────────────────────────────────

// DecayMemories 执行记忆衰减（模拟人类遗忘曲线）
// 长期未被检索命中的记忆降低 importance 分数，低于阈值自动删除
func (s *MemoryService) DecayMemories(ctx context.Context, userID int64) {
	logger := shared.GetLogger()

	// 衰减 7 天内未被访问的记忆
	cutoff := time.Now().AddDate(0, 0, -7).Format("2006-01-02 15:04:05")
	decayed, err := s.memoryRepo.BatchDecayImportance(ctx, userID, cutoff, memory.DecayFactor)
	if err != nil {
		logger.Warn("记忆衰减失败", zap.Error(err))
		return
	}

	// 删除 importance 过低的记忆
	deleted, err := s.memoryRepo.DeleteExpiredMemories(ctx, userID, memory.DecayThreshold)
	if err != nil {
		logger.Warn("删除过期记忆失败", zap.Error(err))
		return
	}

	if decayed > 0 || deleted > 0 {
		logger.Info("记忆衰减完成",
			zap.Int64("user_id", userID),
			zap.Int64("decayed", decayed),
			zap.Int64("deleted", deleted),
		)
	}
}

// enforceMemoryLimit 记忆容量管理：超限时淘汰最低分记忆
func (s *MemoryService) enforceMemoryLimit(ctx context.Context, userID int64) {
	count, err := s.memoryRepo.CountByUser(ctx, userID)
	if err != nil {
		return
	}
	if count > memory.MaxMemoriesPerUser {
		excess := count - memory.MaxMemoriesPerUser
		if err := s.memoryRepo.DeleteLowestImportance(ctx, userID, excess); err != nil {
			shared.GetLogger().Warn("淘汰低分记忆失败", zap.Error(err))
		} else {
			shared.GetLogger().Info("淘汰低分记忆",
				zap.Int64("user_id", userID),
				zap.Int("deleted", excess),
			)
		}
	}
}

// ─── CRUD 接口（供 HTTP Handler 调用）──────────────────────────────────────────

// ListMemories 列出用户记忆（分页）
func (s *MemoryService) ListMemories(ctx context.Context, userID int64, memoryType string, page, pageSize int) ([]*memory.Memory, int, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var list []*memory.Memory
	var err error

	if memoryType != "" && memory.MemoryType(memoryType).IsValid() {
		list, err = s.memoryRepo.ListByUserAndType(ctx, userID, memory.MemoryType(memoryType))
	} else {
		list, err = s.memoryRepo.ListByUser(ctx, userID, offset, pageSize)
	}
	if err != nil {
		return nil, 0, err
	}

	total, _ := s.memoryRepo.CountByUser(ctx, userID)
	return list, total, nil
}

// GetMemory 获取单条记忆
func (s *MemoryService) GetMemory(ctx context.Context, id int64) (*memory.Memory, error) {
	return s.memoryRepo.GetMemory(ctx, id)
}

// CreateMemory 手动创建记忆（用户通过 API 手动添加）
func (s *MemoryService) CreateMemory(ctx context.Context, userID int64, content string, memoryType memory.MemoryType, importance float64) (*memory.Memory, error) {
	if content == "" {
		return nil, fmt.Errorf("记忆内容不能为空")
	}
	if !memoryType.IsValid() {
		memoryType = memory.MemoryTypeFact
	}
	if importance <= 0 || importance > 1 {
		importance = 0.5
	}

	// 向量化
	var embeddingBytes []byte
	if s.embedder != nil {
		embedding, err := s.embedder.Embed(ctx, content)
		if err != nil {
			shared.GetLogger().Warn("手动创建记忆向量化失败", zap.Error(err))
			// 向量化失败不阻塞创建，后续可以补充
		} else {
			embeddingBytes = infra_knowledge.Float32SliceToBytes(embedding)
		}
	}

	m := &memory.Memory{
		UserID:     userID,
		Content:    content,
		Embedding:  embeddingBytes,
		MemoryType: memoryType,
		Importance: importance,
	}
	if err := s.memoryRepo.CreateMemory(ctx, m); err != nil {
		return nil, fmt.Errorf("创建记忆失败: %w", err)
	}

	// 容量管理
	s.enforceMemoryLimit(ctx, userID)

	return m, nil
}

// UpdateMemoryContent 更新记忆内容（用户通过 API 手动编辑）
func (s *MemoryService) UpdateMemoryContent(ctx context.Context, id int64, content string) error {
	m, err := s.memoryRepo.GetMemory(ctx, id)
	if err != nil {
		return fmt.Errorf("记忆不存在: %w", err)
	}

	m.Content = content

	// 重新向量化
	if s.embedder != nil {
		embedding, err := s.embedder.Embed(ctx, content)
		if err != nil {
			shared.GetLogger().Warn("更新记忆向量化失败", zap.Error(err))
		} else {
			m.Embedding = infra_knowledge.Float32SliceToBytes(embedding)
		}
	}

	return s.memoryRepo.UpdateMemory(ctx, m)
}

// DeleteMemory 删除记忆
func (s *MemoryService) DeleteMemory(ctx context.Context, id int64) error {
	return s.memoryRepo.DeleteMemory(ctx, id)
}

// ─── 辅助函数 ──────────────────────────────────────────────────────────────────

func maxFloat64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
