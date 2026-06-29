// Package http 提供 HTTP 接口层实现
// mcp_handler.go — 外部 MCP Server 接入管理（MCP Client）
package http

import (
	"encoding/json"
	"net/http"

	"aiProject/internal/infrastructure/mcpclient"
	"go.uber.org/zap"
)

// SetMCPManager 注入 MCP Client 管理器
func (h *ChatHandler) SetMCPManager(mgr *mcpclient.Manager) {
	h.mcpManager = mgr
}

// HandleListMCPServers GET /api/mcp/servers 列出已接入的外部 MCP Server
func (h *ChatHandler) HandleListMCPServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.mcpManager == nil {
		writeJSONError(w, "MCP Client 未启用", http.StatusServiceUnavailable)
		return
	}
	if _, ok := h.requireLogin(w, r); !ok {
		return
	}
	writeJSON(w, map[string]interface{}{"servers": h.mcpManager.ListServers()})
}

// HandleAddMCPServer POST /api/mcp/servers 接入一个外部 MCP Server（连接 + 发现工具 + 注册）
func (h *ChatHandler) HandleAddMCPServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.mcpManager == nil {
		writeJSONError(w, "MCP Client 未启用", http.StatusServiceUnavailable)
		return
	}
	if _, ok := h.requireLogin(w, r); !ok {
		return
	}
	var cfg mcpclient.ServerConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	count, err := h.mcpManager.AddServer(r.Context(), cfg)
	if err != nil {
		h.logger.Warn("接入 MCP Server 失败", zap.String("name", cfg.Name), zap.Error(err))
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]interface{}{"message": "接入成功", "tool_count": count})
}

// HandleDeleteMCPServer POST /api/mcp/servers/delete 移除一个外部 MCP Server
func (h *ChatHandler) HandleDeleteMCPServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.mcpManager == nil {
		writeJSONError(w, "MCP Client 未启用", http.StatusServiceUnavailable)
		return
	}
	if _, ok := h.requireLogin(w, r); !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	h.mcpManager.RemoveServer(req.Name)
	writeJSON(w, map[string]string{"message": "已移除"})
}
