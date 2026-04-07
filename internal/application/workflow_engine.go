// Package application 应用服务层，编排领域对象完成业务用例
package application

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	domain_model "aiProject/internal/domain/model"
	"aiProject/internal/domain/tool"
	"aiProject/internal/domain/workflow"
	"aiProject/internal/shared"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ─── 执行事件定义 ──────────────────────────────────────────────────────────────

// WorkflowEvent 工作流执行事件（推送给前端）
type WorkflowEvent struct {
	Type       string      `json:"type"`                 // "node_start" | "node_output" | "node_error" | "node_done" | "workflow_done" | "workflow_error"
	NodeID     string      `json:"node_id,omitempty"`
	NodeName   string      `json:"node_name,omitempty"`
	NodeType   string      `json:"node_type,omitempty"`
	Output     interface{} `json:"output,omitempty"`
	Error      string      `json:"error,omitempty"`
	DurationMs int64       `json:"duration_ms,omitempty"`
	RunID      string      `json:"run_id,omitempty"`
	TotalTokens int        `json:"total_tokens,omitempty"`
}

// ─── 执行上下文 ──────────────────────────────────────────────────────────────

// ExecutionContext 工作流执行上下文
type ExecutionContext struct {
	WorkflowID  int64
	RunID       string
	Variables   map[string]interface{} // 全局变量（用户输入 + 默认值）
	NodeOutputs map[string]interface{} // 各节点输出：nodeID → output
	TotalTokens int                    // 累计 token 消耗
}

// ─── 工作流执行引擎 ──────────────────────────────────────────────────────────

// WorkflowEngine DAG 工作流执行引擎
type WorkflowEngine struct {
	workflowRepo workflow.Repository
	runRepo      workflow.RunRepository
	modelFactory func(string) domain_model.Generator
	defaultModel string
	registry     *AgentRegistry
}

// NewWorkflowEngine 创建工作流执行引擎
func NewWorkflowEngine(
	workflowRepo workflow.Repository,
	runRepo workflow.RunRepository,
	modelFactory func(string) domain_model.Generator,
	defaultModel string,
	registry *AgentRegistry,
) *WorkflowEngine {
	return &WorkflowEngine{
		workflowRepo: workflowRepo,
		runRepo:      runRepo,
		modelFactory: modelFactory,
		defaultModel: defaultModel,
		registry:     registry,
	}
}

