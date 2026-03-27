package application

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"aiProject/internal/domain/model"
	"aiProject/internal/domain/session"
	"aiProject/internal/domain/tool"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// ChatRequest 聊天请求值对象
type ChatRequest struct {
	Message      string
	SessionID    session.SessionID
	UserID       int64    // 关联用户ID，0表示未登录
	ModelName    string   // 指定使用的模型名称，空则使用默认模型
	SystemPrompt string   // 会话级别的 System Prompt，空则使用默认
	EnabledTools []string // 前端启用的工具名称列表，模型自主决定调用
}

// ChatResponse 聊天响应值对象
type ChatResponse struct {
	Response           string
	Thinking           string
	SessionID          session.SessionID
	ModelName          string           // 实际使用的模型名称
	TokenUsage         model.TokenUsage // 本次请求 token 用量
	SessionTotalTokens int              // 当前 session 累计 token 数
}

// StreamChatResponse 流式聊天响应事件
type StreamChatResponse struct {
	Type               string // "chunk" | "thought" | "tool_call" | "tool_result" | "done" | "error"
	Content            string // 增量内容（chunk时）
	Thinking           string // 思考过程增量（chunk时，可选）
	ToolName           string // 工具名称（tool_call/tool_result时）
	ToolDisplayName    string // 工具展示名称，优先使用中文（tool_call/tool_result时）
	ToolCallID         string // 工具调用 ID，用于精确匹配 tool_call 与 tool_result
	ToolArgs           string // 工具参数 JSON（tool_call时）
	ToolResult         string // 工具结果（tool_result时）
	Step               int    // ReAct 循环步骤编号
	SessionID          session.SessionID
	ModelName          string
	Usage              model.TokenUsage // done时携带
	SessionTotalTokens int              // done时携带
	Error              string           // error时携带
}

// ChatService 聊天应用服务
type ChatService struct {
	sessionRepo  session.Repository
	modelGen     model.Generator
	defaultModel string
	modelFactory func(modelName string) model.Generator
	genCache     sync.Map // 模型生成器缓存，key=modelName value=model.Generator
}

// NewChatService 创建聊天服务
func NewChatService(sessionRepo session.Repository, modelGen model.Generator) *ChatService {
	return &ChatService{
		sessionRepo:  sessionRepo,
		modelGen:     modelGen,
		defaultModel: "",
		modelFactory: nil,
	}
}

// NewChatServiceWithFactory 创建支持动态切换模型的聊天服务
func NewChatServiceWithFactory(sessionRepo session.Repository, defaultModelGen model.Generator, defaultModel string, factory func(string) model.Generator) *ChatService {
	return &ChatService{
		sessionRepo:  sessionRepo,
		modelGen:     defaultModelGen,
		defaultModel: defaultModel,
		modelFactory: factory,
	}
}

// getModelGen 获取模型生成器（带缓存）
func (s *ChatService) getModelGen(modelName string) model.Generator {
	if modelName == "" || s.modelFactory == nil {
		return s.modelGen
	}
	if cached, ok := s.genCache.Load(modelName); ok {
		return cached.(model.Generator)
	}
	gen := s.modelFactory(modelName)
	s.genCache.Store(modelName, gen)
	return gen
}

