// Package http 提供 HTTP 接口层实现
// 包括聊天、认证、会话管理、工具、Agent、知识库等 RESTful API
package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"aiProject/internal/application"
	"aiProject/internal/config"
	domain_cache "aiProject/internal/domain/cache"
	"aiProject/internal/domain/session"
	domain_trace "aiProject/internal/domain/trace"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// ─── 公共请求/响应结构体 ──────────────────────────────────────────────────────

// ChatRequest HTTP请求结构体
type ChatRequest struct {
	Message         string            `json:"message"`
	SessionID       string            `json:"session_id"`
	ModelName       string            `json:"model_name,omitempty"`
	SystemPrompt    string            `json:"system_prompt,omitempty"`
	EnabledTools    []string          `json:"enabled_tools,omitempty"`
	KnowledgeBaseID int64             `json:"knowledge_base_id,omitempty"` // RAG 知识库 ID
	PromptVars      map[string]string `json:"prompt_vars,omitempty"`       // 请求级 Prompt 模板变量
}

// ChatResponse HTTP响应结构体
type ChatResponse struct {
	Response           string `json:"response"`
	Thinking           string `json:"thinking,omitempty"`
	SessionID          string `json:"session_id"`
	ModelName          string `json:"model_name,omitempty"`
	PromptTokens       int    `json:"prompt_tokens"`
	CompletionTokens   int    `json:"completion_tokens"`
	TotalTokens        int    `json:"total_tokens"`
	SessionTotalTokens int    `json:"session_total_tokens"`
}

// HistoryRequest 历史记录请求结构体
type HistoryRequest struct {
	SessionID string `json:"session_id"`
}

// SessionListResponse 会话列表响应结构体
type SessionListResponse struct {
	Sessions []session.SessionInfo `json:"sessions"`
}

// HistoryResponse 历史记录响应结构体
type HistoryResponse struct {
	SessionID string            `json:"session_id"`
	Messages  []session.Message `json:"messages"`
}

// UpdateSystemPromptRequest 更新 System Prompt 请求
type UpdateSystemPromptRequest struct {
	SessionID    string `json:"session_id"`
	SystemPrompt string `json:"system_prompt"`
}

// ─── 公共辅助函数 ──────────────────────────────────────────────────────────────

// writeJSONError 统一返回 JSON 格式的错误响应
func writeJSONError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message}) //nolint:errcheck
}

// writeServiceError 根据应用层错误类型映射为合适的 HTTP 状态码
// ErrForbidden → 403，ErrUnauthorized → 401，其余 → 500
func writeServiceError(w http.ResponseWriter, err error, fallbackMsg string) {
	switch {
	case errors.Is(err, application.ErrForbidden):
		writeJSONError(w, "无权访问该资源", http.StatusForbidden)
	case errors.Is(err, application.ErrUnauthorized):
		writeJSONError(w, "请先登录", http.StatusUnauthorized)
	default:
		writeJSONError(w, fallbackMsg, http.StatusInternalServerError)
	}
}

// requireLogin 校验当前请求已登录，返回 userID；未登录时写入 401 并返回 (0,false)
func (h *ChatHandler) requireLogin(w http.ResponseWriter, r *http.Request) (int64, bool) {
	userID, _, _ := h.getCurrentUser(r)
	if userID <= 0 {
		writeJSONError(w, "请先登录", http.StatusUnauthorized)
		return 0, false
	}
	return userID, true
}

// truncate 截取字符串前 maxLen 个字符，用于日志预览
func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

// ─── ChatHandler 核心结构体 ────────────────────────────────────────────────────

// ChatHandler 聊天HTTP处理程序（聚合所有子 handler 的依赖）
type ChatHandler struct {
	chatService    *application.ChatService
	authService    *application.AuthService
	agentRegistry  *application.AgentRegistry
	knowledgeSvc   *application.KnowledgeService
	promptVarsSvc  *application.PromptVarsService // Prompt 模板变量服务
	memorySvc      *application.MemoryService     // 记忆服务（跨会话向量记忆）
	evalSvc        *application.EvalService       // Agent 评估服务
	workflowHandler *WorkflowHandler               // Workflow 工作流处理程序
	cache          domain_cache.Cache             // 缓存后端（用于监控/管理）
	cacheStats     domain_cache.StatsRecorder     // 缓存命中率统计
	traceStore     domain_trace.Store             // 可观测性 Trace 存储
	appConfig      *config.Config
	logger         *zap.Logger
}

// NewChatHandler 创建聊天处理程序
func NewChatHandler(chatService *application.ChatService, authService *application.AuthService, appConfig *config.Config) *ChatHandler {
	return &ChatHandler{
		chatService: chatService,
		authService: authService,
		appConfig:   appConfig,
		logger:      shared.GetLogger(),
	}
}

// SetAgentRegistry 注入 Agent 注册中心（多 Agent 模式下调用）
func (h *ChatHandler) SetAgentRegistry(registry *application.AgentRegistry) {
	h.agentRegistry = registry
}

// SetKnowledgeService 注入知识库服务
func (h *ChatHandler) SetKnowledgeService(ks *application.KnowledgeService) {
	h.knowledgeSvc = ks
}

