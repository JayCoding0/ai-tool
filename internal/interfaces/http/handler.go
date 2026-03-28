// Package http 提供 HTTP 接口层实现
// 包括聊天、认证、会话管理、工具、Agent、知识库等 RESTful API
package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"aiProject/internal/application"
	"aiProject/internal/config"
	"aiProject/internal/domain/knowledge"
	"aiProject/internal/domain/session"
	domain_tool "aiProject/internal/domain/tool"
	"aiProject/internal/infrastructure/database"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// ChatRequest HTTP请求结构体
type ChatRequest struct {
	Message         string   `json:"message"`
	SessionID       string   `json:"session_id"`
	ModelName       string   `json:"model_name,omitempty"`
	SystemPrompt    string   `json:"system_prompt,omitempty"`
	EnabledTools    []string `json:"enabled_tools,omitempty"`
	KnowledgeBaseID int64    `json:"knowledge_base_id,omitempty"` // RAG 知识库 ID
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

// writeJSONError 统一返回 JSON 格式的错误响应
func writeJSONError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message}) //nolint:errcheck
}

// setAuthCookie 设置认证 Cookie
func setAuthCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// truncate 截取字符串前 maxLen 个字符，用于日志预览
func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

// ChatHandler 聊天HTTP处理程序
type ChatHandler struct {
	chatService   *application.ChatService
	authService   *application.AuthService
	agentRegistry *application.AgentRegistry
	knowledgeSvc  *application.KnowledgeService
	appConfig     *config.Config
	logger        *zap.Logger
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

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthResponse 认证响应
type AuthResponse struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Token    string `json:"token"`
}

// extractToken 从请求中提取token
func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	cookie, err := r.Cookie("auth_token")
	if err == nil {
		return cookie.Value
	}
	return ""
}

// getCurrentUser 从请求中获取当前用户信息，返回 userID、username、role
func (h *ChatHandler) getCurrentUser(r *http.Request) (int64, string, string) {
	token := extractToken(r)
	if token == "" {
		return 0, "", "guest"
	}
	userID, username, role, err := h.authService.ValidateToken(token)
	if err != nil {
		return 0, "", "guest"
	}
	return userID, username, role
}

// HandleRegister 处理注册请求
func (h *ChatHandler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if h.authService == nil {
		writeJSONError(w, "用户功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	resp, err := h.authService.Register(r.Context(), application.RegisterRequest{
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		h.logger.Warn("注册失败", zap.String("username", req.Username), zap.Error(err))
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	setAuthCookie(w, resp.Token, 86400)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AuthResponse{
		UserID:   resp.UserID,
		Username: resp.Username,
		Role:     "user",
		Token:    resp.Token,
	})
}

// HandleLogin 处理登录请求
func (h *ChatHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if h.authService == nil {
		writeJSONError(w, "用户功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	resp, err := h.authService.Login(r.Context(), application.LoginRequest{
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		h.logger.Warn("登录失败", zap.String("username", req.Username), zap.Error(err))
		writeJSONError(w, err.Error(), http.StatusUnauthorized)
		return
	}

	setAuthCookie(w, resp.Token, 86400)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AuthResponse{
		UserID:   resp.UserID,
		Username: resp.Username,
		Role:     resp.Role,
		Token:    resp.Token,
	})
}

// HandleLogout 处理登出请求
func (h *ChatHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	if token != "" {
		h.authService.Logout(token)
	}
	http.SetCookie(w, &http.Cookie{
		Name:   "auth_token",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "已登出"})
}

// HandleMe 获取当前登录用户信息
func (h *ChatHandler) HandleMe(w http.ResponseWriter, r *http.Request) {
	userID, username, role := h.getCurrentUser(r)
	if userID == 0 {
		writeJSONError(w, "未登录", http.StatusUnauthorized)
		return
	}
	totalTokens, _ := h.chatService.GetUserTotalTokens(r.Context(), userID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user_id":      userID,
		"username":     username,
		"role":         role,
		"total_tokens": totalTokens,
	})
}

