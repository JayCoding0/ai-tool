package bootstrap

import (
	"net/http"
	"strings"

	"aiProject/internal/application"
	"aiProject/internal/config"
	http_handler "aiProject/internal/interfaces/http"
	"aiProject/internal/shared"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// RegisterRoutes 注册所有 HTTP 路由（带中间件链）
func RegisterRoutes(chatHandler *http_handler.ChatHandler, appConfig *config.Config, a2aService *application.A2AService) {
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
	// 工具接口
	mux.HandleFunc("/api/tools", chatHandler.HandleListTools)
	// Agent 列表接口
	mux.HandleFunc("/api/agents", chatHandler.HandleListAgents)
	// Agent 工具动态配置接口（PUT /api/agents/{name}/tools）
	mux.HandleFunc("/api/agents/", chatHandler.HandleUpdateAgentTools)
	// 模型列表接口
	mux.HandleFunc("/api/models", chatHandler.HandleListModels)
	// Prompt 模板变量接口
	mux.HandleFunc("/api/prompt-vars", chatHandler.HandleListPromptVariables)
	mux.HandleFunc("/api/prompt-vars/user/set", chatHandler.HandleSetUserVar)
	mux.HandleFunc("/api/prompt-vars/user/delete", chatHandler.HandleDeleteUserVar)
	mux.HandleFunc("/api/prompt-vars/session/set", chatHandler.HandleSetSessionVar)
	mux.HandleFunc("/api/prompt-vars/session/delete", chatHandler.HandleDeleteSessionVar)
	// 认证相关接口
	mux.HandleFunc("/api/auth/register", chatHandler.HandleRegister)
	mux.HandleFunc("/api/auth/login", chatHandler.HandleLogin)
	mux.HandleFunc("/api/auth/logout", chatHandler.HandleLogout)
	mux.HandleFunc("/api/auth/me", chatHandler.HandleMe)
	// 知识库接口
	mux.HandleFunc("/api/knowledge/bases", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			chatHandler.HandleListKnowledgeBases(w, r)
		case http.MethodPost:
			chatHandler.HandleCreateKnowledgeBase(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/knowledge/bases/delete", chatHandler.HandleDeleteKnowledgeBase)
	mux.HandleFunc("/api/knowledge/documents", chatHandler.HandleListDocuments)
	mux.HandleFunc("/api/knowledge/documents/upload", chatHandler.HandleUploadDocument)
	mux.HandleFunc("/api/knowledge/documents/upload-directory", chatHandler.HandleUploadDirectory)
	mux.HandleFunc("/api/knowledge/documents/delete", chatHandler.HandleDeleteDocument)
	mux.HandleFunc("/api/knowledge/search", chatHandler.HandleKnowledgeSearch)

	// A2A 协议接口
	if a2aService != nil {
		a2aHandler := http_handler.NewA2AHandler(a2aService)
		// AgentCard 发现接口
		mux.HandleFunc("/.well-known/agent.json", a2aHandler.HandleAgentCard)
		// 任务提交接口
		mux.HandleFunc("/a2a/tasks/send", a2aHandler.HandleTaskSend)
		// 任务状态查询 & SSE 流式订阅（通过路径后缀区分）
		mux.HandleFunc("/a2a/tasks/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/stream") {
				a2aHandler.HandleTaskStream(w, r)
			} else {
				a2aHandler.HandleTaskGet(w, r)
			}
		})
	}

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
