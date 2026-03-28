package application

import (
	"context"
	"fmt"
	"sync"

	"aiProject/internal/domain/model"
	"aiProject/internal/domain/session"
	"aiProject/internal/domain/tool"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// agentRunner ReAct 循环执行器，负责管理工具调用的多轮循环
type agentRunner struct {
	modelGen  model.Generator
	modelName string
	sessID    session.SessionID
	outCh     chan<- StreamChatResponse
}

// newAgentRunner 创建 ReAct 执行器
func newAgentRunner(modelGen model.Generator, modelName string, sessID session.SessionID, outCh chan<- StreamChatResponse) *agentRunner {
	return &agentRunner{
		modelGen:  modelGen,
		modelName: modelName,
		sessID:    sessID,
		outCh:     outCh,
	}
}

// runReActLoop 执行 ReAct 循环，返回最终 AI 回复内容和累计 token 用量
// 若 ctx 被取消或发生错误，返回已收集到的内容
func (r *agentRunner) runReActLoop(ctx context.Context, messages []model.Message, toolDefs []model.ToolDefinition) (finalContent string, totalUsage model.TokenUsage, err error) {
	for round := 0; round < maxToolCallRounds; round++ {
		// 调用模型（带工具定义）
		result, callErr := r.modelGen.GenerateWithTools(ctx, messages, toolDefs)
		if callErr != nil {
			return "", totalUsage, fmt.Errorf("模型调用失败: %w", callErr)
		}

		totalUsage.PromptTokens += result.Usage.PromptTokens
		totalUsage.CompletionTokens += result.Usage.CompletionTokens
		totalUsage.TotalTokens += result.Usage.TotalTokens

		shared.GetLogger().Info("ReAct轮次", zap.Int("round", round), zap.Int("tool_calls", len(result.ToolCalls)))

		if len(result.ToolCalls) == 0 {
			// 无工具调用：走流式接口逐字输出最终回复
			content, streamUsage, streamErr := r.streamFinalReply(ctx, messages)
			totalUsage.PromptTokens += streamUsage.PromptTokens
			totalUsage.CompletionTokens += streamUsage.CompletionTokens
			totalUsage.TotalTokens += streamUsage.TotalTokens
			if streamErr != nil {
				// 流式失败降级：使用 GenerateWithTools 已拿到的内容
				if result.Content != "" {
					if !sendEvent(ctx, r.outCh, StreamChatResponse{
						Type: "chunk", Content: result.Content,
						SessionID: r.sessID, ModelName: r.modelName,
					}) {
						return result.Content, totalUsage, nil
					}
				}
				return result.Content, totalUsage, nil
			}
			return content, totalUsage, nil
		}

		// 有工具调用：推送思考内容（若有）
		if result.Content != "" {
			if !sendEvent(ctx, r.outCh, StreamChatResponse{
				Type:      "thought",
				Content:   result.Content,
				Step:      round + 1,
				SessionID: r.sessID,
				ModelName: r.modelName,
			}) {
				return "", totalUsage, nil
			}
		}

		// 追加 assistant 消息（含工具调用）
		messages = append(messages, model.Message{
			Role:      model.RoleAssistant,
			Content:   result.Content,
			ToolCalls: result.ToolCalls,
		})

		// 推送所有 tool_call 事件
		for _, tc := range result.ToolCalls {
			if !sendEvent(ctx, r.outCh, StreamChatResponse{
				Type:            "tool_call",
				ToolName:        tc.Name,
				ToolDisplayName: tool.GetDisplayName(tc.Name),
				ToolCallID:      tc.ID,
				ToolArgs:        tc.Arguments,
				Step:            round + 1,
				SessionID:       r.sessID,
				ModelName:       r.modelName,
			}) {
				return "", totalUsage, nil
			}
		}

		// 并发执行所有工具
		execResults := r.executeToolsConcurrently(ctx, result.ToolCalls, round+1)

		// 追加工具结果消息
		for _, er := range execResults {
			messages = append(messages, model.Message{
				Role:       model.RoleTool,
				Content:    er.result,
				ToolCallID: er.tc.ID,
			})
		}
	}

	// 达到最大轮次：让 AI 总结进度
	return r.summarizeProgress(ctx, messages, &totalUsage)
}

// streamFinalReply 流式输出最终回复，返回完整内容和 token 用量
func (r *agentRunner) streamFinalReply(ctx context.Context, messages []model.Message) (string, model.TokenUsage, error) {
	var usage model.TokenUsage
	streamCh, err := r.modelGen.GenerateStreamWithMessages(ctx, messages)
	if err != nil {
		return "", usage, err
	}

	var content string
	for chunk := range streamCh {
		if chunk.Err != nil {
			return content, usage, chunk.Err
		}
		if chunk.Done {
			usage = chunk.Usage
			break
		}
		if chunk.Content != "" {
			content += chunk.Content
			if !sendEvent(ctx, r.outCh, StreamChatResponse{
				Type: "chunk", Content: chunk.Content,
				SessionID: r.sessID, ModelName: r.modelName,
			}) {
				return content, usage, nil
			}
		}
		if chunk.Thinking != "" {
			if !sendEvent(ctx, r.outCh, StreamChatResponse{
				Type: "chunk", Thinking: chunk.Thinking,
				SessionID: r.sessID, ModelName: r.modelName,
			}) {
				return content, usage, nil
			}
		}
	}
	return content, usage, nil
}

// toolExecResult 单个工具执行结果
type toolExecResult struct {
	tc     model.ToolCall
	result string
}

// executeToolsConcurrently 并发执行所有工具调用，按原始顺序返回结果并推送 tool_result 事件
func (r *agentRunner) executeToolsConcurrently(ctx context.Context, toolCalls []model.ToolCall, step int) []toolExecResult {
	results := make([]toolExecResult, len(toolCalls))
	var wg sync.WaitGroup

	// 将子 Agent 事件回调注入 context，使 call_agent 工具能透传子 Agent 的中间事件
	ctxWithCallback := context.WithValue(ctx, SubAgentEventCallbackKey, SubAgentEventCallback(func(event StreamChatResponse) {
		// 给子 Agent 事件加上当前步骤编号后透传到主 Agent 输出流
		event.Step = step
		sendEvent(ctx, r.outCh, event) //nolint:errcheck
	}))

	for i, tc := range toolCalls {
		wg.Add(1)
		go func(idx int, call model.ToolCall) {
			defer wg.Done()
			res, execErr := tool.Execute(ctxWithCallback, call)
			if execErr != nil {
				res = fmt.Sprintf("工具执行失败: %v", execErr)
			}
			shared.GetLogger().Info("工具调用完成", zap.String("tool", call.Name), zap.Int("result_len", len(res)))
			results[idx] = toolExecResult{tc: call, result: res}
		}(i, tc)
	}
	wg.Wait()

	// 按顺序推送 tool_result 事件
	for _, er := range results {
		sendEvent(ctx, r.outCh, StreamChatResponse{ //nolint:errcheck
			Type:            "tool_result",
			ToolName:        er.tc.Name,
			ToolDisplayName: tool.GetDisplayName(er.tc.Name),
			ToolCallID:      er.tc.ID,
			ToolArgs:        er.tc.Arguments,
			ToolResult:      er.result,
			Step:            step,
			SessionID:       r.sessID,
			ModelName:       r.modelName,
		})
	}
	return results
}

// summarizeProgress 达到最大轮次时，让 AI 总结已完成的进度
func (r *agentRunner) summarizeProgress(ctx context.Context, messages []model.Message, totalUsage *model.TokenUsage) (string, model.TokenUsage, error) {
	summaryHint := "你已经完成了多轮工具调用，但任务尚未完全结束。请用中文简要总结一下：\n" +
		"1. 你已经完成了哪些步骤和操作；\n" +
		"2. 当前进展到哪里；\n" +
		"3. 还剩下哪些步骤尚未完成。\n" +
		"最后告知用户：由于单次对话工具调用次数已达上限，请继续发送消息让你完成剩余任务。"

	summaryMessages := make([]model.Message, len(messages), len(messages)+1)
	copy(summaryMessages, messages)
	summaryMessages = append(summaryMessages, model.Message{
		Role:    model.RoleUser,
		Content: summaryHint,
	})

	content, streamUsage, err := r.streamFinalReply(ctx, summaryMessages)
	totalUsage.PromptTokens += streamUsage.PromptTokens
	totalUsage.CompletionTokens += streamUsage.CompletionTokens
	totalUsage.TotalTokens += streamUsage.TotalTokens

	if err != nil || content == "" {
		fallback := fmt.Sprintf("已完成 %d 轮工具调用，任务尚未结束。由于单次对话工具调用次数已达上限（%d 轮），请继续发送消息让我完成剩余任务。", maxToolCallRounds, maxToolCallRounds)
		if content == "" {
			content = fallback
			sendEvent(ctx, r.outCh, StreamChatResponse{ //nolint:errcheck
				Type: "chunk", Content: fallback,
				SessionID: r.sessID, ModelName: r.modelName,
			})
		}
	}
	return content, *totalUsage, nil
}
