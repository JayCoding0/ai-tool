// Package application 应用服务层
// workflow_engine_nodes.go — 节点执行器（从 workflow_engine.go 拆分）
package application

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	domain_model "aiProject/internal/domain/model"
	"aiProject/internal/domain/tool"
	"aiProject/internal/domain/workflow"
	"aiProject/internal/shared"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ─── 条件表达式评估 ──────────────────────────────────────────────────────────

// evaluateCondition 评估单个条件表达式
func (e *WorkflowEngine) evaluateCondition(cond workflow.ConditionBranch, execCtx *ExecutionContext) bool {
	// 空字段视为未配置条件，直接不命中，走默认分支
	if cond.Field == "" && cond.Operator != "is_empty" && cond.Operator != "is_not_empty" {
		return false
	}

	// 解析字段值（支持模板变量）
	fieldValue := e.resolveTemplate(cond.Field, execCtx)
	compareValue := e.resolveTemplate(cond.Value, execCtx)

	switch cond.Operator {
	case "==", "eq":
		return fieldValue == compareValue
	case "!=", "ne":
		return fieldValue != compareValue
	case ">", "gt":
		return compareNumbers(fieldValue, compareValue, func(a, b float64) bool { return a > b })
	case "<", "lt":
		return compareNumbers(fieldValue, compareValue, func(a, b float64) bool { return a < b })
	case ">=", "gte":
		return compareNumbers(fieldValue, compareValue, func(a, b float64) bool { return a >= b })
	case "<=", "lte":
		return compareNumbers(fieldValue, compareValue, func(a, b float64) bool { return a <= b })
	case "contains":
		return strings.Contains(fieldValue, compareValue)
	case "not_contains":
		return !strings.Contains(fieldValue, compareValue)
	case "is_empty":
		return strings.TrimSpace(fieldValue) == ""
	case "is_not_empty":
		return strings.TrimSpace(fieldValue) != ""
	case "starts_with":
		return strings.HasPrefix(fieldValue, compareValue)
	case "ends_with":
		return strings.HasSuffix(fieldValue, compareValue)
	default:
		shared.GetLogger().Warn("[Workflow] 未知的条件操作符",
			zap.String("operator", cond.Operator),
		)
		return false
	}
}

// compareNumbers 尝试将两个字符串解析为数字并比较
func compareNumbers(a, b string, cmp func(float64, float64) bool) bool {
	numA, errA := strconv.ParseFloat(strings.TrimSpace(a), 64)
	numB, errB := strconv.ParseFloat(strings.TrimSpace(b), 64)
	if errA != nil || errB != nil {
		// 无法解析为数字时，按字符串字典序比较
		return cmp(float64(strings.Compare(a, b)), 0)
	}
	return cmp(numA, numB)
}

// ─── 节点执行器 ──────────────────────────────────────────────────────────────

// executeNode 执行单个节点，返回节点输出
func (e *WorkflowEngine) executeNode(ctx context.Context, wf *workflow.Workflow, node workflow.Node, execCtx *ExecutionContext) (interface{}, error) {
	switch node.Type {
	case workflow.NodeTypeLLM:
		return e.executeLLMNode(ctx, node, execCtx)
	case workflow.NodeTypeTool:
		return e.executeToolNode(ctx, node, execCtx)
	case workflow.NodeTypeAgent:
		return e.executeAgentNode(ctx, node, execCtx)
	case workflow.NodeTypeTemplate:
		return e.executeTemplateNode(ctx, node, execCtx)
	case workflow.NodeTypeHTTP:
		return e.executeHTTPNode(ctx, node, execCtx)
	case workflow.NodeTypeCondition:
		return e.executeConditionNode(ctx, wf, node, execCtx)
	case workflow.NodeTypeParallel:
		return e.executeParallelNode(ctx, wf, node, execCtx)
	default:
		return nil, fmt.Errorf("不支持的节点类型: %s", node.Type)
	}
}

