// Package bootstrap 应用启动编排层
// 负责初始化数据库、仓储、服务、中间件、路由注册和服务器启动
// Agent 注册中心相关代码见 bootstrap_agents.go
package bootstrap

import (
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
	mysql_workflow "aiProject/internal/infrastructure/workflow/mysql"
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

	// 初始化会话摘要服务（需要数据库已连接）
	initSummaryService(appConfig, frontendChatService)

	// 初始化 RAG 知识库服务（需要数据库已连接）
	initKnowledgeService(appConfig, frontendChatService, handler)

	// 初始化 Workflow 工作流服务（需要数据库已连接）
	initWorkflowService(appConfig, handler, registry)

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

// initSummaryService 初始化会话摘要服务
func initSummaryService(appConfig *config.Config, chatService *application.ChatService) {
	if database.GetDB() == nil {
		return
	}
	modelGen := newModelGenerator(appConfig)
	summarySvc := application.NewSummaryService(
		mysql_session.NewMySQLRepository(),
		modelGen,
	)
	summarySvc.SetModelFactory(newModelFactory(appConfig), appConfig.Model.Name)
	chatService.SetSummaryService(summarySvc)
	// 设置模型最大上下文 token 数
	if appConfig.Model.MaxContextLength > 0 {
		chatService.SetMaxContextTokens(appConfig.Model.MaxContextLength)
	}
	shared.GetLogger().Info("会话摘要服务已启用",
		zap.Int("max_context_tokens", appConfig.Model.MaxContextLength),
	)
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

// ─── Workflow 工作流初始化 ──────────────────────────────────────────────────────

// initWorkflowService 初始化 Workflow 工作流服务
func initWorkflowService(appConfig *config.Config, handler *http_handler.ChatHandler, registry *application.AgentRegistry) {
	if database.GetDB() == nil {
		shared.GetLogger().Info("Workflow 服务未启用（数据库未连接）")
		return
	}

	workflowRepo := mysql_workflow.NewWorkflowRepository()
	runRepo := mysql_workflow.NewWorkflowRunRepository()
	modelFactory := newModelFactory(appConfig)

	workflowSvc := application.NewWorkflowService(workflowRepo, runRepo)
	workflowEngine := application.NewWorkflowEngine(workflowRepo, runRepo, modelFactory, appConfig.Model.Name, registry)

	handler.SetWorkflowService(workflowSvc, workflowEngine)

	shared.GetLogger().Info("Workflow 工作流服务已启用")
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