// HandleChatStream 处理流式聊天请求（SSE）
func (h *ChatHandler) HandleChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, _, _ := h.getCurrentUser(r)

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

	// 多 Agent 模式：优先使用主 Agent 的配置（system_prompt / chatSvc / tools）
	enabledTools := req.EnabledTools
	systemPrompt := req.SystemPrompt
	modelName := req.ModelName
	chatSvc := h.chatService
	if h.agentRegistry != nil {
		if master, ok := h.agentRegistry.GetMaster(); ok {
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
		}
	}

	// 调试日志：打印 handler 层实际注入的参数
	h.logger.Debug("HandleChatStream 参数",
		zap.Strings("enabled_tools", enabledTools),
		zap.String("system_prompt_prefix", truncate(systemPrompt, 80)),
		zap.Bool("using_master_chatSvc", chatSvc != h.chatService),
	)

	appReq := application.ChatRequest{
		Message:         req.Message,
		SessionID:       session.SessionID(req.SessionID),
		UserID:          userID,
		ModelName:       modelName,
		SystemPrompt:    systemPrompt,
		EnabledTools:    enabledTools,
		KnowledgeBaseID: req.KnowledgeBaseID,
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

	for event := range streamCh {
		switch event.Type {
		case "chunk":
			sendSSE(map[string]interface{}{
				"type":       "chunk",
				"content":    event.Content,
				"thinking":   event.Thinking,
				"session_id": string(event.SessionID),
				"model_name": event.ModelName,
			})
		case "thought":
			sendSSE(map[string]interface{}{
				"type":               "thought",
				"content":            event.Content,
				"step":               event.Step,
				"parent_tool_call_id": event.ParentToolCallID,
				"session_id":         string(event.SessionID),
			})
		case "tool_call":
			sendSSE(map[string]interface{}{
				"type":               "tool_call",
				"tool_name":          event.ToolName,
				"tool_display_name":  event.ToolDisplayName,
				"tool_call_id":       event.ToolCallID,
				"tool_args":          event.ToolArgs,
				"step":               event.Step,
				"parent_tool_call_id": event.ParentToolCallID,
				"session_id":         string(event.SessionID),
			})
		case "tool_result":
			sendSSE(map[string]interface{}{
				"type":               "tool_result",
				"tool_name":          event.ToolName,
				"tool_display_name":  event.ToolDisplayName,
				"tool_call_id":       event.ToolCallID,
				"tool_args":          event.ToolArgs,
				"tool_result":        event.ToolResult,
				"step":               event.Step,
				"parent_tool_call_id": event.ParentToolCallID,
				"session_id":         string(event.SessionID),
			})
		case "done":
			sendSSE(map[string]interface{}{
				"type":                 "done",
				"session_id":           string(event.SessionID),
				"model_name":           event.ModelName,
				"prompt_tokens":        event.Usage.PromptTokens,
				"completion_tokens":    event.Usage.CompletionTokens,
				"total_tokens":         event.Usage.TotalTokens,
				"session_total_tokens": event.SessionTotalTokens,
			})
		case "error":
			sendSSE(map[string]interface{}{
				"type":  "error",
				"error": event.Error,
			})
		}
	}
}

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

	messages, err := h.chatService.GetSessionHistory(r.Context(), session.SessionID(req.SessionID))
	if err != nil {
		h.logger.Error("获取历史记录失败", zap.String("session_id", req.SessionID), zap.Error(err))
		writeJSONError(w, "Failed to get history", http.StatusInternalServerError)
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

	var sessions []session.SessionInfo
	var err error
	if userID > 0 {
		sessions, err = h.chatService.ListSessionsByUser(r.Context(), userID)
	} else {
		sessions, err = h.chatService.ListSessions(r.Context())
	}
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

	if err := h.chatService.DeleteSession(r.Context(), session.SessionID(req.SessionID)); err != nil {
		h.logger.Error("删除会话失败", zap.String("session_id", req.SessionID), zap.Error(err))
		writeJSONError(w, "Failed to delete session", http.StatusInternalServerError)
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

	if err := h.chatService.RenameSession(r.Context(), session.SessionID(req.SessionID), req.Title); err != nil {
		h.logger.Error("重命名会话失败", zap.String("session_id", req.SessionID), zap.Error(err))
		writeJSONError(w, "Failed to rename session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Session renamed successfully"})
}
type ModelOption struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

// inferModelType 根据模型名和配置推断模型类型
func inferModelType(name, cfgType, globalType string) string {
	if cfgType != "" {
		return cfgType
	}
	if strings.Contains(name, ":") {
		return "local"
	}
	if globalType == "local" {
		return "local"
	}
	return "cloud"
}

// ollamaTagsResponse Ollama /api/tags 接口响应结构
type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// fetchOllamaModels 实时从 Ollama 拉取本地已安装的模型列表
func fetchOllamaModels(ollamaURL string) ([]ModelOption, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/api/tags", strings.TrimRight(ollamaURL, "/")))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama 返回状态码: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var tagsResp ollamaTagsResponse
	if err := json.Unmarshal(body, &tagsResp); err != nil {
		return nil, err
	}
	var models []ModelOption
	for _, m := range tagsResp.Models {
		label := strings.TrimSuffix(m.Name, ":latest")
		models = append(models, ModelOption{Name: m.Name, Label: label + " (本地)", Type: "local"})
	}
	return models, nil
}

// HandleListModels 返回可用模型列表
func (h *ChatHandler) HandleListModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var models []ModelOption
	if h.appConfig != nil {
		globalType := h.appConfig.Model.Type
		for _, m := range h.appConfig.Model.AvailableModels {
			modelType := inferModelType(m.Name, m.Type, globalType)
			if modelType != "local" {
				models = append(models, ModelOption{Name: m.Name, Label: m.Label, Type: modelType})
			}
		}
		ollamaURL := h.appConfig.Model.OllamaURL
		if ollamaURL == "" {
			ollamaURL = "http://localhost:11434"
		}
		if localModels, err := fetchOllamaModels(ollamaURL); err != nil {
			shared.GetLogger().Debug("Ollama 未启动，跳过本地模型拉取", zap.Error(err))
		} else {
			models = append(models, localModels...)
		}
	}
	if len(models) == 0 && h.appConfig != nil {
		models = []ModelOption{
			{Name: h.appConfig.Model.Name, Label: h.appConfig.Model.Name,
				Type: inferModelType(h.appConfig.Model.Name, "", h.appConfig.Model.Type)},
		}
	}

	defaultModel := ""
	if h.appConfig != nil {
		defaultModel = h.appConfig.Model.Name
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"models":        models,
		"default_model": defaultModel,
	})
}

