// Package mcpclient 实现 MCP Client：连接外部 MCP Server，发现其工具并注册到本地工具注册表，
// 使外部 MCP 工具可被 ReAct 循环与 Workflow 工具节点调用。
// 仅支持 streamable-http（URL）传输，不支持 stdio 进程拉起（避免命令执行风险）。
package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	domain_model "aiProject/internal/domain/model"
	"aiProject/internal/domain/tool"
	"aiProject/internal/shared"
	"go.uber.org/zap"
	mcp "trpc.group/trpc-go/trpc-mcp-go"
)

// ServerConfig 外部 MCP Server 配置
type ServerConfig struct {
	Name string `json:"name" yaml:"name"` // 唯一名称（用作工具名前缀）
	URL  string `json:"url" yaml:"url"`   // streamable-http 地址，如 http://localhost:8001/mcp
}

// ServerStatus 外部 MCP Server 运行状态（用于展示）
type ServerStatus struct {
	Name        string   `json:"name"`
	URL         string   `json:"url"`
	Connected   bool     `json:"connected"`
	ToolCount   int      `json:"tool_count"`
	Tools       []string `json:"tools"`
	Error       string   `json:"error,omitempty"`
	ConnectedAt string   `json:"connected_at,omitempty"`
}

// serverConn 单个外部 Server 的连接状态
type serverConn struct {
	cfg         ServerConfig
	client      *mcp.Client
	toolNames   []string // 已注册到本地注册表的（带前缀）工具名
	origToolMap map[string]string // 本地名 → 外部原始工具名
	connectedAt time.Time
	err         string
}

// Manager 管理所有外部 MCP Server 连接
type Manager struct {
	mu      sync.RWMutex
	servers map[string]*serverConn
}

// NewManager 创建 MCP Client 管理器
func NewManager() *Manager {
	return &Manager{servers: make(map[string]*serverConn)}
}

var nameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// sanitize 将名称转为合法的工具名字符集（OpenAI 工具名要求 ^[a-zA-Z0-9_-]+$）
func sanitize(s string) string {
	return strings.Trim(nameSanitizer.ReplaceAllString(s, "_"), "_")
}

// AddServer 连接外部 MCP Server，发现并注册其工具。若同名 Server 已存在，先移除旧的。
// 返回注册的工具数量。
func (m *Manager) AddServer(ctx context.Context, cfg ServerConfig) (int, error) {
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.URL = strings.TrimSpace(cfg.URL)
	if cfg.Name == "" || cfg.URL == "" {
		return 0, fmt.Errorf("name 和 url 不能为空")
	}

	// 已存在则先移除（重连）
	m.RemoveServer(cfg.Name)

	client, err := mcp.NewClient(cfg.URL, mcp.Implementation{Name: "aiproject-mcp-client", Version: "1.0.0"})
	if err != nil {
		return 0, fmt.Errorf("创建 MCP 客户端失败: %w", err)
	}

	if _, err := client.Initialize(ctx, &mcp.InitializeRequest{}); err != nil {
		_ = client.Close()
		return 0, fmt.Errorf("初始化 MCP 连接失败: %w", err)
	}

	listResp, err := client.ListTools(ctx, &mcp.ListToolsRequest{})
	if err != nil {
		_ = client.Close()
		return 0, fmt.Errorf("获取工具列表失败: %w", err)
	}

	conn := &serverConn{
		cfg:         cfg,
		client:      client,
		connectedAt: time.Now(),
		origToolMap: make(map[string]string),
	}

	prefix := "mcp_" + sanitize(cfg.Name) + "_"
	for _, t := range listResp.Tools {
		localName := prefix + sanitize(t.Name)
		conn.origToolMap[localName] = t.Name
		conn.toolNames = append(conn.toolNames, localName)
		m.registerTool(client, localName, t)
	}

	m.mu.Lock()
	m.servers[cfg.Name] = conn
	m.mu.Unlock()

	shared.GetLogger().Info("已接入外部 MCP Server",
		zap.String("name", cfg.Name), zap.String("url", cfg.URL),
		zap.Int("tools", len(conn.toolNames)),
	)
	return len(conn.toolNames), nil
}

