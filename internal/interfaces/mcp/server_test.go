package mcp_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"aiProject/internal/application"
	"aiProject/internal/domain/model"
	"aiProject/internal/infrastructure/session"
	mcp_iface "aiProject/internal/interfaces/mcp"
	mcp "trpc.group/trpc-go/trpc-mcp-go"
)

// mockModelGenerator 模拟模型生成器，不依赖 Ollama
type mockModelGenerator struct {
	response string
}

func (m *mockModelGenerator) Generate(_ context.Context, _ model.Prompt) (model.GenerateResult, error) {
	return model.GenerateResult{Response: model.Response(m.response)}, nil
}

func (m *mockModelGenerator) GenerateStream(_ context.Context, _ model.Prompt) (<-chan model.StreamChunk, error) {
	ch := make(chan model.StreamChunk, 2)
	go func() {
		defer close(ch)
		ch <- model.StreamChunk{Content: m.response}
		ch <- model.StreamChunk{Done: true}
	}()
	return ch, nil
}

func (m *mockModelGenerator) GenerateStreamWithMessages(_ context.Context, _ []model.Message) (<-chan model.StreamChunk, error) {
	ch := make(chan model.StreamChunk, 2)
	go func() {
		defer close(ch)
		ch <- model.StreamChunk{Content: m.response}
		ch <- model.StreamChunk{Done: true}
	}()
	return ch, nil
}

func (m *mockModelGenerator) GenerateWithTools(_ context.Context, _ []model.Message, _ []model.ToolDefinition) (model.GenerateWithToolsResult, error) {
	return model.GenerateWithToolsResult{Content: m.response}, nil
}

// 确保实现了接口
var _ model.Generator = (*mockModelGenerator)(nil)

// newTestChatService 创建用于测试的 ChatService（内存存储 + mock 模型）
func newTestChatService(mockResp string) *application.ChatService {
	repo := session.NewMemoryRepository()
	gen := &mockModelGenerator{response: mockResp}
	return application.NewChatService(repo, gen)
}

// setupTestEnv 创建测试环境：MCP Server + httptest Server + MCP Client
func setupTestEnv(t *testing.T, mockResp string) (*mcp.Client, func()) {
	t.Helper()
	svc := newTestChatService(mockResp)
	mcpServer := mcp_iface.NewMCPServer(svc, "localhost:19999", "/mcp")

	httpServer := httptest.NewServer(mcpServer.HTTPHandler())

	client, err := mcp.NewClient(httpServer.URL+"/mcp", mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	})
	if err != nil {
		t.Fatalf("创建 MCP Client 失败: %v", err)
	}

	// 初始化握手
	ctx := context.Background()
	if _, err := client.Initialize(ctx, &mcp.InitializeRequest{}); err != nil {
		t.Fatalf("MCP 握手失败: %v", err)
	}

	cleanup := func() {
		client.Close()
		httpServer.Close()
	}
	return client, cleanup
}

// ─────────────────────────────────────────────────────────────────────────────
// 测试：工具注册是否完整
// ─────────────────────────────────────────────────────────────────────────────