// UpdateSystemPromptRequest 更新 System Prompt 请求
type UpdateSystemPromptRequest struct {
	SessionID    string `json:"session_id"`
	SystemPrompt string `json:"system_prompt"`
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

	if err := h.chatService.UpdateSessionSystemPrompt(r.Context(), session.SessionID(req.SessionID), req.SystemPrompt); err != nil {
		h.logger.Error("更新 System Prompt 失败", zap.String("session_id", req.SessionID), zap.Error(err))
		writeJSONError(w, "Failed to update system prompt", http.StatusInternalServerError)
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

	prompt, err := h.chatService.GetSessionSystemPrompt(r.Context(), session.SessionID(sessID))
	if err != nil {
		h.logger.Error("获取 System Prompt 失败", zap.String("session_id", sessID), zap.Error(err))
		writeJSONError(w, "Failed to get system prompt", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"system_prompt": prompt})
}

// HandleListTools 列出所有已注册的工具
func (h *ChatHandler) HandleListTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defs := domain_tool.ListAll()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tools": defs,
		"count": len(defs),
	})
}

// AgentToolInfo Agent 工具信息（用于前端展示）
type AgentToolInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
}

// AgentInfo Agent 信息（用于前端展示）
type AgentInfo struct {
	Name         string          `json:"name"`
	DisplayName  string          `json:"display_name"`
	Description  string          `json:"description"`
	IsMaster     bool            `json:"is_master"`
	DefaultTools []string        `json:"default_tools"` // 该 Agent 默认启用的工具名称列表
	Tools        []AgentToolInfo `json:"tools"`         // 该 Agent 可用工具的详细信息
}

// HandleListAgents 列出所有 Agent 及其工具信息
func (h *ChatHandler) HandleListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// 无多 Agent 模式，返回空列表
	if h.agentRegistry == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"agents": []AgentInfo{},
			"count":  0,
		})
		return
	}

	instances := h.agentRegistry.ListAll()
	agents := make([]AgentInfo, 0, len(instances))
	for _, inst := range instances {
		// 获取该 Agent 每个工具的详细信息
		tools := make([]AgentToolInfo, 0, len(inst.Def.EnabledTools))
		for _, toolName := range inst.Def.EnabledTools {
			if t, ok := domain_tool.Get(toolName); ok {
				tools = append(tools, AgentToolInfo{
					Name:        t.Definition.Name,
					DisplayName: t.Definition.DisplayName,
					Description: t.Definition.Description,
				})
			} else {
				// 工具未注册时仍返回名称（如 call_agent 是动态注册的）
				tools = append(tools, AgentToolInfo{
					Name:        toolName,
					DisplayName: domain_tool.GetDisplayName(toolName),
					Description: "",
				})
			}
		}
		agents = append(agents, AgentInfo{
			Name:         inst.Def.Name,
			DisplayName:  inst.Def.DisplayName,
			Description:  inst.Def.Description,
			IsMaster:     inst.Def.IsMaster,
			DefaultTools: inst.Def.EnabledTools,
			Tools:        tools,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"agents": agents,
		"count":  len(agents),
	})
}

