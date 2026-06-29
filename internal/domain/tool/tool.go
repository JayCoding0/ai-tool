// Package tool 提供全局工具注册中心
// 管理所有可供 Agent 调用的工具定义和执行函数
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	domain_model "aiProject/internal/domain/model"
)

// ExecuteFunc 工具执行函数类型
type ExecuteFunc func(ctx context.Context, args map[string]interface{}) (string, error)

// Tool 工具定义（可注册到 Skill 的可执行工具）
type Tool struct {
	Definition      domain_model.ToolDefinition // 传给模型的工具描述
	Execute         ExecuteFunc                 // 实际执行函数
	DescriptionFunc func() string               // 可选：动态描述钩子，每次 GetDefinitions 时调用以获取最新描述
}

// Registry 工具注册中心（全局单例）
type Registry struct {
	mu    sync.RWMutex
	tools map[string]*Tool
}

var globalRegistry = &Registry{
	tools: make(map[string]*Tool),
}

// Register 注册一个工具
func Register(t *Tool) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.tools[t.Definition.Name] = t
}

// Get 获取工具
func Get(name string) (*Tool, bool) {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	t, ok := globalRegistry.tools[name]
	return t, ok
}

// Unregister 注销一个工具（用于动态卸载，如移除外部 MCP Server 的工具）
func Unregister(name string) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	delete(globalRegistry.tools, name)
}

// toolAliases 工具名别名映射（旧名/短名 → 注册名）
// 用于兼容数据库中存储的旧工具名
var toolAliases = map[string]string{
	"weather":      "get_weather",
	"public_ip":    "get_public_ip",
	"current_time": "get_current_time",
}

// resolveToolName 解析工具名，支持别名
func resolveToolName(name string) string {
	if canonical, ok := toolAliases[name]; ok {
		return canonical
	}
	return name
}

// GetDefinitions 获取指定名称列表的工具定义（用于传给模型）
// 若工具注册了 DescriptionFunc，则每次调用时动态生成最新描述
// 支持工具名别名（如 "weather" → "get_weather"）
func GetDefinitions(names []string) []domain_model.ToolDefinition {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	var defs []domain_model.ToolDefinition
	for _, name := range names {
		resolved := resolveToolName(name)
		if t, ok := globalRegistry.tools[resolved]; ok {
			def := t.Definition
			if t.DescriptionFunc != nil {
				def.Description = t.DescriptionFunc()
			}
			defs = append(defs, def)
		}
	}
	return defs
}

// Execute 执行工具调用
func Execute(ctx context.Context, call domain_model.ToolCall) (string, error) {
	globalRegistry.mu.RLock()
	resolved := resolveToolName(call.Name)
	t, ok := globalRegistry.tools[resolved]
	globalRegistry.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("工具 %q 未注册", call.Name)
	}

	// 解析参数
	var args map[string]interface{}
	if call.Arguments != "" {
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return "", fmt.Errorf("工具参数解析失败: %v", err)
		}
	}
	if args == nil {
		args = make(map[string]interface{})
	}

	return t.Execute(ctx, args)
}

// GetDisplayName 获取工具的展示名称，若未配置则返回工具名本身
func GetDisplayName(name string) string {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	if t, ok := globalRegistry.tools[name]; ok && t.Definition.DisplayName != "" {
		return t.Definition.DisplayName
	}
	return name
}

// ListAll 列出所有已注册工具的定义
func ListAll() []domain_model.ToolDefinition {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	var defs []domain_model.ToolDefinition
	for _, t := range globalRegistry.tools {
		defs = append(defs, t.Definition)
	}
	return defs
}
