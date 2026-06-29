// Package http 提供 HTTP 接口层实现
// trace_handler.go — 可观测性调用链查询接口
package http

import (
	"net/http"
	"strconv"
	"strings"

	domain_trace "aiProject/internal/domain/trace"
)

// HandleListTraces GET /api/traces?limit=50 列出最近的调用链
func (h *ChatHandler) HandleListTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.traceStore == nil {
		writeJSONError(w, "可观测性功能未启用", http.StatusServiceUnavailable)
		return
	}
	if _, ok := h.requireLogin(w, r); !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	traces := h.traceStore.List(limit)

	// 列表只返回概要信息，避免传输大量 span 数据
	type summary struct {
		ID          string `json:"id"`
		SessionID   string `json:"session_id"`
		Model       string `json:"model"`
		Input       string `json:"input"`
		DurationMs  int64  `json:"duration_ms"`
		TotalTokens int    `json:"total_tokens"`
		LLMCalls    int    `json:"llm_calls"`
		ToolCalls   int    `json:"tool_calls"`
		SpanCount   int    `json:"span_count"`
		Status      string `json:"status"`
		StartTime   string `json:"start_time"`
	}
	out := make([]summary, 0, len(traces))
	for _, t := range traces {
		out = append(out, summary{
			ID: t.ID, SessionID: t.SessionID, Model: t.Model, Input: t.Input,
			DurationMs: t.DurationMs, TotalTokens: t.TotalTokens,
			LLMCalls: t.LLMCalls, ToolCalls: t.ToolCalls, SpanCount: len(t.Spans),
			Status: string(t.Status), StartTime: t.StartTime.Format("2006-01-02 15:04:05"),
		})
	}
	writeJSON(w, map[string]interface{}{"traces": out})
}

// HandleGetTrace GET /api/traces/{id} 获取调用链详情（含全部 span）
func (h *ChatHandler) HandleGetTrace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.traceStore == nil {
		writeJSONError(w, "可观测性功能未启用", http.StatusServiceUnavailable)
		return
	}
	if _, ok := h.requireLogin(w, r); !ok {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/traces/")
	if id == "" {
		writeJSONError(w, "缺少 trace ID", http.StatusBadRequest)
		return
	}
	t, ok := h.traceStore.Get(id)
	if !ok {
		writeJSONError(w, "trace 不存在或已过期", http.StatusNotFound)
		return
	}
	writeJSON(w, t)
}

// SetTraceStore 注入 Trace 存储
func (h *ChatHandler) SetTraceStore(store domain_trace.Store) {
	h.traceStore = store
}