// HandleUpdateAgentTools 动态更新指定 Agent 的工具列表
// PUT /api/agents/{name}/tools
// Body: { "tools": ["weather", "http_request"] }
func (h *ChatHandler) HandleUpdateAgentTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.agentRegistry == nil {
		writeJSONError(w, "多 Agent 模式未启用", http.StatusServiceUnavailable)
		return
	}

	// 从路径中提取 agent name：/api/agents/{name}/tools
	path := r.URL.Path // e.g. /api/agents/weather_agent/tools
	// 去掉前缀 /api/agents/ 和后缀 /tools
	trimmed := strings.TrimPrefix(path, "/api/agents/")
	agentName := strings.TrimSuffix(trimmed, "/tools")
	if agentName == "" || agentName == path {
		writeJSONError(w, "无效的 Agent 名称", http.StatusBadRequest)
		return
	}

	var req struct {
		Tools []string `json:"tools"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "请求体解析失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Tools == nil {
		req.Tools = []string{}
	}

	// 热更新内存中的 Agent 工具列表
	if err := h.agentRegistry.UpdateTools(agentName, req.Tools); err != nil {
		writeJSONError(w, err.Error(), http.StatusNotFound)
		return
	}

	// 持久化到数据库（如果数据库可用）
	if db := database.GetDB(); db != nil {
		ctx := r.Context()
		// 先删除该 Agent 的旧配置，再批量插入新配置
		_, err := db.Exec(ctx, "DELETE FROM agent_tools WHERE agent_name = ?", agentName)
		if err != nil {
			shared.GetLogger().Warn("删除 Agent 旧工具配置失败", zap.String("agent", agentName), zap.Error(err))
		} else if len(req.Tools) > 0 {
			// 批量插入
			vals := make([]interface{}, 0, len(req.Tools)*2)
			placeholders := make([]string, 0, len(req.Tools))
			for _, t := range req.Tools {
				placeholders = append(placeholders, "(?, ?)")
				vals = append(vals, agentName, t)
			}
			query := "INSERT INTO agent_tools (agent_name, tool_name) VALUES " + strings.Join(placeholders, ",")
			if _, err := db.Exec(ctx, query, vals...); err != nil {
				shared.GetLogger().Warn("保存 Agent 工具配置失败", zap.String("agent", agentName), zap.Error(err))
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"agent_name": agentName,
		"tools":      req.Tools,
	})
}

// ─── 知识库 API ────────────────────────────────────────────────────────────────

// knowledgeServiceRequired 检查知识库服务是否可用
func (h *ChatHandler) knowledgeServiceRequired(w http.ResponseWriter) bool {
	if h.knowledgeSvc == nil {
		writeJSONError(w, "知识库功能未启用（RAG 服务未初始化）", http.StatusServiceUnavailable)
		return false
	}
	return true
}

// HandleListKnowledgeBases 列出知识库
func (h *ChatHandler) HandleListKnowledgeBases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.knowledgeServiceRequired(w) {
		return
	}
	userID, _, _ := h.getCurrentUser(r)
	list, err := h.knowledgeSvc.ListKnowledgeBases(r.Context(), userID)
	if err != nil {
		writeJSONError(w, "获取知识库列表失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = make([]*knowledge.KnowledgeBase, 0)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"knowledge_bases": list, "count": len(list)}) //nolint:errcheck
}

// HandleCreateKnowledgeBase 创建知识库
func (h *ChatHandler) HandleCreateKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.knowledgeServiceRequired(w) {
		return
	}
	userID, _, _ := h.getCurrentUser(r)
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	kb, err := h.knowledgeSvc.CreateKnowledgeBase(r.Context(), userID, req.Name, req.Description)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(kb) //nolint:errcheck
}

// HandleDeleteKnowledgeBase 删除知识库
func (h *ChatHandler) HandleDeleteKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.knowledgeServiceRequired(w) {
		return
	}
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSONError(w, "无效的知识库 ID", http.StatusBadRequest)
		return
	}
	if err := h.knowledgeSvc.DeleteKnowledgeBase(r.Context(), id); err != nil {
		writeJSONError(w, "删除知识库失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "知识库已删除"}) //nolint:errcheck
}

// HandleListDocuments 列出知识库下的文档
func (h *ChatHandler) HandleListDocuments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.knowledgeServiceRequired(w) {
		return
	}
	kbIDStr := r.URL.Query().Get("kb_id")
	kbID, err := strconv.ParseInt(kbIDStr, 10, 64)
	if err != nil || kbID <= 0 {
		writeJSONError(w, "无效的知识库 ID", http.StatusBadRequest)
		return
	}
	docs, err := h.knowledgeSvc.ListDocuments(r.Context(), kbID)
	if err != nil {
		writeJSONError(w, "获取文档列表失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if docs == nil {
		docs = make([]*knowledge.Document, 0)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"documents": docs, "count": len(docs)}) //nolint:errcheck
}

// HandleUploadDocument 上传文档到知识库（multipart/form-data 或 JSON）
func (h *ChatHandler) HandleUploadDocument(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.knowledgeServiceRequired(w) {
		return
	}

	contentType := r.Header.Get("Content-Type")

	var kbID int64
	var docName, docContent, docType string

	if strings.Contains(contentType, "multipart/form-data") {
		// 文件上传模式
		if err := r.ParseMultipartForm(32 << 20); err != nil { // 最大 32MB
			writeJSONError(w, "解析表单失败: "+err.Error(), http.StatusBadRequest)
			return
		}
		kbIDStr := r.FormValue("kb_id")
		var err error
		kbID, err = strconv.ParseInt(kbIDStr, 10, 64)
		if err != nil || kbID <= 0 {
			writeJSONError(w, "无效的知识库 ID", http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSONError(w, "获取上传文件失败: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()

		data, err := io.ReadAll(file)
		if err != nil {
			writeJSONError(w, "读取文件失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		docName = header.Filename
		docContent = string(data)
		docType = detectContentType(header.Filename)
	} else {
		// JSON 模式（直接传文本内容）
		var req struct {
			KbID        int64  `json:"kb_id"`
			Name        string `json:"name"`
			Content     string `json:"content"`
			ContentType string `json:"content_type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		kbID = req.KbID
		docName = req.Name
		docContent = req.Content
		docType = req.ContentType
		if docType == "" {
			docType = "text"
		}
	}

	if kbID <= 0 {
		writeJSONError(w, "无效的知识库 ID", http.StatusBadRequest)
		return
	}
	if utf8.RuneCountInString(docContent) == 0 {
		writeJSONError(w, "文档内容不能为空", http.StatusBadRequest)
		return
	}

	doc, err := h.knowledgeSvc.AddDocument(r.Context(), kbID, docName, docType, docContent)
	if err != nil {
		writeJSONError(w, "添加文档失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(doc) //nolint:errcheck
}

// HandleDeleteDocument 删除文档
func (h *ChatHandler) HandleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.knowledgeServiceRequired(w) {
		return
	}
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSONError(w, "无效的文档 ID", http.StatusBadRequest)
		return
	}
	if err := h.knowledgeSvc.DeleteDocument(r.Context(), id); err != nil {
		writeJSONError(w, "删除文档失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "文档已删除"}) //nolint:errcheck
}

// HandleKnowledgeSearch 手动测试知识库检索
func (h *ChatHandler) HandleKnowledgeSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.knowledgeServiceRequired(w) {
		return
	}
	var req struct {
		KbID  int64  `json:"kb_id"`
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	chunks, err := h.knowledgeSvc.Search(r.Context(), req.KbID, req.Query, req.TopK)
	if err != nil {
		writeJSONError(w, "检索失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	type resultItem struct {
		Content string  `json:"content"`
		Score   float32 `json:"score"`
		DocName string  `json:"doc_name"`
	}
	results := make([]resultItem, 0, len(chunks))
	for _, sc := range chunks {
		results = append(results, resultItem{
			Content: sc.Chunk.Content,
			Score:   sc.Score,
			DocName: sc.DocName,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"results": results, "count": len(results)}) //nolint:errcheck
}

// detectContentType 根据文件名推断内容类型
func detectContentType(filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown"):
		return "markdown"
	case strings.HasSuffix(lower, ".pdf"):
		return "pdf"
	default:
		return "text"
	}
}