// executeConditionNode 执行条件分支节点
// 条件节点本身不产生业务输出，它的作用是评估条件并决定走哪个分支
// 实际的分支路由在 activateConditionDownstream 中处理
func (e *WorkflowEngine) executeConditionNode(ctx context.Context, wf *workflow.Workflow, node workflow.Node, execCtx *ExecutionContext) (interface{}, error) {
	logger := shared.GetLogger()

	if len(node.Config.Conditions) == 0 {
		return nil, fmt.Errorf("Condition 节点 %q 未配置条件分支", node.Name)
	}

	// 评估条件，找到命中的分支
	matchedBranch := ""
	for _, cond := range node.Config.Conditions {
		if cond.IsDefault {
			continue
		}
		if e.evaluateCondition(cond, execCtx) {
			matchedBranch = cond.ID
			break
		}
	}

	// 没有命中任何条件，使用默认分支
	if matchedBranch == "" {
		for _, cond := range node.Config.Conditions {
			if cond.IsDefault {
				matchedBranch = cond.ID
				break
			}
		}
	}

	if matchedBranch == "" {
		logger.Warn("[Workflow] Condition 节点无匹配分支且无默认分支",
			zap.String("node_id", node.ID),
		)
		matchedBranch = "none"
	}

	logger.Info("[Workflow] Condition 节点评估完成",
		zap.String("node_id", node.ID),
		zap.String("matched_branch", matchedBranch),
		zap.Int("total_conditions", len(node.Config.Conditions)),
	)

	// 返回命中的分支 ID 作为输出
	return map[string]interface{}{
		"matched_branch": matchedBranch,
	}, nil
}

// executeParallelNode 执行并行汇聚网关节点
// Parallel 节点作为汇聚点，收集所有上游并行分支的输出并合并
func (e *WorkflowEngine) executeParallelNode(ctx context.Context, wf *workflow.Workflow, node workflow.Node, execCtx *ExecutionContext) (interface{}, error) {
	logger := shared.GetLogger()

	// 获取所有上游节点
	upstreamIDs := wf.GetUpstreamNodes(node.ID)

	// 收集所有上游节点的输出
	mergedOutputs := make(map[string]interface{})
	for _, upID := range upstreamIDs {
		if output, ok := execCtx.GetNodeOutput(upID); ok {
			mergedOutputs[upID] = output
		}
	}

	logger.Info("[Workflow] Parallel 汇聚节点合并完成",
		zap.String("node_id", node.ID),
		zap.Int("upstream_count", len(upstreamIDs)),
		zap.Int("merged_count", len(mergedOutputs)),
	)

	return mergedOutputs, nil
}