// sendEvent 向 channel 发送事件，支持 ctx 取消
func sendEvent(ctx context.Context, ch chan<- StreamChatResponse, event StreamChatResponse) bool {
	select {
	case ch <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

// ProcessMessageStream 流式处理聊天消息，通过 channel 逐块返回内容
func (s *ChatService) ProcessMessageStream(ctx context.Context, req ChatRequest) (<-chan StreamChatResponse, error) {
	modelName := req.ModelName
	if modelName == "" {
		modelName = s.defaultModel
	}
	modelGen := s.getModelGen(modelName)

	sess, err := s.getOrCreateSession(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}

	systemPrompt := req.SystemPrompt
	if systemPrompt == "" && req.SessionID != "" {
		systemPrompt, _ = s.sessionRepo.GetSessionSystemPrompt(ctx, sess.ID())
	}

	sess.AddMessage("user", req.Message)
	if err := s.sessionRepo.SaveMessageWithModel(ctx, sess.ID(), "user", req.Message, req.UserID, ""); err != nil {
		return nil, err
	}

	isFirstMessage := len(sess.GetHistory()) <= 1

	// 统一使用结构化 messages（支持多轮对话格式）
	messages := buildMessages(sess.GetHistory(), systemPrompt)

	// 流式生成（使用 chat 接口）
	streamCh, err := modelGen.GenerateStreamWithMessages(ctx, messages)
	if err != nil {
		return nil, err
	}

	outCh := make(chan StreamChatResponse, 64)
	go func() {
		defer close(outCh)

		var fullContent strings.Builder
		var usage model.TokenUsage

		for chunk := range streamCh {
			if chunk.Err != nil {
				sendEvent(ctx, outCh, StreamChatResponse{Type: "error", Error: chunk.Err.Error()})
				return
			}
			if chunk.Done {
				usage = chunk.Usage
				break
			}
			if chunk.Content != "" {
				fullContent.WriteString(chunk.Content)
				if !sendEvent(ctx, outCh, StreamChatResponse{
					Type:      "chunk",
					Content:   chunk.Content,
					SessionID: sess.ID(),
					ModelName: modelName,
				}) {
					return
				}
			}
			if chunk.Thinking != "" {
				if !sendEvent(ctx, outCh, StreamChatResponse{
					Type:      "chunk",
					Thinking:  chunk.Thinking,
					SessionID: sess.ID(),
					ModelName: modelName,
				}) {
					return
				}
			}
		}

		finalResponse := fullContent.String()
		sess.AddMessage("ai", finalResponse)

		if err := s.sessionRepo.SaveMessageWithTokens(ctx, sess.ID(), "ai", finalResponse, req.UserID, modelName, usage); err != nil {
			shared.GetLogger().Error("流式保存AI回复失败", zap.Error(err))
		}
		if err := s.sessionRepo.Save(sess); err != nil {
			shared.GetLogger().Error("流式保存会话失败", zap.Error(err))
		}

		sessionTotal, _ := s.sessionRepo.GetSessionTotalTokens(ctx, sess.ID())

		sendEvent(ctx, outCh, StreamChatResponse{
			Type:               "done",
			SessionID:          sess.ID(),
			ModelName:          modelName,
			Usage:              usage,
			SessionTotalTokens: sessionTotal,
		})

		if isFirstMessage && s.modelFactory != nil {
			go s.generateSessionTitle(context.Background(), sess.ID(), req.Message, modelName)
		}
	}()

	return outCh, nil
}

// generateSessionTitle 异步调用 AI 生成简短会话标题并更新数据库
func (s *ChatService) generateSessionTitle(ctx context.Context, sessID session.SessionID, userMessage, modelName string) {
	logger := shared.GetLogger()

	titlePrompt := "请根据以下用户消息，生成一个简短的对话标题（不超过15个汉字或30个字符，不要加引号、不要加标点符号结尾）：\n" + userMessage

	gen := s.getModelGen(modelName)
	result, err := gen.Generate(ctx, model.Prompt(titlePrompt))
	if err != nil {
		logger.Warn("生成会话标题失败", zap.String("session_id", string(sessID)), zap.Error(err))
		return
	}

	title := strings.TrimSpace(string(result.Response))
	title = strings.Trim(title, `"'""''`)
	runes := []rune(title)
	if len(runes) > 30 {
		title = string(runes[:30])
	}
	if title == "" {
		return
	}

	if err := s.sessionRepo.UpdateSessionTitle(ctx, sessID, title); err != nil {
		logger.Warn("更新会话标题失败", zap.String("session_id", string(sessID)), zap.Error(err))
		return
	}
	logger.Info("会话标题已生成", zap.String("session_id", string(sessID)), zap.String("title", title))
}

// GetSessionHistory 获取会话历史记录
func (s *ChatService) GetSessionHistory(ctx context.Context, sessionID session.SessionID) ([]session.Message, error) {
	return s.sessionRepo.GetSessionHistory(ctx, sessionID)
}

// ListSessions 列出所有会话
func (s *ChatService) ListSessions(ctx context.Context) ([]session.SessionInfo, error) {
	return s.sessionRepo.ListSessions(ctx)
}

// ListSessionsByUser 列出指定用户的会话
func (s *ChatService) ListSessionsByUser(ctx context.Context, userID int64) ([]session.SessionInfo, error) {
	return s.sessionRepo.ListSessionsByUser(ctx, userID)
}

// DeleteSession 删除会话
func (s *ChatService) DeleteSession(ctx context.Context, sessionID session.SessionID) error {
	return s.sessionRepo.DeleteSession(ctx, sessionID)
}

// RenameSession 重命名会话
func (s *ChatService) RenameSession(ctx context.Context, sessionID session.SessionID, title string) error {
	return s.sessionRepo.UpdateSessionTitle(ctx, sessionID, title)
}

// UpdateSessionSystemPrompt 更新会话的 System Prompt
func (s *ChatService) UpdateSessionSystemPrompt(ctx context.Context, sessionID session.SessionID, systemPrompt string) error {
	return s.sessionRepo.UpdateSessionSystemPrompt(ctx, sessionID, systemPrompt)
}

// GetSessionSystemPrompt 获取会话的 System Prompt
func (s *ChatService) GetSessionSystemPrompt(ctx context.Context, sessionID session.SessionID) (string, error) {
	return s.sessionRepo.GetSessionSystemPrompt(ctx, sessionID)
}

// GetModelTokenStats 获取各模型 token 消耗统计（userID=0 时统计所有用户）
func (s *ChatService) GetModelTokenStats(ctx context.Context, userID int64) ([]session.ModelTokenStat, error) {
	return s.sessionRepo.GetModelTokenStats(ctx, userID)
}

// GetUserTotalTokens 获取指定用户累计消耗的 token 总数
func (s *ChatService) GetUserTotalTokens(ctx context.Context, userID int64) (int, error) {
	return s.sessionRepo.GetUserTotalTokens(ctx, userID)
}

// getOrCreateSession 获取或创建会话
func (s *ChatService) getOrCreateSession(ctx context.Context, sessionID session.SessionID) (*session.Session, error) {
	if sessionID == "" {
		return session.NewSession(), nil
	}
	if s.sessionRepo.Exists(sessionID) {
		return s.sessionRepo.FindByID(sessionID)
	}
	return session.NewSessionWithID(sessionID), nil
}

// maxToolCallRounds Function Calling 最大循环轮次（防止无限循环）
const maxToolCallRounds = 5

// maxPromptMessages 单次请求最多传入的历史消息条数（滑动窗口）
const maxPromptMessages = 20

// ProcessMessageWithTools 支持 Function Calling 的聊天处理，支持流式推送工具调用过程
func (s *ChatService) ProcessMessageWithTools(ctx context.Context, req ChatRequest, toolNames []string) (<-chan StreamChatResponse, error) {
	modelName := req.ModelName
	if modelName == "" {
		modelName = s.defaultModel
	}
	modelGen := s.getModelGen(modelName)

	sess, err := s.getOrCreateSession(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}

	systemPrompt := req.SystemPrompt
	if systemPrompt == "" && req.SessionID != "" {
		systemPrompt, _ = s.sessionRepo.GetSessionSystemPrompt(ctx, sess.ID())
	}
	if systemPrompt == "" {
		systemPrompt = "你是一个智能助手，请始终使用中文回答用户的问题。"
	}

	sess.AddMessage("user", req.Message)
	if err := s.sessionRepo.SaveMessageWithModel(ctx, sess.ID(), "user", req.Message, req.UserID, ""); err != nil {
		return nil, err
	}

	toolDefs := tool.GetDefinitions(toolNames)
	messages := buildMessages(sess.GetHistory(), systemPrompt)

	isFirstMessage := len(sess.GetHistory()) <= 1

	outCh := make(chan StreamChatResponse, 64)
	go func() {
		defer close(outCh)

		var totalUsage model.TokenUsage

		for round := 0; round < maxToolCallRounds; round++ {
			result, err := modelGen.GenerateWithTools(ctx, messages, toolDefs)
			if err != nil {
				sendEvent(ctx, outCh, StreamChatResponse{Type: "error", Error: fmt.Sprintf("模型调用失败: %v", err)})
				return
			}

			totalUsage.PromptTokens += result.Usage.PromptTokens
			totalUsage.CompletionTokens += result.Usage.CompletionTokens
			totalUsage.TotalTokens += result.Usage.TotalTokens

			shared.GetLogger().Info("ReAct轮次", zap.Int("round", round), zap.Int("tool_calls", len(result.ToolCalls)))

			if len(result.ToolCalls) == 0 {
				// 没有工具调用（首轮或工具调用结束后），统一走流式接口逐字输出
				streamCh, streamErr := modelGen.GenerateStreamWithMessages(ctx, messages)
				if streamErr != nil {
					// 流式失败则降级为一次性推送（使用 GenerateWithTools 已拿到的结果）
					if result.Content != "" {
						sendEvent(ctx, outCh, StreamChatResponse{
							Type: "chunk", Content: result.Content,
							SessionID: sess.ID(), ModelName: modelName,
						})
					}
				} else {
					var streamContent strings.Builder
					for chunk := range streamCh {
						if chunk.Err != nil {
							sendEvent(ctx, outCh, StreamChatResponse{Type: "error", Error: chunk.Err.Error()})
							return
						}
						if chunk.Done {
							totalUsage.PromptTokens += chunk.Usage.PromptTokens
							totalUsage.CompletionTokens += chunk.Usage.CompletionTokens
							totalUsage.TotalTokens += chunk.Usage.TotalTokens
							break
						}
						if chunk.Content != "" {
							streamContent.WriteString(chunk.Content)
							if !sendEvent(ctx, outCh, StreamChatResponse{
								Type: "chunk", Content: chunk.Content,
								SessionID: sess.ID(), ModelName: modelName,
							}) {
								return
							}
						}
						if chunk.Thinking != "" {
							if !sendEvent(ctx, outCh, StreamChatResponse{
								Type: "chunk", Thinking: chunk.Thinking,
								SessionID: sess.ID(), ModelName: modelName,
							}) {
								return
							}
						}
					}
					result.Content = streamContent.String()
				}

				finalResponse := result.Content
				if finalResponse == "" {
					finalResponse = "（工具调用完成，但模型未生成最终回复）"
					sendEvent(ctx, outCh, StreamChatResponse{
						Type: "chunk", Content: finalResponse,
						SessionID: sess.ID(), ModelName: modelName,
					})
				}

				sess.AddMessage("ai", finalResponse)
				if err := s.sessionRepo.SaveMessageWithTokens(ctx, sess.ID(), "ai", finalResponse, req.UserID, modelName, totalUsage); err != nil {
					shared.GetLogger().Error("保存AI回复失败", zap.Error(err))
				}
				if err := s.sessionRepo.Save(sess); err != nil {
					shared.GetLogger().Error("保存会话失败", zap.Error(err))
				}

				sessionTotal, _ := s.sessionRepo.GetSessionTotalTokens(ctx, sess.ID())
				sendEvent(ctx, outCh, StreamChatResponse{
					Type:               "done",
					SessionID:          sess.ID(),
					ModelName:          modelName,
					Usage:              totalUsage,
					SessionTotalTokens: sessionTotal,
				})

				if isFirstMessage && s.modelFactory != nil {
					go s.generateSessionTitle(context.Background(), sess.ID(), req.Message, modelName)
				}
				return
			}

			// 有工具调用，继续下一轮

			// 如果 AI 在工具调用前有思考内容，推送 thought 事件
			if result.Content != "" {
				if !sendEvent(ctx, outCh, StreamChatResponse{
					Type:      "thought",
					Content:   result.Content,
					Step:      round + 1,
					SessionID: sess.ID(),
					ModelName: modelName,
				}) {
					return
				}
			}

			messages = append(messages, model.Message{
				Role:      model.RoleAssistant,
				Content:   result.Content,
				ToolCalls: result.ToolCalls,
			})

			// 先推送所有工具调用事件（告知前端即将并发执行哪些工具）
			for _, tc := range result.ToolCalls {
				if !sendEvent(ctx, outCh, StreamChatResponse{
					Type:            "tool_call",
					ToolName:        tc.Name,
					ToolDisplayName: tool.GetDisplayName(tc.Name),
					ToolCallID:      tc.ID,
					ToolArgs:        tc.Arguments,
					Step:            round + 1,
					SessionID:       sess.ID(),
					ModelName:       modelName,
				}) {
					return
				}
			}

			// 并发执行所有工具调用
			type toolExecResult struct {
				tc     model.ToolCall
				result string
			}
			execResults := make([]toolExecResult, len(result.ToolCalls))
			var wg sync.WaitGroup
			for i, tc := range result.ToolCalls {
				wg.Add(1)
				go func(idx int, call model.ToolCall) {
					defer wg.Done()
					res, execErr := tool.Execute(ctx, call)
					if execErr != nil {
						res = fmt.Sprintf("工具执行失败: %v", execErr)
					}
					shared.GetLogger().Info("工具调用完成", zap.String("tool", call.Name), zap.Int("result_len", len(res)))
					execResults[idx] = toolExecResult{tc: call, result: res}
				}(i, tc)
			}
			wg.Wait()

			// 按原始顺序推送工具结果事件并追加消息
			for _, er := range execResults {
				if !sendEvent(ctx, outCh, StreamChatResponse{
					Type:            "tool_result",
					ToolName:        er.tc.Name,
					ToolDisplayName: tool.GetDisplayName(er.tc.Name),
					ToolCallID:      er.tc.ID,
					ToolArgs:        er.tc.Arguments,
					ToolResult:      er.result,
					Step:            round + 1,
					SessionID:       sess.ID(),
					ModelName:       modelName,
				}) {
					return
				}
				messages = append(messages, model.Message{
					Role:       model.RoleTool,
					Content:    er.result,
					ToolCallID: er.tc.ID,
				})
			}
		}

		// 达到最大轮次仍未结束：先让 AI 总结已完成的内容，再提示用户继续对话
		summaryHint := "你已经完成了多轮工具调用，但任务尚未完全结束。请用中文简要总结一下：\n" +
			"1. 你已经完成了哪些步骤和操作；\n" +
			"2. 当前进展到哪里；\n" +
			"3. 还剩下哪些步骤尚未完成。\n" +
			"最后告知用户：由于单次对话工具调用次数已达上限，请继续发送消息让你完成剩余任务。"
		// 使用 copy 构造新切片，避免 append 复用底层数组污染 messages
		summaryMessages := make([]model.Message, len(messages), len(messages)+1)
		copy(summaryMessages, messages)
		summaryMessages = append(summaryMessages, model.Message{
			Role:    model.RoleUser,
			Content: summaryHint,
		})

		var finalResponse strings.Builder
		summaryCh, summaryErr := modelGen.GenerateStreamWithMessages(ctx, summaryMessages)
		if summaryErr != nil {
			// 流式调用失败，降级为固定提示
			fallback := fmt.Sprintf("已完成 %d 轮工具调用，任务尚未结束。由于单次对话工具调用次数已达上限（%d 轮），请继续发送消息让我完成剩余任务。", maxToolCallRounds, maxToolCallRounds)
			finalResponse.WriteString(fallback)
			sendEvent(ctx, outCh, StreamChatResponse{
				Type: "chunk", Content: fallback,
				SessionID: sess.ID(), ModelName: modelName,
			})
		} else {
			for chunk := range summaryCh {
				if chunk.Err != nil {
					// 流式中途出错：用已收到的内容兜底，不直接丢弃
					shared.GetLogger().Warn("总结流式出错", zap.Error(chunk.Err))
					if finalResponse.Len() == 0 {
						// 完全没有内容时补充固定提示
						fallback := fmt.Sprintf("已完成 %d 轮工具调用，任务尚未结束。请继续发送消息让我完成剩余任务。", maxToolCallRounds)
						finalResponse.WriteString(fallback)
						sendEvent(ctx, outCh, StreamChatResponse{
							Type: "chunk", Content: fallback,
							SessionID: sess.ID(), ModelName: modelName,
						})
					}
					break
				}
				if chunk.Done {
					totalUsage.PromptTokens += chunk.Usage.PromptTokens
					totalUsage.CompletionTokens += chunk.Usage.CompletionTokens
					totalUsage.TotalTokens += chunk.Usage.TotalTokens
					break
				}
				if chunk.Content != "" {
					finalResponse.WriteString(chunk.Content)
					// ctx 取消时先跳出循环，后续统一保存消息再退出
					if !sendEvent(ctx, outCh, StreamChatResponse{
						Type: "chunk", Content: chunk.Content,
						SessionID: sess.ID(), ModelName: modelName,
					}) {
						break
					}
				}
				if chunk.Thinking != "" {
					if !sendEvent(ctx, outCh, StreamChatResponse{
						Type: "chunk", Thinking: chunk.Thinking,
						SessionID: sess.ID(), ModelName: modelName,
					}) {
						break
					}
				}
			}
		}

		aiReply := finalResponse.String()
		if aiReply == "" {
			aiReply = fmt.Sprintf("已完成 %d 轮工具调用，任务尚未结束。请继续发送消息让我完成剩余任务。", maxToolCallRounds)
			sendEvent(ctx, outCh, StreamChatResponse{
				Type: "chunk", Content: aiReply,
				SessionID: sess.ID(), ModelName: modelName,
			})
		}

		sess.AddMessage("ai", aiReply)
		if err := s.sessionRepo.SaveMessageWithTokens(ctx, sess.ID(), "ai", aiReply, req.UserID, modelName, totalUsage); err != nil {
			shared.GetLogger().Error("保存AI回复失败", zap.Error(err))
		}
		if err := s.sessionRepo.Save(sess); err != nil {
			shared.GetLogger().Error("保存会话失败", zap.Error(err))
		}
		sessionTotal, _ := s.sessionRepo.GetSessionTotalTokens(ctx, sess.ID())
		sendEvent(ctx, outCh, StreamChatResponse{
			Type:               "done",
			SessionID:          sess.ID(),
			ModelName:          modelName,
			Usage:              totalUsage,
			SessionTotalTokens: sessionTotal,
		})
	}()

	return outCh, nil
}

// buildMessages 将会话历史转换为 model.Message 列表（统一用于流式和工具调用）
func buildMessages(history []session.Message, systemPrompt string) []model.Message {
	if systemPrompt == "" {
		systemPrompt = "你是一个智能助手，请始终使用中文回答用户的问题。"
	}
	messages := []model.Message{
		{Role: model.RoleSystem, Content: systemPrompt},
	}

	start := 0
	if len(history) > maxPromptMessages {
		start = len(history) - maxPromptMessages
	}
	for _, msg := range history[start:] {
		role := model.RoleUser
		if msg.Role == "ai" {
			role = model.RoleAssistant
		}
		messages = append(messages, model.Message{
			Role:    role,
			Content: msg.Content,
		})
	}
	return messages
}