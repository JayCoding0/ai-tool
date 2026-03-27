package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"aiProject/internal/domain/model"
)

// defaultOllamaTimeout Ollama请求默认超时
const defaultOllamaTimeout = 120 * time.Second

// OllamaGenerator Ollama模型生成器
type OllamaGenerator struct {
	modelName string
	ollamaURL string
	httpClient *http.Client
}

// NewOllamaGenerator 创建Ollama生成器实例
func NewOllamaGenerator(modelName, ollamaURL string) *OllamaGenerator {
	return &OllamaGenerator{
		modelName: modelName,
		ollamaURL: ollamaURL,
		httpClient: &http.Client{
			Timeout: defaultOllamaTimeout,
		},
	}
}

// OllamaRequest Ollama API请求结构体
type OllamaRequest struct {
	Model   string                 `json:"model"`
	Prompt  string                 `json:"prompt"`
	Stream  bool                   `json:"stream"`
	Options map[string]interface{} `json:"options"`
}

// OllamaResponse Ollama API响应结构体
type OllamaResponse struct {
	Model           string `json:"model"`
	CreatedAt       string `json:"created_at"`
	Response        string `json:"response"`
	Thinking        string `json:"thinking,omitempty"`
	Done            bool   `json:"done"`
	PromptEvalCount int    `json:"prompt_eval_count"` // 输入 token 数
	EvalCount       int    `json:"eval_count"`        // 输出 token 数
}

// Generate 生成模型响应
func (g *OllamaGenerator) Generate(ctx context.Context, prompt model.Prompt) (model.GenerateResult, error) {
	// 构建Ollama API请求
	request := OllamaRequest{
		Model:  g.modelName,
		Prompt: string(prompt),
		Stream: false,
		Options: map[string]interface{}{
			"temperature": 0.7,
			"top_p":       0.9,
		},
	}

	// 序列化请求
	requestBody, err := json.Marshal(request)
	if err != nil {
		return model.GenerateResult{}, fmt.Errorf("序列化请求失败: %v", err)
	}

	// 使用带超时的 http.Client 发送请求，同时传递 ctx 支持主动取消
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.ollamaURL+"/api/generate",
		bytes.NewBuffer(requestBody))
	if err != nil {
		return model.GenerateResult{}, fmt.Errorf("创建请求失败: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return model.GenerateResult{}, fmt.Errorf("Ollama请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return model.GenerateResult{}, fmt.Errorf("读取响应失败: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return model.GenerateResult{}, fmt.Errorf("Ollama API错误: %s - %s", resp.Status, string(body))
	}

	// 解析响应
	var ollamaResp OllamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return model.GenerateResult{}, fmt.Errorf("解析响应失败: %v", err)
	}

	// 构建 token 用量
	usage := model.TokenUsage{
		PromptTokens:     ollamaResp.PromptEvalCount,
		CompletionTokens: ollamaResp.EvalCount,
		TotalTokens:      ollamaResp.PromptEvalCount + ollamaResp.EvalCount,
	}

	// 如果包含思考过程，返回结构化数据
	if ollamaResp.Thinking != "" {
		responseData := map[string]string{
			"thinking": ollamaResp.Thinking,
			"response": ollamaResp.Response,
		}
		responseJSON, err := json.Marshal(responseData)
		if err != nil {
			return model.GenerateResult{}, fmt.Errorf("序列化响应失败: %v", err)
		}
		return model.GenerateResult{Response: model.Response(responseJSON), Usage: usage}, nil
	}

	return model.GenerateResult{Response: model.Response(ollamaResp.Response), Usage: usage}, nil
}

