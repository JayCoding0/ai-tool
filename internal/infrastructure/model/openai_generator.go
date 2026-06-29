package model

import (
	"context"
	"encoding/json"
	"fmt"

	"aiProject/internal/domain/model"
	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

// OpenAIGenerator OpenAI兼容接口的模型生成器（支持阿里云DashScope等）
type OpenAIGenerator struct {
	client    openai.Client
	modelName string
	// enableThinking 是否开启 DeepSeek V4 等模型的思考模式。
	// 开启后会在请求体注入 DeepSeek 专有字段 thinking: {"type": "enabled"}。
	enableThinking bool
	// reasoningEffort 思考强度（low/medium/high），仅在 enableThinking 为 true 时生效。
	reasoningEffort string
}

// Option OpenAIGenerator 的可选配置项
type Option func(*OpenAIGenerator)

// WithThinking 开启思考模式（DeepSeek V4 等），可选指定思考强度 effort（low/medium/high，留空则不传）。
func WithThinking(enabled bool, effort string) Option {
	return func(g *OpenAIGenerator) {
		g.enableThinking = enabled
		g.reasoningEffort = effort
	}
}

// NewOpenAIGenerator 创建OpenAI兼容接口的生成器实例
// baseURL: API地址，例如 https://dashscope.aliyuncs.com/compatible-mode/v1
// apiKey:  API密钥
// modelName: 模型名称，例如 qwen-plus、qwen-max 等
// opts: 可选配置，如 WithThinking 开启 DeepSeek 思考模式
func NewOpenAIGenerator(baseURL, apiKey, modelName string, opts ...Option) *OpenAIGenerator {
	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
	)
	g := &OpenAIGenerator{
		client:    client,
		modelName: modelName,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// requestOptions 构建每次请求附加的 RequestOption。
// 当开启思考模式时，注入 DeepSeek 专有字段 thinking（及可选的 reasoning_effort）。
// 这些字段不在标准 OpenAI SDK 参数中，需通过 WithJSONSet 写入原始请求体。
func (g *OpenAIGenerator) requestOptions() []option.RequestOption {
	if !g.enableThinking {
		return nil
	}
	reqOpts := []option.RequestOption{
		option.WithJSONSet("thinking", map[string]string{"type": "enabled"}),
	}
	if g.reasoningEffort != "" {
		reqOpts = append(reqOpts, option.WithJSONSet("reasoning_effort", g.reasoningEffort))
	}
	return reqOpts
}

// Generate 生成模型响应
func (g *OpenAIGenerator) Generate(ctx context.Context, prompt model.Prompt) (model.GenerateResult, error) {
	resp, err := g.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: g.modelName,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(string(prompt)),
		},
	}, g.requestOptions()...)
	if err != nil {
		return model.GenerateResult{}, fmt.Errorf("OpenAI接口请求失败: %v", err)
	}

	if len(resp.Choices) == 0 {
		return model.GenerateResult{}, fmt.Errorf("模型返回空响应")
	}

	usage := model.TokenUsage{
		PromptTokens:     int(resp.Usage.PromptTokens),
		CompletionTokens: int(resp.Usage.CompletionTokens),
		TotalTokens:      int(resp.Usage.TotalTokens),
	}

	return model.GenerateResult{
		Response: model.Response(resp.Choices[0].Message.Content),
		Usage:    usage,
	}, nil
}

// GenerateStream 流式生成模型响应（OpenAI stream）
func (g *OpenAIGenerator) GenerateStream(ctx context.Context, prompt model.Prompt) (<-chan model.StreamChunk, error) {
	stream := g.client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
		Model: g.modelName,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(string(prompt)),
		},
	}, g.requestOptions()...)

	ch := make(chan model.StreamChunk, 32)
	go func() {
		defer close(ch)
		var promptTokens, completionTokens int
		for stream.Next() {
			event := stream.Current()
			if len(event.Choices) > 0 {
				delta := event.Choices[0].Delta.Content
				if delta != "" {
					ch <- model.StreamChunk{Content: delta}
				}
			}
			// 最后一个事件可能携带 usage
			if event.Usage.TotalTokens > 0 {
				promptTokens = int(event.Usage.PromptTokens)
				completionTokens = int(event.Usage.CompletionTokens)
			}
		}
		if err := stream.Err(); err != nil {
			ch <- model.StreamChunk{Err: fmt.Errorf("OpenAI流式请求失败: %v", err)}
			return
		}
		ch <- model.StreamChunk{
			Done: true,
			Usage: model.TokenUsage{
				PromptTokens:     promptTokens,
				CompletionTokens: completionTokens,
				TotalTokens:      promptTokens + completionTokens,
			},
		}
	}()

	return ch, nil
}