// executeLLMNode 执行 LLM 对话节点
func (e *WorkflowEngine) executeLLMNode(ctx context.Context, node workflow.Node, execCtx *ExecutionContext) (interface{}, error) {
	logger := shared.GetLogger()

	modelName := node.Config.ModelName
	if modelName == "" {
		modelName = e.defaultModel
	}

	modelGen := e.modelFactory(modelName)
	if modelGen == nil {
		return nil, fmt.Errorf("无法创建模型生成器: %s", modelName)
	}

	// 渲染 System Prompt 中的变量引用
	systemPrompt := e.resolveTemplate(node.Config.SystemPrompt, execCtx)
	// 渲染 User Prompt 中的变量引用
	userPrompt := e.resolveTemplate(node.Config.UserPrompt, execCtx)

	if userPrompt == "" {
		// 如果没有配置 UserPrompt，尝试使用上游节点的输出作为用户消息
		upstreamOutput := e.getFirstUpstreamOutput(node, execCtx)
		if upstreamOutput != "" {
			userPrompt = upstreamOutput
		} else {
			return nil, fmt.Errorf("LLM 节点 %q 缺少用户输入（请配置 user_prompt 或连接上游节点）", node.Name)
		}
	}

	messages := []domain_model.Message{
		{Role: domain_model.RoleSystem, Content: systemPrompt},
		{Role: domain_model.RoleUser, Content: userPrompt},
	}

	logger.Info("[Workflow] LLM 节点调用",
		zap.String("node_id", node.ID),
		zap.String("model", modelName),
		zap.String("system_prompt_preview", msgPreview(systemPrompt, 80)),
		zap.String("user_prompt_preview", msgPreview(userPrompt, 80)),
	)

	result, err := modelGen.GenerateWithTools(ctx, messages, nil)
	if err != nil {
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	execCtx.AddTokens(result.Usage.TotalTokens)

	logger.Info("[Workflow] LLM 节点完成",
		zap.String("node_id", node.ID),
		zap.Int("content_len", len(result.Content)),
		zap.Int("tokens", result.Usage.TotalTokens),
	)

	return result.Content, nil
}

// executeToolNode 执行工具调用节点
func (e *WorkflowEngine) executeToolNode(ctx context.Context, node workflow.Node, execCtx *ExecutionContext) (interface{}, error) {
	logger := shared.GetLogger()

	toolName := node.Config.ToolName
	if toolName == "" {
		return nil, fmt.Errorf("Tool 节点 %q 未配置工具名称", node.Name)
	}

	// 构建工具参数
	args := make(map[string]interface{})
	for key, tmpl := range node.Config.ToolArgs {
		args[key] = e.resolveTemplate(tmpl, execCtx)
	}

	// 如果没有配置参数模板，尝试使用上游输出作为参数
	if len(args) == 0 {
		upstreamOutput := e.getFirstUpstreamOutput(node, execCtx)
		if upstreamOutput != "" {
			args["input"] = upstreamOutput
		}
	}

	argsJSON, _ := json.Marshal(args)

	logger.Info("[Workflow] Tool 节点调用",
		zap.String("node_id", node.ID),
		zap.String("tool_name", toolName),
		zap.String("args", string(argsJSON)),
	)

	toolCall := domain_model.ToolCall{
		ID:        fmt.Sprintf("wf_%s_%s", execCtx.RunID[:8], node.ID),
		Name:      toolName,
		Arguments: string(argsJSON),
	}

	result, err := tool.Execute(ctx, toolCall)
	if err != nil {
		return nil, fmt.Errorf("工具 %q 执行失败: %w", toolName, err)
	}

	logger.Info("[Workflow] Tool 节点完成",
		zap.String("node_id", node.ID),
		zap.String("tool_name", toolName),
		zap.Int("result_len", len(result)),
	)

	return result, nil
}

// executeAgentNode 执行子 Agent 节点
func (e *WorkflowEngine) executeAgentNode(ctx context.Context, node workflow.Node, execCtx *ExecutionContext) (interface{}, error) {
	logger := shared.GetLogger()

	if e.registry == nil {
		return nil, fmt.Errorf("Agent 注册中心未初始化")
	}

	agentName := node.Config.AgentName
	if agentName == "" {
		return nil, fmt.Errorf("Agent 节点 %q 未配置 agent_name", node.Name)
	}

	// 渲染发送给 Agent 的消息
	message := e.resolveTemplate(node.Config.AgentMessage, execCtx)
	if message == "" {
		upstreamOutput := e.getFirstUpstreamOutput(node, execCtx)
		if upstreamOutput != "" {
			message = upstreamOutput
		} else {
			return nil, fmt.Errorf("Agent 节点 %q 缺少消息内容", node.Name)
		}
	}

	// 为每个 Agent 节点生成独立的 sessionID（runID + nodeID），避免多个 Agent 节点共用同一 sessionID 导致主键冲突
	agentSessionID := uuid.NewSHA1(uuid.MustParse(execCtx.RunID), []byte(node.ID)).String()

	logger.Info("[Workflow] Agent 节点调用",
		zap.String("node_id", node.ID),
		zap.String("agent_name", agentName),
		zap.String("session_id", agentSessionID),
		zap.String("message_preview", msgPreview(message, 80)),
	)

	result, err := e.registry.CallSubAgent(agentName, message, agentSessionID, nil)
	if err != nil {
		return nil, fmt.Errorf("子 Agent %q 调用失败: %w", agentName, err)
	}

	return result, nil
}

// executeTemplateNode 执行模板转换节点
func (e *WorkflowEngine) executeTemplateNode(ctx context.Context, node workflow.Node, execCtx *ExecutionContext) (interface{}, error) {
	tmpl := node.Config.Template
	if tmpl == "" {
		return nil, fmt.Errorf("Template 节点 %q 未配置模板", node.Name)
	}

	result := e.resolveTemplate(tmpl, execCtx)
	return result, nil
}

// executeHTTPNode 执行 HTTP 请求节点
func (e *WorkflowEngine) executeHTTPNode(ctx context.Context, node workflow.Node, execCtx *ExecutionContext) (interface{}, error) {
	logger := shared.GetLogger()

	url := e.resolveTemplate(node.Config.URL, execCtx)
	if url == "" {
		return nil, fmt.Errorf("HTTP 节点 %q 未配置 URL", node.Name)
	}

	method := node.Config.Method
	if method == "" {
		method = "GET"
	}

	// 构建请求体
	var bodyReader io.Reader
	if node.Config.Body != "" {
		body := e.resolveTemplate(node.Config.Body, execCtx)
		bodyReader = strings.NewReader(body)
	}

	// 设置超时
	timeout := 30 * time.Second
	if node.Config.TimeoutSec > 0 {
		timeout = time.Duration(node.Config.TimeoutSec) * time.Second
	}
	httpCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(httpCtx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}

	// 设置请求头
	for key, val := range node.Config.Headers {
		req.Header.Set(key, e.resolveTemplate(val, execCtx))
	}
	if req.Header.Get("Content-Type") == "" && bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	logger.Info("[Workflow] HTTP 节点请求",
		zap.String("node_id", node.ID),
		zap.String("method", method),
		zap.String("url", url),
	)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 HTTP 响应失败: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP 请求返回错误状态码 %d: %s", resp.StatusCode, string(respBody))
	}

	return string(respBody), nil
}

