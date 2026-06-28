package bootstrap

import (
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"aiProject/internal/application"
	http_handler "aiProject/internal/interfaces/http"
	"aiProject/internal/shared"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// trustProxyHeaders 是否信任 X-Forwarded-For / X-Real-IP 头。
// 仅当服务部署在可信反向代理之后时才应置为 true，否则客户端可伪造该头绕过限流。
// 由 RegisterRoutes 根据配置设置。
var trustProxyHeaders bool

// recoveryMiddleware panic 恢复中间件，防止单个请求 panic 导致服务崩溃
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				shared.GetLogger().Error("HTTP handler panic",
					zap.Any("panic", rec),
					zap.String("stack", string(debug.Stack())),
				)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// responseWriter 包装 http.ResponseWriter，记录状态码和响应大小
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

// Flush 实现 http.Flusher 接口，透传给底层 ResponseWriter
// 确保 SSE 流式响应（/api/chat/stream）能正常工作
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// loggingMiddleware 请求日志中间件（记录 IP、方法、路径、状态码、耗时、响应大小）
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := newResponseWriter(w)
		next.ServeHTTP(rw, r)
		duration := time.Since(start)
		logger := shared.GetLogger()
		// 静态资源降为 debug 级别，减少日志噪音
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/a2a/") {
			logger.Info("HTTP请求",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.String("ip", realIP(r)),
				zap.Int("status", rw.statusCode),
				zap.Int("bytes", rw.bytesWritten),
				zap.Duration("duration", duration),
			)
		} else {
			logger.Debug("HTTP静态资源",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", rw.statusCode),
				zap.Duration("duration", duration),
			)
		}
	})
}

// corsMiddleware CORS 中间件（从配置读取允许的 Origin，不再硬编码 *）
func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	// 将允许的 Origin 转为集合，方便 O(1) 查找
	originSet := make(map[string]struct{}, len(allowedOrigins))
	allowAll := false
	for _, o := range allowedOrigins {
		if o == "*" {
			allowAll = true
			break
		}
		originSet[o] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				_, allowed := originSet[origin]
				if allowAll || allowed {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
					w.Header().Set("Vary", "Origin")
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ipRateLimiter 基于 IP 的令牌桶限流器
type ipRateLimiter struct {
	limiters sync.Map // map[string]*rateLimiterEntry
	rate     rate.Limit
	burst    int
}

type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// newIPRateLimiter 创建 IP 限流器
func newIPRateLimiter(r rate.Limit, burst int) *ipRateLimiter {
	rl := &ipRateLimiter{rate: r, burst: burst}
	// 定期清理超过 5 分钟未活跃的 IP，防止内存泄漏
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			rl.limiters.Range(func(k, v interface{}) bool {
				entry := v.(*rateLimiterEntry)
				if now.Sub(entry.lastSeen) > 10*time.Minute {
					rl.limiters.Delete(k)
				}
				return true
			})
		}
	}()
	return rl
}

// getLimiter 获取或创建指定 IP 的限流器
func (rl *ipRateLimiter) getLimiter(ip string) *rate.Limiter {
	entry := &rateLimiterEntry{
		limiter:  rate.NewLimiter(rl.rate, rl.burst),
		lastSeen: time.Now(),
	}
	actual, loaded := rl.limiters.LoadOrStore(ip, entry)
	if loaded {
		e := actual.(*rateLimiterEntry)
		e.lastSeen = time.Now()
		return e.limiter
	}
	return entry.limiter
}

// rateLimitMiddleware 基于 IP 的限流中间件
func rateLimitMiddleware(rl *ipRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := realIP(r)
			if !rl.getLimiter(ip).Allow() {
				shared.GetLogger().Warn("请求频率超限",
					zap.String("ip", ip),
					zap.String("path", r.URL.Path),
				)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"请求过于频繁，请稍后重试"}`)) //nolint:errcheck
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// realIP 提取客户端真实 IP。
// 仅在 trustProxyHeaders 为 true（位于可信反代之后）时才信任 X-Forwarded-For / X-Real-IP，
// 否则一律使用 RemoteAddr，防止客户端伪造头绕过限流。
func realIP(r *http.Request) string {
	if trustProxyHeaders {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// X-Forwarded-For 可能包含多个 IP，取第一个（客户端真实 IP）
			parts := strings.SplitN(xff, ",", 2)
			return strings.TrimSpace(parts[0])
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
	}
	// 使用 net.SplitHostPort 正确处理 IPv4/IPv6 地址
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ─── 认证中间件 ────────────────────────────────────────────────────────────────

// extractAuthToken 从请求中提取认证 token（优先 Authorization Bearer，其次 Cookie）
func extractAuthToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if c, err := r.Cookie("auth_token"); err == nil {
		return c.Value
	}
	return ""
}

// authMiddleware 可选认证中间件：
//   - 携带有效 token 时，解析用户信息并注入 request context（供下游识别登录用户）
//   - 未携带 token 或 token 无效时，作为游客（userID=0）继续，不拒绝请求
//
// 访问控制策略：公共功能（聊天、模型/工具列表等）允许游客使用；
// 私有资源（知识库、记忆、工作流等）由各 handler 通过 requireLogin 强制登录；
// 跨用户越权由 service 层的资源归属校验兜底。
func authMiddleware(authService *application.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token := extractAuthToken(r); token != "" {
				if userID, username, role, err := authService.ValidateToken(token); err == nil && userID > 0 {
					r = r.WithContext(http_handler.WithUser(r.Context(), userID, username, role))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// chain 将多个中间件链式组合（从左到右依次包裹）
func chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
