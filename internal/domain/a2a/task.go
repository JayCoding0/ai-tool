// Package a2a 定义 Agent-to-Agent 协议的核心领域模型
// 包括任务状态机、消息格式、流式事件等
package a2a

import (
	"time"
)

// TaskState 任务状态枚举
type TaskState string

const (
	// TaskStateSubmitted 任务已提交，等待处理
	TaskStateSubmitted TaskState = "submitted"
	// TaskStateWorking 任务处理中
	TaskStateWorking TaskState = "working"
	// TaskStateCompleted 任务已完成
	TaskStateCompleted TaskState = "completed"
	// TaskStateFailed 任务失败
	TaskStateFailed TaskState = "failed"
	// TaskStateCanceled 任务已取消
	TaskStateCanceled TaskState = "canceled"
)

// TaskMessage 任务中的消息（输入或输出）
type TaskMessage struct {
	// 消息角色：user | agent
	Role string `json:"role"`
	// 消息内容列表（支持多种内容类型）
	Parts []TaskPart `json:"parts"`
}

// TaskPart 消息内容片段
type TaskPart struct {
	// 内容类型：text | tool_call | tool_result
	Type string `json:"type"`
	// 文本内容（type=text 时）
	Text string `json:"text,omitempty"`
	// 工具名称（type=tool_call/tool_result 时）
	ToolName string `json:"toolName,omitempty"`
	// 工具参数（type=tool_call 时）
	ToolArgs string `json:"toolArgs,omitempty"`
	// 工具结果（type=tool_result 时）
	ToolResult string `json:"toolResult,omitempty"`
}

// TaskStatus 任务当前状态快照
type TaskStatus struct {
	// 当前状态
	State TaskState `json:"state"`
	// 状态描述（可选）
	Message string `json:"message,omitempty"`
	// 状态更新时间
	Timestamp time.Time `json:"timestamp"`
}

// Task A2A 任务实体
type Task struct {
	// 任务唯一 ID
	ID string `json:"id"`
	// 会话 ID（支持多轮对话）
	SessionID string `json:"sessionId,omitempty"`
	// 当前状态
	Status TaskStatus `json:"status"`
	// 输入消息
	Input TaskMessage `json:"input"`
	// 输出消息列表（流式场景下逐步追加）
	Output []TaskMessage `json:"output,omitempty"`
	// 任务元数据（可选，透传给 Agent）
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	// 创建时间
	CreatedAt time.Time `json:"createdAt"`
	// 更新时间
	UpdatedAt time.Time `json:"updatedAt"`
}

// TaskSendRequest 发送任务请求（POST /a2a/tasks/send）
type TaskSendRequest struct {
	// 任务 ID（可选，不传则自动生成）
	ID string `json:"id,omitempty"`
	// 会话 ID（可选，用于多轮对话）
	SessionID string `json:"sessionId,omitempty"`
	// 输入消息
	Message TaskMessage `json:"message"`
	// 任务元数据（可选）
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// TaskSendResponse 发送任务响应
type TaskSendResponse struct {
	// 任务实体
	Task Task `json:"task"`
}

// TaskStreamEvent 流式推送事件（SSE）
type TaskStreamEvent struct {
	// 事件类型：status_update | message_chunk | tool_call | tool_result | completed | error
	Type string `json:"type"`
	// 任务 ID
	TaskID string `json:"taskId"`
	// 状态更新（type=status_update 时）
	Status *TaskStatus `json:"status,omitempty"`
	// 消息片段（type=message_chunk 时）
	Delta string `json:"delta,omitempty"`
	// 思考过程片段（type=message_chunk 时，可选）
	Thinking string `json:"thinking,omitempty"`
	// 工具名称（type=tool_call/tool_result 时）
	ToolName string `json:"toolName,omitempty"`
	// 工具展示名称
	ToolDisplayName string `json:"toolDisplayName,omitempty"`
	// 工具调用 ID
	ToolCallID string `json:"toolCallId,omitempty"`
	// 工具参数（type=tool_call 时）
	ToolArgs string `json:"toolArgs,omitempty"`
	// 工具结果（type=tool_result 时）
	ToolResult string `json:"toolResult,omitempty"`
	// 步骤编号
	Step int `json:"step,omitempty"`
	// 错误信息（type=error 时）
	Error string `json:"error,omitempty"`
	// 完整任务（type=completed 时）
	Task *Task `json:"task,omitempty"`
}

// NewTask 创建新任务
func NewTask(id string, req TaskSendRequest) *Task {
	now := time.Now()
	return &Task{
		ID:        id,
		SessionID: req.SessionID,
		Status: TaskStatus{
			State:     TaskStateSubmitted,
			Timestamp: now,
		},
		Input:     req.Message,
		Output:    []TaskMessage{},
		Metadata:  req.Metadata,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Transition 状态转换（带校验）
func (t *Task) Transition(state TaskState, message string) {
	t.Status = TaskStatus{
		State:     state,
		Message:   message,
		Timestamp: time.Now(),
	}
	t.UpdatedAt = time.Now()
}

// AppendOutput 追加输出消息
func (t *Task) AppendOutput(msg TaskMessage) {
	t.Output = append(t.Output, msg)
	t.UpdatedAt = time.Now()
}

// IsTerminal 判断是否为终态
func (t *Task) IsTerminal() bool {
	return t.Status.State == TaskStateCompleted ||
		t.Status.State == TaskStateFailed ||
		t.Status.State == TaskStateCanceled
}
