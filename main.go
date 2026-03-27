package main

import (
	"aiProject/internal/bootstrap"
	"aiProject/internal/config"
	"aiProject/internal/infrastructure/cleaner"
	"aiProject/internal/shared"
)

func main() {
	// 初始化全局日志
	logger, err := shared.InitLogger()
	if err != nil {
		panic("初始化日志失败: " + err.Error())
	}
	defer logger.Sync()

	// 从 trpc_go.yaml 的 custom 块加载应用配置
	appConfig := config.LoadConfig()

	// 初始化应用组件（数据库、仓储、服务、Handler）
	chatHandler, chatService := bootstrap.InitComponents(appConfig)

	// 初始化 A2A 服务
	a2aService := bootstrap.InitA2AService(chatService, appConfig)

	// 注册 HTTP 路由（含 A2A 接口）
	bootstrap.RegisterRoutes(chatHandler, appConfig, a2aService)

	// 初始化 MCP Server
	mcpServer := bootstrap.InitMCPServer(chatService, appConfig)

	// 启动时清理游客会话（user_id=0 的记录）
	cleaner.CleanGuestSessions(logger)

	// 启动 MCP 和 HTTP 服务器
	bootstrap.StartServers(mcpServer, appConfig, logger)
}
