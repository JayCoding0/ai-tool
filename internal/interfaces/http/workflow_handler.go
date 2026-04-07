// Package http 提供 Workflow HTTP 接口层实现
package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"aiProject/internal/application"
	"aiProject/internal/domain/workflow"
	"go.uber.org/zap"
)

// ─── 请求/响应结构体 ──────────────────────────────────────────────────────────

// CreateWorkflowRequest 创建工作流请求
type CreateWorkflowRequest struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Nodes       []workflow.Node     `json:"nodes"`
	Edges       []workflow.Edge     `json:"edges"`
	Variables   []workflow.Variable `json:"variables,omitempty"`
}

// UpdateWorkflowRequest 更新工作流请求
type UpdateWorkflowRequest struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Nodes       []workflow.Node     `json:"nodes"`
	Edges       []workflow.Edge     `json:"edges"`
	Variables   []workflow.Variable `json:"variables,omitempty"`
	Status      string              `json:"status,omitempty"`
}

// ExecuteWorkflowRequest 执行工作流请求
type ExecuteWorkflowRequest struct {
	Inputs map[string]interface{} `json:"inputs,omitempty"`
}

// WorkflowResponse 工作流响应
type WorkflowResponse struct {
	ID          int64               `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Nodes       []workflow.Node     `json:"nodes"`
	Edges       []workflow.Edge     `json:"edges"`
	Variables   []workflow.Variable `json:"variables,omitempty"`
	Status      string              `json:"status"`
	Version     int                 `json:"version"`
	CreatedAt   string              `json:"created_at"`
	UpdatedAt   string              `json:"updated_at"`
}

// ─── WorkflowHandler ──────────────────────────────────────────────────────────

// WorkflowHandler 工作流 HTTP 处理程序
type WorkflowHandler struct {
	workflowSvc    *application.WorkflowService
	workflowEngine *application.WorkflowEngine
	logger         *zap.Logger
	getUserID      func(r *http.Request) int64 // 从请求中获取用户 ID 的函数
}

// NewWorkflowHandler 创建工作流处理程序
func NewWorkflowHandler(
	workflowSvc *application.WorkflowService,
	workflowEngine *application.WorkflowEngine,
	logger *zap.Logger,
	getUserID func(r *http.Request) int64,
) *WorkflowHandler {
	return &WorkflowHandler{
		workflowSvc:    workflowSvc,
		workflowEngine: workflowEngine,
		logger:         logger,
		getUserID:      getUserID,
	}
}

// ─── CRUD 接口 ──────────────────────────────────────────────────────────────

// HandleCreateWorkflow 创建工作流 POST /api/workflows
func (h *WorkflowHandler) HandleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CreateWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		writeJSONError(w, "工作流名称不能为空", http.StatusBadRequest)
		return
	}

	userID := h.getUserID(r)

	wf := &workflow.Workflow{
		Name:        req.Name,
		Description: req.Description,
		Nodes:       req.Nodes,
		Edges:       req.Edges,
		Variables:   req.Variables,
		Status:      workflow.StatusDraft,
		UserID:      userID,
	}

	if err := h.workflowSvc.CreateWorkflow(r.Context(), wf); err != nil {
		h.logger.Error("创建工作流失败", zap.Error(err))
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(toWorkflowResponse(wf))
}

// HandleListWorkflows 列出工作流 GET /api/workflows
func (h *WorkflowHandler) HandleListWorkflows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := h.getUserID(r)
	status := workflow.Status(r.URL.Query().Get("status"))

	workflows, err := h.workflowSvc.ListWorkflows(r.Context(), userID, status)
	if err != nil {
		h.logger.Error("列出工作流失败", zap.Error(err))
		writeJSONError(w, "获取工作流列表失败", http.StatusInternalServerError)
		return
	}

	var resp []WorkflowResponse
	for _, wf := range workflows {
		resp = append(resp, toWorkflowResponse(wf))
	}
	if resp == nil {
		resp = []WorkflowResponse{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"workflows": resp,
	})
}

// HandleGetWorkflow 获取工作流详情 GET /api/workflows/{id}
func (h *WorkflowHandler) HandleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, err := h.parseWorkflowID(r.URL.Path, "/api/workflows/")
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	wf, err := h.workflowSvc.GetWorkflow(r.Context(), id)
	if err != nil {
		h.logger.Error("获取工作流失败", zap.Int64("id", id), zap.Error(err))
		writeJSONError(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toWorkflowResponse(wf))
}

// HandleUpdateWorkflow 更新工作流 PUT /api/workflows/{id}
func (h *WorkflowHandler) HandleUpdateWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, err := h.parseWorkflowID(r.URL.Path, "/api/workflows/")
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req UpdateWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	wf := &workflow.Workflow{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Nodes:       req.Nodes,
		Edges:       req.Edges,
		Variables:   req.Variables,
		Status:      workflow.Status(req.Status),
	}

	if err := h.workflowSvc.UpdateWorkflow(r.Context(), wf); err != nil {
		h.logger.Error("更新工作流失败", zap.Int64("id", id), zap.Error(err))
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "工作流更新成功"})
}

// HandleDeleteWorkflow 删除工作流 DELETE /api/workflows/{id}
func (h *WorkflowHandler) HandleDeleteWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, err := h.parseWorkflowID(r.URL.Path, "/api/workflows/")
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.workflowSvc.DeleteWorkflow(r.Context(), id); err != nil {
		h.logger.Error("删除工作流失败", zap.Int64("id", id), zap.Error(err))
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "工作流删除成功"})
}

// HandlePublishWorkflow 发布工作流 POST /api/workflows/{id}/publish
func (h *WorkflowHandler) HandlePublishWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 解析路径：/api/workflows/{id}/publish
	path := strings.TrimSuffix(r.URL.Path, "/publish")
	id, err := h.parseWorkflowID(path, "/api/workflows/")
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.workflowSvc.PublishWorkflow(r.Context(), id); err != nil {
		h.logger.Error("发布工作流失败", zap.Int64("id", id), zap.Error(err))
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "工作流发布成功"})
}

// ─── 执行接口 ──────────────────────────────────────────────────────────────

// HandleExecuteWorkflow 执行工作流 POST /api/workflows/{id}/execute（SSE 流式返回）
func (h *WorkflowHandler) HandleExecuteWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 解析路径：/api/workflows/{id}/execute
	path := strings.TrimSuffix(r.URL.Path, "/execute")
	id, err := h.parseWorkflowID(path, "/api/workflows/")
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req ExecuteWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// 允许空 body
		req = ExecuteWorkflowRequest{}
	}

	userID := h.getUserID(r)

	// 设置 SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONError(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// 执行工作流
	eventCh, err := h.workflowEngine.Execute(r.Context(), id, req.Inputs, userID)
	if err != nil {
		h.logger.Error("执行工作流失败", zap.Int64("id", id), zap.Error(err))
		// SSE 格式返回错误
		errData, _ := json.Marshal(map[string]string{"type": "workflow_error", "error": err.Error()})
		fmt.Fprintf(w, "data: %s\n\n", errData)
		flusher.Flush()
		return
	}

	// 消费事件并推送 SSE
	for event := range eventCh {
		data, _ := json.Marshal(event)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
}

// HandleGetWorkflowRuns 获取工作流执行记录 GET /api/workflows/{id}/runs
func (h *WorkflowHandler) HandleGetWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 解析路径：/api/workflows/{id}/runs
	path := strings.TrimSuffix(r.URL.Path, "/runs")
	id, err := h.parseWorkflowID(path, "/api/workflows/")
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, parseErr := strconv.Atoi(l); parseErr == nil && parsed > 0 {
			limit = parsed
		}
	}

	runs, err := h.workflowSvc.GetWorkflowRuns(r.Context(), id, limit)
	if err != nil {
		h.logger.Error("获取执行记录失败", zap.Int64("workflow_id", id), zap.Error(err))
		writeJSONError(w, "获取执行记录失败", http.StatusInternalServerError)
		return
	}
	if runs == nil {
		runs = []*workflow.WorkflowRun{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"runs": runs,
	})
}

// HandleGetWorkflowRun 获取单次执行详情 GET /api/workflow-runs/{run_id}
func (h *WorkflowHandler) HandleGetWorkflowRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	runID := strings.TrimPrefix(r.URL.Path, "/api/workflow-runs/")
	if runID == "" {
		writeJSONError(w, "run_id 不能为空", http.StatusBadRequest)
		return
	}

	run, err := h.workflowSvc.GetWorkflowRun(r.Context(), runID)
	if err != nil {
		h.logger.Error("获取执行详情失败", zap.String("run_id", runID), zap.Error(err))
		writeJSONError(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(run)
}

// ─── 辅助方法 ──────────────────────────────────────────────────────────────

// parseWorkflowID 从 URL 路径中解析工作流 ID
func (h *WorkflowHandler) parseWorkflowID(path, prefix string) (int64, error) {
	idStr := strings.TrimPrefix(path, prefix)
	// 去掉可能的尾部斜杠和子路径
	if idx := strings.Index(idStr, "/"); idx >= 0 {
		idStr = idStr[:idx]
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("无效的工作流 ID: %s", idStr)
	}
	return id, nil
}

// toWorkflowResponse 将领域对象转换为 HTTP 响应
func toWorkflowResponse(wf *workflow.Workflow) WorkflowResponse {
	nodes := wf.Nodes
	if nodes == nil {
		nodes = []workflow.Node{}
	}
	edges := wf.Edges
	if edges == nil {
		edges = []workflow.Edge{}
	}
	return WorkflowResponse{
		ID:          wf.ID,
		Name:        wf.Name,
		Description: wf.Description,
		Nodes:       nodes,
		Edges:       edges,
		Variables:   wf.Variables,
		Status:      string(wf.Status),
		Version:     wf.Version,
		CreatedAt:   wf.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:   wf.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}
