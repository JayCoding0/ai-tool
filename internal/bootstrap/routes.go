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
	// 记忆管理接口（跨会话向量记忆）
	mux.HandleFunc("/api/memory", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			chatHandler.HandleListMemories(w, r)
		case http.MethodPost:
			chatHandler.HandleCreateMemory(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/memory/update", chatHandler.HandleUpdateMemory)
	mux.HandleFunc("/api/memory/delete", chatHandler.HandleDeleteMemory)
	mux.HandleFunc("/api/memory/search", chatHandler.HandleSearchMemories)

	// Agent 评估体系接口
	mux.HandleFunc("/api/eval/datasets", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			chatHandler.HandleListDatasets(w, r)
		case http.MethodPost:
			chatHandler.HandleCreateDataset(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/eval/datasets/delete", chatHandler.HandleDeleteDataset)
	mux.HandleFunc("/api/eval/cases", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			chatHandler.HandleListCases(w, r)
		case http.MethodPost:
			chatHandler.HandleAddCase(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/eval/cases/delete", chatHandler.HandleDeleteCase)
	// 运行列表/启动 与 运行详情（/api/eval/runs/{id}）通过路径区分
	mux.HandleFunc("/api/eval/runs", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			chatHandler.HandleListRuns(w, r)
		case http.MethodPost:
			chatHandler.HandleRunEval(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	// 运行对比（精确路径，优先于 /api/eval/runs/ 前缀匹配）
	mux.HandleFunc("/api/eval/runs/compare", chatHandler.HandleCompareRuns)
	mux.HandleFunc("/api/eval/runs/", chatHandler.HandleGetRun)

	// Workflow 工作流接口
	if chatHandler.GetWorkflowHandler() != nil {
		wfHandler := chatHandler.GetWorkflowHandler()
		// 工作流 CRUD
		mux.HandleFunc("/api/workflows", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				wfHandler.HandleListWorkflows(w, r)
			case http.MethodPost:
				wfHandler.HandleCreateWorkflow(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})
		// 工作流导入接口（Phase 3）
		mux.HandleFunc("/api/workflows/import", wfHandler.HandleImportWorkflow)
		// 工作流执行记录查询
		mux.HandleFunc("/api/workflow-runs/", wfHandler.HandleGetWorkflowRun)
		// 工作流详情/更新/删除/发布/执行/导出/执行记录（通过路径后缀区分）
		mux.HandleFunc("/api/workflows/", func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			switch {
			case strings.HasSuffix(path, "/publish"):
				wfHandler.HandlePublishWorkflow(w, r)
			case strings.HasSuffix(path, "/execute"):
				wfHandler.HandleExecuteWorkflow(w, r)
			case strings.HasSuffix(path, "/runs"):
				wfHandler.HandleGetWorkflowRuns(w, r)
			case strings.HasSuffix(path, "/export"):
				wfHandler.HandleExportWorkflow(w, r)
			default:
				switch r.Method {
				case http.MethodGet:
					wfHandler.HandleGetWorkflow(w, r)
				case http.MethodPut:
					wfHandler.HandleUpdateWorkflow(w, r)
				case http.MethodDelete:
					wfHandler.HandleDeleteWorkflow(w, r)
				default:
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			}
		})
	}

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

	// 是否信任反向代理头（X-Forwarded-For 等），影响限流取 IP 的方式
	if appConfig != nil {
		trustProxyHeaders = appConfig.Security.TrustProxyHeaders
	}

	// 构建中间件链：recover → logging → rate_limit → cors → auth
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

	// 认证中间件：强制受保护接口校验登录态（authService 不可用时跳过，仅内存降级模式）
	if authService := chatHandler.GetAuthService(); authService != nil {
		middlewares = append(middlewares, authMiddleware(authService))
		shared.GetLogger().Info("认证中间件已启用")
	} else {
		shared.GetLogger().Warn("认证服务不可用（数据库未连接），认证中间件未启用，接口处于无鉴权降级状态")
	}

	http.Handle("/", chain(mux, middlewares...))
}
