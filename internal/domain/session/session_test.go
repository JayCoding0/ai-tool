package session

import (
	"sync"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// SessionID 测试
// ─────────────────────────────────────────────────────────────────────────────

func TestNewSessionID_Unique(t *testing.T) {
	id1 := NewSessionID()
	id2 := NewSessionID()
	if id1 == id2 {
		t.Errorf("两次生成的 SessionID 不应相同: %s", id1)
	}
}

func TestSessionID_String(t *testing.T) {
	id := SessionID("test-id-123")
	if id.String() != "test-id-123" {
		t.Errorf("期望 test-id-123，实际 %s", id.String())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Session 创建测试
// ─────────────────────────────────────────────────────────────────────────────

func TestNewSession(t *testing.T) {
	sess := NewSession()
	if sess.ID() == "" {
		t.Error("新会话 ID 不应为空")
	}
	if len(sess.GetHistory()) != 0 {
		t.Error("新会话历史应为空")
	}
	if sess.CreatedAt().IsZero() {
		t.Error("创建时间不应为零值")
	}
	if sess.UpdatedAt().IsZero() {
		t.Error("更新时间不应为零值")
	}
}

func TestNewSessionWithID(t *testing.T) {
	id := SessionID("custom-id")
	sess := NewSessionWithID(id)
	if sess.ID() != id {
		t.Errorf("期望 ID=%s，实际 %s", id, sess.ID())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 消息管理测试
// ─────────────────────────────────────────────────────────────────────────────

func TestSession_AddMessage(t *testing.T) {
	sess := NewSession()
	sess.AddMessage("user", "你好")
	sess.AddMessage("assistant", "你好！有什么可以帮你的？")

	history := sess.GetHistory()
	if len(history) != 2 {
		t.Fatalf("期望 2 条消息，实际 %d 条", len(history))
	}
	if history[0].Role != "user" || history[0].Content != "你好" {
		t.Errorf("第一条消息不匹配: %+v", history[0])
	}
	if history[1].Role != "assistant" {
		t.Errorf("第二条消息角色不匹配: %s", history[1].Role)
	}
}

func TestSession_AddMessage_UpdatesTime(t *testing.T) {
	sess := NewSession()
	before := sess.UpdatedAt()
	sess.AddMessage("user", "测试")
	after := sess.UpdatedAt()
	if !after.After(before) && after != before {
		t.Error("添加消息后 UpdatedAt 应更新")
	}
}

func TestSession_ClearHistory(t *testing.T) {
	sess := NewSession()
	sess.AddMessage("user", "消息1")
	sess.AddMessage("assistant", "消息2")
	sess.ClearHistory()

	if len(sess.GetHistory()) != 0 {
		t.Error("清空后历史应为空")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 并发安全测试
// ─────────────────────────────────────────────────────────────────────────────

func TestSession_ConcurrentAccess(t *testing.T) {
	sess := NewSession()
	var wg sync.WaitGroup
	n := 100

	// 并发写入
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sess.AddMessage("user", "并发消息")
		}(i)
	}

	// 并发读取
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sess.GetHistory()
		}()
	}

	wg.Wait()

	history := sess.GetHistory()
	if len(history) != n {
		t.Errorf("期望 %d 条消息，实际 %d 条", n, len(history))
	}
}
