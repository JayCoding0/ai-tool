// Package http 提供 HTTP 接口层实现
package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"aiProject/internal/application"
	"go.uber.org/zap"
)

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
