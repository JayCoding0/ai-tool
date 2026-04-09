// Package application 应用服务层，编排领域对象完成业务用例
package application

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	domain_model "aiProject/internal/domain/model"
	"aiProject/internal/domain/tool"
	"aiProject/internal/domain/workflow"
	"aiProject/internal/shared"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ─── 执行事件定义 ──────────────────────────────────────────────────────────────

// WorkflowEvent 工作流执行事件（推送给前端）
type WorkflowEvent struct {
	Type        string      `json:"type"`                  // "node_start" | "node_output" | "node_error" | "node_done" | "workflow_done" | "workflow_error"
	NodeID      string      `json:"node_id,omitempty"`
	NodeName    string      `json:"node_name,omitempty"`
	NodeType    string      `json:"node_type,omitempty"`
	Output      interface{} `json:"output,omitempty"`
	Error       string      `json:"error,omitempty"`
	DurationMs  int64       `json:"duration_ms,omitempty"`
	RunID       string      `json:"run_id,omitempty"`
	TotalTokens int         `json:"total_tokens,omitempty"`
}

// ─── 执行上下文（并发安全）──────────────────────────────────────────────────────

// ExecutionContext 工作流执行上下文（并发安全）
type ExecutionContext struct {
	WorkflowID  int64
	RunID       string
	Variables   map[string]interface{} // 全局变量（用户输入 + 默认值）
	NodeOutputs map[string]interface{} // 各节点输出：nodeID → output
	TotalTokens int                    // 累计 token 消耗

	mu          sync.Mutex             // 保护 NodeOutputs 和 TotalTokens 的并发写入
	skippedNodes map[string]bool       // 被条件分支跳过的节点集合
}

// SetNodeOutput 并发安全地设置节点输出
func (ec *ExecutionContext) SetNodeOutput(nodeID string, output interface{}) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.NodeOutputs[nodeID] = output
}

// GetNodeOutput 并发安全地获取节点输出
func (ec *ExecutionContext) GetNodeOutput(nodeID string) (interface{}, bool) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	v, ok := ec.NodeOutputs[nodeID]
	return v, ok
}

// AddTokens 并发安全地累加 token 消耗
func (ec *ExecutionContext) AddTokens(tokens int) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.TotalTokens += tokens
}

// MarkSkipped 标记节点为已跳过（条件分支未命中的路径）
func (ec *ExecutionContext) MarkSkipped(nodeID string) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.skippedNodes[nodeID] = true
}

// IsSkipped 检查节点是否被跳过
func (ec *ExecutionContext) IsSkipped(nodeID string) bool {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	return ec.skippedNodes[nodeID]
}

// GetNodeOutputsSnapshot 获取 NodeOutputs 的快照（用于最终结果）
func (ec *ExecutionContext) GetNodeOutputsSnapshot() map[string]interface{} {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	snapshot := make(map[string]interface{}, len(ec.NodeOutputs))
	for k, v := range ec.NodeOutputs {
		snapshot[k] = v
	}
	return snapshot
}

// ─── 工作流执行引擎 ──────────────────────────────────────────────────────────

// WorkflowEngine DAG 工作流执行引擎（Phase 2：支持条件分支 + 并行执行）
type WorkflowEngine struct {
	workflowRepo workflow.Repository
	runRepo      workflow.RunRepository
	modelFactory func(string) domain_model.Generator
	defaultModel string
	registry     *AgentRegistry
}

// NewWorkflowEngine 创建工作流执行引擎
func NewWorkflowEngine(
	workflowRepo workflow.Repository,
	runRepo workflow.RunRepository,
	modelFactory func(string) domain_model.Generator,
	defaultModel string,
	registry *AgentRegistry,
) *WorkflowEngine {
	return &WorkflowEngine{
		workflowRepo: workflowRepo,
		runRepo:      runRepo,
		modelFactory: modelFactory,
		defaultModel: defaultModel,
		registry:     registry,
	}
}

