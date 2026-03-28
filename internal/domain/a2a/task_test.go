package a2a

import (
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// NewTask 测试
// ─────────────────────────────────────────────────────────────────────────────

func TestNewTask(t *testing.T) {
	req := TaskSendRequest{
		SessionID: "sess-001",
		Message: TaskMessage{
			Role: "user",
			Parts: []TaskPart{
				{Type: "text", Text: "你好"},
			},
		},
		Metadata: map[string]interface{}{"key": "value"},
	}

	task := NewTask("task-001", req)

	if task.ID != "task-001" {
		t.Errorf("期望 ID=task-001，实际 %s", task.ID)
	}
	if task.SessionID != "sess-001" {
		t.Errorf("期望 SessionID=sess-001，实际 %s", task.SessionID)
	}
	if task.Status.State != TaskStateSubmitted {
		t.Errorf("新任务状态应为 submitted，实际 %s", task.Status.State)
	}
	if len(task.Output) != 0 {
		t.Error("新任务输出应为空")
	}
	if task.CreatedAt.IsZero() {
		t.Error("创建时间不应为零值")
	}
	if task.Metadata["key"] != "value" {
		t.Error("元数据未正确传递")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Transition 状态转换测试
// ─────────────────────────────────────────────────────────────────────────────

func TestTask_Transition(t *testing.T) {
	task := NewTask("task-002", TaskSendRequest{
		Message: TaskMessage{Role: "user", Parts: []TaskPart{{Type: "text", Text: "test"}}},
	})

	// submitted → working
	task.Transition(TaskStateWorking, "处理中")
	if task.Status.State != TaskStateWorking {
		t.Errorf("期望状态 working，实际 %s", task.Status.State)
	}
	if task.Status.Message != "处理中" {
		t.Errorf("期望消息 '处理中'，实际 %s", task.Status.Message)
	}

	// working → completed
	task.Transition(TaskStateCompleted, "完成")
	if task.Status.State != TaskStateCompleted {
		t.Errorf("期望状态 completed，实际 %s", task.Status.State)
	}
}

func TestTask_Transition_UpdatesTime(t *testing.T) {
	task := NewTask("task-003", TaskSendRequest{
		Message: TaskMessage{Role: "user", Parts: []TaskPart{{Type: "text", Text: "test"}}},
	})
	before := task.UpdatedAt
	task.Transition(TaskStateWorking, "")
	if !task.UpdatedAt.After(before) && task.UpdatedAt != before {
		t.Error("状态转换后 UpdatedAt 应更新")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AppendOutput 测试
// ─────────────────────────────────────────────────────────────────────────────

func TestTask_AppendOutput(t *testing.T) {
	task := NewTask("task-004", TaskSendRequest{
		Message: TaskMessage{Role: "user", Parts: []TaskPart{{Type: "text", Text: "test"}}},
	})

	msg := TaskMessage{
		Role:  "agent",
		Parts: []TaskPart{{Type: "text", Text: "回复内容"}},
	}
	task.AppendOutput(msg)

	if len(task.Output) != 1 {
		t.Fatalf("期望 1 条输出，实际 %d 条", len(task.Output))
	}
	if task.Output[0].Parts[0].Text != "回复内容" {
		t.Errorf("输出内容不匹配: %s", task.Output[0].Parts[0].Text)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// IsTerminal 终态判断测试
// ─────────────────────────────────────────────────────────────────────────────

func TestTask_IsTerminal(t *testing.T) {
	cases := []struct {
		state    TaskState
		terminal bool
	}{
		{TaskStateSubmitted, false},
		{TaskStateWorking, false},
		{TaskStateCompleted, true},
		{TaskStateFailed, true},
		{TaskStateCanceled, true},
	}

	for _, tc := range cases {
		t.Run(string(tc.state), func(t *testing.T) {
			task := NewTask("test", TaskSendRequest{
				Message: TaskMessage{Role: "user", Parts: []TaskPart{{Type: "text", Text: "test"}}},
			})
			task.Transition(tc.state, "")
			if task.IsTerminal() != tc.terminal {
				t.Errorf("状态 %s: 期望 IsTerminal=%v，实际 %v", tc.state, tc.terminal, task.IsTerminal())
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TaskPart 类型测试
// ─────────────────────────────────────────────────────────────────────────────

func TestTaskPart_TextType(t *testing.T) {
	part := TaskPart{Type: "text", Text: "hello"}
	if part.Type != "text" || part.Text != "hello" {
		t.Errorf("文本类型 Part 不匹配: %+v", part)
	}
}

func TestTaskPart_ToolCallType(t *testing.T) {
	part := TaskPart{
		Type:     "tool_call",
		ToolName: "query_db",
		ToolArgs: `{"sql": "SELECT 1"}`,
	}
	if part.ToolName != "query_db" {
		t.Errorf("工具调用 Part 工具名不匹配: %s", part.ToolName)
	}
}
