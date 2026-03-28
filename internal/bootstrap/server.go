// Package bootstrap 应用启动编排层
package bootstrap

import (
	"net/http"
	"os"

	"aiProject/internal/config"
	"go.uber.org/zap"
	mcp "trpc.group/trpc-go/trpc-mcp-go"
)

// StartServers 启动 MCP 服务器和 HTTP 服务器
func StartServers(mcpServer *mcp.Server, appConfig *config.Config, logger *zap.Logger) {
	port := appConfig.Server.HTTPPort
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}

	mcpPort := appConfig.Server.MCPPort
	if mcpPort == "" {
		mcpPort = "8001"
	}

	logger.Info("🚀 启动智能小助手",
		zap.String("model", appConfig.Model.Name),
		zap.String("frontend", "http://localhost:"+port),
		zap.String("api", "http://localhost:"+port+"/api/chat/stream"),
		zap.String("mcp_server", "http://localhost:"+mcpPort+"/mcp"),
	)

	// 在后台启动 MCP 服务器
	go func() {
		logger.Info("启动MCP服务器", zap.String("port", mcpPort))
		if err := mcpServer.Start(); err != nil {
			logger.Error("MCP服务器已停止", zap.Error(err))
		}
	}()

	// 启动 HTTP 服务器（阻塞）
	logger.Info("启动HTTP服务器", zap.String("port", port))
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		logger.Fatal("HTTP服务器启动失败", zap.Error(err))
	}
}