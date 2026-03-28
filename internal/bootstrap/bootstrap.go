// Package bootstrap 应用启动编排层
// 负责初始化数据库、仓储、服务、中间件、路由注册和服务器启动
// Package bootstrap 应用启动编排层
// 负责初始化数据库、仓储、服务、中间件、路由注册和服务器启动
package bootstrap

import (
	"context"
	"database/sql"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"aiProject/internal/application"
	"aiProject/internal/config"
	"aiProject/internal/domain/a2a"
	domain_model "aiProject/internal/domain/model"
	mysql_a2a "aiProject/internal/infrastructure/a2a/mysql"
	"aiProject/internal/infrastructure/database"
	infra_knowledge "aiProject/internal/infrastructure/knowledge"
	mysql_knowledge "aiProject/internal/infrastructure/knowledge/mysql"
	infra_model "aiProject/internal/infrastructure/model"
	infra_session "aiProject/internal/infrastructure/session"
	mysql_session "aiProject/internal/infrastructure/session/mysql"
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

// buildAgentCard 根据配置构建 AgentCard
func buildAgentCard(appConfig *config.Config) *a2a.AgentCard {
	serverPort := "8080"
	if appConfig != nil && appConfig.Server.HTTPPort != "" {
		serverPort = appConfig.Server.HTTPPort
	}
	return &a2a.AgentCard{
		ProtocolVersion: "0.1",
		Name:            "AI Agent",
		Description:     "一个支持多工具调用的智能对话 Agent，可以帮助你查询天气、搜索信息、执行代码等任务。",
		Version:         "1.0.0",
		URL:             "http://localhost:" + serverPort + "/a2a/tasks/send",
		Capabilities: a2a.AgentCapabilities{
			Streaming:   true,
			MultiTurn:   true,
			ToolCalling: true,
		},
		Skills: []a2a.AgentSkill{
			{
				ID:          "general_chat",
				Name:        "通用对话",
				Description: "支持多轮对话，可以回答各类问题",
				Tags:        []string{"chat", "general"},
				Examples:    []string{"你好", "帮我写一段代码", "解释一下量子计算"},
			},
			{
				ID:          "tool_calling",
				Name:        "工具调用",
				Description: "可以调用天气查询、地图搜索、代码执行等工具",
				Tags:        []string{"tools", "weather", "search", "code"},
				Examples:    []string{"今天北京天气怎么样", "帮我搜索最近的咖啡店"},
			},
		},
		Provider: &a2a.AgentProvider{
			Organization: "aiProject",
		},
	}
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
	modelGen := newModelGenerator(appConfig)
	chatService := application.NewChatServiceWithFactory(sessionRepo, modelGen, appConfig.Model.Name, modelFactory)
	authService := application.NewAuthService(userRepo)
	// 从 skills/*/scripts/ 目录加载并注册工具（传入百度 AK，避免硬编码）
	infra_tools.LoadToolsFromSkillsDir("skills", appConfig.Tools.BaiduAK)

	// 初始化多 Agent 注册中心，让前端聊天也走主 Agent（call_agent 编排模式）
	registry := InitAgentRegistry(chatService, appConfig)
	// 前端聊天使用主 Agent 的 ChatService
	frontendChatService := chatService
	if master, ok := registry.GetMaster(); ok {
		frontendChatService = master.ChatService
	}

	handler := http_handler.NewChatHandler(frontendChatService, authService, appConfig)
	handler.SetAgentRegistry(registry)

	// 初始化 RAG 知识库服务（需要数据库已连接）
	if appConfig.RAG.Enabled {
		embedModel := appConfig.RAG.EmbedModel
		if embedModel == "" {
			embedModel = infra_knowledge.DefaultEmbedModel
		}
		embedder := infra_knowledge.NewOpenAIEmbedder(appConfig.Model.OpenAIBaseURL, appConfig.Model.OpenAIAPIKey, embedModel)
		knowledgeRepo := mysql_knowledge.NewKnowledgeRepository()
		knowledgeSvc := application.NewKnowledgeService(knowledgeRepo, embedder)
		// 注入到前端 ChatService，启用 RAG 能力
		frontendChatService.SetKnowledgeService(knowledgeSvc)
		handler.SetKnowledgeService(knowledgeSvc)
		shared.GetLogger().Info("RAG 知识库服务已启用", zap.String("embed_model", embedModel))
	}

	return handler, frontendChatService
}

// InitAgentRegistry 初始化多 Agent 注册中心
// 创建主 Agent 和各专项子 Agent，并注册 call_agent 工具
func InitAgentRegistry(chatService *application.ChatService, appConfig *config.Config) *application.AgentRegistry {
	registry := application.GetAgentRegistry()

	// 构建模型工厂（子 Agent 可以使用不同模型）
	modelFactory := func(modelName string) domain_model.Generator {
		return newModelGeneratorByName(appConfig, modelName)
	}
	sessionRepo := mysql_session.NewMySQLRepository()

	// ── 注册主 Agent（总调度，负责理解用户意图并编排子 Agent）──
	masterChatService := application.NewChatServiceWithFactory(
		sessionRepo,
		newModelGenerator(appConfig),
		appConfig.Model.Name,
		modelFactory,
	)
	registry.Register(application.AgentDefinition{
		Name:        "master_agent",
		DisplayName: "主调度 Agent",
		Description: "负责理解用户意图，拆解任务，并调用合适的子 Agent 完成专项工作",
		SystemPrompt: "你是一个智能任务调度助手，请始终使用中文回答。" +
			"你只有一个工具可以使用：call_agent。" +
			"【强制规则】以下类型的问题，无论任何情况，必须立即调用 call_agent 工具，绝对不能直接回答或询问用户：\n" +
			"- 天气相关（如：天气怎么样、今天热不热、要不要带伞）→ 必须调用 weather_agent\n" +
			"- 地点/景点/游玩推荐 → 必须调用 search_agent\n" +
			"- 搜索/查询实时信息 → 必须调用 search_agent\n" +
			"- 代码执行/文件操作/数学计算/数据库查询 → 必须调用 code_agent\n" +
			"【重要】天气查询时，不需要先询问用户位置，weather_agent 会自动通过 IP 获取位置。" +
			"【重要】当用户询问天气+游玩推荐时，先调用 weather_agent 获取天气，再调用 search_agent 推荐景点。" +
			"【重要】无论历史对话中是否有类似回答，都必须重新调用工具获取最新数据，不得使用历史旧数据。" +
			"只能调用 call_agent 并在参数 agent_name 中指定子 Agent 名称，绝对不能直接用子 Agent 名称作为工具名。" +
			"只有纯粹的问候、闲聊（如：你好、谢谢）才可以直接回复，其他一律调用工具。",
		EnabledTools: []string{"call_agent"},
		IsMaster:     true,
	}, masterChatService)

	// ── 注册天气子 Agent ──
	weatherChatService := application.NewChatServiceWithFactory(
		sessionRepo,
		newModelGenerator(appConfig),
		appConfig.Model.Name,
		modelFactory,
	)
	registry.Register(application.AgentDefinition{
		Name:        "weather_agent",
		DisplayName: "天气查询 Agent",
		Description: "专门负责天气查询，可以查询任意城市的实时天气和天气预报",
		SystemPrompt: "你是一个专业的天气查询助手，请始终使用中文回答。" +
			"你只负责天气相关的查询任务，请使用 get_weather 工具获取准确的天气信息，并以友好的方式呈现给用户。",
		EnabledTools: []string{"get_weather", "get_public_ip"},
		IsMaster:     false,
	}, weatherChatService)

	// ── 注册搜索子 Agent ──
	searchChatService := application.NewChatServiceWithFactory(
		sessionRepo,
		newModelGenerator(appConfig),
		appConfig.Model.Name,
		modelFactory,
	)
	registry.Register(application.AgentDefinition{
		Name:        "search_agent",
		DisplayName: "搜索 Agent",
		Description: "专门负责信息搜索、景点推荐、地点查询、新闻等",
		SystemPrompt: "你是一个专业的信息搜索与推荐助手，请始终使用中文回答。" +
			"你可以使用 http_request 工具调用外部 API 或搜索引擎获取实时信息。" +
			"你负责根据用户提供的城市、天气等信息，推荐合适的景点、游玩地点、餐厅等，" +
			"并整理成清晰、详细的格式返回给用户。" +
			"如需获取实时信息，请使用 http_request 工具发起 HTTP 请求。",
		EnabledTools: []string{"http_request"},
		IsMaster:     false,
	}, searchChatService)

	// ── 注册代码/工具子 Agent ──
	codeChatService := application.NewChatServiceWithFactory(
		sessionRepo,
		newModelGenerator(appConfig),
		appConfig.Model.Name,
		modelFactory,
	)
	registry.Register(application.AgentDefinition{
		Name:        "code_agent",
		DisplayName: "代码工具 Agent",
		Description: "负责代码查询、数学计算、文件写入、执行命令及数据库查询等工具类任务",
		SystemPrompt: "你是一个专业的代码与工具执行助手，请始终使用中文回答。" +
			"你可以执行数学计算、写入文件、执行 Shell 命令、查询 MySQL 数据库等操作。" +
			"请根据用户需求选择合适的工具，并将结果以清晰易读的格式返回。",
		EnabledTools: []string{"calculate", "write_file", "execute_command", "mysql_query"},
		IsMaster:     false,
	}, codeChatService)

	// 向全局工具注册中心注册 call_agent 工具（主 Agent 用来调用子 Agent）
	application.RegisterCallAgentTool(registry.ListSubAgents())

	// 从数据库恢复动态工具配置（覆盖硬编码的 EnabledTools）
	if db := database.GetDB(); db != nil {
		ctx := context.Background()
		toolsMap := make(map[string][]string)
		err := db.Query(ctx, func(rows *sql.Rows) error {
			var agentName, toolName string
			if err := rows.Scan(&agentName, &toolName); err != nil {
				return err
			}
			toolsMap[agentName] = append(toolsMap[agentName], toolName)
			return nil
		}, "SELECT agent_name, tool_name FROM agent_tools ORDER BY agent_name, id")
		if err == nil {
			for agentName, tools := range toolsMap {
				if err := registry.UpdateTools(agentName, tools); err == nil {
					shared.GetLogger().Info("从数据库恢复 Agent 工具配置",
						zap.String("agent", agentName),
						zap.Strings("tools", tools),
					)
				}
			}
		}
	}

	shared.GetLogger().Info("多 Agent 注册中心初始化完成",
		zap.Int("agent_count", len(registry.ListAll())),
	)
	return registry
}

// InitA2AService 初始化 A2A 服务
// 如果数据库已连接，则使用 MySQL 持久化存储；否则退回纯内存模式
func InitA2AService(chatService *application.ChatService, appConfig *config.Config) *application.A2AService {
	agentCard := buildAgentCard(appConfig)
	var a2aSvc *application.A2AService

	// 尝试使用 MySQL 持久化（database.GetDB() 不为 nil 说明数据库已初始化）
	if database.GetDB() != nil {
		taskRepo := mysql_a2a.NewTaskRepository()
		shared.GetLogger().Info("A2A 任务使用 MySQL 持久化存储")
		a2aSvc = application.NewA2AServiceWithPersist(chatService, agentCard, taskRepo)
	} else {
		shared.GetLogger().Info("A2A 任务使用纯内存存储（数据库未连接）")
		a2aSvc = application.NewA2AService(chatService, agentCard)
	}

	// 复用已在 InitComponents 中初始化的多 Agent 注册中心（避免重复注册）
	registry := application.GetAgentRegistry()
	if len(registry.ListAll()) > 0 {
		a2aSvc.WithAgentRegistry(registry)
	} else if database.GetDB() != nil {
		// 数据库已连接但注册中心为空（单独调用 InitA2AService 的场景），则初始化
		reg := InitAgentRegistry(chatService, appConfig)
		a2aSvc.WithAgentRegistry(reg)
	}

	return a2aSvc
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