// registerTool 将一个外部 MCP 工具注册到本地工具注册表
func (m *Manager) registerTool(client *mcp.Client, localName string, t mcp.Tool) {
	origName := t.Name
	desc := t.Description
	if desc == "" {
		desc = "外部 MCP 工具：" + origName
	}

	tool.Register(&tool.Tool{
		Definition: domain_model.ToolDefinition{
			Name:        localName,
			DisplayName: origName,
			Description: desc,
			Parameters:  convertSchema(t.InputSchema),
		},
		Execute: func(ctx context.Context, args map[string]interface{}) (string, error) {
			resp, err := client.CallTool(ctx, &mcp.CallToolRequest{
				Params: mcp.CallToolParams{Name: origName, Arguments: args},
			})
			if err != nil {
				return "", fmt.Errorf("MCP 工具调用失败: %w", err)
			}
			var sb strings.Builder
			for _, c := range resp.Content {
				if tc, ok := c.(mcp.TextContent); ok {
					sb.WriteString(tc.Text)
				}
			}
			out := sb.String()
			if resp.IsError {
				return out, fmt.Errorf("MCP 工具返回错误: %s", out)
			}
			return out, nil
		},
	})
}

// RemoveServer 断开并移除外部 MCP Server，注销其全部工具
func (m *Manager) RemoveServer(name string) {
	m.mu.Lock()
	conn, ok := m.servers[name]
	if ok {
		delete(m.servers, name)
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	for _, tn := range conn.toolNames {
		tool.Unregister(tn)
	}
	if conn.client != nil {
		_ = conn.client.Close()
	}
	shared.GetLogger().Info("已移除外部 MCP Server", zap.String("name", name))
}

// ListServers 返回所有外部 MCP Server 状态
func (m *Manager) ListServers() []ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ServerStatus, 0, len(m.servers))
	for _, conn := range m.servers {
		st := ServerStatus{
			Name:      conn.cfg.Name,
			URL:       conn.cfg.URL,
			Connected: conn.err == "",
			ToolCount: len(conn.toolNames),
			Tools:     conn.toolNames,
			Error:     conn.err,
		}
		if !conn.connectedAt.IsZero() {
			st.ConnectedAt = conn.connectedAt.Format("2006-01-02 15:04:05")
		}
		out = append(out, st)
	}
	return out
}

// convertSchema 将 MCP 工具的 openapi3 输入 schema 转为本地 ToolParameters。
// 通过 JSON 中转，避免与 kin-openapi 的具体 Go 类型耦合（type 字段可能是 string 或 []string）。
func convertSchema(schema interface{}) domain_model.ToolParameters {
	out := domain_model.ToolParameters{
		Type:       "object",
		Properties: map[string]domain_model.ToolParameterProperty{},
	}
	if schema == nil {
		return out
	}
	data, err := json.Marshal(schema)
	if err != nil || len(data) == 0 || string(data) == "null" {
		return out
	}

	var parsed struct {
		Type       json.RawMessage `json:"type"`
		Properties map[string]struct {
			Type        json.RawMessage `json:"type"`
			Description string          `json:"description"`
			Enum        []interface{}   `json:"enum"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return out
	}

	if t := firstType(parsed.Type, "object"); t != "" {
		out.Type = t
	}
	for name, p := range parsed.Properties {
		prop := domain_model.ToolParameterProperty{
			Type:        firstType(p.Type, "string"),
			Description: p.Description,
		}
		for _, e := range p.Enum {
			prop.Enum = append(prop.Enum, fmt.Sprint(e))
		}
		out.Properties[name] = prop
	}
	out.Required = parsed.Required
	return out
}

// firstType 解析 JSON Schema 的 type 字段（兼容字符串与字符串数组两种形式）
func firstType(raw json.RawMessage, def string) string {
	if len(raw) == 0 {
		return def
	}
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		return s
	}
	var arr []string
	if json.Unmarshal(raw, &arr) == nil && len(arr) > 0 {
		return arr[0]
	}
	return def
}
