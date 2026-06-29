// Package trace 定义可观测性领域模型：一次请求(Trace)由若干步骤(Span)组成。
// 用于记录 LLM 调用、工具执行、RAG 检索等的输入/输出/耗时/token，支撑调试面板与性能分析。
package trace

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SpanType 步骤类型
type SpanType string

const (
	SpanLLM    SpanType = "llm"    // LLM 调用（含工具决策轮 / 最终回复）
	SpanTool   SpanType = "tool"   // 工具执行
	SpanRAG    SpanType = "rag"    // 知识库检索
	SpanMemory SpanType = "memory" // 记忆检索
	SpanAgent  SpanType = "agent"  // 子 Agent 调用
)

// Status 状态
type Status string

const (
	StatusOK    Status = "ok"
	StatusError Status = "error"
)

// maxFieldLen 单个 input/output 字段保留的最大字符数，避免 trace 占用过多内存
const maxFieldLen = 4000

// Span 一次请求中的单个步骤
type Span struct {
	Name             string    `json:"name"`
	Type             SpanType  `json:"type"`
	Step             int       `json:"step"`               // ReAct 轮次（可选）
	StartTime        time.Time `json:"start_time"`
	DurationMs       int64     `json:"duration_ms"`
	Input            string    `json:"input,omitempty"`
	Output           string    `json:"output,omitempty"`
	PromptTokens     int       `json:"prompt_tokens,omitempty"`
	CompletionTokens int       `json:"completion_tokens,omitempty"`
	TotalTokens      int       `json:"total_tokens,omitempty"`
	Error            string    `json:"error,omitempty"`
}

// Trace 一次完整请求（一轮对话）的调用链
type Trace struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	UserID      int64     `json:"user_id"`
	Model       string    `json:"model"`
	Input       string    `json:"input"`       // 用户输入
	StartTime   time.Time `json:"start_time"`
	DurationMs  int64     `json:"duration_ms"`
	Spans       []Span    `json:"spans"`
	TotalTokens int       `json:"total_tokens"`
	LLMCalls    int       `json:"llm_calls"`
	ToolCalls   int       `json:"tool_calls"`
	Status      Status    `json:"status"`
	Error       string    `json:"error,omitempty"`

	mu sync.Mutex `json:"-"`
}

// New 创建一个新的 Trace
func New(sessionID, model, input string, userID int64) *Trace {
	return &Trace{
		ID:        uuid.NewString(),
		SessionID: sessionID,
		UserID:    userID,
		Model:     model,
		Input:     truncate(input),
		StartTime: time.Now(),
		Status:    StatusOK,
	}
}

// AddSpan 追加一个步骤（并发安全），同时累计 token 与计数
func (t *Trace) AddSpan(s Span) {
	if t == nil {
		return
	}
	s.Input = truncate(s.Input)
	s.Output = truncate(s.Output)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Spans = append(t.Spans, s)
	t.TotalTokens += s.TotalTokens
	switch s.Type {
	case SpanLLM:
		t.LLMCalls++
	case SpanTool:
		t.ToolCalls++
	}
	if s.Error != "" {
		t.Status = StatusError
		if t.Error == "" {
			t.Error = s.Error
		}
	}
}

// Finish 标记 Trace 结束
func (t *Trace) Finish() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.DurationMs = time.Since(t.StartTime).Milliseconds()
}

func truncate(s string) string {
	r := []rune(s)
	if len(r) > maxFieldLen {
		return string(r[:maxFieldLen]) + "…(truncated)"
	}
	return s
}

// Store Trace 存储接口
type Store interface {
	Save(t *Trace)
	List(limit int) []*Trace
	Get(id string) (*Trace, bool)
}

// ─── context 传播 ──────────────────────────────────────────────────────────────

type ctxKey struct{}

// WithTrace 将 Trace 注入 context
func WithTrace(ctx context.Context, t *Trace) context.Context {
	return context.WithValue(ctx, ctxKey{}, t)
}

// FromContext 从 context 取出当前 Trace
func FromContext(ctx context.Context) (*Trace, bool) {
	t, ok := ctx.Value(ctxKey{}).(*Trace)
	return t, ok && t != nil
}
