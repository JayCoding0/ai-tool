package application

import (
	"context"
	"fmt"
	"sync"

	"aiProject/internal/domain/session"
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
	mu      sync.RWMutex
	agents  map[string]*AgentInstance
	master  *AgentInstance
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

// CallSubAgent 调用指定子 Agent 执行任务，返回文本结果
// 这是主 Agent 调用子 Agent 的核心方法
func (r *AgentRegistry) CallSubAgent(agentName, message, sessionID string) (string, error) {
	inst, ok := r.Get(agentName)
	if !ok {
		return "", fmt.Errorf("子 Agent %q 未注册", agentName)
	}

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
		return "", fmt.Errorf("子 Agent %q 启动失败: %w", agentName, err)
	}

	// 收集子 Agent 的完整输出
	var result string
	for event := range streamCh {
		switch event.Type {
		case "chunk":
			result += event.Content
		case "error":
			return result, fmt.Errorf("子 Agent %q 执行失败: %s", agentName, event.Error)
		}
	}
	return result, nil
}
