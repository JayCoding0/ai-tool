package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"aiProject/internal/application"
	"aiProject/internal/domain/a2a"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// A2AHandler A2A 协议 HTTP 处理器
// 实现以下接口：
//   - GET  /.well-known/agent.json   → 暴露 AgentCard
//   - POST /a2a/tasks/send           → 提交任务
//   - GET  /a2a/tasks/{id}           → 查询任务状态
//   - GET  /a2a/tasks/{id}/stream    → SSE 流式订阅
type A2AHandler struct {
	a2aService *application.A2AService
	logger     *zap.Logger
}

// NewA2AHandler 创建 A2A 处理器
func NewA2AHandler(a2aService *application.A2AService) *A2AHandler {
	return &A2AHandler{
		a2aService: a2aService,
		logger:     shared.GetLogger(),
	}
}

// HandleAgentCard 返回 AgentCard（GET /.well-known/agent.json）
func (h *A2AHandler) HandleAgentCard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	card := h.a2aService.GetAgentCard()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(card) //nolint:errcheck
}

// HandleTaskSend 提交任务（POST /a2a/tasks/send）
func (h *A2AHandler) HandleTaskSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req a2a.TaskSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// 校验输入
	if len(req.Message.Parts) == 0 {
		writeJSONError(w, "message.parts 不能为空", http.StatusBadRequest)
		return
	}

	task, err := h.a2aService.SubmitTask(r.Context(), req)
	if err != nil {
		h.logger.Error("A2A 提交任务失败", zap.Error(err))
		writeJSONError(w, "提交任务失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted) // 202 Accepted
	json.NewEncoder(w).Encode(a2a.TaskSendResponse{Task: *task}) //nolint:errcheck
}

// HandleTaskGet 查询任务状态（GET /a2a/tasks/{id}）
func (h *A2AHandler) HandleTaskGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	taskID := extractTaskID(r.URL.Path, "/a2a/tasks/")
	if taskID == "" {
		writeJSONError(w, "task id 不能为空", http.StatusBadRequest)
		return
	}

	task, ok := h.a2aService.GetTask(taskID)
	if !ok {
		writeJSONError(w, "任务不存在", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task) //nolint:errcheck
}

// HandleTaskStream SSE 流式订阅任务事件（GET /a2a/tasks/{id}/stream）
func (h *A2AHandler) HandleTaskStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 提取任务 ID（路径格式：/a2a/tasks/{id}/stream）
	path := strings.TrimSuffix(r.URL.Path, "/stream")
	taskID := extractTaskID(path, "/a2a/tasks/")
	if taskID == "" {
		writeJSONError(w, "task id 不能为空", http.StatusBadRequest)
		return
	}

	// 检查任务是否存在
	task, ok := h.a2aService.GetTask(taskID)
	if !ok {
		writeJSONError(w, "任务不存在", http.StatusNotFound)
		return
	}

	// 如果任务已经是终态，直接返回最终状态
	if task.IsTerminal() {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(task) //nolint:errcheck
		return
	}

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

	// 订阅任务事件
	eventCh, cancel := h.a2aService.SubscribeTask(taskID)
	defer cancel()

	// 推送 SSE 事件的辅助函数
	sendSSE := func(event a2a.TaskStreamEvent) bool {
		b, err := json.Marshal(event)
		if err != nil {
			return true
		}
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
		return true
	}

	// 消费事件流
	for {
		select {
		case event, open := <-eventCh:
			if !open {
				// channel 已关闭，任务结束
				return
			}
			sendSSE(event)
			// 收到终态事件后退出
			if event.Type == "completed" || event.Type == "error" {
				return
			}
		case <-r.Context().Done():
			// 客户端断开连接
			h.logger.Info("A2A SSE 客户端断开", zap.String("task_id", taskID))
			return
		}
	}
}

// extractTaskID 从 URL 路径中提取任务 ID
// 例如：/a2a/tasks/task-123456 → task-123456
func extractTaskID(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	id := strings.TrimPrefix(path, prefix)
	// 去掉可能的尾部斜杠
	id = strings.TrimSuffix(id, "/")
	return id
}
