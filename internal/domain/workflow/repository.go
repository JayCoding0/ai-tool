// Package workflow 定义工作流领域的仓储接口
package workflow

import "context"

// Repository Workflow 仓储接口
type Repository interface {
	// Create 创建工作流，返回自增 ID
	Create(ctx context.Context, wf *Workflow) error
	// Update 更新工作流（包括图数据）
	Update(ctx context.Context, wf *Workflow) error
	// GetByID 按 ID 获取工作流
	GetByID(ctx context.Context, id int64) (*Workflow, error)
	// List 列出指定用户的工作流（status 为空时不过滤状态）
	List(ctx context.Context, userID int64, status Status) ([]*Workflow, error)
	// Delete 删除工作流
	Delete(ctx context.Context, id int64) error
	// UpdateStatus 更新工作流状态
	UpdateStatus(ctx context.Context, id int64, status Status) error
}

// RunStatus 工作流执行状态
type RunStatus string

const (
	RunStatusRunning   RunStatus = "running"   // 执行中
	RunStatusCompleted RunStatus = "completed" // 已完成
	RunStatusFailed    RunStatus = "failed"    // 失败
	RunStatusCancelled RunStatus = "cancelled" // 已取消
)

// WorkflowRun 工作流执行记录
type WorkflowRun struct {
	ID           int64                  `json:"id"`
	WorkflowID   int64                  `json:"workflow_id"`
	RunID        string                 `json:"run_id"`         // 执行唯一 ID（UUID）
	Status       RunStatus              `json:"status"`
	Inputs       map[string]interface{} `json:"inputs"`         // 输入变量
	Outputs      map[string]interface{} `json:"outputs"`        // 最终输出
	NodeResults  map[string]interface{} `json:"node_results"`   // 各节点执行结果快照
	TotalTokens  int                    `json:"total_tokens"`   // 总 token 消耗
	DurationMs   int64                  `json:"duration_ms"`    // 总耗时（毫秒）
	ErrorMessage string                 `json:"error_message"`  // 错误信息
	UserID       int64                  `json:"user_id"`
	CreatedAt    string                 `json:"created_at"`
}

// RunRepository 工作流执行记录仓储接口
type RunRepository interface {
	// Save 保存执行记录
	Save(ctx context.Context, run *WorkflowRun) error
	// GetByRunID 按执行 ID 获取记录
	GetByRunID(ctx context.Context, runID string) (*WorkflowRun, error)
	// ListByWorkflowID 列出指定工作流的执行记录
	ListByWorkflowID(ctx context.Context, workflowID int64, limit int) ([]*WorkflowRun, error)
	// UpdateStatus 更新执行状态
	UpdateStatus(ctx context.Context, runID string, status RunStatus, outputs map[string]interface{}, nodeResults map[string]interface{}, totalTokens int, durationMs int64, errMsg string) error
}
