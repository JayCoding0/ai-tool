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
	Definition domain_model.ToolDefinition // 传给模型的工具描述
	Execute    ExecuteFunc                 // 实际执行函数
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

// GetDefinitions 获取指定名称列表的工具定义（用于传给模型）
func GetDefinitions(names []string) []domain_model.ToolDefinition {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	var defs []domain_model.ToolDefinition
	for _, name := range names {
		if t, ok := globalRegistry.tools[name]; ok {
			defs = append(defs, t.Definition)
		}
	}
	return defs
}

// Execute 执行工具调用
func Execute(ctx context.Context, call domain_model.ToolCall) (string, error) {
	globalRegistry.mu.RLock()
	t, ok := globalRegistry.tools[call.Name]
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
