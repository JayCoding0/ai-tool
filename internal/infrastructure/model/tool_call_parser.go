package model

import (
	"encoding/json"
	"fmt"
	"strings"

	"aiProject/internal/domain/model"
)

// extractToolCallsFromText 从模型的文本输出中提取工具调用。
// 本地模型（如 qwen2.5:7b）有时会把工具调用写成文本而非标准 tool_calls 格式，例如：
//
//	query_database({"table": "orders"})
//	或 {"name": "query_database", "arguments": {"table": "orders"}}
func extractToolCallsFromText(content string, toolNames []string) []model.ToolCall {
	var calls []model.ToolCall
	callIdx := 0

	// 模式1：工具名(JSON参数)  例如：query_database({"table": "orders"})
	for _, name := range toolNames {
		pattern := name + "("
		start := strings.Index(content, pattern)
		if start == -1 {
			continue
		}
		argsStart := start + len(pattern)
		depth := 1
		end := -1
		for i := argsStart; i < len(content); i++ {
			switch content[i] {
			case '{', '(':
				depth++
			case '}', ')':
				depth--
				if depth == 0 {
					end = i
				}
			}
			if end != -1 {
				break
			}
		}
		if end == -1 {
			continue
		}
		argsStr := content[argsStart:end]
		var argsMap map[string]interface{}
		if err := json.Unmarshal([]byte(argsStr), &argsMap); err != nil {
			continue
		}
		calls = append(calls, model.ToolCall{
			ID:        fmt.Sprintf("call_%d", callIdx),
			Name:      name,
			Arguments: argsStr,
		})
		callIdx++
	}
	if len(calls) > 0 {
		return calls
	}

	// 模式2：JSON 对象格式  {"name": "xxx", "arguments": {...}}
	// 或 {"function": {"name": "xxx", "arguments": {...}}}
	decoder := json.NewDecoder(strings.NewReader(content))
	for {
		var raw map[string]interface{}
		if err := decoder.Decode(&raw); err != nil {
			break
		}
		name, args := "", map[string]interface{}{}

		if n, ok := raw["name"].(string); ok {
			name = n
			if a, ok := raw["arguments"].(map[string]interface{}); ok {
				args = a
			}
		}
		if fn, ok := raw["function"].(map[string]interface{}); ok {
			if n, ok := fn["name"].(string); ok {
				name = n
			}
			if a, ok := fn["arguments"].(map[string]interface{}); ok {
				args = a
			}
		}

		if name == "" {
			continue
		}
		valid := false
		for _, tn := range toolNames {
			if tn == name {
				valid = true
				break
			}
		}
		if !valid {
			continue
		}
		argsJSON, _ := json.Marshal(args)
		calls = append(calls, model.ToolCall{
			ID:        fmt.Sprintf("call_%d", callIdx),
			Name:      name,
			Arguments: string(argsJSON),
		})
		callIdx++
	}

	return calls
}
