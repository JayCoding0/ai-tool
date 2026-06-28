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
	"github.com/dop251/goja"
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
	case workflow.NodeTypeCode:
		return e.executeCodeNode(ctx, node, execCtx)
	case workflow.NodeTypeLoop:
		return e.executeLoopNode(ctx, node, execCtx)
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

	// 解析结构化输出配置（#24）
	opts := buildLLMGenerateOptions(node.Config)

	logger.Info("[Workflow] LLM 节点调用",
		zap.String("node_id", node.ID),
		zap.String("model", modelName),
		zap.String("system_prompt_preview", msgPreview(systemPrompt, 80)),
		zap.String("user_prompt_preview", msgPreview(userPrompt, 80)),
		zap.String("response_format", string(responseFormatType(opts))),
	)

	var result domain_model.GenerateWithToolsResult
	var err error
	// 需要高级选项（结构化输出 / 推理强度）且生成器支持时，走 opts 路径
	needsOpts := opts.ResponseFormat != nil || opts.ReasoningEffort != ""
	if sg, ok := modelGen.(domain_model.StructuredGenerator); ok && needsOpts {
		// 生成器原生支持高级选项（OpenAI 兼容接口）
		result, err = sg.GenerateWithToolsOpts(ctx, messages, nil, opts)
	} else {
		// 降级：生成器不支持结构化输出时，在 Prompt 中注入 JSON 指令
		if opts.ResponseFormat != nil {
			messages = injectJSONInstruction(messages, opts.ResponseFormat)
		}
		result, err = modelGen.GenerateWithTools(ctx, messages, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	execCtx.AddTokens(result.Usage.TotalTokens)

	content := result.Content
	// 结构化输出校验：要求 JSON 时，确保返回内容是合法 JSON（去除可能的 markdown 代码块包裹）
	if opts.ResponseFormat != nil && opts.ResponseFormat.Type != domain_model.ResponseFormatText {
		cleaned := extractJSONContent(content)
		var probe interface{}
		if jsonErr := json.Unmarshal([]byte(cleaned), &probe); jsonErr != nil {
			return nil, fmt.Errorf("LLM 节点 %q 要求结构化输出，但模型返回内容不是合法 JSON: %v", node.Name, jsonErr)
		}
		content = cleaned
	}

	logger.Info("[Workflow] LLM 节点完成",
		zap.String("node_id", node.ID),
		zap.Int("content_len", len(content)),
		zap.Int("tokens", result.Usage.TotalTokens),
	)

	return content, nil
}

// buildLLMGenerateOptions 从 LLM 节点配置构建生成选项（结构化输出 + 温度）
func buildLLMGenerateOptions(cfg workflow.NodeConfig) domain_model.GenerateOptions {
	opts := domain_model.GenerateOptions{}

	if cfg.Temperature > 0 {
		t := cfg.Temperature
		opts.Temperature = &t
	}

	// 推理模型思考强度
	switch strings.TrimSpace(cfg.ReasoningEffort) {
	case "low", "medium", "high":
		opts.ReasoningEffort = strings.TrimSpace(cfg.ReasoningEffort)
	}

	switch domain_model.ResponseFormatType(strings.TrimSpace(cfg.ResponseFormat)) {
	case domain_model.ResponseFormatJSONObject:
		opts.ResponseFormat = &domain_model.ResponseFormat{Type: domain_model.ResponseFormatJSONObject}
	case domain_model.ResponseFormatJSONSchema:
		rf := &domain_model.ResponseFormat{
			Type:       domain_model.ResponseFormatJSONSchema,
			SchemaName: "workflow_node_output",
			Strict:     cfg.SchemaStrict,
		}
		if s := strings.TrimSpace(cfg.JSONSchema); s != "" {
			var schema map[string]interface{}
			if err := json.Unmarshal([]byte(s), &schema); err == nil {
				rf.Schema = schema
			}
		}
		// 若 schema 解析失败或为空，降级为 json_object（仍保证合法 JSON）
		if rf.Schema == nil {
			opts.ResponseFormat = &domain_model.ResponseFormat{Type: domain_model.ResponseFormatJSONObject}
		} else {
			opts.ResponseFormat = rf
		}
	}

	return opts
}

// responseFormatType 返回选项中的输出格式类型（用于日志），未设置时返回 text
func responseFormatType(opts domain_model.GenerateOptions) domain_model.ResponseFormatType {
	if opts.ResponseFormat == nil {
		return domain_model.ResponseFormatText
	}
	return opts.ResponseFormat.Type
}

// injectJSONInstruction 为不支持原生 response_format 的生成器，在消息中注入 JSON 输出指令（降级方案）
func injectJSONInstruction(messages []domain_model.Message, rf *domain_model.ResponseFormat) []domain_model.Message {
	instruction := "\n\n[输出要求] 请只输出合法的 JSON，不要包含任何额外的解释文字，也不要使用 markdown 代码块包裹。"
	if rf.Type == domain_model.ResponseFormatJSONSchema && rf.Schema != nil {
		if schemaBytes, err := json.Marshal(rf.Schema); err == nil {
			instruction += "\n输出必须严格符合以下 JSON Schema：\n" + string(schemaBytes)
		}
	}
	// 追加到 System 消息；若没有 System 消息则插入一条
	for i := range messages {
		if messages[i].Role == domain_model.RoleSystem {
			messages[i].Content += instruction
			return messages
		}
	}
	return append([]domain_model.Message{{Role: domain_model.RoleSystem, Content: strings.TrimSpace(instruction)}}, messages...)
}

// extractJSONContent 从模型返回中提取 JSON 内容，去除可能的 ```json ... ``` markdown 代码块包裹
func extractJSONContent(content string) string {
	s := strings.TrimSpace(content)
	if strings.HasPrefix(s, "```") {
		// 去掉首行围栏（```json 或 ```）
		if idx := strings.IndexByte(s, '\n'); idx != -1 {
			s = s[idx+1:]
		}
		// 去掉结尾围栏
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
		s = strings.TrimSpace(s)
	}
	return s
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

	// 使用带 SSRF 防护的 HTTP 客户端，阻断指向内网/保留地址的请求
	resp, err := shared.SafeHTTPClient(timeout).Do(req)
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

// ─── Code 节点执行器（Phase 3：嵌入式 JS 沙箱）──────────────────────────────────

// executeCodeNode 执行代码节点（使用 goja JS 引擎，沙箱隔离）
// 用户编写的 JS 代码可以通过 inputs 对象读取上游节点输出，通过 return 返回结果
func (e *WorkflowEngine) executeCodeNode(ctx context.Context, node workflow.Node, execCtx *ExecutionContext) (interface{}, error) {
	logger := shared.GetLogger()

	code := node.Config.Code
	if code == "" {
		return nil, fmt.Errorf("Code 节点 %q 未配置代码", node.Name)
	}

	// 构建输入变量
	inputs := make(map[string]interface{})
	for varName, tmpl := range node.Config.CodeInputs {
		resolved := e.resolveTemplate(tmpl, execCtx)
		// 尝试解析为 JSON 对象，失败则保留为字符串
		var parsed interface{}
		if err := json.Unmarshal([]byte(resolved), &parsed); err == nil {
			inputs[varName] = parsed
		} else {
			inputs[varName] = resolved
		}
	}

	// 如果没有配置 CodeInputs，自动注入所有上游节点输出
	if len(inputs) == 0 {
		snapshot := execCtx.GetNodeOutputsSnapshot()
		for k, v := range snapshot {
			// 尝试将字符串类型的输出解析为 JSON
			if s, ok := v.(string); ok {
				var parsed interface{}
				if err := json.Unmarshal([]byte(s), &parsed); err == nil {
					inputs[k] = parsed
				} else {
					inputs[k] = s
				}
			} else {
				inputs[k] = v
			}
		}
	}

	logger.Info("[Workflow] Code 节点执行",
		zap.String("node_id", node.ID),
		zap.String("language", node.Config.CodeLanguage),
		zap.Int("code_len", len(code)),
		zap.Int("inputs_count", len(inputs)),
	)

	// 设置超时
	timeout := 10 * time.Second
	if node.Config.TimeoutSec > 0 {
		timeout = time.Duration(node.Config.TimeoutSec) * time.Second
	}

	// 在 goja 沙箱中执行 JS 代码
	result, err := e.runJavaScript(ctx, code, inputs, timeout)
	if err != nil {
		return nil, fmt.Errorf("Code 节点 %q 执行失败: %w", node.Name, err)
	}

	logger.Info("[Workflow] Code 节点完成",
		zap.String("node_id", node.ID),
	)

	return result, nil
}

// runJavaScript 在 goja 沙箱中执行 JavaScript 代码
// 提供 inputs 对象和 console.log 支持，限制执行时间
func (e *WorkflowEngine) runJavaScript(ctx context.Context, code string, inputs map[string]interface{}, timeout time.Duration) (interface{}, error) {
	vm := goja.New()

	// 注入 inputs 对象
	if err := vm.Set("inputs", inputs); err != nil {
		return nil, fmt.Errorf("注入 inputs 失败: %w", err)
	}

	// 注入 console.log（输出到日志）
	logger := shared.GetLogger()
	consoleObj := vm.NewObject()
	_ = consoleObj.Set("log", func(call goja.FunctionCall) goja.Value {
		args := make([]interface{}, len(call.Arguments))
		for i, arg := range call.Arguments {
			args[i] = arg.Export()
		}
		logger.Info("[Workflow] Code console.log", zap.Any("args", args))
		return goja.Undefined()
	})
	_ = vm.Set("console", consoleObj)

	// 注入 JSON 辅助（goja 内置支持 JSON.parse/JSON.stringify）

	// 将用户代码包装为立即执行函数（支持 return 语句）
	wrappedCode := fmt.Sprintf("(function() {\n%s\n})()", code)

	// 超时控制：启动定时器，超时后中断 VM
	timer := time.AfterFunc(timeout, func() {
		vm.Interrupt("代码执行超时（超过 " + timeout.String() + "）")
	})
	defer timer.Stop()

	// 执行代码
	val, err := vm.RunString(wrappedCode)
	if err != nil {
		return nil, fmt.Errorf("JS 执行错误: %w", err)
	}

	// 导出结果
	result := val.Export()

	// 将结果转为 JSON 友好格式
	switch v := result.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case map[string]interface{}, []interface{}:
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v), nil
		}
		return string(jsonBytes), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// ─── Loop 循环节点执行器（Phase 3）──────────────────────────────────────────────

// executeLoopNode 执行循环节点
// 支持两种模式：
//   - foreach：遍历列表，每次迭代执行循环体代码
//   - while：条件循环，每次迭代先检查条件再执行循环体
func (e *WorkflowEngine) executeLoopNode(ctx context.Context, node workflow.Node, execCtx *ExecutionContext) (interface{}, error) {
	logger := shared.GetLogger()

	loopType := node.Config.LoopType
	if loopType == "" {
		loopType = "foreach"
	}

	maxIter := node.Config.LoopMaxIter
	if maxIter <= 0 {
		maxIter = 100 // 默认最大迭代次数
	}

	// 设置超时
	timeout := 30 * time.Second
	if node.Config.TimeoutSec > 0 {
		timeout = time.Duration(node.Config.TimeoutSec) * time.Second
	}
	loopCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	logger.Info("[Workflow] Loop 节点开始",
		zap.String("node_id", node.ID),
		zap.String("loop_type", loopType),
		zap.Int("max_iter", maxIter),
	)

	var results []interface{}

	switch loopType {
	case "foreach":
		results, _ = e.executeForEachLoop(loopCtx, node, execCtx, maxIter)
	case "while":
		results, _ = e.executeWhileLoop(loopCtx, node, execCtx, maxIter)
	default:
		return nil, fmt.Errorf("Loop 节点 %q 不支持的循环类型: %s", node.Name, loopType)
	}

	logger.Info("[Workflow] Loop 节点完成",
		zap.String("node_id", node.ID),
		zap.Int("iterations", len(results)),
	)

	// 返回所有迭代结果的 JSON 数组
	jsonBytes, err := json.Marshal(results)
	if err != nil {
		return fmt.Sprintf("%v", results), nil
	}
	return string(jsonBytes), nil
}

// executeForEachLoop 执行 for-each 循环
func (e *WorkflowEngine) executeForEachLoop(ctx context.Context, node workflow.Node, execCtx *ExecutionContext, maxIter int) ([]interface{}, error) {
	logger := shared.GetLogger()

	// 解析要遍历的列表
	listStr := e.resolveTemplate(node.Config.LoopList, execCtx)
	if listStr == "" {
		return nil, fmt.Errorf("Loop 节点 %q 未配置遍历列表（loop_list）", node.Name)
	}

	// 尝试解析为 JSON 数组
	var list []interface{}
	if err := json.Unmarshal([]byte(listStr), &list); err != nil {
		// 如果不是 JSON 数组，按换行符分割为字符串列表
		lines := strings.Split(listStr, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				list = append(list, trimmed)
			}
		}
	}

	if len(list) == 0 {
		return []interface{}{}, nil
	}

	// 限制列表长度
	if len(list) > maxIter {
		logger.Warn("[Workflow] Loop 列表长度超过最大迭代次数，截断",
			zap.Int("list_len", len(list)),
			zap.Int("max_iter", maxIter),
		)
		list = list[:maxIter]
	}

	itemVar := node.Config.LoopItemVar
	if itemVar == "" {
		itemVar = "item"
	}
	indexVar := node.Config.LoopIndexVar
	if indexVar == "" {
		indexVar = "index"
	}

	bodyCode := node.Config.LoopBody
	if bodyCode == "" {
		// 没有循环体代码，直接返回列表本身
		return list, nil
	}

	// 设置超时
	timeout := 10 * time.Second
	if node.Config.TimeoutSec > 0 {
		timeout = time.Duration(node.Config.TimeoutSec) * time.Second
	}

	var results []interface{}
	for i, item := range list {
		select {
		case <-ctx.Done():
			return results, fmt.Errorf("循环被取消或超时")
		default:
		}

		// 构建本次迭代的输入
		inputs := make(map[string]interface{})
		// 注入所有上游节点输出
		snapshot := execCtx.GetNodeOutputsSnapshot()
		for k, v := range snapshot {
			if s, ok := v.(string); ok {
				var parsed interface{}
				if err := json.Unmarshal([]byte(s), &parsed); err == nil {
					inputs[k] = parsed
				} else {
					inputs[k] = s
				}
			} else {
				inputs[k] = v
			}
		}
		// 注入当前元素和索引
		inputs[itemVar] = item
		inputs[indexVar] = i
		// 注入之前的迭代结果
		inputs["results"] = results

		result, err := e.runJavaScript(ctx, bodyCode, inputs, timeout)
		if err != nil {
			logger.Warn("[Workflow] Loop 迭代执行失败",
				zap.String("node_id", node.ID),
				zap.Int("index", i),
				zap.Error(err),
			)
			results = append(results, map[string]interface{}{
				"error": err.Error(),
				"index": i,
			})
			continue
		}
		results = append(results, result)
	}

	return results, nil
}