// Execute 执行工作流，返回事件 channel（SSE 流式推送）
// Phase 2 改造：基于入度的并发调度，支持条件分支和并行执行
func (e *WorkflowEngine) Execute(ctx context.Context, workflowID int64, inputs map[string]interface{}, userID int64) (<-chan WorkflowEvent, error) {
	logger := shared.GetLogger()

	// 1. 加载 Workflow 定义
	wf, err := e.workflowRepo.GetByID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("加载工作流失败: %w", err)
	}

	// 2. 校验
	if err := wf.Validate(); err != nil {
		return nil, fmt.Errorf("工作流校验失败: %w", err)
	}

	// 3. 拓扑排序（仅用于校验无环，实际执行使用入度调度）
	if _, err := wf.TopologicalSort(); err != nil {
		return nil, fmt.Errorf("拓扑排序失败: %w", err)
	}

	// 4. 初始化执行上下文
	runID := uuid.New().String()
	execCtx := &ExecutionContext{
		WorkflowID:   workflowID,
		RunID:        runID,
		Variables:    make(map[string]interface{}),
		NodeOutputs:  make(map[string]interface{}),
		skippedNodes: make(map[string]bool),
	}

	// 合并默认变量和用户输入
	for _, v := range wf.Variables {
		if v.DefaultValue != "" {
			execCtx.Variables[v.Name] = v.DefaultValue
		}
	}
	for k, v := range inputs {
		execCtx.Variables[k] = v
	}

	// 5. 保存执行记录
	run := &workflow.WorkflowRun{
		WorkflowID: workflowID,
		RunID:      runID,
		Status:     workflow.RunStatusRunning,
		Inputs:     inputs,
		UserID:     userID,
	}
	if saveErr := e.runRepo.Save(ctx, run); saveErr != nil {
		logger.Warn("保存执行记录失败（不影响执行）", zap.Error(saveErr))
	}

	logger.Info("[Workflow] 开始执行（Phase 2 并发调度引擎）",
		zap.Int64("workflow_id", workflowID),
		zap.String("run_id", runID),
		zap.String("name", wf.Name),
		zap.Int("node_count", len(wf.Nodes)),
	)

	// 6. 启动异步并发执行
	outCh := make(chan WorkflowEvent, 64)
	go e.runDAG(ctx, wf, execCtx, inputs, outCh)

	return outCh, nil
}

// ─── DAG 并发调度器 ──────────────────────────────────────────────────────────

