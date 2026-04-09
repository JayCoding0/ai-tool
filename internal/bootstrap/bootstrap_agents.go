// Package bootstrap 应用启动编排层
// bootstrap_agents.go — 多 Agent 注册中心初始化（从 bootstrap.go 拆分）
package bootstrap

import (
	"context"
	"database/sql"

	"aiProject/internal/application"
	"aiProject/internal/config"
	domain_model "aiProject/internal/domain/model"
	mysql_session "aiProject/internal/infrastructure/session/mysql"
	"aiProject/internal/infrastructure/database"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

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
			"- 代码执行/文件操作/数学计算 → 必须调用 code_agent\n" +
			"- 数据库查询/数据查询/查用户信息/查订单/查记录等任何涉及数据查询的请求 → 必须调用 code_agent\n" +
			"【重要】天气查询时，不需要先询问用户位置，weather_agent 会自动通过 IP 获取位置。" +
			"【重要】当用户询问天气+游玩推荐时，先调用 weather_agent 获取天气，再调用 search_agent 推荐景点。" +
			"【重要】无论历史对话中是否有类似回答，都必须重新调用工具获取最新数据，不得使用历史旧数据。" +
			"只能调用 call_agent 并在参数 agent_name 中指定子 Agent 名称，绝对不能直接用子 Agent 名称作为工具名。" +
			"【兜底规则】只有纯粹的问候、闲聊（如：你好、谢谢、你是谁）才可以直接回复。如果用户的请求涉及任何查询、操作、计算、搜索等需要获取外部信息的任务，即使你不确定该调用哪个子 Agent，也必须调用 call_agent（优先选择 code_agent），绝对不能直接回答。",
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
