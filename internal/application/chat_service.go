// Package application 应用服务层，编排领域对象完成业务用例
package application

import (
	"context"
	"strings"
	"sync"

	"aiProject/internal/domain/model"
	"aiProject/internal/domain/session"
	"aiProject/internal/domain/tool"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// msgPreview 截取消息前 N 个字符作为日志预览，避免日志过长
func msgPreview(msg string, maxLen int) string {
	if len(msg) > maxLen {
		return msg[:maxLen] + "..."
	}
	return msg
}

// ChatRequest 聊天请求值对象
type ChatRequest struct {
	Message         string
	SessionID       session.SessionID
	UserID          int64             // 关联用户ID，0表示未登录
	UserName        string            // 当前登录用户名（用于 Prompt 模板变量）
	ModelName       string            // 指定使用的模型名称，空则使用默认模型
	SystemPrompt    string            // 会话级别的 System Prompt，空则使用默认
	EnabledTools    []string          // 前端启用的工具名称列表，模型自主决定调用
	KnowledgeBaseID int64             // 关联的知识库 ID，0 表示不使用 RAG
	PromptVars      map[string]string // 请求级 Prompt 模板变量
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
	ParentToolCallID   string // 父级 call_agent 工具调用 ID（子 Agent 内部事件透传时携带，用于前端层级归属）
	SessionID          session.SessionID
	ModelName          string
	Usage              model.TokenUsage // done时携带
	SessionTotalTokens int              // done时携带
	Error              string           // error时携带
}

// ChatService 聊天应用服务
type ChatService struct {
	sessionRepo    session.Repository
	modelGen       model.Generator
	defaultModel   string
	modelFactory   func(modelName string) model.Generator
	genCache       sync.Map              // 模型生成器缓存，key=modelName value=model.Generator
	knowledgeSvc   *KnowledgeService     // RAG 知识库服务（可选）
	promptVarsSvc  *PromptVarsService    // Prompt 模板变量服务（可选）
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

// SetKnowledgeService 注入知识库服务（启用 RAG 能力）
func (s *ChatService) SetKnowledgeService(ks *KnowledgeService) {
	s.knowledgeSvc = ks
}

// SetPromptVarsService 注入 Prompt 模板变量服务
func (s *ChatService) SetPromptVarsService(pvs *PromptVarsService) {
	s.promptVarsSvc = pvs
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

	shared.GetLogger().Info("[流式聊天] 收到请求",
		zap.String("session_id", string(req.SessionID)),
		zap.String("model", modelName),
		zap.Int64("user_id", req.UserID),
		zap.Int("msg_len", len(req.Message)),
		zap.String("msg_preview", msgPreview(req.Message, 60)),
		zap.Bool("has_system_prompt", req.SystemPrompt != ""),
	)

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

	// 渲染 Prompt 模板变量
	systemPrompt = s.renderPromptTemplate(ctx, systemPrompt, req, "")

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
// ReAct 循环逻辑已抽取到 agentRunner（agent_runner.go）
func (s *ChatService) ProcessMessageWithTools(ctx context.Context, req ChatRequest, toolNames []string) (<-chan StreamChatResponse, error) {
	modelName := req.ModelName
	if modelName == "" {
		modelName = s.defaultModel
	}

	shared.GetLogger().Info("[工具聊天] 收到请求",
		zap.String("session_id", string(req.SessionID)),
		zap.String("model", modelName),
		zap.Int64("user_id", req.UserID),
		zap.Strings("enabled_tools", toolNames),
		zap.Int("msg_len", len(req.Message)),
		zap.String("msg_preview", msgPreview(req.Message, 60)),
		zap.Bool("has_system_prompt", req.SystemPrompt != ""),
		zap.Int64("kb_id", req.KnowledgeBaseID),
	)

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

	// 调试日志：工具解析结果
	{
		toolNames2 := make([]string, len(toolDefs))
		for i, d := range toolDefs {
			toolNames2[i] = d.Name
		}
		shared.GetLogger().Debug("[工具聊天] 工具解析完成",
			zap.String("session_id", string(sess.ID())),
			zap.Int("history_msgs", len(sess.GetHistory())),
			zap.Strings("requested_tools", toolNames),
			zap.Strings("resolved_tool_defs", toolNames2),
			zap.Int("tool_def_count", len(toolDefs)),
			zap.String("system_prompt_prefix", msgPreview(systemPrompt, 100)),
		)
	}

	// RAG：若会话绑定了知识库，先检索相关内容注入 System Prompt
	var ragContext string
	if req.KnowledgeBaseID > 0 && s.knowledgeSvc != nil {
		if chunks, searchErr := s.knowledgeSvc.Search(ctx, req.KnowledgeBaseID, req.Message, 5); searchErr == nil {
			ragContext = BuildRAGContext(chunks)
		} else {
			shared.GetLogger().Warn("RAG 检索失败", zap.Error(searchErr))
		}
	}

	// 渲染 Prompt 模板变量（RAG 上下文作为内置变量注入）
	systemPrompt = s.renderPromptTemplate(ctx, systemPrompt, req, ragContext)

	messages := buildMessagesWithRAG(sess.GetHistory(), systemPrompt, ragContext)
	isFirstMessage := len(sess.GetHistory()) <= 1

	outCh := make(chan StreamChatResponse, 64)
	go func() {
		defer close(outCh)

		runner := newAgentRunner(modelGen, modelName, sess.ID(), outCh)
		finalContent, totalUsage, runErr := runner.runReActLoop(ctx, messages, toolDefs)
		if runErr != nil {
			sendEvent(ctx, outCh, StreamChatResponse{Type: "error", Error: runErr.Error()})
			return
		}

		if finalContent == "" {
			finalContent = "（工具调用完成，但模型未生成最终回复）"
			sendEvent(ctx, outCh, StreamChatResponse{
				Type: "chunk", Content: finalContent,
				SessionID: sess.ID(), ModelName: modelName,
			})
		}

		sess.AddMessage("ai", finalContent)
		// 使用 Background context 保存，避免客户端断开后 ctx 取消导致保存失败
		saveCtx := context.Background()
		if err := s.sessionRepo.SaveMessageWithTokens(saveCtx, sess.ID(), "ai", finalContent, req.UserID, modelName, totalUsage); err != nil {
			shared.GetLogger().Error("保存AI回复失败", zap.Error(err))
		}
		if err := s.sessionRepo.Save(sess); err != nil {
			shared.GetLogger().Error("保存会话失败", zap.Error(err))
		}

		sessionTotal, _ := s.sessionRepo.GetSessionTotalTokens(saveCtx, sess.ID())
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
	}()

	return outCh, nil
}

// buildMessages 将会话历史转换为 model.Message 列表（统一用于流式和工具调用）
func buildMessages(history []session.Message, systemPrompt string) []model.Message {
	return buildMessagesWithRAG(history, systemPrompt, "")
}

// buildMessagesWithRAG 将会话历史转换为 model.Message 列表，支持 RAG 上下文注入
func buildMessagesWithRAG(history []session.Message, systemPrompt, ragContext string) []model.Message {
	if systemPrompt == "" {
		systemPrompt = "你是一个智能助手，请始终使用中文回答用户的问题。"
	}
	// 将 RAG 检索结果注入 System Prompt
	if ragContext != "" {
		systemPrompt += "\n\n## 参考知识库\n以下是与用户问题相关的知识库内容，请优先基于这些内容回答，并在回答末尾注明参考来源编号（如 [1][2]）：\n\n" + ragContext
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

// renderPromptTemplate 渲染 System Prompt 中的模板变量
// 合并优先级：内置变量 → 用户级变量（DB） → 会话级变量（DB） → 请求级变量
func (s *ChatService) renderPromptTemplate(ctx context.Context, systemPrompt string, req ChatRequest, ragContext string) string {
	if systemPrompt == "" {
		return systemPrompt
	}

	modelName := req.ModelName
	if modelName == "" {
		modelName = s.defaultModel
	}

	var pctx PromptContext
	if s.promptVarsSvc != nil {
		// 使用 PromptVarsService 构建完整上下文（自动合并用户级 + 会话级 + 请求级变量）
		pctx = s.promptVarsSvc.BuildPromptContext(ctx, req.UserID, req.UserName, string(req.SessionID), modelName, req.PromptVars)
	} else {
		// 无 PromptVarsService 时，仅使用内置变量和请求级变量
		pctx = PromptContext{
			UserName:   req.UserName,
			UserID:     req.UserID,
			SessionID:  string(req.SessionID),
			ModelName:  modelName,
			CustomVars: req.PromptVars,
		}
	}

	// 注入 RAG 上下文
	pctx.KnowledgeContext = ragContext

	return RenderPromptTemplate(systemPrompt, pctx)
}