func TestToolsRegistered(t *testing.T) {
	client, cleanup := setupTestEnv(t, "")
	defer cleanup()

	result, err := client.ListTools(context.Background(), &mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools 失败: %v", err)
	}

	expected := map[string]bool{
		"chat":           false,
		"get_history":    false,
		"list_sessions":  false,
		"delete_session": false,
	}
	for _, tool := range result.Tools {
		if _, ok := expected[tool.Name]; ok {
			expected[tool.Name] = true
			t.Logf("✅ 工具已注册: %-20s — %s", tool.Name, tool.Description)
		}
	}
	for name, found := range expected {
		if !found {
			t.Errorf("❌ 工具 '%s' 未注册", name)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 测试：chat 工具 - 基本消息
// ─────────────────────────────────────────────────────────────────────────────

func TestChatTool_BasicMessage(t *testing.T) {
	client, cleanup := setupTestEnv(t, "你好！我是 AI 助手。")
	defer cleanup()

	result, err := client.CallTool(context.Background(), &mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "chat",
			Arguments: map[string]interface{}{"message": "你好"},
		},
	})
	if err != nil {
		t.Fatalf("调用 chat 工具失败: %v", err)
	}

	text := getTextContent(t, result)
	t.Logf("✅ chat 返回: %s", text)

	if !contains(text, "你好！我是 AI 助手。") {
		t.Errorf("期望包含 '你好！我是 AI 助手。'，实际: %s", text)
	}
	if !contains(text, "session_id") {
		t.Error("返回结果应包含 session_id 字段")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 测试：chat 工具 - 多轮对话（复用 session_id）
// ─────────────────────────────────────────────────────────────────────────────

func TestChatTool_MultiTurn(t *testing.T) {
	client, cleanup := setupTestEnv(t, "第二轮回复")
	defer cleanup()

	ctx := context.Background()

	// 第一轮：创建会话
	r1, err := client.CallTool(ctx, &mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "chat",
			Arguments: map[string]interface{}{"message": "第一条消息"},
		},
	})
	if err != nil {
		t.Fatalf("第一轮 chat 失败: %v", err)
	}
	text1 := getTextContent(t, r1)

	// 从返回中提取 session_id
	sessionID := extractField(t, text1, "session_id")
	if sessionID == "" {
		t.Fatal("第一轮未返回 session_id")
	}
	t.Logf("第一轮 session_id: %s", sessionID)

	// 第二轮：复用同一会话
	r2, err := client.CallTool(ctx, &mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "chat",
			Arguments: map[string]interface{}{
				"message":    "第二条消息",
				"session_id": sessionID,
			},
		},
	})
	if err != nil {
		t.Fatalf("第二轮 chat 失败: %v", err)
	}
	text2 := getTextContent(t, r2)
	t.Logf("✅ 第二轮返回: %s", text2)

	if !contains(text2, sessionID) {
		t.Errorf("第二轮返回的 session_id 应与第一轮相同 (%s)", sessionID)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 测试：chat 工具 - 空消息校验
// ─────────────────────────────────────────────────────────────────────────────

func TestChatTool_EmptyMessage(t *testing.T) {
	client, cleanup := setupTestEnv(t, "")
	defer cleanup()

	result, err := client.CallTool(context.Background(), &mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "chat",
			Arguments: map[string]interface{}{"message": ""},
		},
	})
	if err != nil {
		t.Fatalf("调用失败: %v", err)
	}

	text := getTextContent(t, result)
	if !contains(text, "错误：message 参数不能为空") {
		t.Errorf("期望错误提示，实际: %s", text)
	}
	t.Logf("✅ 空消息校验通过: %s", text)
}

// ─────────────────────────────────────────────────────────────────────────────
// 测试：get_history 工具
// ─────────────────────────────────────────────────────────────────────────────