// executeWhileLoop 执行 while 条件循环
func (e *WorkflowEngine) executeWhileLoop(ctx context.Context, node workflow.Node, execCtx *ExecutionContext, maxIter int) ([]interface{}, error) {
	logger := shared.GetLogger()

	conditionCode := node.Config.LoopCondition
	if conditionCode == "" {
		return nil, fmt.Errorf("Loop 节点 %q while 模式未配置条件表达式（loop_condition）", node.Name)
	}

	bodyCode := node.Config.LoopBody
	if bodyCode == "" {
		return nil, fmt.Errorf("Loop 节点 %q while 模式未配置循环体代码（loop_body）", node.Name)
	}

	// 设置超时
	timeout := 10 * time.Second
	if node.Config.TimeoutSec > 0 {
		timeout = time.Duration(node.Config.TimeoutSec) * time.Second
	}

	var results []interface{}
	for i := 0; i < maxIter; i++ {
		select {
		case <-ctx.Done():
			return results, fmt.Errorf("循环被取消或超时")
		default:
		}

		// 构建输入
		inputs := make(map[string]interface{})
		snapshot := execCtx.GetNodeOutputsSnapshot()
		for k, v := range snapshot {
			if s, ok := v.(string); ok {
				var parsed interface{}
				if err := json.Unmarshal([]byte(s), &parsed); err == nil {
					inputs[k] = parsed
				} else {
					inputs[k] = s
				}
			} else {
				inputs[k] = v
			}
		}
		inputs["index"] = i
		inputs["results"] = results

		// 评估条件
		condResult, err := e.runJavaScript(ctx, conditionCode, inputs, timeout)
		if err != nil {
			logger.Warn("[Workflow] Loop while 条件评估失败",
				zap.String("node_id", node.ID),
				zap.Int("iteration", i),
				zap.Error(err),
			)
			break
		}

		// 判断条件是否为真
		if !isTruthy(condResult) {
			logger.Info("[Workflow] Loop while 条件不满足，退出循环",
				zap.String("node_id", node.ID),
				zap.Int("iteration", i),
			)
			break
		}

		// 执行循环体
		result, err := e.runJavaScript(ctx, bodyCode, inputs, timeout)
		if err != nil {
			logger.Warn("[Workflow] Loop while 循环体执行失败",
				zap.String("node_id", node.ID),
				zap.Int("iteration", i),
				zap.Error(err),
			)
			results = append(results, map[string]interface{}{
				"error": err.Error(),
				"index": i,
			})
			break
		}
		results = append(results, result)

		// 将本次迭代结果也存入执行上下文（供条件表达式引用）
		execCtx.SetNodeOutput(node.ID+"_iter", result)
	}

	return results, nil
}

// isTruthy 判断一个值是否为"真"（JS 风格的 truthy 判断）
func isTruthy(val interface{}) bool {
	if val == nil {
		return false
	}
	switch v := val.(type) {
	case bool:
		return v
	case string:
		return v != "" && v != "false" && v != "0" && v != "null" && v != "undefined"
	case float64:
		return v != 0
	case int:
		return v != 0
	case int64:
		return v != 0
	default:
		return true
	}
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