// Execute 执行工作流，返回事件 channel（SSE 流式推送）
func (e *WorkflowEngine) Execute(ctx context.Context, workflowID int64, inputs map[string]interface{}, userID int64) (<-chan WorkflowEvent, error) {
	logger := shared.GetLogger()

	// 1. 加载 Workflow 定义
	wf, err := e.workflowRepo.GetByID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("加载工作流失败: %w", err)
	}

	// 2. 校验
	if err := wf.Validate(); err != nil {
		return nil, fmt.Errorf("工作流校验失败: %w", err)
	}

	// 3. 拓扑排序
	sortedNodeIDs, err := wf.TopologicalSort()
	if err != nil {
		return nil, fmt.Errorf("拓扑排序失败: %w", err)
	}

	// 4. 初始化执行上下文
	runID := uuid.New().String()
	execCtx := &ExecutionContext{
		WorkflowID:  workflowID,
		RunID:       runID,
		Variables:   make(map[string]interface{}),
		NodeOutputs: make(map[string]interface{}),
	}

	// 合并默认变量和用户输入
	for _, v := range wf.Variables {
		if v.DefaultValue != "" {
			execCtx.Variables[v.Name] = v.DefaultValue
		}
	}
	for k, v := range inputs {
		execCtx.Variables[k] = v
	}

	// 5. 保存执行记录
	run := &workflow.WorkflowRun{
		WorkflowID: workflowID,
		RunID:      runID,
		Status:     workflow.RunStatusRunning,
		Inputs:     inputs,
		UserID:     userID,
	}
	if saveErr := e.runRepo.Save(ctx, run); saveErr != nil {
		logger.Warn("保存执行记录失败（不影响执行）", zap.Error(saveErr))
	}

	logger.Info("[Workflow] 开始执行",
		zap.Int64("workflow_id", workflowID),
		zap.String("run_id", runID),
		zap.String("name", wf.Name),
		zap.Int("node_count", len(wf.Nodes)),
		zap.Strings("topo_order", sortedNodeIDs),
	)

	// 6. 启动异步执行
	outCh := make(chan WorkflowEvent, 64)
	go func() {
		defer close(outCh)
		startTime := time.Now()

		var lastErr error
		for _, nodeID := range sortedNodeIDs {
			node, ok := wf.GetNodeByID(nodeID)
			if !ok {
				continue
			}

			// 跳过 start 和 end 节点的实际执行
			if node.Type == workflow.NodeTypeStart {
				// start 节点：将用户输入作为输出
				execCtx.NodeOutputs[nodeID] = inputs
				sendWorkflowEvent(ctx, outCh, WorkflowEvent{
					Type:     "node_done",
					NodeID:   nodeID,
					NodeName: node.Name,
					NodeType: string(node.Type),
					RunID:    runID,
				})
				continue
			}
			if node.Type == workflow.NodeTypeEnd {
				// end 节点：收集最终输出
				sendWorkflowEvent(ctx, outCh, WorkflowEvent{
					Type:     "node_done",
					NodeID:   nodeID,
					NodeName: node.Name,
					NodeType: string(node.Type),
					RunID:    runID,
				})
				continue
			}

			// 推送节点开始事件
			sendWorkflowEvent(ctx, outCh, WorkflowEvent{
				Type:     "node_start",
				NodeID:   nodeID,
				NodeName: node.Name,
				NodeType: string(node.Type),
				RunID:    runID,
			})

			// 执行节点
			nodeStart := time.Now()
			output, execErr := e.executeNode(ctx, *node, execCtx)
			duration := time.Since(nodeStart).Milliseconds()

			if execErr != nil {
				lastErr = execErr
				logger.Error("[Workflow] 节点执行失败",
					zap.String("node_id", nodeID),
					zap.String("node_name", node.Name),
					zap.Error(execErr),
				)
				sendWorkflowEvent(ctx, outCh, WorkflowEvent{
					Type:       "node_error",
					NodeID:     nodeID,
					NodeName:   node.Name,
					NodeType:   string(node.Type),
					Error:      execErr.Error(),
					DurationMs: duration,
					RunID:      runID,
				})
				break // 节点失败，终止执行
			}

			// 存储节点输出
			execCtx.NodeOutputs[nodeID] = output

			logger.Info("[Workflow] 节点执行完成",
				zap.String("node_id", nodeID),
				zap.String("node_name", node.Name),
				zap.Int64("duration_ms", duration),
			)

			sendWorkflowEvent(ctx, outCh, WorkflowEvent{
				Type:       "node_output",
				NodeID:     nodeID,
				NodeName:   node.Name,
				NodeType:   string(node.Type),
				Output:     output,
				DurationMs: duration,
				RunID:      runID,
			})
		}

		totalDuration := time.Since(startTime).Milliseconds()

		// 更新执行记录
		if lastErr != nil {
			_ = e.runRepo.UpdateStatus(context.Background(), runID, workflow.RunStatusFailed,
				nil, execCtx.NodeOutputs, execCtx.TotalTokens, totalDuration, lastErr.Error())
			sendWorkflowEvent(ctx, outCh, WorkflowEvent{
				Type:        "workflow_error",
				Error:       lastErr.Error(),
				RunID:       runID,
				DurationMs:  totalDuration,
				TotalTokens: execCtx.TotalTokens,
			})
		} else {
			// 收集最终输出（取最后一个非 end 节点的输出）
			finalOutputs := execCtx.NodeOutputs
			_ = e.runRepo.UpdateStatus(context.Background(), runID, workflow.RunStatusCompleted,
				finalOutputs, execCtx.NodeOutputs, execCtx.TotalTokens, totalDuration, "")
			sendWorkflowEvent(ctx, outCh, WorkflowEvent{
				Type:        "workflow_done",
				Output:      finalOutputs,
				RunID:       runID,
				DurationMs:  totalDuration,
				TotalTokens: execCtx.TotalTokens,
			})
		}

		logger.Info("[Workflow] 执行完成",
			zap.String("run_id", runID),
			zap.Int64("duration_ms", totalDuration),
			zap.Int("total_tokens", execCtx.TotalTokens),
			zap.Bool("success", lastErr == nil),
		)
	}()

	return outCh, nil
}