// ─── 模板解析辅助方法 ──────────────────────────────────────────────────────────

// nodeRefRegex 匹配 ${node_id.output} 格式的节点输出引用
var nodeRefRegex = regexp.MustCompile(`\$\{(\w+)\.output\}`)

// varRefRegex 匹配 ${var_name} 格式的全局变量引用
var varRefRegex = regexp.MustCompile(`\$\{(\w+)\}`)

// resolveTemplate 解析模板字符串中的变量引用
// 支持两种引用格式：
//   - ${node_id.output} — 引用指定节点的输出
//   - ${var_name} — 引用全局变量或内置变量
func (e *WorkflowEngine) resolveTemplate(tmpl string, execCtx *ExecutionContext) string {
	if tmpl == "" {
		return ""
	}

	// 先替换节点输出引用 ${node_id.output}
	result := nodeRefRegex.ReplaceAllStringFunc(tmpl, func(match string) string {
		nodeID := match[2 : strings.Index(match, ".")]
		if output, ok := execCtx.GetNodeOutput(nodeID); ok {
			return fmt.Sprintf("%v", output)
		}
		return match // 未找到的引用保留原样
	})

	// 再替换全局变量引用 ${var_name}（排除已替换的节点引用）
	result = varRefRegex.ReplaceAllStringFunc(result, func(match string) string {
		varName := match[2 : len(match)-1]
		// 内置变量
		switch varName {
		case "current_time":
			return time.Now().Format("2006-01-02 15:04:05")
		case "current_date":
			return time.Now().Format("2006-01-02")
		}
		// 全局变量
		if val, ok := execCtx.Variables[varName]; ok {
			return fmt.Sprintf("%v", val)
		}
		return match
	})

	return result
}

// getFirstUpstreamOutput 获取节点的第一个上游节点的输出（字符串形式）
// 用于节点未显式配置输入时的自动推导
func (e *WorkflowEngine) getFirstUpstreamOutput(node workflow.Node, execCtx *ExecutionContext) string {
	// 先检查 InputMapping
	if len(node.Config.InputMapping) > 0 {
		for _, ref := range node.Config.InputMapping {
			resolved := e.resolveTemplate("${"+ref+"}", execCtx)
			if resolved != "" && resolved != "${"+ref+"}" {
				return resolved
			}
		}
	}

	// 没有 InputMapping，遍历所有已有输出，找到最近的上游
	execCtx.mu.Lock()
	defer execCtx.mu.Unlock()
	var lastOutput string
	for _, output := range execCtx.NodeOutputs {
		if output != nil {
			lastOutput = fmt.Sprintf("%v", output)
		}
	}
	return lastOutput
}
