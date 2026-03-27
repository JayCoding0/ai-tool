package bootstrap

import (
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"aiProject/internal/application"
	"aiProject/internal/config"
	domain_model "aiProject/internal/domain/model"
	"aiProject/internal/infrastructure/database"
	infra_model "aiProject/internal/infrastructure/model"
	infra_session "aiProject/internal/infrastructure/session"
	mysql_session "aiProject/internal/infrastructure/session/mysql"
	mysql_skill "aiProject/internal/infrastructure/skill/mysql"
	infra_tools "aiProject/internal/infrastructure/tools"
	mysql_user "aiProject/internal/infrastructure/user/mysql"
	http_handler "aiProject/internal/interfaces/http"
	mcp_server "aiProject/internal/interfaces/mcp"
	"aiProject/internal/shared"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
	mcp "trpc.group/trpc-go/trpc-mcp-go"
)

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

// loggingMiddleware 请求日志中间件
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		shared.GetLogger().Info("HTTP请求",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Duration("duration", time.Since(start)),
		)
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
	limiters sync.Map          // map[string]*rateLimiterEntry
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

// realIP 提取客户端真实 IP，支持 X-Forwarded-For 和 X-Real-IP 头
func realIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For 可能包含多个 IP，取第一个（客户端真实 IP）
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// 去掉端口号
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

// chain 将多个中间件链式组合（从左到右依次包裹）
func chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}



// newModelGenerator 根据配置创建对应的模型生成器
func newModelGenerator(cfg *config.Config) domain_model.Generator {
	return newModelGeneratorByName(cfg, cfg.Model.Name)
}

// newModelGeneratorByName 根据模型名称创建对应的模型生成器
// 判断规则：
//   - cfg.Model.Type == "openai" → 全部走 OpenAI 兼容接口（阿里云 DashScope 等）
//   - cfg.Model.Type == "local" → 全部走 Ollama
//   - 模型名包含 ":" (如 qwen2.5:14b、llama3.2:3b) → Ollama 本地模型
//   - 其余 → OpenAI 兼容接口
func newModelGeneratorByName(cfg *config.Config, modelName string) domain_model.Generator {
	isLocal := cfg.Model.Type == "local" || (cfg.Model.Type != "openai" && strings.Contains(modelName, ":"))
	if isLocal {
		shared.GetLogger().Info("使用Ollama本地模型",
			zap.String("model", modelName),
			zap.String("ollama_url", cfg.Model.OllamaURL),
		)
		return infra_model.NewOllamaGenerator(modelName, cfg.Model.OllamaURL)
	}
	shared.GetLogger().Info("使用OpenAI兼容接口模型",
		zap.String("model", modelName),
		zap.String("base_url", cfg.Model.OpenAIBaseURL),
	)
	return infra_model.NewOpenAIGenerator(cfg.Model.OpenAIBaseURL, cfg.Model.OpenAIAPIKey, modelName)
}

// InitComponents 初始化应用组件，返回 ChatHandler 和 ChatService
func InitComponents(appConfig *config.Config) (*http_handler.ChatHandler, *application.ChatService) {
	// 构建模型工厂函数（支持动态切换模型）
	modelFactory := func(modelName string) domain_model.Generator {
		return newModelGeneratorByName(appConfig, modelName)
	}

	// 初始化MySQL数据库
	_, err := database.InitMySQL(&appConfig.Database.MySQL)
	if err != nil {
		shared.GetLogger().Error("数据库初始化失败", zap.Error(err))
		// 如果数据库连接失败，回退到内存存储
		shared.GetLogger().Info("使用内存存储作为回退方案")
		sessionRepo := infra_session.NewMemoryRepository()
		modelGen := newModelGenerator(appConfig)
		chatService := application.NewChatServiceWithFactory(sessionRepo, modelGen, appConfig.Model.Name, modelFactory)
		// 内存模式下无法使用用户功能，authService传nil
		return http_handler.NewChatHandler(chatService, nil, appConfig), chatService
	}
	// 注意：不要在这里关闭数据库连接，连接需要在应用运行期间保持打开

	// 使用MySQL存储
	sessionRepo := mysql_session.NewMySQLRepository()
	userRepo := mysql_user.NewUserRepository()
	skillRepo := mysql_skill.NewSkillRepository()
	modelGen := newModelGenerator(appConfig)
	chatService := application.NewChatServiceWithFactory(sessionRepo, modelGen, appConfig.Model.Name, modelFactory)
	authService := application.NewAuthService(userRepo)
	skillService := application.NewSkillService(skillRepo)
	skillService.SetModelFactory(modelFactory, appConfig.Model.Name)
	// 从 skills/*/scripts/ 目录加载并注册工具
	infra_tools.LoadToolsFromSkillsDir("skills")
	handler := http_handler.NewChatHandler(chatService, authService, appConfig)
	handler.SetSkillService(skillService)
	return handler, chatService
}

