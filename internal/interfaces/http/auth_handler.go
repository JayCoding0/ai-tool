// Package http 提供 HTTP 接口层实现
package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"aiProject/internal/application"
	"go.uber.org/zap"
)

// ─── 认证上下文传递 ────────────────────────────────────────────────────────────

// ctxKey 认证上下文键类型（避免与其他包的 context key 冲突）
type ctxKey string

const userCtxKey ctxKey = "auth_user"

// authUser 存入 context 的当前用户信息
type authUser struct {
	UserID   int64
	Username string
	Role     string
}

// WithUser 将认证用户信息注入 context（由认证中间件调用）
func WithUser(ctx context.Context, userID int64, username, role string) context.Context {
	return context.WithValue(ctx, userCtxKey, &authUser{
		UserID:   userID,
		Username: username,
		Role:     role,
	})
}

// userFromContext 从 context 中读取认证用户信息
func userFromContext(ctx context.Context) (*authUser, bool) {
	u, ok := ctx.Value(userCtxKey).(*authUser)
	return u, ok
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

// extractToken 从请求中提取token（优先 Authorization Header，其次 Cookie）
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

// getCurrentUser 从请求中获取当前用户信息，返回 userID、username、role
// 优先读取认证中间件注入的 context；缺失时回退到直接校验 token（兼容未经过中间件的内部调用）
func (h *ChatHandler) getCurrentUser(r *http.Request) (int64, string, string) {
	if u, ok := userFromContext(r.Context()); ok {
		return u.UserID, u.Username, u.Role
	}
	if h.authService == nil {
		return 0, "", "guest"
	}
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

// GetAuthService 返回认证服务实例（供 bootstrap 构建认证中间件使用）
func (h *ChatHandler) GetAuthService() *application.AuthService {
	return h.authService
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