// GenerateStream 流式生成模型响应（Ollama stream=true）
func (g *OllamaGenerator) GenerateStream(ctx context.Context, prompt model.Prompt) (<-chan model.StreamChunk, error) {
	request := OllamaRequest{
		Model:  g.modelName,
		Prompt: string(prompt),
		Stream: true,
		Options: map[string]interface{}{
			"temperature": 0.7,
			"top_p":       0.9,
		},
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %v", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.ollamaURL+"/api/generate",
		bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// 流式请求不设置超时（由 ctx 控制）
	streamClient := &http.Client{}
	resp, err := streamClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("Ollama流式请求失败: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("Ollama API错误: %s", resp.Status)
	}

	ch := make(chan model.StreamChunk, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		var promptTokens, evalTokens int
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var chunk OllamaResponse
			if err := json.Unmarshal(line, &chunk); err != nil {
				continue
			}
			if chunk.Done {
				promptTokens = chunk.PromptEvalCount
				evalTokens = chunk.EvalCount
				ch <- model.StreamChunk{
					Done: true,
					Usage: model.TokenUsage{
						PromptTokens:     promptTokens,
						CompletionTokens: evalTokens,
						TotalTokens:      promptTokens + evalTokens,
					},
				}
				return
			}
			ch <- model.StreamChunk{
				Content:  chunk.Response,
				Thinking: chunk.Thinking,
			}
		}
		if err := scanner.Err(); err != nil && err != io.EOF {
			ch <- model.StreamChunk{Err: err}
		}
	}()

	return ch, nil
}

// GenerateStreamWithMessages 流式生成模型响应（使用结构化消息列表，调用 /api/chat 接口）
func (g *OllamaGenerator) GenerateStreamWithMessages(ctx context.Context, messages []model.Message) (<-chan model.StreamChunk, error) {
	var chatMessages []ollamaChatMessage
	for _, msg := range messages {
		chatMessages = append(chatMessages, ollamaChatMessage{
			Role:    string(msg.Role),
			Content: msg.Content,
		})
	}

	reqBody := ollamaChatRequest{
		Model:    g.modelName,
		Messages: chatMessages,
		Stream:   true,
	}
	requestBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %v", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.ollamaURL+"/api/chat",
		bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	streamClient := &http.Client{}
	resp, err := streamClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("Ollama流式请求失败: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("Ollama API错误: %s", resp.Status)
	}

	ch := make(chan model.StreamChunk, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		var promptTokens, evalTokens int
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var chunk ollamaChatResponse
			if err := json.Unmarshal(line, &chunk); err != nil {
				continue
			}
			if chunk.Done {
				promptTokens = chunk.PromptEvalCount
				evalTokens = chunk.EvalCount
				ch <- model.StreamChunk{
					Done: true,
					Usage: model.TokenUsage{
						PromptTokens:     promptTokens,
						CompletionTokens: evalTokens,
						TotalTokens:      promptTokens + evalTokens,
					},
				}
				return
			}
			if chunk.Message.Content != "" {
				ch <- model.StreamChunk{Content: chunk.Message.Content}
			}
		}
		if err := scanner.Err(); err != nil && err != io.EOF {
			ch <- model.StreamChunk{Err: err}
		}
	}()
	return ch, nil
}

// ollamaChatToolCallFunc Ollama chat 工具调用函数
type ollamaChatToolCallFunc struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ollamaChatToolCall Ollama chat 工具调用
type ollamaChatToolCall struct {
	Function ollamaChatToolCallFunc `json:"function"`
}

// ollamaChatMessage Ollama chat 消息
type ollamaChatMessage struct {
	Role       string               `json:"role"`
	Content    string               `json:"content"`
	ToolCalls  []ollamaChatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
}

// ollamaToolFunction Ollama 工具函数定义
type ollamaToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ollamaTool Ollama 工具定义
type ollamaTool struct {
	Type     string             `json:"type"`
	Function ollamaToolFunction `json:"function"`
}

// ollamaChatRequest Ollama /api/chat 请求
type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Tools    []ollamaTool        `json:"tools,omitempty"`
	Stream   bool                `json:"stream"`
}

// ollamaChatResponseMessage Ollama chat 响应消息
type ollamaChatResponseMessage struct {
	Role      string               `json:"role"`
	Content   string               `json:"content"`
	ToolCalls []ollamaChatToolCall `json:"tool_calls,omitempty"`
}

// ollamaChatResponse Ollama /api/chat 响应
type ollamaChatResponse struct {
	Message         ollamaChatResponseMessage `json:"message"`
	Done            bool                      `json:"done"`
	PromptEvalCount int                       `json:"prompt_eval_count"`
	EvalCount       int                       `json:"eval_count"`
}

