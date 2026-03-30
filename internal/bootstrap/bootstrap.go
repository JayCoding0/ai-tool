// Package bootstrap 应用启动编排层
// 负责初始化数据库、仓储、服务、中间件、路由注册和服务器启动
package bootstrap

import (
	"context"
	"database/sql"
	"strings"

	"aiProject/internal/application"
	"aiProject/internal/config"
	"aiProject/internal/domain/a2a"
	domain_model "aiProject/internal/domain/model"
	mysql_a2a "aiProject/internal/infrastructure/a2a/mysql"
	"aiProject/internal/infrastructure/database"
	infra_knowledge "aiProject/internal/infrastructure/knowledge"
	mysql_knowledge "aiProject/internal/infrastructure/knowledge/mysql"
	infra_model "aiProject/internal/infrastructure/model"
	mysql_promptvars "aiProject/internal/infrastructure/promptvars/mysql"
	infra_session "aiProject/internal/infrastructure/session"
	mysql_session "aiProject/internal/infrastructure/session/mysql"
	infra_tools "aiProject/internal/infrastructure/tools"
	mysql_user "aiProject/internal/infrastructure/user/mysql"
	http_handler "aiProject/internal/interfaces/http"
	mcp_server "aiProject/internal/interfaces/mcp"
	"aiProject/internal/shared"
	"go.uber.org/zap"
	mcp "trpc.group/trpc-go/trpc-mcp-go"
)

// ─── 模型工厂 ──────────────────────────────────────────────────────────────────

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

// newModelFactory 创建模型工厂函数（支持动态切换模型）
func newModelFactory(cfg *config.Config) func(string) domain_model.Generator {
	return func(modelName string) domain_model.Generator {
		return newModelGeneratorByName(cfg, modelName)
	}
}

// ─── AgentCard 构建 ────────────────────────────────────────────────────────────

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

// ─── 组件初始化 ────────────────────────────────────────────────────────────────

// InitComponents 初始化应用组件，返回 ChatHandler 和 ChatService
func InitComponents(appConfig *config.Config) (*http_handler.ChatHandler, *application.ChatService) {
	modelFactory := newModelFactory(appConfig)

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

	// 初始化 Prompt 模板变量服务（需要数据库已连接）
	initPromptVarsService(frontendChatService, handler)

	// 初始化 RAG 知识库服务（需要数据库已连接）
	initKnowledgeService(appConfig, frontendChatService, handler)

	return handler, frontendChatService
}

// initPromptVarsService 初始化 Prompt 模板变量服务
func initPromptVarsService(chatService *application.ChatService, handler *http_handler.ChatHandler) {
	if database.GetDB() == nil {
		return
	}
	promptVarsRepo := mysql_promptvars.NewPromptVarsRepository()
	promptVarsSvc := application.NewPromptVarsService(promptVarsRepo)
	chatService.SetPromptVarsService(promptVarsSvc)
	handler.SetPromptVarsService(promptVarsSvc)
	shared.GetLogger().Info("Prompt 模板变量服务已启用")
}

// initKnowledgeService 初始化 RAG 知识库服务（如果配置启用）
func initKnowledgeService(appConfig *config.Config, chatService *application.ChatService, handler *http_handler.ChatHandler) {
	if !appConfig.RAG.Enabled {
		return
	}
	embedModel := appConfig.RAG.EmbedModel
	if embedModel == "" {
		embedModel = infra_knowledge.DefaultEmbedModel
	}
	embedder := infra_knowledge.NewOpenAIEmbedder(appConfig.Model.OpenAIBaseURL, appConfig.Model.OpenAIAPIKey, embedModel)
	knowledgeRepo := mysql_knowledge.NewKnowledgeRepository()
	knowledgeSvc := application.NewKnowledgeService(knowledgeRepo, embedder)
	// 注入到前端 ChatService，启用 RAG 能力
	chatService.SetKnowledgeService(knowledgeSvc)
	handler.SetKnowledgeService(knowledgeSvc)
	shared.GetLogger().Info("RAG 知识库服务已启用", zap.String("embed_model", embedModel))
}

// ─── 多 Agent 注册中心 ─────────────────────────────────────────────────────────

// InitAgentRegistry 初始化多 Agent 注册中心
// 创建主 Agent 和各专项子 Agent，并注册 call_agent 工具
func InitAgentRegistry(chatService *application.ChatService, appConfig *config.Config) *application.AgentRegistry {
	registry := application.GetAgentRegistry()
	modelFactory := newModelFactory(appConfig)
	sessionRepo := mysql_session.NewMySQLRepository()

	// 注册所有 Agent
	registerAgents(registry, sessionRepo, appConfig, modelFactory)

	// 向全局工具注册中心注册 call_agent 工具（主 Agent 用来调用子 Agent）
	application.RegisterCallAgentTool(registry.ListSubAgents())

	// 从数据库恢复动态工具配置（覆盖硬编码的 EnabledTools）
	restoreAgentToolsFromDB(registry)

	shared.GetLogger().Info("多 Agent 注册中心初始化完成",
		zap.Int("agent_count", len(registry.ListAll())),
	)
	return registry
}

