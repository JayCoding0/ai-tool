// Package http 提供 HTTP 接口层实现
package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"aiProject/internal/domain/memory"
	"go.uber.org/zap"
)

// ─── 记忆管理接口 ──────────────────────────────────────────────────────────────

// CreateMemoryRequest 创建记忆请求
type CreateMemoryRequest struct {
	Content    string  `json:"content"`
	MemoryType string  `json:"memory_type"` // fact/preference/episode/summary
	Importance float64 `json:"importance"`   // 0-1
}

// UpdateMemoryRequest 更新记忆请求
type UpdateMemoryRequest struct {
	Content string `json:"content"`
}

// MemoryResponse 记忆响应
type MemoryResponse struct {
	ID              int64   `json:"id"`
	Content         string  `json:"content"`
	MemoryType      string  `json:"memory_type"`
	SourceSessionID string  `json:"source_session_id,omitempty"`
	Importance      float64 `json:"importance"`
	AccessCount     int     `json:"access_count"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// toMemoryResponse 将领域对象转换为 HTTP 响应
func toMemoryResponse(m *memory.Memory) MemoryResponse {
	return MemoryResponse{
		ID:              m.ID,
		Content:         m.Content,
		MemoryType:      string(m.MemoryType),
		SourceSessionID: m.SourceSessionID,
		Importance:      m.Importance,
		AccessCount:     m.AccessCount,
		CreatedAt:       m.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:       m.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}

// memoryServiceRequired 检查记忆服务是否可用
func (h *ChatHandler) memoryServiceRequired(w http.ResponseWriter) bool {
	if h.memorySvc == nil {
		writeJSONError(w, "记忆功能未启用", http.StatusServiceUnavailable)
		return false
	}
	return true
}

// HandleListMemories 列出用户记忆
// GET /api/memory?type=fact&page=1&page_size=20
func (h *ChatHandler) HandleListMemories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.memoryServiceRequired(w) {
		return
	}

	userID, _, _ := h.getCurrentUser(r)
	if userID == 0 {
		writeJSONError(w, "请先登录", http.StatusUnauthorized)
		return
	}

	memoryType := r.URL.Query().Get("type")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))

	list, total, err := h.memorySvc.ListMemories(r.Context(), userID, memoryType, page, pageSize)
	if err != nil {
		writeJSONError(w, "获取记忆列表失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	memories := make([]MemoryResponse, 0, len(list))
	for _, m := range list {
		memories = append(memories, toMemoryResponse(m))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"memories": memories,
		"total":    total,
	}) //nolint:errcheck
}

// HandleCreateMemory 手动创建记忆
// POST /api/memory
func (h *ChatHandler) HandleCreateMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.memoryServiceRequired(w) {
		return
	}

	userID, _, _ := h.getCurrentUser(r)
	if userID == 0 {
		writeJSONError(w, "请先登录", http.StatusUnauthorized)
		return
	}

	var req CreateMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Content == "" {
		writeJSONError(w, "记忆内容不能为空", http.StatusBadRequest)
		return
	}

	m, err := h.memorySvc.CreateMemory(r.Context(), userID, req.Content, memory.MemoryType(req.MemoryType), req.Importance)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(toMemoryResponse(m)) //nolint:errcheck
}

// HandleUpdateMemory 更新记忆
// PUT /api/memory/update?id=123
func (h *ChatHandler) HandleUpdateMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.memoryServiceRequired(w) {
		return
	}

	userID, _, _ := h.getCurrentUser(r)
	if userID == 0 {
		writeJSONError(w, "请先登录", http.StatusUnauthorized)
		return
	}

	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSONError(w, "无效的记忆 ID", http.StatusBadRequest)
		return
	}

	// 验证记忆归属
	m, err := h.memorySvc.GetMemory(r.Context(), id)
	if err != nil {
		writeJSONError(w, "记忆不存在", http.StatusNotFound)
		return
	}
	if m.UserID != userID {
		writeJSONError(w, "无权操作此记忆", http.StatusForbidden)
		return
	}

	var req UpdateMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Content == "" {
		writeJSONError(w, "记忆内容不能为空", http.StatusBadRequest)
		return
	}

	if err := h.memorySvc.UpdateMemoryContent(r.Context(), id, req.Content); err != nil {
		writeJSONError(w, "更新记忆失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "记忆已更新"}) //nolint:errcheck
}

// HandleDeleteMemory 删除记忆
// DELETE /api/memory/delete?id=123
func (h *ChatHandler) HandleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.memoryServiceRequired(w) {
		return
	}

	userID, _, _ := h.getCurrentUser(r)
	if userID == 0 {
		writeJSONError(w, "请先登录", http.StatusUnauthorized)
		return
	}

	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSONError(w, "无效的记忆 ID", http.StatusBadRequest)
		return
	}

	// 验证记忆归属
	m, err := h.memorySvc.GetMemory(r.Context(), id)
	if err != nil {
		writeJSONError(w, "记忆不存在", http.StatusNotFound)
		return
	}
	if m.UserID != userID {
		writeJSONError(w, "无权操作此记忆", http.StatusForbidden)
		return
	}

	if err := h.memorySvc.DeleteMemory(r.Context(), id); err != nil {
		writeJSONError(w, "删除记忆失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "记忆已删除"}) //nolint:errcheck
}

// HandleSearchMemories 搜索记忆（语义检索）
// POST /api/memory/search
func (h *ChatHandler) HandleSearchMemories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.memoryServiceRequired(w) {
		return
	}

	userID, _, _ := h.getCurrentUser(r)
	if userID == 0 {
		writeJSONError(w, "请先登录", http.StatusUnauthorized)
		return
	}

	var req struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Query == "" {
		writeJSONError(w, "查询内容不能为空", http.StatusBadRequest)
		return
	}

	result, err := h.memorySvc.RetrieveRelevantMemories(r.Context(), userID, req.Query, req.TopK)
	if err != nil {
		h.logger.Warn("记忆搜索失败", zap.Error(err))
		writeJSONError(w, "搜索失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"result": result,
	}) //nolint:errcheck
}