// runDAG 基于入度的 DAG 并发调度执行
// 核心思路：维护每个节点的"剩余入度"，当某节点的所有上游都执行完毕（入度降为 0）时，
// 立即启动该节点的执行（goroutine）。同一层级的多个节点可以并行执行。
func (e *WorkflowEngine) runDAG(ctx context.Context, wf *workflow.Workflow, execCtx *ExecutionContext, inputs map[string]interface{}, outCh chan<- WorkflowEvent) {
	defer close(outCh)
	logger := shared.GetLogger()
	startTime := time.Now()
	runID := execCtx.RunID

	// 构建入度表和邻接表
	inDegree := make(map[string]*int32)   // nodeID → 剩余入度（atomic 操作）
	adjacency := make(map[string][]string) // nodeID → 下游节点列表

	for _, node := range wf.Nodes {
		deg := int32(0)
		inDegree[node.ID] = &deg
		adjacency[node.ID] = nil
	}
	for _, edge := range wf.Edges {
		adjacency[edge.Source] = append(adjacency[edge.Source], edge.Target)
		atomic.AddInt32(inDegree[edge.Target], 1)
	}

	// 去重邻接表（一个节点可能有多条边指向同一个下游）
	for nodeID, targets := range adjacency {
		adjacency[nodeID] = uniqueStrings(targets)
	}

	// 错误通道：任一节点失败时通知调度器
	errCh := make(chan error, 1)
	var firstErr error

	// WaitGroup 追踪所有正在执行的节点
	var wg sync.WaitGroup

	// cancelCtx 用于在某个节点失败时取消其他正在执行的节点
	cancelCtx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()

	// dispatchNode 尝试调度一个节点执行
	// 该函数本身在 goroutine 中被调用，需要并发安全
	var dispatchNode func(nodeID string)
	dispatchNode = func(nodeID string) {
		node, ok := wf.GetNodeByID(nodeID)
		if !ok {
			return
		}

		// 检查是否已取消
		select {
		case <-cancelCtx.Done():
			return
		default:
		}

		// 检查节点是否被条件分支跳过
		if execCtx.IsSkipped(nodeID) {
			logger.Info("[Workflow] 节点被条件分支跳过",
				zap.String("node_id", nodeID),
				zap.String("node_name", node.Name),
			)
			// 跳过的节点也需要递减下游入度，但同时标记下游也被跳过
			e.propagateSkip(wf, nodeID, execCtx, inDegree, &wg, dispatchNode)
			return
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			// ── 处理 Start 节点 ──
			if node.Type == workflow.NodeTypeStart {
				execCtx.SetNodeOutput(nodeID, inputs)
				sendWorkflowEvent(cancelCtx, outCh, WorkflowEvent{
					Type:     "node_done",
					NodeID:   nodeID,
					NodeName: node.Name,
					NodeType: string(node.Type),
					RunID:    runID,
				})
				e.activateDownstream(cancelCtx, wf, nodeID, execCtx, inDegree, &wg, dispatchNode, outCh)
				return
			}

			// ── 处理 End 节点 ──
			if node.Type == workflow.NodeTypeEnd {
				sendWorkflowEvent(cancelCtx, outCh, WorkflowEvent{
					Type:     "node_done",
					NodeID:   nodeID,
					NodeName: node.Name,
					NodeType: string(node.Type),
					RunID:    runID,
				})
				// End 节点没有下游，不需要激活
				return
			}

			// ── 推送节点开始事件 ──
			sendWorkflowEvent(cancelCtx, outCh, WorkflowEvent{
				Type:     "node_start",
				NodeID:   nodeID,
				NodeName: node.Name,
				NodeType: string(node.Type),
				RunID:    runID,
			})

			// ── 执行节点 ──
			nodeStart := time.Now()
			output, execErr := e.executeNode(cancelCtx, wf, *node, execCtx)
			duration := time.Since(nodeStart).Milliseconds()

			if execErr != nil {
				logger.Error("[Workflow] 节点执行失败",
					zap.String("node_id", nodeID),
					zap.String("node_name", node.Name),
					zap.Error(execErr),
				)
				sendWorkflowEvent(cancelCtx, outCh, WorkflowEvent{
					Type:       "node_error",
					NodeID:     nodeID,
					NodeName:   node.Name,
					NodeType:   string(node.Type),
					Error:      execErr.Error(),
					DurationMs: duration,
					RunID:      runID,
				})
				// 通知调度器有错误发生
				select {
				case errCh <- execErr:
				default:
				}
				cancelFunc() // 取消其他节点
				return
			}

			// ── 存储节点输出 ──
			execCtx.SetNodeOutput(nodeID, output)

			logger.Info("[Workflow] 节点执行完成",
				zap.String("node_id", nodeID),
				zap.String("node_name", node.Name),
				zap.String("node_type", string(node.Type)),
				zap.Int64("duration_ms", duration),
			)

			sendWorkflowEvent(cancelCtx, outCh, WorkflowEvent{
				Type:       "node_output",
				NodeID:     nodeID,
				NodeName:   node.Name,
				NodeType:   string(node.Type),
				Output:     output,
				DurationMs: duration,
				RunID:      runID,
			})

			// ── 激活下游节点 ──
			e.activateDownstream(cancelCtx, wf, nodeID, execCtx, inDegree, &wg, dispatchNode, outCh)
		}()
	}

	// 启动所有入度为 0 的节点（通常只有 Start 节点）
	for _, node := range wf.Nodes {
		if atomic.LoadInt32(inDegree[node.ID]) == 0 {
			dispatchNode(node.ID)
		}
	}

	// 等待所有节点执行完毕
	wg.Wait()

	// 检查是否有错误
	select {
	case firstErr = <-errCh:
	default:
	}

	totalDuration := time.Since(startTime).Milliseconds()
	nodeOutputs := execCtx.GetNodeOutputsSnapshot()
	totalTokens := execCtx.TotalTokens

	// 更新执行记录
	if firstErr != nil {
		_ = e.runRepo.UpdateStatus(context.Background(), runID, workflow.RunStatusFailed,
			nil, nodeOutputs, totalTokens, totalDuration, firstErr.Error())
		sendWorkflowEvent(ctx, outCh, WorkflowEvent{
			Type:        "workflow_error",
			Error:       firstErr.Error(),
			RunID:       runID,
			DurationMs:  totalDuration,
			TotalTokens: totalTokens,
		})
	} else {
		_ = e.runRepo.UpdateStatus(context.Background(), runID, workflow.RunStatusCompleted,
			nodeOutputs, nodeOutputs, totalTokens, totalDuration, "")
		sendWorkflowEvent(ctx, outCh, WorkflowEvent{
			Type:        "workflow_done",
			Output:      nodeOutputs,
			RunID:       runID,
			DurationMs:  totalDuration,
			TotalTokens: totalTokens,
		})
	}

	logger.Info("[Workflow] 执行完成",
		zap.String("run_id", runID),
		zap.Int64("duration_ms", totalDuration),
		zap.Int("total_tokens", totalTokens),
		zap.Bool("success", firstErr == nil),
	)
}