// registerAgents 注册主 Agent 和各专项子 Agent
func registerAgents(registry *application.AgentRegistry, sessionRepo *mysql_session.MySQLRepository, appConfig *config.Config, modelFactory func(string) domain_model.Generator) {
	newChatSvc := func() *application.ChatService {
		return application.NewChatServiceWithFactory(
			sessionRepo,
			newModelGenerator(appConfig),
			appConfig.Model.Name,
			modelFactory,
		)
	}

	// 主 Agent（总调度，负责理解用户意图并编排子 Agent）
	registry.Register(application.AgentDefinition{
		Name:        "master_agent",
		DisplayName: "主调度 Agent",
		Description: "负责理解用户意图，拆解任务，并调用合适的子 Agent 完成专项工作",
	SystemPrompt: "你是一个智能任务调度助手，请始终使用中文回答。当前时间：{{current_time}}。" +
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
	}, newChatSvc())

	// 天气子 Agent
	registry.Register(application.AgentDefinition{
		Name:        "weather_agent",
		DisplayName: "天气查询 Agent",
		Description: "专门负责天气查询，可以查询任意城市的实时天气和天气预报",
	SystemPrompt: "你是一个专业的天气查询助手，请始终使用中文回答。当前时间：{{current_time}}。" +
			"你只负责天气相关的查询任务，请使用 get_weather 工具获取准确的天气信息，并以友好的方式呈现给用户。",
		EnabledTools: []string{"get_weather", "get_public_ip"},
		IsMaster:     false,
	}, newChatSvc())

	// 搜索子 Agent
	registry.Register(application.AgentDefinition{
		Name:        "search_agent",
		DisplayName: "搜索 Agent",
		Description: "专门负责信息搜索、景点推荐、地点查询、新闻等",
	SystemPrompt: "你是一个专业的信息搜索与推荐助手，请始终使用中文回答。当前时间：{{current_time}}。" +
			"你可以使用 http_request 工具调用外部 API 或搜索引擎获取实时信息。" +
			"你负责根据用户提供的城市、天气等信息，推荐合适的景点、游玩地点、餐厅等，" +
			"并整理成清晰、详细的格式返回给用户。" +
			"如需获取实时信息，请使用 http_request 工具发起 HTTP 请求。",
		EnabledTools: []string{"http_request"},
		IsMaster:     false,
	}, newChatSvc())

	// 代码/工具子 Agent
	registry.Register(application.AgentDefinition{
		Name:        "code_agent",
		DisplayName: "代码工具 Agent",
		Description: "负责代码查询、数学计算、文件写入、执行命令及数据库查询等工具类任务",
	SystemPrompt: "你是一个专业的代码与工具执行助手，请始终使用中文回答。当前时间：{{current_time}}。" +
			"你可以执行数学计算、写入文件、执行 Shell 命令、查询 MySQL 数据库等操作。" +
			"请根据用户需求选择合适的工具，并将结果以清晰易读的格式返回。",
		EnabledTools: []string{"calculate", "write_file", "execute_command", "mysql_query"},
		IsMaster:     false,
	}, newChatSvc())
}

// restoreAgentToolsFromDB 从数据库恢复 Agent 的动态工具配置
func restoreAgentToolsFromDB(registry *application.AgentRegistry) {
	db := database.GetDB()
	if db == nil {
		return
	}
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
	if err != nil {
		return
	}
	for agentName, tools := range toolsMap {
		if err := registry.UpdateTools(agentName, tools); err == nil {
			shared.GetLogger().Info("从数据库恢复 Agent 工具配置",
				zap.String("agent", agentName),
				zap.Strings("tools", tools),
			)
		}
	}
}

// ─── A2A & MCP 初始化 ─────────────────────────────────────────────────────────

// InitA2AService 初始化 A2A 服务
// 如果数据库已连接，则使用 MySQL 持久化存储；否则退回纯内存模式
func InitA2AService(chatService *application.ChatService, appConfig *config.Config) *application.A2AService {
	agentCard := buildAgentCard(appConfig)
	var a2aSvc *application.A2AService

	// 尝试使用 MySQL 持久化
	if database.GetDB() != nil {
		taskRepo := mysql_a2a.NewTaskRepository()
		shared.GetLogger().Info("A2A 任务使用 MySQL 持久化存储")
		a2aSvc = application.NewA2AServiceWithPersist(chatService, agentCard, taskRepo)
	} else {
		shared.GetLogger().Info("A2A 任务使用纯内存存储（数据库未连接）")
		a2aSvc = application.NewA2AService(chatService, agentCard)
	}

	// 复用已在 InitComponents 中初始化的多 Agent 注册中心
	registry := application.GetAgentRegistry()
	if len(registry.ListAll()) > 0 {
		a2aSvc.WithAgentRegistry(registry)
	} else if database.GetDB() != nil {
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
