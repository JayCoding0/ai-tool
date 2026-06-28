// Package http 提供 HTTP 接口层实现
// export_handler.go — 对话导出（Markdown / JSON）
package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"aiProject/internal/domain/session"
)

// HandleExportSession 导出会话对话记录
// GET /api/sessions/export?session_id=xxx&format=md|json （默认 md）
func (h *ChatHandler) HandleExportSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessID := r.URL.Query().Get("session_id")
	if sessID == "" {
		writeJSONError(w, "session_id is required", http.StatusBadRequest)
		return
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "md"
	}

	userID, _, _ := h.getCurrentUser(r)

	messages, err := h.chatService.GetSessionHistory(r.Context(), session.SessionID(sessID), userID)
	if err != nil {
		writeServiceError(w, err, "导出失败")
		return
	}

	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"chat_%s.json\"", sessID))
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"session_id": sessID,
			"exported_at": time.Now().Format(time.RFC3339),
			"messages":   messages,
		})
	default: // markdown
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"chat_%s.md\"", sessID))
		w.Write([]byte{0xEF, 0xBB, 0xBF}) //nolint:errcheck // UTF-8 BOM
		w.Write([]byte(buildMarkdownExport(sessID, messages))) //nolint:errcheck
	}
}

// buildMarkdownExport 将会话消息渲染为 Markdown 文本
func buildMarkdownExport(sessID string, messages []session.Message) string {
	var sb strings.Builder
	sb.WriteString("# 对话记录\n\n")
	sb.WriteString(fmt.Sprintf("> 会话 ID：`%s`　导出时间：%s\n\n", sessID, time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString("---\n\n")

	for _, m := range messages {
		role := "🧑 用户"
		if m.Role == "ai" {
			role = "🤖 助手"
		}
		header := role
		if m.ModelName != "" {
			header += fmt.Sprintf("（%s）", m.ModelName)
		}
		if !m.Time.IsZero() {
			header += "　" + m.Time.Format("2006-01-02 15:04:05")
		}
		sb.WriteString("### " + header + "\n\n")
		sb.WriteString(m.Content)
		sb.WriteString("\n\n")
	}
	return sb.String()
}
