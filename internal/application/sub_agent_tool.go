package application

import (
	"context"
	"fmt"
	"strings"

	domain_model "aiProject/internal/domain/model"
	"aiProject/internal/domain/tool"
)

// subAgentContextKey context key 类型，避免与其他包冲突
type subAgentContextKey struct{}

// SubAgentEventCallbackKey 用于在 context 中传递子 Agent 事件回调的 key
// 主 Agent 的 agentRunner 在调用工具前将回调注入 context，
// call_agent 工具从 context 中取出并传给 CallSubAgent
var SubAgentEventCallbackKey = subAgentContextKey{}

// buildCallAgentDescription 动态构建 call_agent 工具描述（实时反映当前子 Agent 列表）
func buildCallAgentDescription() string {
	subs := GetAgentRegistry().ListSubAgents()
	var sb strings.Builder
	sb.WriteString("调用指定的子 Agent 完成专项任务。可用的子 Agent 列表：\n")
	for _, inst := range subs {
		sb.WriteString(fmt.Sprintf("- %s（%s）：%s\n",
			inst.Def.Name,
			inst.Def.DisplayName,
			inst.Def.Description,
		))
	}
	sb.WriteString("\n请根据任务类型选择最合适的子 Agent。")
	return sb.String()
}

// RegisterCallAgentTool 向全局工具注册中心注册 call_agent 工具
// 该工具让主 Agent 可以通过 LLM 工具调用的方式调用子 Agent
// subAgents: 子 Agent 列表，仅用于判断是否有子 Agent 可用（描述在运行时动态生成）
func RegisterCallAgentTool(subAgents []*AgentInstance) {
	if len(subAgents) == 0 {
		return
	}

	tool.Register(&tool.Tool{
		// DescriptionFunc 每次 GetDefinitions 时动态生成，确保主 Agent 始终看到最新子 Agent 列表
		DescriptionFunc: buildCallAgentDescription,
		Definition: domain_model.ToolDefinition{
			Name:        "call_agent",
			DisplayName: "调用子Agent",
			Description: buildCallAgentDescription(), // 初始值，后续由 DescriptionFunc 覆盖
			Parameters: domain_model.ToolParameters{
				Type: "object",
				Properties: map[string]domain_model.ToolParameterProperty{
					"agent_name": {
						Type:        "string",
						Description: "子 Agent 的名称（使用上方列表中的英文名称）",
					},
					"message": {
						Type:        "string",
						Description: "发送给子 Agent 的任务描述，要清晰具体",
					},
					"session_id": {
						Type:        "string",
						Description: "会话 ID（可选，用于子 Agent 保持上下文，留空则创建新会话）",
					},
				},
				Required: []string{"agent_name", "message"},
			},
		},
		Execute: func(ctx context.Context, args map[string]interface{}) (string, error) {
			agentName, _ := args["agent_name"].(string)
			message, _ := args["message"].(string)
			sessionID, _ := args["session_id"].(string)

			if agentName == "" {
				return "", fmt.Errorf("agent_name 不能为空")
			}
			if message == "" {
				return "", fmt.Errorf("message 不能为空")
			}

			// 从 context 中取出事件回调（由主 Agent 的 agentRunner 注入）
			var callback SubAgentEventCallback
			if cb, ok := ctx.Value(SubAgentEventCallbackKey).(SubAgentEventCallback); ok {
				callback = cb
			}

			result, err := GetAgentRegistry().CallSubAgent(agentName, message, sessionID, callback)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("[%s 的回复]\n%s", agentName, result), nil
		},
	})
}