// SetPromptVarsService 注入 Prompt 变量服务
func (h *ChatHandler) SetPromptVarsService(pvs *application.PromptVarsService) {
	h.promptVarsSvc = pvs
}

// SetMemoryService 注入记忆服务（启用跨会话向量记忆能力）
func (h *ChatHandler) SetMemoryService(ms *application.MemoryService) {
	h.memorySvc = ms
}

// SetEvalService 注入 Agent 评估服务
func (h *ChatHandler) SetEvalService(es *application.EvalService) {
	h.evalSvc = es
}

// SetCacheService 注入缓存后端与命中率统计（用于缓存监控页面）
func (h *ChatHandler) SetCacheService(c domain_cache.Cache, stats domain_cache.StatsRecorder) {
	h.cache = c
	h.cacheStats = stats
}

// SetWorkflowService 注入 Workflow 工作流服务
func (h *ChatHandler) SetWorkflowService(workflowSvc *application.WorkflowService, workflowEngine *application.WorkflowEngine) {
	h.workflowHandler = NewWorkflowHandler(workflowSvc, workflowEngine, h.logger, func(r *http.Request) int64 {
		userID, _, _ := h.getCurrentUser(r)
		return userID
	})
}

// GetWorkflowHandler 获取 Workflow 处理程序（供路由注册使用）
func (h *ChatHandler) GetWorkflowHandler() *WorkflowHandler {
	return h.workflowHandler
}

// ─── 聊天流式处理 ──────────────────────────────────────────────────────────────

// HandleChatStream 处理流式聊天请求（SSE）
func (h *ChatHandler) HandleChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, userName, _ := h.getCurrentUser(r)

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Message == "" {
		writeJSONError(w, "Message is required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONError(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// 解析多 Agent 模式下的参数（system_prompt / chatSvc / tools）
	enabledTools, systemPrompt, modelName, chatSvc := h.resolveMasterAgent(req)

	h.logger.Debug("HandleChatStream 参数",
		zap.Strings("enabled_tools", enabledTools),
		zap.String("system_prompt_prefix", truncate(systemPrompt, 80)),
		zap.Bool("using_master_chatSvc", chatSvc != h.chatService),
	)

	appReq := application.ChatRequest{
		Message:         req.Message,
		SessionID:       session.SessionID(req.SessionID),
		UserID:          userID,
		UserName:        userName,
		ModelName:       modelName,
		SystemPrompt:    systemPrompt,
		EnabledTools:    enabledTools,
		KnowledgeBaseID: req.KnowledgeBaseID,
		PromptVars:      req.PromptVars,
	}

	sendSSE := func(data interface{}) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	// 根据是否启用工具选择处理路径（均为流式 SSE）
	var (
		streamCh <-chan application.StreamChatResponse
		err      error
	)
	if len(enabledTools) > 0 {
		streamCh, err = chatSvc.ProcessMessageWithTools(r.Context(), appReq, enabledTools)
	} else {
		streamCh, err = chatSvc.ProcessMessageStream(r.Context(), appReq)
	}
	if err != nil {
		h.logger.Error("启动流式处理失败", zap.Error(err))
		sendSSE(map[string]string{"type": "error", "error": "启动失败"})
		return
	}

	// 消费流式事件并推送 SSE
	for event := range streamCh {
		sendSSE(buildSSEPayload(event))
	}
}

// resolveMasterAgent 解析多 Agent 模式下的参数
// 优先使用主 Agent 的配置（system_prompt / chatSvc / tools）
func (h *ChatHandler) resolveMasterAgent(req ChatRequest) (enabledTools []string, systemPrompt, modelName string, chatSvc *application.ChatService) {
	enabledTools = req.EnabledTools
	systemPrompt = req.SystemPrompt
	modelName = req.ModelName
	chatSvc = h.chatService

	if h.agentRegistry == nil {
		return
	}
	master, ok := h.agentRegistry.GetMaster()
	if !ok {
		return
	}

	// 无论前端是否传了工具，始终使用主 Agent 的 ChatService 和 SystemPrompt
	chatSvc = master.ChatService
	if systemPrompt == "" {
		systemPrompt = master.Def.SystemPrompt
	}
	if modelName == "" {
		modelName = master.Def.ModelName
	}
	// 前端未指定工具时，自动使用主 Agent 的工具列表
	if len(enabledTools) == 0 {
		enabledTools = master.Def.EnabledTools
	}
	return
}

// buildSSEPayload 根据流式事件类型构建 SSE 推送数据
func buildSSEPayload(event application.StreamChatResponse) map[string]interface{} {
	switch event.Type {
	case "chunk":
		return map[string]interface{}{
			"type":       "chunk",
			"content":    event.Content,
			"thinking":   event.Thinking,
			"session_id": string(event.SessionID),
			"model_name": event.ModelName,
		}
	case "thought":
		return map[string]interface{}{
			"type":                "thought",
			"content":             event.Content,
			"step":                event.Step,
			"parent_tool_call_id": event.ParentToolCallID,
			"session_id":          string(event.SessionID),
		}
	case "tool_call":
		return map[string]interface{}{
			"type":                "tool_call",
			"tool_name":           event.ToolName,
			"tool_display_name":   event.ToolDisplayName,
			"tool_call_id":        event.ToolCallID,
			"tool_args":           event.ToolArgs,
			"step":                event.Step,
			"parent_tool_call_id": event.ParentToolCallID,
			"session_id":          string(event.SessionID),
		}
	case "tool_result":
		return map[string]interface{}{
			"type":                "tool_result",
			"tool_name":           event.ToolName,
			"tool_display_name":   event.ToolDisplayName,
			"tool_call_id":        event.ToolCallID,
			"tool_args":           event.ToolArgs,
			"tool_result":         event.ToolResult,
			"step":                event.Step,
			"parent_tool_call_id": event.ParentToolCallID,
			"session_id":          string(event.SessionID),
		}
	case "done":
		return map[string]interface{}{
			"type":                 "done",
			"session_id":           string(event.SessionID),
			"model_name":           event.ModelName,
			"prompt_tokens":        event.Usage.PromptTokens,
			"completion_tokens":    event.Usage.CompletionTokens,
			"total_tokens":         event.Usage.TotalTokens,
			"session_total_tokens": event.SessionTotalTokens,
		}
	case "error":
		return map[string]interface{}{
			"type":  "error",
			"error": event.Error,
		}
	default:
		return map[string]interface{}{"type": event.Type}
	}
}

// ─── 会话管理 ──────────────────────────────────────────────────────────────────

// HandleGetHistory 获取会话历史记录
func (h *ChatHandler) HandleGetHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req HistoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		writeJSONError(w, "Session ID is required", http.StatusBadRequest)
		return
	}

	userID, _, _ := h.getCurrentUser(r)

	messages, err := h.chatService.GetSessionHistory(r.Context(), session.SessionID(req.SessionID), userID)
	if err != nil {
		h.logger.Error("获取历史记录失败", zap.String("session_id", req.SessionID), zap.Error(err))
		writeServiceError(w, err, "Failed to get history")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(HistoryResponse{
		SessionID: req.SessionID,
		Messages:  messages,
	})
}

