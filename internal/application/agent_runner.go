// Package application 应用服务层，编排领域对象完成业务用例
package application

import (
	"context"
	"fmt"
	"sync"
	"time"

	"aiProject/internal/domain/model"
	"aiProject/internal/domain/session"
	"aiProject/internal/domain/tool"
	domain_trace "aiProject/internal/domain/trace"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// agentRunner ReAct 循环执行器，负责管理工具调用的多轮循环
type agentRunner struct {
	modelGen  model.Generator
	modelName string
	sessID    session.SessionID
	outCh     chan<- StreamChatResponse
	usedTool  bool // 本次循环是否实际发生过工具调用（用于语义缓存判定，纯文本回答才可缓存）
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
	logger := shared.GetLogger()
	toolDefNames := make([]string, len(toolDefs))
	for i, d := range toolDefs {
		toolDefNames[i] = d.Name
	}
	logger.Info("[ReAct] 循环开始",
		zap.String("session_id", string(r.sessID)),
		zap.String("model", r.modelName),
		zap.Int("msg_count", len(messages)),
		zap.Strings("tool_defs", toolDefNames),
		zap.Int("max_rounds", maxToolCallRounds),
	)

	for round := 0; round < maxToolCallRounds; round++ {
		// 调用模型（带工具定义）
		llmStart := time.Now()
		result, callErr := r.modelGen.GenerateWithTools(ctx, messages, toolDefs)
		if callErr != nil {
			if tr, ok := domain_trace.FromContext(ctx); ok {
				tr.AddSpan(domain_trace.Span{
					Name: fmt.Sprintf("LLM 工具决策 (round %d)", round+1), Type: domain_trace.SpanLLM,
					Step: round + 1, StartTime: llmStart, DurationMs: time.Since(llmStart).Milliseconds(),
					Error: callErr.Error(),
				})
			}
			logger.Error("[ReAct] 模型调用失败",
				zap.Int("round", round),
				zap.Error(callErr),
			)
			return "", totalUsage, fmt.Errorf("模型调用失败: %w", callErr)
		}

		totalUsage.PromptTokens += result.Usage.PromptTokens
		totalUsage.CompletionTokens += result.Usage.CompletionTokens
		totalUsage.TotalTokens += result.Usage.TotalTokens

		// 记录 LLM 决策轮 span
		if tr, ok := domain_trace.FromContext(ctx); ok {
			toolNames := make([]string, len(result.ToolCalls))
			for i, tc := range result.ToolCalls {
				toolNames[i] = tc.Name
			}
			output := result.Content
			if len(toolNames) > 0 {
				output = fmt.Sprintf("决定调用工具: %v\n%s", toolNames, result.Content)
			}
			tr.AddSpan(domain_trace.Span{
				Name: fmt.Sprintf("LLM 工具决策 (round %d)", round+1), Type: domain_trace.SpanLLM,
				Step: round + 1, StartTime: llmStart, DurationMs: time.Since(llmStart).Milliseconds(),
				Output: output, PromptTokens: result.Usage.PromptTokens,
				CompletionTokens: result.Usage.CompletionTokens, TotalTokens: result.Usage.TotalTokens,
			})
		}

		logger.Info("[ReAct] 轮次结果",
			zap.Int("round", round),
			zap.Int("tool_calls", len(result.ToolCalls)),
			zap.Int("content_len", len(result.Content)),
			zap.Int("prompt_tokens", result.Usage.PromptTokens),
			zap.Int("completion_tokens", result.Usage.CompletionTokens),
			zap.Int("total_tokens", result.Usage.TotalTokens),
		)

		if len(result.ToolCalls) == 0 {
			logger.Info("[ReAct] 无工具调用，进入流式输出",
				zap.Int("round", round),
				zap.Int("msg_count", len(messages)),
			)
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
		r.usedTool = true
		if result.Content != "" {
		logger.Debug("[ReAct] 模型思考过程",
				zap.Int("round", round),
				zap.String("thought_preview", msgPreview(result.Content, 80)),
			)
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
		logger.Info("[ReAct] 工具调用",
				zap.Int("round", round),
				zap.String("tool_name", tc.Name),
				zap.String("tool_call_id", tc.ID),
				zap.String("args_preview", msgPreview(tc.Arguments, 120)),
			)
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
	start := time.Now()
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
	// 记录流式回复 span
	if tr, ok := domain_trace.FromContext(ctx); ok {
		tr.AddSpan(domain_trace.Span{
			Name: "LLM 流式回复", Type: domain_trace.SpanLLM,
			StartTime: start, DurationMs: time.Since(start).Milliseconds(),
			Output: content, PromptTokens: usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens, TotalTokens: usage.TotalTokens,
		})
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

	for i, tc := range toolCalls {
		wg.Add(1)
		go func(idx int, call model.ToolCall) {
			defer wg.Done()
			start := time.Now()

			// 为每个工具调用单独注入回调，call_agent 工具可通过 ParentToolCallID 标识子 Agent 事件归属
			callCtx := context.WithValue(ctx, SubAgentEventCallbackKey, SubAgentEventCallback(func(event StreamChatResponse) {
				// 给子 Agent 事件加上当前步骤编号和父级 call_agent 的 tool_call_id，用于前端层级归属
				event.Step = step
				event.ParentToolCallID = call.ID
				shared.GetLogger().Info("[ReAct] 子Agent事件透传到主outCh",
					zap.String("event_type", event.Type),
					zap.String("tool_name", event.ToolName),
					zap.String("tool_call_id", event.ToolCallID),
					zap.String("parent_tool_call_id", event.ParentToolCallID),
				)
				sendEvent(ctx, r.outCh, event) //nolint:errcheck
			}))
			res, execErr := tool.Execute(callCtx, call)
			duration := time.Since(start)
			// 记录工具执行 span
			if tr, ok := domain_trace.FromContext(ctx); ok {
				span := domain_trace.Span{
					Name: call.Name, Type: domain_trace.SpanTool, Step: step,
					StartTime: start, DurationMs: duration.Milliseconds(),
					Input: call.Arguments, Output: res,
				}
				if execErr != nil {
					span.Error = execErr.Error()
				}
				tr.AddSpan(span)
			}
			if execErr != nil {
				shared.GetLogger().Error("[ReAct] 工具执行失败",
					zap.String("tool", call.Name),
					zap.String("tool_call_id", call.ID),
					zap.Duration("duration", duration),
					zap.Error(execErr),
				)
				res = fmt.Sprintf("工具执行失败: %v", execErr)
			} else {
				shared.GetLogger().Info("[ReAct] 工具执行完成",
					zap.String("tool", call.Name),
					zap.String("tool_call_id", call.ID),
					zap.Duration("duration", duration),
					zap.Int("result_len", len(res)),
					zap.String("result_preview", msgPreview(res, 120)),
				)
			}
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