// activateDownstream 激活下游节点：递减入度，入度归零时调度执行
// 对于 Condition 节点，只激活命中分支的下游；对于普通节点，激活所有下游
func (e *WorkflowEngine) activateDownstream(
	ctx context.Context,
	wf *workflow.Workflow,
	nodeID string,
	execCtx *ExecutionContext,
	inDegree map[string]*int32,
	wg *sync.WaitGroup,
	dispatchNode func(string),
	outCh chan<- WorkflowEvent,
) {
	node, ok := wf.GetNodeByID(nodeID)
	if !ok {
		return
	}

	// Condition 节点：只激活命中分支的下游，其他分支标记为跳过
	if node.Type == workflow.NodeTypeCondition {
		e.activateConditionDownstream(ctx, wf, nodeID, execCtx, inDegree, wg, dispatchNode, outCh)
		return
	}

	// 普通节点 / Parallel 节点：激活所有下游
	downstreamEdges := wf.GetDownstreamEdges(nodeID)
	for _, edge := range downstreamEdges {
		targetDeg, ok := inDegree[edge.Target]
		if !ok {
			continue
		}
		newDeg := atomic.AddInt32(targetDeg, -1)
		if newDeg == 0 {
			dispatchNode(edge.Target)
		}
	}
}

// activateConditionDownstream 条件分支节点的下游激活逻辑
// 评估条件后，只激活命中分支的下游，未命中分支的下游标记为跳过
func (e *WorkflowEngine) activateConditionDownstream(
	ctx context.Context,
	wf *workflow.Workflow,
	nodeID string,
	execCtx *ExecutionContext,
	inDegree map[string]*int32,
	wg *sync.WaitGroup,
	dispatchNode func(string),
	outCh chan<- WorkflowEvent,
) {
	node, _ := wf.GetNodeByID(nodeID)
	logger := shared.GetLogger()

	// 获取所有出边
	allEdges := wf.GetDownstreamEdges(nodeID)

	// 找到命中的分支 handle
	matchedHandle := ""
	for _, cond := range node.Config.Conditions {
		if cond.IsDefault {
			continue
		}
		if e.evaluateCondition(cond, execCtx) {
			matchedHandle = cond.ID
			logger.Info("[Workflow] 条件分支命中",
				zap.String("node_id", nodeID),
				zap.String("branch_id", cond.ID),
				zap.String("label", cond.Label),
			)
			break
		}
	}

	// 如果没有命中任何条件，走默认分支
	if matchedHandle == "" {
		for _, cond := range node.Config.Conditions {
			if cond.IsDefault {
				matchedHandle = cond.ID
				logger.Info("[Workflow] 条件分支走默认路径",
					zap.String("node_id", nodeID),
					zap.String("branch_id", cond.ID),
				)
				break
			}
		}
	}

	// 激活命中分支的下游，跳过未命中分支的下游
	for _, edge := range allEdges {
		targetDeg, ok := inDegree[edge.Target]
		if !ok {
			continue
		}

		if edge.SourceHandle == matchedHandle {
			// 命中分支：正常递减入度
			newDeg := atomic.AddInt32(targetDeg, -1)
			if newDeg == 0 {
				dispatchNode(edge.Target)
			}
		} else {
			// 未命中分支：标记跳过并递减入度
			execCtx.MarkSkipped(edge.Target)
			newDeg := atomic.AddInt32(targetDeg, -1)
			if newDeg == 0 {
				dispatchNode(edge.Target) // dispatchNode 内部会检查 IsSkipped
			}
		}
	}
}