// GenerateStreamWithMessages 流式生成模型响应（使用结构化消息列表，支持多轮对话）
func (g *OpenAIGenerator) GenerateStreamWithMessages(ctx context.Context, messages []model.Message) (<-chan model.StreamChunk, error) {
	var openaiMessages []openai.ChatCompletionMessageParamUnion
	for _, msg := range messages {
		switch msg.Role {
		case model.RoleSystem:
			openaiMessages = append(openaiMessages, openai.SystemMessage(msg.Content))
		case model.RoleUser:
			openaiMessages = append(openaiMessages, openai.UserMessage(msg.Content))
		case model.RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				// 带工具调用的 assistant 消息（ReAct 循环中间轮次）
				var toolCalls []openai.ChatCompletionMessageToolCallParam
				for _, tc := range msg.ToolCalls {
					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallParam{
						ID:   tc.ID,
						Type: "function",
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      tc.Name,
							Arguments: tc.Arguments,
						},
					})
				}
				openaiMessages = append(openaiMessages, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &openai.ChatCompletionAssistantMessageParam{
						ToolCalls: toolCalls,
					},
				})
			} else {
				openaiMessages = append(openaiMessages, openai.AssistantMessage(msg.Content))
			}
		case model.RoleTool:
			// 工具执行结果，必须传回给模型
			openaiMessages = append(openaiMessages, openai.ToolMessage(msg.Content, msg.ToolCallID))
		}
	}

	stream := g.client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
		Model:    g.modelName,
		Messages: openaiMessages,
	}, g.requestOptions()...)

	ch := make(chan model.StreamChunk, 32)
	go func() {
		defer close(ch)
		var promptTokens, completionTokens int
		for stream.Next() {
			event := stream.Current()
			if len(event.Choices) > 0 {
				delta := event.Choices[0].Delta
				// 推理模型：分离 reasoning_content（DashScope/DeepSeek-R1 等的扩展字段）作为思考过程
				if reasoning := extractReasoningContent(delta); reasoning != "" {
					ch <- model.StreamChunk{Thinking: reasoning}
				}
				if delta.Content != "" {
					ch <- model.StreamChunk{Content: delta.Content}
				}
			}
			if event.Usage.TotalTokens > 0 {
				promptTokens = int(event.Usage.PromptTokens)
				completionTokens = int(event.Usage.CompletionTokens)
			}
		}
		if err := stream.Err(); err != nil {
			ch <- model.StreamChunk{Err: fmt.Errorf("OpenAI流式请求失败: %v", err)}
			return
		}
		ch <- model.StreamChunk{
			Done: true,
			Usage: model.TokenUsage{
				PromptTokens:     promptTokens,
				CompletionTokens: completionTokens,
				TotalTokens:      promptTokens + completionTokens,
			},
		}
	}()
	return ch, nil
}

// GenerateWithTools 支持 Function Calling 的生成（非流式）
func (g *OpenAIGenerator) GenerateWithTools(ctx context.Context, messages []model.Message, tools []model.ToolDefinition) (model.GenerateWithToolsResult, error) {
	return g.GenerateWithToolsOpts(ctx, messages, tools, model.GenerateOptions{})
}

