// Package application 应用服务层，编排领域对象完成业务用例
package application

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// PromptVariable 模板变量定义（供前端展示可用变量列表）
type PromptVariable struct {
	Name        string `json:"name"`        // 变量名，如 "current_time"
	Description string `json:"description"` // 变量描述，用于前端展示
	Example     string `json:"example"`     // 示例值
	Scope       string `json:"scope"`       // 变量作用域："builtin" | "user" | "session" | "request"
}

// BuiltinVariables 内置变量列表（供前端展示可用变量）
var BuiltinVariables = []PromptVariable{
	{Name: "current_time", Description: "当前日期时间（格式：2006-01-02 15:04:05）", Example: "2026-03-30 11:30:00", Scope: "builtin"},
	{Name: "current_date", Description: "当前日期（格式：2006-01-02）", Example: "2026-03-30", Scope: "builtin"},
	{Name: "user_name", Description: "当前登录用户名", Example: "张三", Scope: "builtin"},
	{Name: "user_id", Description: "当前用户ID", Example: "42", Scope: "builtin"},
	{Name: "session_id", Description: "当前会话ID", Example: "abc-123", Scope: "builtin"},
	{Name: "model_name", Description: "当前使用的模型名称", Example: "qwen-plus", Scope: "builtin"},
	{Name: "knowledge_context", Description: "RAG 知识库检索结果（自动注入）", Example: "[1] 文档内容...", Scope: "builtin"},
}

// PromptContext 模板渲染时的上下文数据
type PromptContext struct {
	UserName         string            // 当前登录用户名
	UserID           int64             // 当前用户ID
	SessionID        string            // 当前会话ID
	ModelName        string            // 当前使用的模型名称
	KnowledgeContext string            // RAG 知识库检索结果
	CustomVars       map[string]string // 合并后的自定义变量（用户级 + 会话级 + 请求级）
}

// templateVarRegex 匹配 {{variable_name}} 格式的模板变量（变量名支持字母、数字、下划线）
var templateVarRegex = regexp.MustCompile(`\{\{(\w+)\}\}`)

// RenderPromptTemplate 渲染 Prompt 模板，将 {{变量名}} 替换为实际值
// 优先级：请求级自定义变量 > 内置变量（内置变量不可被覆盖的有 current_time/current_date）
// 未匹配的变量保留原样（不报错），方便渐进式迁移
func RenderPromptTemplate(template string, pctx PromptContext) string {
	if !strings.Contains(template, "{{") {
		return template // 快速路径：无模板变量，直接返回
	}

	// 构建内置变量映射表
	vars := map[string]string{
		"current_time":      time.Now().Format("2006-01-02 15:04:05"),
		"current_date":      time.Now().Format("2006-01-02"),
		"user_name":         pctx.UserName,
		"user_id":           fmt.Sprintf("%d", pctx.UserID),
		"session_id":        pctx.SessionID,
		"model_name":        pctx.ModelName,
		"knowledge_context": pctx.KnowledgeContext,
	}

	// 合并自定义变量（自定义变量可覆盖除时间外的内置变量）
	for k, v := range pctx.CustomVars {
		vars[k] = v
	}

	// 替换所有匹配的模板变量
	result := templateVarRegex.ReplaceAllStringFunc(template, func(match string) string {
		varName := match[2 : len(match)-2] // 去掉 {{ 和 }}
		if val, ok := vars[varName]; ok && val != "" {
			return val
		}
		return match // 未匹配的变量保留原样
	})

	return result
}

// MergePromptVars 合并多层级变量，优先级从低到高：用户级 → 会话级 → 请求级
// 后面的覆盖前面的
func MergePromptVars(userVars, sessionVars, requestVars map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range userVars {
		merged[k] = v
	}
	for k, v := range sessionVars {
		merged[k] = v
	}
	for k, v := range requestVars {
		merged[k] = v
	}
	return merged
}
