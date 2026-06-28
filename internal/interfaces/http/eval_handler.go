// Package http 提供 HTTP 接口层实现
// eval_handler.go — Agent 评估体系接口（数据集/用例/运行/报告）
package http

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"aiProject/internal/application"
	"go.uber.org/zap"
)

// ─── 请求结构体 ────────────────────────────────────────────────────────────────

// CreateDatasetRequest 创建数据集请求
type CreateDatasetRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// AddCaseRequest 新增用例请求（单条或批量二选一）
type AddCaseRequest struct {
	DatasetID int64  `json:"dataset_id"`
	Input     string `json:"input"`
	Expected  string `json:"expected"`
	// 批量导入：cases 非空时优先批量
	Cases []struct {
		Input    string `json:"input"`
		Expected string `json:"expected"`
	} `json:"cases,omitempty"`
}

// RunEvalRequest 启动评测运行请求
type RunEvalRequest struct {
	DatasetID    int64    `json:"dataset_id"`
	Name         string   `json:"name"`
	ModelName    string   `json:"model_name"`
	SystemPrompt string   `json:"system_prompt"`
	Tools        []string `json:"tools"`
	Scorer       string   `json:"scorer"` // judge | exact | semantic
	JudgeModel   string   `json:"judge_model"`
	Threshold    float64  `json:"threshold"`
}

// ─── 数据集接口 ────────────────────────────────────────────────────────────────

// HandleListDatasets GET /api/eval/datasets 列出当前用户的数据集
func (h *ChatHandler) HandleListDatasets(w http.ResponseWriter, r *http.Request) {
	if h.evalSvc == nil {
		writeJSONError(w, "评估功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}
	userID, ok := h.requireLogin(w, r)
	if !ok {
		return
	}
	list, err := h.evalSvc.ListDatasets(r.Context(), userID)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"datasets": list})
}

// HandleCreateDataset POST /api/eval/datasets 创建数据集
func (h *ChatHandler) HandleCreateDataset(w http.ResponseWriter, r *http.Request) {
	if h.evalSvc == nil {
		writeJSONError(w, "评估功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}
	userID, ok := h.requireLogin(w, r)
	if !ok {
		return
	}
	var req CreateDatasetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	d, err := h.evalSvc.CreateDataset(r.Context(), userID, req.Name, req.Description)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, d)
}

// HandleDeleteDataset POST /api/eval/datasets/delete 删除数据集
func (h *ChatHandler) HandleDeleteDataset(w http.ResponseWriter, r *http.Request) {
	if h.evalSvc == nil {
		writeJSONError(w, "评估功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}
	userID, ok := h.requireLogin(w, r)
	if !ok {
		return
	}
	var req struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if err := h.evalSvc.DeleteDataset(r.Context(), req.ID, userID); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"message": "数据集已删除"})
}

// ─── 用例接口 ──────────────────────────────────────────────────────────────────

// HandleListCases GET /api/eval/cases?dataset_id=xx 列出用例
func (h *ChatHandler) HandleListCases(w http.ResponseWriter, r *http.Request) {
	if h.evalSvc == nil {
		writeJSONError(w, "评估功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}
	if _, ok := h.requireLogin(w, r); !ok {
		return
	}
	datasetID, _ := strconv.ParseInt(r.URL.Query().Get("dataset_id"), 10, 64)
	list, err := h.evalSvc.ListCases(r.Context(), datasetID)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"cases": list})
}

// HandleAddCase POST /api/eval/cases 新增用例（单条或批量）
func (h *ChatHandler) HandleAddCase(w http.ResponseWriter, r *http.Request) {
	if h.evalSvc == nil {
		writeJSONError(w, "评估功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}
	if _, ok := h.requireLogin(w, r); !ok {
		return
	}
	var req AddCaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.DatasetID == 0 {
		writeJSONError(w, "dataset_id 不能为空", http.StatusBadRequest)
		return
	}
	// 批量导入
	if len(req.Cases) > 0 {
		count, err := h.evalSvc.ImportCases(r.Context(), req.DatasetID, req.Cases)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]interface{}{"message": "批量导入完成", "imported": count})
		return
	}
	// 单条新增
	c, err := h.evalSvc.AddCase(r.Context(), req.DatasetID, req.Input, req.Expected)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, c)
}

// HandleDeleteCase POST /api/eval/cases/delete 删除用例
func (h *ChatHandler) HandleDeleteCase(w http.ResponseWriter, r *http.Request) {
	if h.evalSvc == nil {
		writeJSONError(w, "评估功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}
	if _, ok := h.requireLogin(w, r); !ok {
		return
	}
	var req struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if err := h.evalSvc.DeleteCase(r.Context(), req.ID); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"message": "用例已删除"})
}

// ─── 运行 / 报告接口 ───────────────────────────────────────────────────────────