// propagateSkip 传播跳过状态：被跳过的节点的所有下游也应被跳过
func (e *WorkflowEngine) propagateSkip(
	wf *workflow.Workflow,
	nodeID string,
	execCtx *ExecutionContext,
	inDegree map[string]*int32,
	wg *sync.WaitGroup,
	dispatchNode func(string),
) {
	downstreamEdges := wf.GetDownstreamEdges(nodeID)
	for _, edge := range downstreamEdges {
		targetDeg, ok := inDegree[edge.Target]
		if !ok {
			continue
		}
		// 标记下游也被跳过（除非下游还有其他未跳过的上游）
		execCtx.MarkSkipped(edge.Target)
		newDeg := atomic.AddInt32(targetDeg, -1)
		if newDeg == 0 {
			dispatchNode(edge.Target) // 会在 dispatchNode 中检查 IsSkipped 并继续传播
		}
	}
}

// ─── 条件表达式评估 ──────────────────────────────────────────────────────────

// evaluateCondition 评估单个条件表达式
func (e *WorkflowEngine) evaluateCondition(cond workflow.ConditionBranch, execCtx *ExecutionContext) bool {
	// 解析字段值（支持模板变量）
	fieldValue := e.resolveTemplate(cond.Field, execCtx)
	compareValue := e.resolveTemplate(cond.Value, execCtx)

	switch cond.Operator {
	case "==", "eq":
		return fieldValue == compareValue
	case "!=", "ne":
		return fieldValue != compareValue
	case ">", "gt":
		return compareNumbers(fieldValue, compareValue, func(a, b float64) bool { return a > b })
	case "<", "lt":
		return compareNumbers(fieldValue, compareValue, func(a, b float64) bool { return a < b })
	case ">=", "gte":
		return compareNumbers(fieldValue, compareValue, func(a, b float64) bool { return a >= b })
	case "<=", "lte":
		return compareNumbers(fieldValue, compareValue, func(a, b float64) bool { return a <= b })
	case "contains":
		return strings.Contains(fieldValue, compareValue)
	case "not_contains":
		return !strings.Contains(fieldValue, compareValue)
	case "is_empty":
		return strings.TrimSpace(fieldValue) == ""
	case "is_not_empty":
		return strings.TrimSpace(fieldValue) != ""
	case "starts_with":
		return strings.HasPrefix(fieldValue, compareValue)
	case "ends_with":
		return strings.HasSuffix(fieldValue, compareValue)
	default:
		shared.GetLogger().Warn("[Workflow] 未知的条件操作符",
			zap.String("operator", cond.Operator),
		)
		return false
	}
}

// compareNumbers 尝试将两个字符串解析为数字并比较
func compareNumbers(a, b string, cmp func(float64, float64) bool) bool {
	numA, errA := strconv.ParseFloat(strings.TrimSpace(a), 64)
	numB, errB := strconv.ParseFloat(strings.TrimSpace(b), 64)
	if errA != nil || errB != nil {
		// 无法解析为数字时，按字符串字典序比较
		return cmp(float64(strings.Compare(a, b)), 0)
	}
	return cmp(numA, numB)
}

// ─── 节点执行器 ──────────────────────────────────────────────────────────────

// executeNode 执行单个节点，返回节点输出
func (e *WorkflowEngine) executeNode(ctx context.Context, wf *workflow.Workflow, node workflow.Node, execCtx *ExecutionContext) (interface{}, error) {
	switch node.Type {
	case workflow.NodeTypeLLM:
		return e.executeLLMNode(ctx, node, execCtx)
	case workflow.NodeTypeTool:
		return e.executeToolNode(ctx, node, execCtx)
	case workflow.NodeTypeAgent:
		return e.executeAgentNode(ctx, node, execCtx)
	case workflow.NodeTypeTemplate:
		return e.executeTemplateNode(ctx, node, execCtx)
	case workflow.NodeTypeHTTP:
		return e.executeHTTPNode(ctx, node, execCtx)
	case workflow.NodeTypeCondition:
		return e.executeConditionNode(ctx, wf, node, execCtx)
	case workflow.NodeTypeParallel:
		return e.executeParallelNode(ctx, wf, node, execCtx)
	default:
		return nil, fmt.Errorf("不支持的节点类型: %s", node.Type)
	}
}