// GenerateWithTools 支持 Function Calling 的生成（使用 Ollama /api/chat 接口）
func (g *OllamaGenerator) GenerateWithTools(ctx context.Context, messages []model.Message, tools []model.ToolDefinition) (model.GenerateWithToolsResult, error) {
	var chatMessages []ollamaChatMessage
	for _, msg := range messages {
		cm := ollamaChatMessage{
			Role:    string(msg.Role),
			Content: msg.Content,
		}
		if msg.ToolCallID != "" {
			cm.ToolCallID = msg.ToolCallID
		}
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				var argsMap map[string]interface{}
				json.Unmarshal([]byte(tc.Arguments), &argsMap) //nolint:errcheck
				otc := ollamaChatToolCall{}
				otc.Function.Name = tc.Name
				otc.Function.Arguments = argsMap
				cm.ToolCalls = append(cm.ToolCalls, otc)
			}
		}
		chatMessages = append(chatMessages, cm)
	}

	// 转换工具定义
	var ollamaTools []ollamaTool
	for _, t := range tools {
		propsJSON, _ := json.Marshal(t.Parameters.Properties)
		var propsMap map[string]interface{}
		json.Unmarshal(propsJSON, &propsMap) //nolint:errcheck

		ollamaTools = append(ollamaTools, ollamaTool{
			Type: "function",
			Function: ollamaToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters: map[string]interface{}{
					"type":       t.Parameters.Type,
					"properties": propsMap,
					"required":   t.Parameters.Required,
				},
			},
		})
	}

	reqBody := ollamaChatRequest{
		Model:    g.modelName,
		Messages: chatMessages,
		Tools:    ollamaTools,
		Stream:   false,
	}

	requestBody, err := json.Marshal(reqBody)
	if err != nil {
		return model.GenerateWithToolsResult{}, fmt.Errorf("序列化请求失败: %v", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.ollamaURL+"/api/chat",
		bytes.NewBuffer(requestBody))
	if err != nil {
		return model.GenerateWithToolsResult{}, fmt.Errorf("创建请求失败: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return model.GenerateWithToolsResult{}, fmt.Errorf("Ollama工具调用请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return model.GenerateWithToolsResult{}, fmt.Errorf("读取响应失败: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return model.GenerateWithToolsResult{}, fmt.Errorf("Ollama API错误: %s - %s", resp.Status, string(body))
	}

	var chatResp ollamaChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return model.GenerateWithToolsResult{}, fmt.Errorf("解析响应失败: %v", err)
	}

	result := model.GenerateWithToolsResult{
		Content: chatResp.Message.Content,
		Usage: model.TokenUsage{
			PromptTokens:     chatResp.PromptEvalCount,
			CompletionTokens: chatResp.EvalCount,
			TotalTokens:      chatResp.PromptEvalCount + chatResp.EvalCount,
		},
	}

	// 解析工具调用（标准格式）
	for i, tc := range chatResp.Message.ToolCalls {
		argsJSON, _ := json.Marshal(tc.Function.Arguments)
		result.ToolCalls = append(result.ToolCalls, model.ToolCall{
			ID:        fmt.Sprintf("call_%d", i),
			Name:      tc.Function.Name,
			Arguments: string(argsJSON),
		})
	}

	// 兜底：本地模型有时不返回标准 tool_calls，而是把调用写在文本里
	// 尝试从文本内容中提取工具调用
	if len(result.ToolCalls) == 0 && result.Content != "" {
		toolNames := make([]string, 0, len(tools))
		for _, t := range tools {
			toolNames = append(toolNames, t.Name)
		}
		if extracted := extractToolCallsFromText(result.Content, toolNames); len(extracted) > 0 {
			result.ToolCalls = extracted
			result.Content = "" // 清空文本，让 ReAct 循环继续
		}
	}

	return result, nil
}

// 确保编译期间 time 包被使用（已在 defaultOllamaTimeout 中使用）
var _ = time.Second