// GenerateWithToolsOpts 支持 Function Calling + 结构化输出选项的生成（非流式）
// 实现 model.StructuredGenerator 接口：支持 response_format（JSON mode / JSON Schema）与温度等高级选项。
func (g *OpenAIGenerator) GenerateWithToolsOpts(ctx context.Context, messages []model.Message, tools []model.ToolDefinition, opts model.GenerateOptions) (model.GenerateWithToolsResult, error) {
	// 转换消息格式
	var openaiMessages []openai.ChatCompletionMessageParamUnion
	for _, msg := range messages {
		switch msg.Role {
		case model.RoleSystem:
			openaiMessages = append(openaiMessages, openai.SystemMessage(msg.Content))
		case model.RoleUser:
			openaiMessages = append(openaiMessages, openai.UserMessage(msg.Content))
		case model.RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				// 带工具调用的 assistant 消息
				var toolCalls []openai.ChatCompletionMessageToolCallParam
				for _, tc := range msg.ToolCalls {
					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallParam{
						ID:   tc.ID,
						Type: "function",
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      tc.Name,
							Arguments: tc.Arguments,
						},
					})
				}
				openaiMessages = append(openaiMessages, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &openai.ChatCompletionAssistantMessageParam{
						ToolCalls: toolCalls,
					},
				})
			} else {
				openaiMessages = append(openaiMessages, openai.AssistantMessage(msg.Content))
			}
		case model.RoleTool:
			openaiMessages = append(openaiMessages, openai.ToolMessage(msg.Content, msg.ToolCallID))
		}
	}

	// 转换工具定义格式
	var openaiTools []openai.ChatCompletionToolParam
	for _, t := range tools {
		propsJSON, _ := json.Marshal(t.Parameters.Properties)
		var propsMap map[string]interface{}
		json.Unmarshal(propsJSON, &propsMap) //nolint:errcheck

		requiredJSON, _ := json.Marshal(t.Parameters.Required)
		var requiredSlice []string
		json.Unmarshal(requiredJSON, &requiredSlice) //nolint:errcheck

		openaiTools = append(openaiTools, openai.ChatCompletionToolParam{
			Type: "function",
			Function: openai.FunctionDefinitionParam{
				Name:        t.Name,
				Description: openai.String(t.Description),
				Parameters: openai.FunctionParameters{
					"type":       t.Parameters.Type,
					"properties": propsMap,
					"required":   requiredSlice,
				},
			},
		})
	}

	params := openai.ChatCompletionNewParams{
		Model:    g.modelName,
		Messages: openaiMessages,
	}
	if len(openaiTools) > 0 {
		params.Tools = openaiTools
	}

	// 应用温度参数
	if opts.Temperature != nil {
		params.Temperature = openai.Float(*opts.Temperature)
	}

	// 应用推理模型思考强度（reasoning models，如 o1/o3/DeepSeek-R1）
	if opts.ReasoningEffort != "" {
		params.ReasoningEffort = shared.ReasoningEffort(opts.ReasoningEffort)
	}

	// 应用结构化输出格式（#24）
	if rf := opts.ResponseFormat; rf != nil {
		switch rf.Type {
		case model.ResponseFormatJSONObject:
			params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
				OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
			}
		case model.ResponseFormatJSONSchema:
			name := rf.SchemaName
			if name == "" {
				name = "structured_output"
			}
			schemaParam := shared.ResponseFormatJSONSchemaJSONSchemaParam{
				Name:   name,
				Strict: openai.Bool(rf.Strict),
			}
			if rf.Schema != nil {
				schemaParam.Schema = rf.Schema
			}
			params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
				OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
					JSONSchema: schemaParam,
				},
			}
		}
	}

	resp, err := g.client.Chat.Completions.New(ctx, params, g.requestOptions()...)
	if err != nil {
		return model.GenerateWithToolsResult{}, fmt.Errorf("OpenAI工具调用请求失败: %v", err)
	}
	if len(resp.Choices) == 0 {
		return model.GenerateWithToolsResult{}, fmt.Errorf("模型返回空响应")
	}

	choice := resp.Choices[0]
	result := model.GenerateWithToolsResult{
		Content: choice.Message.Content,
		Usage: model.TokenUsage{
			PromptTokens:     int(resp.Usage.PromptTokens),
			CompletionTokens: int(resp.Usage.CompletionTokens),
			TotalTokens:      int(resp.Usage.TotalTokens),
		},
	}

	// 解析工具调用
	for _, tc := range choice.Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, model.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	return result, nil
}

// extractReasoningContent 从流式 delta 中提取推理模型的 reasoning_content 思考过程。
// reasoning_content 是 DashScope / DeepSeek-R1 等推理模型的非标准扩展字段，
// 不在 openai-go 的标准 delta 结构中，需通过 ExtraFields 的原始 JSON 取出并解码。
func extractReasoningContent(delta openai.ChatCompletionChunkChoiceDelta) string {
	field, ok := delta.JSON.ExtraFields["reasoning_content"]
	if !ok || !field.Valid() {
		return ""
	}
	raw := field.Raw()
	if raw == "" || raw == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return ""
	}
	return s
}