func TestGetHistoryTool(t *testing.T) {
	client, cleanup := setupTestEnv(t, "历史测试回复")
	defer cleanup()

	ctx := context.Background()

	// 先发一条消息，产生历史记录
	r1, err := client.CallTool(ctx, &mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "chat",
			Arguments: map[string]interface{}{"message": "测试消息"},
		},
	})
	if err != nil {
		t.Fatalf("chat 失败: %v", err)
	}
	sessionID := extractField(t, getTextContent(t, r1), "session_id")

	// 查询历史
	r2, err := client.CallTool(ctx, &mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "get_history",
			Arguments: map[string]interface{}{"session_id": sessionID},
		},
	})
	if err != nil {
		t.Fatalf("get_history 失败: %v", err)
	}

	text := getTextContent(t, r2)
	t.Logf("✅ get_history 返回: %s", text)

	if !contains(text, "messages") {
		t.Error("返回结果应包含 messages 字段")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 测试：list_sessions 工具
// ─────────────────────────────────────────────────────────────────────────────

func TestListSessionsTool(t *testing.T) {
	client, cleanup := setupTestEnv(t, "回复")
	defer cleanup()

	ctx := context.Background()

	// 创建两个会话
	client.CallTool(ctx, &mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "chat", Arguments: map[string]interface{}{"message": "会话1"}}})
	client.CallTool(ctx, &mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "chat", Arguments: map[string]interface{}{"message": "会话2"}}})

	result, err := client.CallTool(ctx, &mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "list_sessions",
			Arguments: map[string]interface{}{},
		},
	})
	if err != nil {
		t.Fatalf("list_sessions 失败: %v", err)
	}

	text := getTextContent(t, result)
	t.Logf("✅ list_sessions 返回: %s", text)

	if !contains(text, "sessions") {
		t.Error("返回结果应包含 sessions 字段")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 测试：delete_session 工具
// ─────────────────────────────────────────────────────────────────────────────

func TestDeleteSessionTool(t *testing.T) {
	client, cleanup := setupTestEnv(t, "回复")
	defer cleanup()

	ctx := context.Background()

	// 先创建一个会话
	r1, err := client.CallTool(ctx, &mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "chat",
			Arguments: map[string]interface{}{"message": "待删除的会话"},
		},
	})
	if err != nil {
		t.Fatalf("chat 失败: %v", err)
	}
	sessionID := extractField(t, getTextContent(t, r1), "session_id")

	// 删除该会话
	r2, err := client.CallTool(ctx, &mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "delete_session",
			Arguments: map[string]interface{}{"session_id": sessionID},
		},
	})
	if err != nil {
		t.Fatalf("delete_session 失败: %v", err)
	}

	text := getTextContent(t, r2)
	t.Logf("✅ delete_session 返回: %s", text)

	if !contains(text, "已成功删除") {
		t.Errorf("期望包含 '已成功删除'，实际: %s", text)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 测试：chat 工具 - 返回 thinking 字段
// ─────────────────────────────────────────────────────────────────────────────

func TestChatTool_WithThinking(t *testing.T) {
	thinkingResp := `{"thinking":"这是思考过程","response":"这是最终回复"}`
	client, cleanup := setupTestEnv(t, thinkingResp)
	defer cleanup()

	result, err := client.CallTool(context.Background(), &mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "chat",
			Arguments: map[string]interface{}{"message": "需要思考的问题"},
		},
	})
	if err != nil {
		t.Fatalf("调用失败: %v", err)
	}

	text := getTextContent(t, result)
	t.Logf("✅ thinking 响应: %s", text)

	if !contains(text, "这是最终回复") {
		t.Errorf("期望包含 '这是最终回复'，实际: %s", text)
	}
	if !contains(text, "这是思考过程") {
		t.Errorf("期望包含 '这是思考过程'，实际: %s", text)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 辅助函数
// ─────────────────────────────────────────────────────────────────────────────

// getTextContent 从工具调用结果中提取文本内容
func getTextContent(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("工具返回内容为空")
	}
	// TextContent 可能是值类型或指针类型，两种都尝试
	switch v := result.Content[0].(type) {
	case *mcp.TextContent:
		return v.Text
	case mcp.TextContent:
		return v.Text
	default:
		t.Fatalf("期望 TextContent，实际类型: %T", result.Content[0])
		return ""
	}
}

// contains 检查字符串是否包含子串
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

// extractField 从 JSON 字符串中提取指定字段的值（简单字符串提取）
func extractField(t *testing.T, jsonStr, field string) string {
	t.Helper()
	key := `"` + field + `":"`
	idx := 0
	for i := 0; i < len(jsonStr)-len(key); i++ {
		if jsonStr[i:i+len(key)] == key {
			idx = i + len(key)
			end := idx
			for end < len(jsonStr) && jsonStr[end] != '"' {
				end++
			}
			return jsonStr[idx:end]
		}
	}
	return ""
}