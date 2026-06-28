// Package model 定义模型领域的核心类型和接口
// 包括模型生成器接口、消息格式、工具调用协议等
package model

import (
	"context"
)

// ModelName 模型名称值对象
type ModelName string

// Prompt 提示词值对象
type Prompt string

// Response 模型响应值对象
type Response string

// TokenUsage token 使用量
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// GenerateResult 模型生成结果（包含响应内容和 token 用量）
type GenerateResult struct {
	Response Response
	Usage    TokenUsage
}

// StreamChunk 流式输出的单个数据块
type StreamChunk struct {
	Content  string     // 本次增量内容
	Thinking string     // 思考过程增量（可选）
	Done     bool       // 是否结束
	Usage    TokenUsage // 结束时携带 token 用量
	Err      error      // 错误信息
}

// ToolParameterProperty 工具参数属性定义
type ToolParameterProperty struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
}

// ToolParameters 工具参数 JSON Schema
type ToolParameters struct {
	Type       string                           `json:"type"`
	Properties map[string]ToolParameterProperty `json:"properties"`
	Required   []string                         `json:"required,omitempty"`
}

// ToolDefinition 工具定义（传给模型的工具描述）
type ToolDefinition struct {
	Name        string         `json:"name"`
	DisplayName string         `json:"display_name,omitempty"` // 展示给用户的中文名称（可选）
	Description string         `json:"description"`
	Parameters  ToolParameters `json:"parameters"`
}

// ToolCall 模型返回的工具调用请求
type ToolCall struct {
	ID       string `json:"id"`       // 工具调用 ID（OpenAI 格式）
	Name     string `json:"name"`     // 工具名称
	Arguments string `json:"arguments"` // JSON 格式的参数字符串
}

// MessageRole 消息角色
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool" // 工具调用结果
)

// Message 对话消息（支持 Function Calling 的完整消息格式）
type Message struct {
	Role       MessageRole `json:"role"`
	Content    string      `json:"content"`
	ToolCallID string      `json:"tool_call_id,omitempty"` // role=tool 时使用
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`   // role=assistant 且有工具调用时使用
}

// GenerateWithToolsResult 带工具调用的生成结果
type GenerateWithToolsResult struct {
	Content   string     // 文本回复（工具调用时可能为空）
	ToolCalls []ToolCall // 模型请求调用的工具列表（非空时需执行工具）
	Usage     TokenUsage
}

// ─── 结构化输出（#24）─────────────────────────────────────────────────────────

// ResponseFormatType 模型输出格式类型
type ResponseFormatType string

const (
	// ResponseFormatText 默认纯文本输出
	ResponseFormatText ResponseFormatType = "text"
	// ResponseFormatJSONObject JSON mode，保证输出为合法 JSON（不约束具体结构）
	ResponseFormatJSONObject ResponseFormatType = "json_object"
	// ResponseFormatJSONSchema 严格的 JSON Schema 约束输出（推荐，节点间结构化传参可靠）
	ResponseFormatJSONSchema ResponseFormatType = "json_schema"
)

// ResponseFormat 结构化输出格式定义
type ResponseFormat struct {
	Type       ResponseFormatType     `json:"type"`                  // 输出格式类型
	SchemaName string                 `json:"schema_name,omitempty"` // json_schema 模式下的 schema 名称（a-z/A-Z/0-9/_/-）
	Schema     map[string]interface{} `json:"schema,omitempty"`      // json_schema 模式下的 JSON Schema 对象
	Strict     bool                   `json:"strict,omitempty"`      // 是否严格遵循 schema（json_schema 模式）
}

// GenerateOptions 生成选项（结构化输出、采样温度等高级参数）
type GenerateOptions struct {
	ResponseFormat  *ResponseFormat // 为 nil 时使用默认文本输出
	Temperature     *float64        // 为 nil 时使用模型默认温度
	ReasoningEffort string          // 推理模型思考强度："" | "low" | "medium" | "high"，为空不透传
}

// StructuredGenerator 支持结构化输出选项的生成器（可选能力接口）
// 实现了此接口的 Generator 支持 response_format（JSON mode / JSON Schema）、温度等高级选项；
// 未实现的生成器（如部分本地模型）可由调用方降级为「在 Prompt 中注入 JSON 指令」的方式。
type StructuredGenerator interface {
	GenerateWithToolsOpts(ctx context.Context, messages []Message, tools []ToolDefinition, opts GenerateOptions) (GenerateWithToolsResult, error)
}

// Generator 模型生成器接口
type Generator interface {
	// Generate 生成模型响应（非流式）
	Generate(ctx context.Context, prompt Prompt) (GenerateResult, error)
	// GenerateStream 流式生成模型响应（兼容旧接口，使用字符串 prompt）
	GenerateStream(ctx context.Context, prompt Prompt) (<-chan StreamChunk, error)
	// GenerateStreamWithMessages 流式生成模型响应（使用结构化消息列表，支持多轮对话）
	GenerateStreamWithMessages(ctx context.Context, messages []Message) (<-chan StreamChunk, error)
	// GenerateWithTools 支持 Function Calling 的生成（非流式）
	// messages 为完整对话历史，tools 为可用工具列表
	GenerateWithTools(ctx context.Context, messages []Message, tools []ToolDefinition) (GenerateWithToolsResult, error)
}