// executeConditionNode 执行条件分支节点
// 条件节点本身不产生业务输出，它的作用是评估条件并决定走哪个分支
// 实际的分支路由在 activateConditionDownstream 中处理
func (e *WorkflowEngine) executeConditionNode(ctx context.Context, wf *workflow.Workflow, node workflow.Node, execCtx *ExecutionContext) (interface{}, error) {
	logger := shared.GetLogger()

	if len(node.Config.Conditions) == 0 {
		return nil, fmt.Errorf("Condition 节点 %q 未配置条件分支", node.Name)
	}

	// 评估条件，找到命中的分支
	matchedBranch := ""
	for _, cond := range node.Config.Conditions {
		if cond.IsDefault {
			continue
		}
		if e.evaluateCondition(cond, execCtx) {
			matchedBranch = cond.ID
			break
		}
	}

	// 没有命中任何条件，使用默认分支
	if matchedBranch == "" {
		for _, cond := range node.Config.Conditions {
			if cond.IsDefault {
				matchedBranch = cond.ID
				break
			}
		}
	}

	if matchedBranch == "" {
		logger.Warn("[Workflow] Condition 节点无匹配分支且无默认分支",
			zap.String("node_id", node.ID),
		)
		matchedBranch = "none"
	}

	logger.Info("[Workflow] Condition 节点评估完成",
		zap.String("node_id", node.ID),
		zap.String("matched_branch", matchedBranch),
		zap.Int("total_conditions", len(node.Config.Conditions)),
	)

	// 返回命中的分支 ID 作为输出
	return map[string]interface{}{
		"matched_branch": matchedBranch,
	}, nil
}

// executeParallelNode 执行并行汇聚网关节点
// Parallel 节点作为汇聚点，收集所有上游并行分支的输出并合并
func (e *WorkflowEngine) executeParallelNode(ctx context.Context, wf *workflow.Workflow, node workflow.Node, execCtx *ExecutionContext) (interface{}, error) {
	logger := shared.GetLogger()

	// 获取所有上游节点
	upstreamIDs := wf.GetUpstreamNodes(node.ID)

	// 收集所有上游节点的输出
	mergedOutputs := make(map[string]interface{})
	for _, upID := range upstreamIDs {
		if output, ok := execCtx.GetNodeOutput(upID); ok {
			mergedOutputs[upID] = output
		}
	}

	logger.Info("[Workflow] Parallel 汇聚节点合并完成",
		zap.String("node_id", node.ID),
		zap.Int("upstream_count", len(upstreamIDs)),
		zap.Int("merged_count", len(mergedOutputs)),
	)

	return mergedOutputs, nil
}

