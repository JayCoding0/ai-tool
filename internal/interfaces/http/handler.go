package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aiProject/internal/application"
	"aiProject/internal/config"
	"aiProject/internal/domain/session"
	domain_skill "aiProject/internal/domain/skill"
	domain_tool "aiProject/internal/domain/tool"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// ChatRequest HTTP请求结构体
type ChatRequest struct {
	Message      string   `json:"message"`
	SessionID    string   `json:"session_id"`
	ModelName    string   `json:"model_name,omitempty"`
	SystemPrompt string   `json:"system_prompt,omitempty"`
	EnabledTools []string `json:"enabled_tools,omitempty"`
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

// ChatHandler 聊天HTTP处理程序
type ChatHandler struct {
	chatService  *application.ChatService
	authService  *application.AuthService
	skillService *application.SkillService
	appConfig    *config.Config
	logger       *zap.Logger
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

// SetSkillService 设置技能服务
func (h *ChatHandler) SetSkillService(skillService *application.SkillService) {
	h.skillService = skillService
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

	appReq := application.ChatRequest{
		Message:      req.Message,
		SessionID:    session.SessionID(req.SessionID),
		UserID:       userID,
		ModelName:    req.ModelName,
		SystemPrompt: req.SystemPrompt,
		EnabledTools: req.EnabledTools,
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
	if len(req.EnabledTools) > 0 {
		streamCh, err = h.chatService.ProcessMessageWithTools(r.Context(), appReq, req.EnabledTools)
	} else {
		streamCh, err = h.chatService.ProcessMessageStream(r.Context(), appReq)
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
				"type":       "thought",
				"content":    event.Content,
				"step":       event.Step,
				"session_id": string(event.SessionID),
			})
		case "tool_call":
			sendSSE(map[string]interface{}{
				"type":             "tool_call",
				"tool_name":        event.ToolName,
				"tool_display_name": event.ToolDisplayName,
				"tool_call_id":     event.ToolCallID,
				"tool_args":        event.ToolArgs,
				"step":             event.Step,
				"session_id":       string(event.SessionID),
			})
		case "tool_result":
			sendSSE(map[string]interface{}{
				"type":             "tool_result",
				"tool_name":        event.ToolName,
				"tool_display_name": event.ToolDisplayName,
				"tool_call_id":     event.ToolCallID,
				"tool_args":        event.ToolArgs,
				"tool_result":      event.ToolResult,
				"step":             event.Step,
				"session_id":       string(event.SessionID),
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
			shared.GetLogger().Warn("拉取 Ollama 模型列表失败", zap.Error(err))
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

// ===== Skills 接口 =====

// SkillRequest 创建/更新技能请求
type SkillRequest struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Icon         string   `json:"icon"`
	Pattern      string   `json:"pattern"`
	SystemPrompt string   `json:"system_prompt"`
	Tools        []string `json:"tools"`
	IsPublic     bool     `json:"is_public"`
}

// HandleListSkills 列出技能列表
func (h *ChatHandler) HandleListSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.skillService == nil {
		writeJSONError(w, "技能功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}

	userID, _, role := h.getCurrentUser(r)
	isAdmin := role == "admin"
	skills, err := h.skillService.ListSkills(r.Context(), userID, isAdmin)
	if err != nil {
		h.logger.Error("获取技能列表失败", zap.Error(err))
		writeJSONError(w, "Failed to list skills", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"skills": skills})
}

// HandleCreateSkill 创建技能
func (h *ChatHandler) HandleCreateSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.skillService == nil {
		writeJSONError(w, "技能功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}

	userID, _, _ := h.getCurrentUser(r)
	if userID == 0 {
		writeJSONError(w, "请先登录", http.StatusUnauthorized)
		return
	}

	var req SkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	sk, err := h.skillService.CreateSkill(r.Context(), application.CreateSkillRequest{
		UserID:       userID,
		Name:         req.Name,
		Description:  req.Description,
		Icon:         req.Icon,
		Pattern:      domain_skill.SkillPattern(req.Pattern),
		SystemPrompt: req.SystemPrompt,
		Tools:        req.Tools,
		IsPublic:     req.IsPublic,
	})
	if err != nil {
		h.logger.Error("创建技能失败", zap.Error(err))
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(sk)
}

// HandleUpdateSkill 更新技能
func (h *ChatHandler) HandleUpdateSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.skillService == nil {
		writeJSONError(w, "技能功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}

	userID, _, role := h.getCurrentUser(r)
	if userID == 0 {
		writeJSONError(w, "请先登录", http.StatusUnauthorized)
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeJSONError(w, "id is required", http.StatusBadRequest)
		return
	}
	var skillID int64
	if _, err := fmt.Sscanf(idStr, "%d", &skillID); err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}

	var req SkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	sk, err := h.skillService.UpdateSkill(r.Context(), application.UpdateSkillRequest{
		ID:           domain_skill.SkillID(skillID),
		UserID:       userID,
		IsAdmin:      role == "admin",
		Name:         req.Name,
		Description:  req.Description,
		Icon:         req.Icon,
		Pattern:      domain_skill.SkillPattern(req.Pattern),
		SystemPrompt: req.SystemPrompt,
		Tools:        req.Tools,
		IsPublic:     req.IsPublic,
	})
	if err != nil {
		h.logger.Error("更新技能失败", zap.Error(err))
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sk)
}

// HandleDeleteSkill 删除技能
func (h *ChatHandler) HandleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.skillService == nil {
		writeJSONError(w, "技能功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}

	userID, _, role := h.getCurrentUser(r)
	if userID == 0 {
		writeJSONError(w, "请先登录", http.StatusUnauthorized)
		return
	}

	var body struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := h.skillService.DeleteSkill(r.Context(), domain_skill.SkillID(body.ID), userID, role == "admin"); err != nil {
		h.logger.Error("删除技能失败", zap.Error(err))
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "技能已删除"})
}

// HandleApplySkill 将技能应用到会话
func (h *ChatHandler) HandleApplySkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.skillService == nil {
		writeJSONError(w, "技能功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}

	var body struct {
		SkillID   int64  `json:"skill_id"`
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if body.SessionID == "" {
		writeJSONError(w, "session_id is required", http.StatusBadRequest)
		return
	}

	sk, err := h.skillService.GetSkill(r.Context(), domain_skill.SkillID(body.SkillID))
	if err != nil || sk == nil {
		writeJSONError(w, "技能不存在", http.StatusNotFound)
		return
	}

	if err := h.chatService.UpdateSessionSystemPrompt(r.Context(), session.SessionID(body.SessionID), sk.SystemPrompt); err != nil {
		h.logger.Error("应用技能失败", zap.Error(err))
		writeJSONError(w, "Failed to apply skill", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":       "技能已应用",
		"skill_name":    sk.Name,
		"system_prompt": sk.SystemPrompt,
	})
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

// requireAdmin 检查当前请求是否为 admin，不是则直接返回 403
func (h *ChatHandler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	_, _, role := h.getCurrentUser(r)
	if role != "admin" {
		writeJSONError(w, "需要管理员权限", http.StatusForbidden)
		return false
	}
	return true
}

// HandleAdminDownloadSkill admin 下载指定技能（返回完整 JSON）
func (h *ChatHandler) HandleAdminDownloadSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.skillService == nil {
		writeJSONError(w, "技能功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}
	if !h.requireAdmin(w, r) {
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeJSONError(w, "id is required", http.StatusBadRequest)
		return
	}
	var skillID int64
	if _, err := fmt.Sscanf(idStr, "%d", &skillID); err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}

	sk, err := h.skillService.GetSkill(r.Context(), domain_skill.SkillID(skillID))
	if err != nil || sk == nil {
		writeJSONError(w, "技能不存在", http.StatusNotFound)
		return
	}

	filename := fmt.Sprintf("skill_%d_%s.json", sk.ID, sk.Name)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	json.NewEncoder(w).Encode(sk)
}

// HandleAdminUploadSkill admin 上传/导入技能（JSON 格式）
func (h *ChatHandler) HandleAdminUploadSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.skillService == nil {
		writeJSONError(w, "技能功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}
	if !h.requireAdmin(w, r) {
		return
	}

	var req SkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// admin 上传的 skill 归属为系统预设（user_id=0）
	sk, err := h.skillService.CreateSkill(r.Context(), application.CreateSkillRequest{
		UserID:       0, // 系统预设
		Name:         req.Name,
		Description:  req.Description,
		Icon:         req.Icon,
		Pattern:      domain_skill.SkillPattern(req.Pattern),
		SystemPrompt: req.SystemPrompt,
		Tools:        req.Tools,
		IsPublic:     req.IsPublic,
	})
	if err != nil {
		h.logger.Error("admin 上传技能失败", zap.Error(err))
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(sk)
}