// Package bootstrap 应用启动编排层
package bootstrap

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aiProject/internal/config"
	"go.uber.org/zap"
	mcp "trpc.group/trpc-go/trpc-mcp-go"
)

// StartServers 启动 MCP 服务器和 HTTP 服务器，支持优雅退出（SIGINT/SIGTERM）
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
		zap.String("health", "http://localhost:"+port+"/healthz"),
	)

	// 在后台启动 MCP 服务器
	go func() {
		logger.Info("启动MCP服务器", zap.String("port", mcpPort))
		if err := mcpServer.Start(); err != nil {
			logger.Error("MCP服务器已停止", zap.Error(err))
		}
	}()

	// 使用 http.Server 以支持优雅退出
	srv := &http.Server{Addr: ":" + port}

	// 在后台启动 HTTP 服务器
	go func() {
		logger.Info("启动HTTP服务器", zap.String("port", port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTP服务器启动失败", zap.Error(err))
		}
	}()

	// 监听退出信号，收到后优雅关闭（等待在途请求完成，最多 15 秒）
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("收到退出信号，开始优雅关闭…")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("HTTP服务器优雅关闭失败", zap.Error(err))
	} else {
		logger.Info("HTTP服务器已优雅关闭")
	}
}