// executeLLMNode 执行 LLM 对话节点
func (e *WorkflowEngine) executeLLMNode(ctx context.Context, node workflow.Node, execCtx *ExecutionContext) (interface{}, error) {
	logger := shared.GetLogger()

	modelName := node.Config.ModelName
	if modelName == "" {
		modelName = e.defaultModel
	}

	modelGen := e.modelFactory(modelName)
	if modelGen == nil {
		return nil, fmt.Errorf("无法创建模型生成器: %s", modelName)
	}

	// 渲染 System Prompt 中的变量引用
	systemPrompt := e.resolveTemplate(node.Config.SystemPrompt, execCtx)
	// 渲染 User Prompt 中的变量引用
	userPrompt := e.resolveTemplate(node.Config.UserPrompt, execCtx)

	if userPrompt == "" {
		// 如果没有配置 UserPrompt，尝试使用上游节点的输出作为用户消息
		upstreamOutput := e.getFirstUpstreamOutput(node, execCtx)
		if upstreamOutput != "" {
			userPrompt = upstreamOutput
		} else {
			return nil, fmt.Errorf("LLM 节点 %q 缺少用户输入（请配置 user_prompt 或连接上游节点）", node.Name)
		}
	}

	messages := []domain_model.Message{
		{Role: domain_model.RoleSystem, Content: systemPrompt},
		{Role: domain_model.RoleUser, Content: userPrompt},
	}

	logger.Info("[Workflow] LLM 节点调用",
		zap.String("node_id", node.ID),
		zap.String("model", modelName),
		zap.String("system_prompt_preview", msgPreview(systemPrompt, 80)),
		zap.String("user_prompt_preview", msgPreview(userPrompt, 80)),
	)

	result, err := modelGen.GenerateWithTools(ctx, messages, nil)
	if err != nil {
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	execCtx.AddTokens(result.Usage.TotalTokens)

	logger.Info("[Workflow] LLM 节点完成",
		zap.String("node_id", node.ID),
		zap.Int("content_len", len(result.Content)),
		zap.Int("tokens", result.Usage.TotalTokens),
	)

	return result.Content, nil
}

// executeToolNode 执行工具调用节点
func (e *WorkflowEngine) executeToolNode(ctx context.Context, node workflow.Node, execCtx *ExecutionContext) (interface{}, error) {
	logger := shared.GetLogger()

	toolName := node.Config.ToolName
	if toolName == "" {
		return nil, fmt.Errorf("Tool 节点 %q 未配置工具名称", node.Name)
	}

	// 构建工具参数
	args := make(map[string]interface{})
	for key, tmpl := range node.Config.ToolArgs {
		args[key] = e.resolveTemplate(tmpl, execCtx)
	}

	// 如果没有配置参数模板，尝试使用上游输出作为参数
	if len(args) == 0 {
		upstreamOutput := e.getFirstUpstreamOutput(node, execCtx)
		if upstreamOutput != "" {
			args["input"] = upstreamOutput
		}
	}

	argsJSON, _ := json.Marshal(args)

	logger.Info("[Workflow] Tool 节点调用",
		zap.String("node_id", node.ID),
		zap.String("tool_name", toolName),
		zap.String("args", string(argsJSON)),
	)

	toolCall := domain_model.ToolCall{
		ID:        fmt.Sprintf("wf_%s_%s", execCtx.RunID[:8], node.ID),
		Name:      toolName,
		Arguments: string(argsJSON),
	}

	result, err := tool.Execute(ctx, toolCall)
	if err != nil {
		return nil, fmt.Errorf("工具 %q 执行失败: %w", toolName, err)
	}

	logger.Info("[Workflow] Tool 节点完成",
		zap.String("node_id", node.ID),
		zap.String("tool_name", toolName),
		zap.Int("result_len", len(result)),
	)

	return result, nil
}

// executeAgentNode 执行子 Agent 节点
func (e *WorkflowEngine) executeAgentNode(ctx context.Context, node workflow.Node, execCtx *ExecutionContext) (interface{}, error) {
	logger := shared.GetLogger()

	if e.registry == nil {
		return nil, fmt.Errorf("Agent 注册中心未初始化")
	}

	agentName := node.Config.AgentName
	if agentName == "" {
		return nil, fmt.Errorf("Agent 节点 %q 未配置 agent_name", node.Name)
	}

	// 渲染发送给 Agent 的消息
	message := e.resolveTemplate(node.Config.AgentMessage, execCtx)
	if message == "" {
		upstreamOutput := e.getFirstUpstreamOutput(node, execCtx)
		if upstreamOutput != "" {
			message = upstreamOutput
		} else {
			return nil, fmt.Errorf("Agent 节点 %q 缺少消息内容", node.Name)
		}
	}

	logger.Info("[Workflow] Agent 节点调用",
		zap.String("node_id", node.ID),
		zap.String("agent_name", agentName),
		zap.String("message_preview", msgPreview(message, 80)),
	)

	// 使用工作流的 runID 作为 sessionID，保持上下文
	result, err := e.registry.CallSubAgent(agentName, message, execCtx.RunID, nil)
	if err != nil {
		return nil, fmt.Errorf("子 Agent %q 调用失败: %w", agentName, err)
	}

	return result, nil
}

// executeTemplateNode 执行模板转换节点
func (e *WorkflowEngine) executeTemplateNode(ctx context.Context, node workflow.Node, execCtx *ExecutionContext) (interface{}, error) {
	tmpl := node.Config.Template
	if tmpl == "" {
		return nil, fmt.Errorf("Template 节点 %q 未配置模板", node.Name)
	}

	result := e.resolveTemplate(tmpl, execCtx)
	return result, nil
}

// executeHTTPNode 执行 HTTP 请求节点
func (e *WorkflowEngine) executeHTTPNode(ctx context.Context, node workflow.Node, execCtx *ExecutionContext) (interface{}, error) {
	logger := shared.GetLogger()

	url := e.resolveTemplate(node.Config.URL, execCtx)
	if url == "" {
		return nil, fmt.Errorf("HTTP 节点 %q 未配置 URL", node.Name)
	}

	method := node.Config.Method
	if method == "" {
		method = "GET"
	}

	// 构建请求体
	var bodyReader io.Reader
	if node.Config.Body != "" {
		body := e.resolveTemplate(node.Config.Body, execCtx)
		bodyReader = strings.NewReader(body)
	}

	// 设置超时
	timeout := 30 * time.Second
	if node.Config.TimeoutSec > 0 {
		timeout = time.Duration(node.Config.TimeoutSec) * time.Second
	}
	httpCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(httpCtx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}

	// 设置请求头
	for key, val := range node.Config.Headers {
		req.Header.Set(key, e.resolveTemplate(val, execCtx))
	}
	if req.Header.Get("Content-Type") == "" && bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	logger.Info("[Workflow] HTTP 节点请求",
		zap.String("node_id", node.ID),
		zap.String("method", method),
		zap.String("url", url),
	)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 HTTP 响应失败: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP 请求返回错误状态码 %d: %s", resp.StatusCode, string(respBody))
	}

	return string(respBody), nil
}