// HandleListSessions 列出会话
func (h *ChatHandler) HandleListSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, _, _ := h.getCurrentUser(r)

	sessions, err := h.chatService.ListSessionsByUser(r.Context(), userID)
	if err != nil {
		h.logger.Error("获取会话列表失败", zap.Error(err))
		writeJSONError(w, "Failed to list sessions", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SessionListResponse{Sessions: sessions})
}

// HandleDeleteSession 删除会话
func (h *ChatHandler) HandleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req HistoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		writeJSONError(w, "Session ID is required", http.StatusBadRequest)
		return
	}

	userID, _, _ := h.getCurrentUser(r)

	if err := h.chatService.DeleteSession(r.Context(), session.SessionID(req.SessionID), userID); err != nil {
		h.logger.Error("删除会话失败", zap.String("session_id", req.SessionID), zap.Error(err))
		writeServiceError(w, err, "Failed to delete session")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Session deleted successfully"})
}

// HandleRenameSession 重命名会话
func (h *ChatHandler) HandleRenameSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
		Title     string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		writeJSONError(w, "Session ID is required", http.StatusBadRequest)
		return
	}
	if req.Title == "" {
		writeJSONError(w, "Title is required", http.StatusBadRequest)
		return
	}

	userID, _, _ := h.getCurrentUser(r)

	if err := h.chatService.RenameSession(r.Context(), session.SessionID(req.SessionID), userID, req.Title); err != nil {
		h.logger.Error("重命名会话失败", zap.String("session_id", req.SessionID), zap.Error(err))
		writeServiceError(w, err, "Failed to rename session")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Session renamed successfully"})
}

// HandleUpdateSystemPrompt 更新会话的 System Prompt
func (h *ChatHandler) HandleUpdateSystemPrompt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req UpdateSystemPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		writeJSONError(w, "Session ID is required", http.StatusBadRequest)
		return
	}

	userID, _, _ := h.getCurrentUser(r)

	if err := h.chatService.UpdateSessionSystemPrompt(r.Context(), session.SessionID(req.SessionID), userID, req.SystemPrompt); err != nil {
		h.logger.Error("更新 System Prompt 失败", zap.String("session_id", req.SessionID), zap.Error(err))
		writeServiceError(w, err, "Failed to update system prompt")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "System Prompt 已更新"})
}

// HandleGetSystemPrompt 获取会话的 System Prompt
func (h *ChatHandler) HandleGetSystemPrompt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessID := r.URL.Query().Get("session_id")
	if sessID == "" {
		writeJSONError(w, "session_id is required", http.StatusBadRequest)
		return
	}

	userID, _, _ := h.getCurrentUser(r)

	prompt, err := h.chatService.GetSessionSystemPrompt(r.Context(), session.SessionID(sessID), userID)
	if err != nil {
		h.logger.Error("获取 System Prompt 失败", zap.String("session_id", sessID), zap.Error(err))
		writeServiceError(w, err, "Failed to get system prompt")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"system_prompt": prompt})
}
