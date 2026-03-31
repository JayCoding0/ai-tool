package model

import (
	"context"
	"encoding/json"
	"fmt"

	"aiProject/internal/domain/model"
	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// OpenAIGenerator OpenAI兼容接口的模型生成器（支持阿里云DashScope等）
type OpenAIGenerator struct {
	client    openai.Client
	modelName string
}

// NewOpenAIGenerator 创建OpenAI兼容接口的生成器实例
// baseURL: API地址，例如 https://dashscope.aliyuncs.com/compatible-mode/v1
// apiKey:  API密钥
// modelName: 模型名称，例如 qwen-plus、qwen-max 等
func NewOpenAIGenerator(baseURL, apiKey, modelName string) *OpenAIGenerator {
	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
	)
	return &OpenAIGenerator{
		client:    client,
		modelName: modelName,
	}
}

// Generate 生成模型响应
func (g *OpenAIGenerator) Generate(ctx context.Context, prompt model.Prompt) (model.GenerateResult, error) {
	resp, err := g.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: g.modelName,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(string(prompt)),
		},
	})
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
	})

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
	})

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

	resp, err := g.client.Chat.Completions.New(ctx, params)
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