// HandleRunEval POST /api/eval/runs 启动一次评测运行
func (h *ChatHandler) HandleRunEval(w http.ResponseWriter, r *http.Request) {
	if h.evalSvc == nil {
		writeJSONError(w, "评估功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}
	userID, ok := h.requireLogin(w, r)
	if !ok {
		return
	}
	var req RunEvalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	run, err := h.evalSvc.StartRun(r.Context(), application.RunConfig{
		DatasetID:    req.DatasetID,
		Name:         req.Name,
		ModelName:    req.ModelName,
		SystemPrompt: req.SystemPrompt,
		Tools:        req.Tools,
		Scorer:       req.Scorer,
		JudgeModel:   req.JudgeModel,
		Threshold:    req.Threshold,
		UserID:       userID,
	})
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.logger.Info("[Eval] 评测运行已启动", zap.Int64("run_id", run.ID), zap.Int64("user_id", userID))
	writeJSON(w, run)
}

// HandleListRuns GET /api/eval/runs?dataset_id=xx 列出运行记录
func (h *ChatHandler) HandleListRuns(w http.ResponseWriter, r *http.Request) {
	if h.evalSvc == nil {
		writeJSONError(w, "评估功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}
	userID, ok := h.requireLogin(w, r)
	if !ok {
		return
	}
	datasetID, _ := strconv.ParseInt(r.URL.Query().Get("dataset_id"), 10, 64)
	list, err := h.evalSvc.ListRuns(r.Context(), datasetID, userID)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"runs": list})
}

// HandleCompareRuns GET /api/eval/runs/compare?base=xx&target=yy 对比两次运行
func (h *ChatHandler) HandleCompareRuns(w http.ResponseWriter, r *http.Request) {
	if h.evalSvc == nil {
		writeJSONError(w, "评估功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}
	if _, ok := h.requireLogin(w, r); !ok {
		return
	}
	baseID, _ := strconv.ParseInt(r.URL.Query().Get("base"), 10, 64)
	targetID, _ := strconv.ParseInt(r.URL.Query().Get("target"), 10, 64)
	if baseID == 0 || targetID == 0 {
		writeJSONError(w, "需提供 base 和 target 运行 ID", http.StatusBadRequest)
		return
	}
	cmp, err := h.evalSvc.CompareRuns(r.Context(), baseID, targetID)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, cmp)
}

// HandleGetRun GET /api/eval/runs/{id} 获取运行详情（含每条结果）
func (h *ChatHandler) HandleGetRun(w http.ResponseWriter, r *http.Request) {
	if h.evalSvc == nil {
		writeJSONError(w, "评估功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}
	if _, ok := h.requireLogin(w, r); !ok {
		return
	}
	// 从路径 /api/eval/runs/{id} 解析 ID
	idStr := strings.TrimPrefix(r.URL.Path, "/api/eval/runs/")
	runID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, "无效的运行 ID", http.StatusBadRequest)
		return
	}
	run, results, err := h.evalSvc.GetRunDetail(r.Context(), runID)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"run": run, "results": results})
}

// writeJSON 统一写出 JSON 响应
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// HandleExportRun GET /api/eval/runs/{id}/export 导出运行报告为 CSV
func (h *ChatHandler) HandleExportRun(w http.ResponseWriter, r *http.Request) {
	if h.evalSvc == nil {
		writeJSONError(w, "评估功能不可用（数据库未连接）", http.StatusServiceUnavailable)
		return
	}
	if _, ok := h.requireLogin(w, r); !ok {
		return
	}
	// 从路径 /api/eval/runs/{id}/export 解析 ID
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/eval/runs/"), "/export")
	runID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, "无效的运行 ID", http.StatusBadRequest)
		return
	}
	run, results, err := h.evalSvc.GetRunDetail(r.Context(), runID)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"eval_run_"+idStr+".csv\"")
	// 写入 UTF-8 BOM，确保 Excel 正确识别中文
	w.Write([]byte{0xEF, 0xBB, 0xBF}) //nolint:errcheck

	cw := csv.NewWriter(w)
	defer cw.Flush()

	// 报告头部元信息
	_ = cw.Write([]string{"评测运行报告"})
	_ = cw.Write([]string{"运行名称", run.Name})
	_ = cw.Write([]string{"被测模型", run.ModelName})
	_ = cw.Write([]string{"评分方式", string(run.Scorer)})
	_ = cw.Write([]string{"通过阈值", strconv.FormatFloat(run.Threshold, 'f', 2, 64)})
	_ = cw.Write([]string{"通过率", strconv.Itoa(run.PassedCases) + "/" + strconv.Itoa(run.TotalCases)})
	_ = cw.Write([]string{"平均分", strconv.FormatFloat(run.AvgScore, 'f', 4, 64)})
	_ = cw.Write([]string{})

	// 逐条结果
	_ = cw.Write([]string{"序号", "输入", "期望", "实际输出", "得分", "是否通过", "评分理由", "耗时(ms)", "tokens"})
	for i, res := range results {
		passed := "否"
		if res.Passed {
			passed = "是"
		}
		_ = cw.Write([]string{
			strconv.Itoa(i + 1),
			res.Input,
			res.Expected,
			res.Actual,
			strconv.FormatFloat(res.Score, 'f', 4, 64),
			passed,
			res.Reason,
			strconv.FormatInt(res.LatencyMs, 10),
			strconv.Itoa(res.Tokens),
		})
	}
}
