// Package application 应用服务层，编排领域对象完成业务用例
// workflow_engine.go — 工作流引擎核心定义（DAG 调度见 workflow_engine_dag.go，节点执行见 workflow_engine_nodes.go）
package application

import (
	"context"
	"fmt"
	"sync"

	domain_model "aiProject/internal/domain/model"
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

	// 1.1 归属校验，防止越权执行他人工作流
	if userID <= 0 || wf.UserID != userID {
		return nil, ErrForbidden
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

	// 6. 启动异步并发执行（DAG 调度器在 workflow_engine_dag.go）
	outCh := make(chan WorkflowEvent, 64)
	go e.runDAG(ctx, wf, execCtx, inputs, outCh)

	return outCh, nil
}