// ─── 辅助方法 ──────────────────────────────────────────────────────────────

// nodeRefRegex 匹配 ${node_id.output} 格式的节点输出引用
var nodeRefRegex = regexp.MustCompile(`\$\{(\w+)\.output\}`)

// varRefRegex 匹配 ${var_name} 格式的全局变量引用
var varRefRegex = regexp.MustCompile(`\$\{(\w+)\}`)

// resolveTemplate 解析模板字符串中的变量引用
// 支持两种引用格式：
//   - ${node_id.output} — 引用指定节点的输出
//   - ${var_name} — 引用全局变量或内置变量
func (e *WorkflowEngine) resolveTemplate(tmpl string, execCtx *ExecutionContext) string {
	if tmpl == "" {
		return ""
	}

	// 先替换节点输出引用 ${node_id.output}
	result := nodeRefRegex.ReplaceAllStringFunc(tmpl, func(match string) string {
		nodeID := match[2 : strings.Index(match, ".")]
		if output, ok := execCtx.GetNodeOutput(nodeID); ok {
			return fmt.Sprintf("%v", output)
		}
		return match // 未找到的引用保留原样
	})

	// 再替换全局变量引用 ${var_name}（排除已替换的节点引用）
	result = varRefRegex.ReplaceAllStringFunc(result, func(match string) string {
		varName := match[2 : len(match)-1]
		// 内置变量
		switch varName {
		case "current_time":
			return time.Now().Format("2006-01-02 15:04:05")
		case "current_date":
			return time.Now().Format("2006-01-02")
		}
		// 全局变量
		if val, ok := execCtx.Variables[varName]; ok {
			return fmt.Sprintf("%v", val)
		}
		return match
	})

	return result
}

// getFirstUpstreamOutput 获取节点的第一个上游节点的输出（字符串形式）
// 用于节点未显式配置输入时的自动推导
func (e *WorkflowEngine) getFirstUpstreamOutput(node workflow.Node, execCtx *ExecutionContext) string {
	// 先检查 InputMapping
	if len(node.Config.InputMapping) > 0 {
		for _, ref := range node.Config.InputMapping {
			resolved := e.resolveTemplate("${"+ref+"}", execCtx)
			if resolved != "" && resolved != "${"+ref+"}" {
				return resolved
			}
		}
	}

	// 没有 InputMapping，遍历所有已有输出，找到最近的上游
	// 这里简单地返回 NodeOutputs 中最后一个非空输出
	// 实际应该根据 Edge 关系查找，但需要 Workflow 引用
	// Phase 1 简化处理：按 NodeOutputs 的插入顺序取最后一个
	execCtx.mu.Lock()
	defer execCtx.mu.Unlock()
	var lastOutput string
	for _, output := range execCtx.NodeOutputs {
		if output != nil {
			lastOutput = fmt.Sprintf("%v", output)
		}
	}
	return lastOutput
}

// sendWorkflowEvent 向 channel 发送工作流事件
func sendWorkflowEvent(ctx context.Context, ch chan<- WorkflowEvent, event WorkflowEvent) bool {
	select {
	case ch <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

// uniqueStrings 字符串切片去重
func uniqueStrings(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
