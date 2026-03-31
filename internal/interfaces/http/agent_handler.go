package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aiProject/internal/config"
	domain_tool "aiProject/internal/domain/tool"
	"aiProject/internal/infrastructure/database"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// ModelOption 模型选项（用于前端展示）
type ModelOption struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

// AgentToolInfo Agent 工具信息（用于前端展示）
type AgentToolInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
}

// AgentInfo Agent 信息（用于前端展示）
type AgentInfo struct {
	Name         string          `json:"name"`
	DisplayName  string          `json:"display_name"`
	Description  string          `json:"description"`
	IsMaster     bool            `json:"is_master"`
	DefaultTools []string        `json:"default_tools"`
	Tools        []AgentToolInfo `json:"tools"`
}

// inferModelType 根据模型名和配置推断模型类型
func inferModelType(name, cfgType, globalType string) string {
	if cfgType != "" {
		return cfgType
	}
	if strings.Contains(name, ":") {
		return "local"
	}
	if globalType == "local" {
		return "local"
	}
	return "cloud"
}

// ollamaTagsResponse Ollama /api/tags 接口响应结构
type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// fetchOllamaModels 实时从 Ollama 拉取本地已安装的模型列表
func fetchOllamaModels(ollamaURL string) ([]ModelOption, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/api/tags", strings.TrimRight(ollamaURL, "/")))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama 返回状态码: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var tagsResp ollamaTagsResponse
	if err := json.Unmarshal(body, &tagsResp); err != nil {
		return nil, err
	}
	var models []ModelOption
	for _, m := range tagsResp.Models {
		label := strings.TrimSuffix(m.Name, ":latest")
		models = append(models, ModelOption{Name: m.Name, Label: label + " (本地)", Type: "local"})
	}
	return models, nil
}

// buildModelList 构建可用模型列表（云端 + 本地 Ollama）
func buildModelList(appConfig *config.Config) []ModelOption {
	var models []ModelOption
	if appConfig == nil {
		return models
	}

	globalType := appConfig.Model.Type
	for _, m := range appConfig.Model.AvailableModels {
		modelType := inferModelType(m.Name, m.Type, globalType)
		if modelType != "local" {
			models = append(models, ModelOption{Name: m.Name, Label: m.Label, Type: modelType})
		}
	}

	ollamaURL := appConfig.Model.OllamaURL
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}
	if localModels, err := fetchOllamaModels(ollamaURL); err != nil {
		shared.GetLogger().Debug("Ollama 未启动，跳过本地模型拉取", zap.Error(err))
	} else {
		models = append(models, localModels...)
	}

	// 如果没有任何模型，使用默认模型
	if len(models) == 0 {
		models = []ModelOption{
			{Name: appConfig.Model.Name, Label: appConfig.Model.Name,
				Type: inferModelType(appConfig.Model.Name, "", appConfig.Model.Type)},
		}
	}
	return models
}

// HandleListModels 返回可用模型列表
func (h *ChatHandler) HandleListModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	models := buildModelList(h.appConfig)
	defaultModel := ""
	if h.appConfig != nil {
		defaultModel = h.appConfig.Model.Name
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"models":        models,
		"default_model": defaultModel,
	})
}

// HandleListTools 列出所有已注册的工具
func (h *ChatHandler) HandleListTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defs := domain_tool.ListAll()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tools": defs,
		"count": len(defs),
	})
}

// HandleListAgents 列出所有 Agent 及其工具信息
func (h *ChatHandler) HandleListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// 无多 Agent 模式，返回空列表
	if h.agentRegistry == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"agents": []AgentInfo{},
			"count":  0,
		})
		return
	}

	instances := h.agentRegistry.ListAll()
	agents := make([]AgentInfo, 0, len(instances))
	for _, inst := range instances {
		tools := buildAgentToolInfoList(inst.Def.EnabledTools)
		agents = append(agents, AgentInfo{
			Name:         inst.Def.Name,
			DisplayName:  inst.Def.DisplayName,
			Description:  inst.Def.Description,
			IsMaster:     inst.Def.IsMaster,
			DefaultTools: inst.Def.EnabledTools,
			Tools:        tools,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"agents": agents,
		"count":  len(agents),
	})
}

