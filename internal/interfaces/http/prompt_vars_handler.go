// Package http 提供 HTTP 接口层实现
package http

import (
	"encoding/json"
	"net/http"

	"aiProject/internal/application"
	"go.uber.org/zap"
)

// ─── Prompt 变量管理接口 ───────────────────────────────────────────────────────

// SetVarRequest 设置变量请求
type SetVarRequest struct {
	SessionID string `json:"session_id,omitempty"` // 会话级变量时必填
	Key       string `json:"key"`
	Value     string `json:"value"`
}

// DeleteVarRequest 删除变量请求
type DeleteVarRequest struct {
	SessionID string `json:"session_id,omitempty"` // 会话级变量时必填
	Key       string `json:"key"`
}

// HandleListPromptVariables 列出可用的 Prompt 模板变量（内置 + 用户自定义 + 会话自定义）
func (h *ChatHandler) HandleListPromptVariables(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, _, _ := h.getCurrentUser(r)

	result := map[string]interface{}{
		"builtin_variables": application.BuiltinVariables,
	}

	// 如果有 PromptVarsService 且用户已登录，返回用户级变量
	if h.promptVarsSvc != nil && userID > 0 {
		userVars, err := h.promptVarsSvc.GetUserVars(r.Context(), userID)
		if err == nil {
			result["user_variables"] = userVars
		}
	}

	// 如果指定了 session_id，返回会话级变量
	sessID := r.URL.Query().Get("session_id")
	if h.promptVarsSvc != nil && sessID != "" {
		sessionVars, err := h.promptVarsSvc.GetSessionVars(r.Context(), sessID)
		if err == nil {
			result["session_variables"] = sessionVars
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result) //nolint:errcheck
}

// HandleSetUserVar 设置用户级 Prompt 变量
func (h *ChatHandler) HandleSetUserVar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.promptVarsSvc == nil {
		writeJSONError(w, "Prompt 变量功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}

	userID, _, _ := h.getCurrentUser(r)
	if userID == 0 {
		writeJSONError(w, "请先登录", http.StatusUnauthorized)
		return
	}

	var req SetVarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Key == "" {
		writeJSONError(w, "变量名 key 不能为空", http.StatusBadRequest)
		return
	}

	if err := h.promptVarsSvc.SetUserVar(r.Context(), userID, req.Key, req.Value); err != nil {
		h.logger.Error("设置用户级变量失败", zap.Int64("user_id", userID), zap.Error(err))
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "用户级变量已设置"}) //nolint:errcheck
}

// HandleDeleteUserVar 删除用户级 Prompt 变量
func (h *ChatHandler) HandleDeleteUserVar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.promptVarsSvc == nil {
		writeJSONError(w, "Prompt 变量功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}

	userID, _, _ := h.getCurrentUser(r)
	if userID == 0 {
		writeJSONError(w, "请先登录", http.StatusUnauthorized)
		return
	}

	var req DeleteVarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Key == "" {
		writeJSONError(w, "变量名 key 不能为空", http.StatusBadRequest)
		return
	}

	if err := h.promptVarsSvc.DeleteUserVar(r.Context(), userID, req.Key); err != nil {
		h.logger.Error("删除用户级变量失败", zap.Int64("user_id", userID), zap.Error(err))
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "用户级变量已删除"}) //nolint:errcheck
}

// HandleSetSessionVar 设置会话级 Prompt 变量
func (h *ChatHandler) HandleSetSessionVar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.promptVarsSvc == nil {
		writeJSONError(w, "Prompt 变量功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}

	var req SetVarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		writeJSONError(w, "session_id 不能为空", http.StatusBadRequest)
		return
	}
	if req.Key == "" {
		writeJSONError(w, "变量名 key 不能为空", http.StatusBadRequest)
		return
	}

	if err := h.promptVarsSvc.SetSessionVar(r.Context(), req.SessionID, req.Key, req.Value); err != nil {
		h.logger.Error("设置会话级变量失败", zap.String("session_id", req.SessionID), zap.Error(err))
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "会话级变量已设置"}) //nolint:errcheck
}

// HandleDeleteSessionVar 删除会话级 Prompt 变量
func (h *ChatHandler) HandleDeleteSessionVar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.promptVarsSvc == nil {
		writeJSONError(w, "Prompt 变量功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}

	var req DeleteVarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		writeJSONError(w, "session_id 不能为空", http.StatusBadRequest)
		return
	}
	if req.Key == "" {
		writeJSONError(w, "变量名 key 不能为空", http.StatusBadRequest)
		return
	}

	if err := h.promptVarsSvc.DeleteSessionVar(r.Context(), req.SessionID, req.Key); err != nil {
		h.logger.Error("删除会话级变量失败", zap.String("session_id", req.SessionID), zap.Error(err))
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "会话级变量已删除"}) //nolint:errcheck
}