// InitMCPServer 初始化 MCP Server
func InitMCPServer(chatService *application.ChatService, appConfig *config.Config) *mcp.Server {
	mcpPort := appConfig.Server.MCPPort
	if mcpPort == "" {
		mcpPort = "8001"
	}
	addr := "0.0.0.0:" + mcpPort
	return mcp_server.NewMCPServer(chatService, addr, "/mcp")
}

// RegisterRoutes 注册所有 HTTP 路由（带中间件链）
func RegisterRoutes(chatHandler *http_handler.ChatHandler, appConfig *config.Config) {
	mux := http.NewServeMux()

	// 会话 & 聊天接口
	mux.HandleFunc("/api/chat/stream", chatHandler.HandleChatStream)
	mux.HandleFunc("/api/history", chatHandler.HandleGetHistory)
	mux.HandleFunc("/api/sessions", chatHandler.HandleListSessions)
	mux.HandleFunc("/api/sessions/delete", chatHandler.HandleDeleteSession)
	mux.HandleFunc("/api/sessions/rename", chatHandler.HandleRenameSession)
	// System Prompt 接口
	mux.HandleFunc("/api/sessions/system-prompt", chatHandler.HandleUpdateSystemPrompt)
	mux.HandleFunc("/api/sessions/system-prompt/get", chatHandler.HandleGetSystemPrompt)
	// Skills 技能接口
	mux.HandleFunc("/api/skills", chatHandler.HandleListSkills)
	mux.HandleFunc("/api/skills/create", chatHandler.HandleCreateSkill)
	mux.HandleFunc("/api/skills/update", chatHandler.HandleUpdateSkill)
	mux.HandleFunc("/api/skills/delete", chatHandler.HandleDeleteSkill)
	mux.HandleFunc("/api/skills/apply", chatHandler.HandleApplySkill)
	// Admin 专用技能接口（需要 admin 角色）
	mux.HandleFunc("/api/admin/skills/download", chatHandler.HandleAdminDownloadSkill)
	mux.HandleFunc("/api/admin/skills/upload", chatHandler.HandleAdminUploadSkill)
	// 工具接口
	mux.HandleFunc("/api/tools", chatHandler.HandleListTools)
	// 模型列表接口
	mux.HandleFunc("/api/models", chatHandler.HandleListModels)
	// 认证相关接口
	mux.HandleFunc("/api/auth/register", chatHandler.HandleRegister)
	mux.HandleFunc("/api/auth/login", chatHandler.HandleLogin)
	mux.HandleFunc("/api/auth/logout", chatHandler.HandleLogout)
	mux.HandleFunc("/api/auth/me", chatHandler.HandleMe)
	// 前端静态文件
	mux.Handle("/", http.FileServer(http.Dir("./frontend")))

	// 构建中间件链：recover → logging → rate_limit → cors
	middlewares := []func(http.Handler) http.Handler{
		recoveryMiddleware,
		loggingMiddleware,
	}

	// 限流中间件（可配置开关）
	if appConfig != nil && appConfig.Security.RateLimit.Enabled {
		rl := newIPRateLimiter(
			rate.Limit(appConfig.Security.RateLimit.RequestsPerSecond),
			appConfig.Security.RateLimit.Burst,
		)
		middlewares = append(middlewares, rateLimitMiddleware(rl))
		shared.GetLogger().Info("限流中间件已启用",
			zap.Float64("rps", appConfig.Security.RateLimit.RequestsPerSecond),
			zap.Int("burst", appConfig.Security.RateLimit.Burst),
		)
	}

	// CORS 中间件（从配置读取允许的 Origin）
	allowedOrigins := []string{"http://localhost:8081", "http://127.0.0.1:8081"}
	if appConfig != nil && len(appConfig.Security.AllowedOrigins) > 0 {
		allowedOrigins = appConfig.Security.AllowedOrigins
	}
	middlewares = append(middlewares, corsMiddleware(allowedOrigins))

	http.Handle("/", chain(mux, middlewares...))
}
