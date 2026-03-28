package application

import (
	"context"
	"fmt"
	"sync"
	"time"

	"aiProject/internal/domain/session"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// AgentDefinition 定义一个 Agent 的配置
type AgentDefinition struct {
	// Agent 唯一名称（英文，用于 call_agent 工具调用）
	Name string
	// Agent 展示名称（中文，用于日志和前端展示）
	DisplayName string
	// Agent 职责描述（告知主 Agent 什么时候调用它）
	Description string
	// 该 Agent 使用的 System Prompt
	SystemPrompt string
	// 该 Agent 可以使用的工具名称列表（空表示不使用工具）
	EnabledTools []string
	// 该 Agent 使用的模型名称（空表示使用默认模型）
	ModelName string
	// 是否为主 Agent（主 Agent 负责编排，自动获得 call_agent 工具）
	IsMaster bool
}

// AgentRegistry Agent 注册中心，管理所有命名 Agent 实例
type AgentRegistry struct {
	mu     sync.RWMutex
	agents map[string]*AgentInstance
	master *AgentInstance
}

// AgentInstance 运行时 Agent 实例
type AgentInstance struct {
	Def         AgentDefinition
	ChatService *ChatService
}

var globalAgentRegistry = &AgentRegistry{
	agents: make(map[string]*AgentInstance),
}

// GetAgentRegistry 获取全局 Agent 注册中心
func GetAgentRegistry() *AgentRegistry {
	return globalAgentRegistry
}

// Register 注册一个 Agent 实例
func (r *AgentRegistry) Register(def AgentDefinition, chatService *ChatService) *AgentInstance {
	r.mu.Lock()
	defer r.mu.Unlock()

	inst := &AgentInstance{
		Def:         def,
		ChatService: chatService,
	}
	r.agents[def.Name] = inst
	if def.IsMaster {
		r.master = inst
	}
	return inst
}

// Get 按名称获取 Agent 实例
func (r *AgentRegistry) Get(name string) (*AgentInstance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	inst, ok := r.agents[name]
	return inst, ok
}

// GetMaster 获取主 Agent
func (r *AgentRegistry) GetMaster() (*AgentInstance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.master == nil {
		return nil, false
	}
	return r.master, true
}

// ListSubAgents 列出所有子 Agent（非主 Agent）
func (r *AgentRegistry) ListSubAgents() []*AgentInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var subs []*AgentInstance
	for _, inst := range r.agents {
		if !inst.Def.IsMaster {
			subs = append(subs, inst)
		}
	}
	return subs
}

// ListAll 列出所有 Agent
func (r *AgentRegistry) ListAll() []*AgentInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []*AgentInstance
	for _, inst := range r.agents {
		all = append(all, inst)
	}
	return all
}

// UpdateTools 动态更新指定 Agent 的工具列表（热更新，无需重启）
// 更新后会自动刷新 call_agent 工具的描述，确保主 Agent 感知到最新子 Agent 能力
func (r *AgentRegistry) UpdateTools(agentName string, tools []string) error {
	r.mu.Lock()
	inst, ok := r.agents[agentName]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("agent %q 未注册", agentName)
	}
	inst.Def.EnabledTools = tools
	// 收集当前所有子 Agent，用于刷新 call_agent 描述
	var subs []*AgentInstance
	for _, a := range r.agents {
		if !a.Def.IsMaster {
			subs = append(subs, a)
		}
	}
	r.mu.Unlock()

	// 重新注册 call_agent 工具，刷新子 Agent 列表描述
	RegisterCallAgentTool(subs)
	return nil
}

// SubAgentEventCallback 子 Agent 事件透传回调函数类型
// 主 Agent 可通过此回调实时接收子 Agent 的思考/工具调用过程
type SubAgentEventCallback func(event StreamChatResponse)

// CallSubAgent 调用指定子 Agent 执行任务，返回文本结果
// eventCallback: 可选，用于实时接收子 Agent 的 tool_call/tool_result/thought 事件（传 nil 则忽略）
func (r *AgentRegistry) CallSubAgent(agentName, message, sessionID string, eventCallback SubAgentEventCallback) (string, error) {
	logger := shared.GetLogger()
	inst, ok := r.Get(agentName)
	if !ok {
		logger.Warn("[CallSubAgent] 子 Agent 未注册", zap.String("agent", agentName))
		return "", fmt.Errorf("子 Agent %q 未注册", agentName)
	}

	logger.Info("[CallSubAgent] 开始调用子 Agent",
		zap.String("agent", agentName),
		zap.String("display_name", inst.Def.DisplayName),
		zap.String("session_id", sessionID),
		zap.Strings("enabled_tools", inst.Def.EnabledTools),
		zap.String("model", inst.Def.ModelName),
		zap.String("msg_preview", func() string {
			if len(message) > 60 {
				return message[:60] + "..."
			}
			return message
		}()),
	)
	start := time.Now()

	req := ChatRequest{
		Message:      message,
		SessionID:    session.SessionID(sessionID),
		SystemPrompt: inst.Def.SystemPrompt,
		ModelName:    inst.Def.ModelName,
	}

	var streamCh <-chan StreamChatResponse
	var err error

	if len(inst.Def.EnabledTools) > 0 {
		streamCh, err = inst.ChatService.ProcessMessageWithTools(
			context.Background(), req, inst.Def.EnabledTools,
		)
	} else {
		streamCh, err = inst.ChatService.ProcessMessageStream(context.Background(), req)
	}
	if err != nil {
		logger.Error("[CallSubAgent] 子 Agent 启动失败",
			zap.String("agent", agentName),
			zap.Error(err),
		)
		return "", fmt.Errorf("子 Agent %q 启动失败: %w", agentName, err)
	}

	// 收集子 Agent 的完整输出，并透传中间事件
	var result string
	for event := range streamCh {
		switch event.Type {
		case "chunk":
			result += event.Content
		case "thought", "tool_call", "tool_result":
			// 透传子 Agent 的思考/工具调用过程给调用方
			logger.Info("[CallSubAgent] 透传子 Agent 事件",
				zap.String("agent", agentName),
				zap.String("event_type", event.Type),
				zap.String("tool_name", event.ToolName),
				zap.String("tool_call_id", event.ToolCallID),
				zap.Bool("has_callback", eventCallback != nil),
			)
			if eventCallback != nil {
				eventCallback(event)
			}
		case "error":
			logger.Error("[CallSubAgent] 子 Agent 执行失败",
				zap.String("agent", agentName),
				zap.String("error", event.Error),
			)
			return result, fmt.Errorf("子 Agent %q 执行失败: %s", agentName, event.Error)
		}
	}

	logger.Info("[CallSubAgent] 子 Agent 执行完成",
		zap.String("agent", agentName),
		zap.Duration("duration", time.Since(start)),
		zap.Int("result_len", len(result)),
		zap.String("result_preview", func() string {
			if len(result) > 100 {
				return result[:100] + "..."
			}
			return result
		}()),
	)
	return result, nil
}