// ─── 节点执行器 ──────────────────────────────────────────────────────────────

// executeNode 执行单个节点，返回节点输出
func (e *WorkflowEngine) executeNode(ctx context.Context, node workflow.Node, execCtx *ExecutionContext) (interface{}, error) {
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
	default:
		return nil, fmt.Errorf("不支持的节点类型: %s", node.Type)
	}
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

	execCtx.TotalTokens += result.Usage.TotalTokens

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

	logger.Info("[Workflow] Agent 节点调用",
		zap.String("node_id", node.ID),
		zap.String("agent_name", agentName),
		zap.String("message_preview", msgPreview(message, 80)),
	)

	// 使用工作流的 runID 作为 sessionID，保持上下文
	result, err := e.registry.CallSubAgent(agentName, message, execCtx.RunID, nil)
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

// ─── 辅助方法 ──────────────────────────────────────────────────────────────

// nodeRefRegex 匹配 {{node_id.output}} 格式的节点输出引用
var nodeRefRegex = regexp.MustCompile(`\{\{(\w+)\.output\}\}`)

// varRefRegex 匹配 {{var_name}} 格式的全局变量引用
var varRefRegex = regexp.MustCompile(`\{\{(\w+)\}\}`)

// resolveTemplate 解析模板字符串中的变量引用
// 支持两种引用格式：
//   - {{node_id.output}} — 引用指定节点的输出
//   - {{var_name}} — 引用全局变量或内置变量
func (e *WorkflowEngine) resolveTemplate(tmpl string, execCtx *ExecutionContext) string {
	if tmpl == "" {
		return ""
	}

	// 先替换节点输出引用 {{node_id.output}}
	result := nodeRefRegex.ReplaceAllStringFunc(tmpl, func(match string) string {
		nodeID := match[2 : strings.Index(match, ".")]
		if output, ok := execCtx.NodeOutputs[nodeID]; ok {
			return fmt.Sprintf("%v", output)
		}
		return match // 未找到的引用保留原样
	})

	// 再替换全局变量引用 {{var_name}}（排除已替换的节点引用）
	result = varRefRegex.ReplaceAllStringFunc(result, func(match string) string {
		varName := match[2 : len(match)-2]
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
			resolved := e.resolveTemplate("{{"+ref+"}}", execCtx)
			if resolved != "" && resolved != "{{"+ref+"}}" {
				return resolved
			}
		}
	}

	// 没有 InputMapping，遍历所有已有输出，找到最近的上游
	// 这里简单地返回 NodeOutputs 中最后一个非空输出
	// 实际应该根据 Edge 关系查找，但需要 Workflow 引用
	// Phase 1 简化处理：按 NodeOutputs 的插入顺序取最后一个
	var lastOutput string
	for _, output := range execCtx.NodeOutputs {
		if output != nil {
			lastOutput = fmt.Sprintf("%v", output)
		}
	}
	return lastOutput
}

// sendWorkflowEvent 向 channel 发送工作流事件
func sendWorkflowEvent(ctx context.Context, ch chan<- WorkflowEvent, event WorkflowEvent) bool {
	select {
	case ch <- event:
		return true
	case <-ctx.Done():
		return false
	}
}
