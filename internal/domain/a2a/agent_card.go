package a2a

// AgentCard 是 A2A 协议中 Agent 的自我描述文档
// 通过 GET /.well-known/agent.json 对外暴露
type AgentCard struct {
	// 协议版本
	ProtocolVersion string `json:"protocolVersion"`
	// Agent 名称
	Name string `json:"name"`
	// Agent 描述
	Description string `json:"description"`
	// Agent 版本
	Version string `json:"version"`
	// Agent 服务地址（任务提交入口）
	URL string `json:"url"`
	// 支持的能力列表
	Capabilities AgentCapabilities `json:"capabilities"`
	// 技能列表（Agent 能做什么）
	Skills []AgentSkill `json:"skills"`
	// 联系信息（可选）
	Provider *AgentProvider `json:"provider,omitempty"`
}

// AgentCapabilities 描述 Agent 支持的协议能力
type AgentCapabilities struct {
	// 是否支持流式推送（SSE）
	Streaming bool `json:"streaming"`
	// 是否支持多轮对话
	MultiTurn bool `json:"multiTurn"`
	// 是否支持工具调用
	ToolCalling bool `json:"toolCalling"`
}

// AgentSkill 描述 Agent 的单项技能
type AgentSkill struct {
	// 技能唯一标识
	ID string `json:"id"`
	// 技能名称
	Name string `json:"name"`
	// 技能描述
	Description string `json:"description"`
	// 输入示例
	Examples []string `json:"examples,omitempty"`
	// 标签（用于分类）
	Tags []string `json:"tags,omitempty"`
}

// AgentProvider Agent 提供方信息
type AgentProvider struct {
	Organization string `json:"organization"`
	URL          string `json:"url,omitempty"`
}