// buildAgentToolInfoList 根据工具名称列表构建工具详情列表
func buildAgentToolInfoList(toolNames []string) []AgentToolInfo {
	tools := make([]AgentToolInfo, 0, len(toolNames))
	for _, toolName := range toolNames {
		if t, ok := domain_tool.Get(toolName); ok {
			tools = append(tools, AgentToolInfo{
				Name:        t.Definition.Name,
				DisplayName: t.Definition.DisplayName,
				Description: t.Definition.Description,
			})
		} else {
			// 工具未注册时仍返回名称（如 call_agent 是动态注册的）
			tools = append(tools, AgentToolInfo{
				Name:        toolName,
				DisplayName: domain_tool.GetDisplayName(toolName),
				Description: "",
			})
		}
	}
	return tools
}

// HandleUpdateAgentTools 动态更新指定 Agent 的工具列表
// PUT /api/agents/{name}/tools
// Body: { "tools": ["weather", "http_request"] }
func (h *ChatHandler) HandleUpdateAgentTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.agentRegistry == nil {
		writeJSONError(w, "多 Agent 模式未启用", http.StatusServiceUnavailable)
		return
	}

	// 从路径中提取 agent name：/api/agents/{name}/tools
	agentName := extractPathSegment(r.URL.Path, "/api/agents/", "/tools")
	if agentName == "" {
		writeJSONError(w, "无效的 Agent 名称", http.StatusBadRequest)
		return
	}

	var req struct {
		Tools []string `json:"tools"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "请求体解析失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Tools == nil {
		req.Tools = []string{}
	}

	// 热更新内存中的 Agent 工具列表
	if err := h.agentRegistry.UpdateTools(agentName, req.Tools); err != nil {
		writeJSONError(w, err.Error(), http.StatusNotFound)
		return
	}

	// 持久化到数据库（如果数据库可用）
	persistAgentTools(r, agentName, req.Tools)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"agent_name": agentName,
		"tools":      req.Tools,
	})
}

// extractPathSegment 从 URL 路径中提取指定前缀和后缀之间的片段
// 例如：extractPathSegment("/api/agents/weather_agent/tools", "/api/agents/", "/tools") → "weather_agent"
func extractPathSegment(path, prefix, suffix string) string {
	trimmed := strings.TrimPrefix(path, prefix)
	segment := strings.TrimSuffix(trimmed, suffix)
	if segment == "" || segment == path {
		return ""
	}
	return segment
}

// persistAgentTools 将 Agent 工具配置持久化到数据库
func persistAgentTools(r *http.Request, agentName string, tools []string) {
	db := database.GetDB()
	if db == nil {
		return
	}
	ctx := r.Context()
	// 先删除该 Agent 的旧配置，再批量插入新配置
	_, err := db.Exec(ctx, "DELETE FROM agent_tools WHERE agent_name = ?", agentName)
	if err != nil {
		shared.GetLogger().Warn("删除 Agent 旧工具配置失败", zap.String("agent", agentName), zap.Error(err))
		return
	}
	if len(tools) == 0 {
		return
	}
	// 批量插入
	vals := make([]interface{}, 0, len(tools)*2)
	placeholders := make([]string, 0, len(tools))
	for _, t := range tools {
		placeholders = append(placeholders, "(?, ?)")
		vals = append(vals, agentName, t)
	}
	query := "INSERT INTO agent_tools (agent_name, tool_name) VALUES " + strings.Join(placeholders, ",")
	if _, err := db.Exec(ctx, query, vals...); err != nil {
		shared.GetLogger().Warn("保存 Agent 工具配置失败", zap.String("agent", agentName), zap.Error(err))
	}
}
