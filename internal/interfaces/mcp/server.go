package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"aiProject/internal/application"
	"aiProject/internal/domain/session"
	mcp "trpc.group/trpc-go/trpc-mcp-go"
)

// NewMCPServer 创建并配置 MCP Server，注册所有工具
// addr 示例: "localhost:8001"，path 示例: "/mcp"
func NewMCPServer(chatService *application.ChatService, addr, path string) *mcp.Server {
	server := mcp.NewServer(
		"ai-chat-mcp-server",
		"1.0.0",
		mcp.WithServerAddress(addr),
		mcp.WithServerPath(path),
		mcp.WithStatelessMode(true), // 无状态模式，每次请求独立
	)

	registerChatTool(server, chatService)
	registerGetHistoryTool(server, chatService)
	registerListSessionsTool(server, chatService)
	registerDeleteSessionTool(server, chatService)

	return server
}

// registerChatTool 注册聊天工具
func registerChatTool(server *mcp.Server, chatService *application.ChatService) {
	tool := mcp.NewTool(
		"chat",
		mcp.WithDescription("向 AI 助手发送消息并获取回复。支持多轮对话，通过 session_id 维持上下文。"),
		mcp.WithString("message",
			mcp.Description("要发送给 AI 的消息内容"),
			mcp.Required(),
		),
		mcp.WithString("session_id",
			mcp.Description("会话 ID，用于维持多轮对话上下文。留空则创建新会话。"),
		),
	)

	server.RegisterTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		message, _ := req.Params.Arguments["message"].(string)
		if message == "" {
			return mcp.NewTextResult("错误：message 参数不能为空"), nil
		}
		sessionID, _ := req.Params.Arguments["session_id"].(string)

		// 消费 channel，收集最终回复
		streamCh, err := chatService.ProcessMessageWithTools(ctx, application.ChatRequest{
			Message:   message,
			SessionID: session.SessionID(sessionID),
			UserID:    0, // MCP 调用默认为游客
		}, nil)
		if err != nil {
			return nil, fmt.Errorf("处理消息失败: %w", err)
		}

		var finalContent string
		var finalSessionID string
		for event := range streamCh {
			switch event.Type {
			case "chunk":
				finalContent += event.Content
			case "done":
				finalSessionID = string(event.SessionID)
			case "error":
				return nil, fmt.Errorf("处理消息失败: %s", event.Error)
			}
		}

		result := map[string]interface{}{
			"response":   finalContent,
			"session_id": finalSessionID,
		}

		resultJSON, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("序列化响应失败: %w", err)
		}
		return mcp.NewTextResult(string(resultJSON)), nil
	})
}

// registerGetHistoryTool 注册获取会话历史工具
func registerGetHistoryTool(server *mcp.Server, chatService *application.ChatService) {
	tool := mcp.NewTool(
		"get_history",
		mcp.WithDescription("获取指定会话的历史消息记录。"),
		mcp.WithString("session_id",
			mcp.Description("要查询历史记录的会话 ID"),
			mcp.Required(),
		),
	)

	server.RegisterTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sessionID, _ := req.Params.Arguments["session_id"].(string)
		if sessionID == "" {
			return mcp.NewTextResult("错误：session_id 参数不能为空"), nil
		}

		messages, err := chatService.GetSessionHistory(ctx, session.SessionID(sessionID))
		if err != nil {
			return nil, fmt.Errorf("获取历史记录失败: %w", err)
		}

		result := map[string]interface{}{
			"session_id": sessionID,
			"messages":   messages,
			"count":      len(messages),
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("序列化响应失败: %w", err)
		}
		return mcp.NewTextResult(string(resultJSON)), nil
	})
}

// registerListSessionsTool 注册列出会话工具
func registerListSessionsTool(server *mcp.Server, chatService *application.ChatService) {
	tool := mcp.NewTool(
		"list_sessions",
		mcp.WithDescription("列出所有聊天会话的摘要信息，包括会话 ID、消息数量、最后活跃时间等。"),
	)

	server.RegisterTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sessions, err := chatService.ListSessions(ctx)
		if err != nil {
			return nil, fmt.Errorf("获取会话列表失败: %w", err)
		}

		result := map[string]interface{}{
			"sessions": sessions,
			"count":    len(sessions),
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("序列化响应失败: %w", err)
		}
		return mcp.NewTextResult(string(resultJSON)), nil
	})
}

// registerDeleteSessionTool 注册删除会话工具
func registerDeleteSessionTool(server *mcp.Server, chatService *application.ChatService) {
	tool := mcp.NewTool(
		"delete_session",
		mcp.WithDescription("删除指定的聊天会话及其所有历史消息。"),
		mcp.WithString("session_id",
			mcp.Description("要删除的会话 ID"),
			mcp.Required(),
		),
	)

	server.RegisterTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sessionID, _ := req.Params.Arguments["session_id"].(string)
		if sessionID == "" {
			return mcp.NewTextResult("错误：session_id 参数不能为空"), nil
		}

		if err := chatService.DeleteSession(ctx, session.SessionID(sessionID)); err != nil {
			return nil, fmt.Errorf("删除会话失败: %w", err)
		}

		return mcp.NewTextResult(fmt.Sprintf(`{"message":"会话 %s 已成功删除"}`, sessionID)), nil
	})
}
