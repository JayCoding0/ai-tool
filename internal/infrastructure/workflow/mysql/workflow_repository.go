// Package mysql 提供 Workflow 的 MySQL 持久化实现
package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"aiProject/internal/domain/workflow"
	"aiProject/internal/infrastructure/database"
	"aiProject/internal/shared"
	trmysql "git.code.oa.com/trpc-go/trpc-database/mysql"
	"go.uber.org/zap"
)

// WorkflowRepository MySQL 工作流存储库
type WorkflowRepository struct {
	db trmysql.Client
}

// NewWorkflowRepository 创建 MySQL 工作流存储库
func NewWorkflowRepository() *WorkflowRepository {
	return &WorkflowRepository{
		db: database.GetDB(),
	}
}

// Create 创建工作流
func (r *WorkflowRepository) Create(ctx context.Context, wf *workflow.Workflow) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	graphJSON, err := json.Marshal(wf.GetGraphData())
	if err != nil {
		return fmt.Errorf("序列化图数据失败: %w", err)
	}

	now := time.Now()
	wf.CreatedAt = now
	wf.UpdatedAt = now
	if wf.Status == "" {
		wf.Status = workflow.StatusDraft
	}
	if wf.Version == 0 {
		wf.Version = 1
	}

	query := `INSERT INTO workflows (name, description, graph_json, status, version, user_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := r.db.Exec(ctx, query,
		wf.Name, wf.Description, string(graphJSON),
		string(wf.Status), wf.Version, wf.UserID,
		now, now,
	)
	if err != nil {
		shared.GetLogger().Error("创建工作流失败", zap.Error(err))
		return fmt.Errorf("创建工作流失败: %w", err)
	}

	// 获取自增 ID
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("获取自增 ID 失败: %w", err)
	}
	wf.ID = id

	shared.GetLogger().Info("工作流创建成功",
		zap.Int64("id", wf.ID),
		zap.String("name", wf.Name),
	)
	return nil
}

// Update 更新工作流
func (r *WorkflowRepository) Update(ctx context.Context, wf *workflow.Workflow) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	graphJSON, err := json.Marshal(wf.GetGraphData())
	if err != nil {
		return fmt.Errorf("序列化图数据失败: %w", err)
	}

	now := time.Now()
	wf.UpdatedAt = now

	query := `UPDATE workflows SET name = ?, description = ?, graph_json = ?, status = ?, version = ?, updated_at = ? WHERE id = ?`
	_, err = r.db.Exec(ctx, query,
		wf.Name, wf.Description, string(graphJSON),
		string(wf.Status), wf.Version, now, wf.ID,
	)
	if err != nil {
		shared.GetLogger().Error("更新工作流失败", zap.Int64("id", wf.ID), zap.Error(err))
		return fmt.Errorf("更新工作流失败: %w", err)
	}
	return nil
}

// GetByID 按 ID 获取工作流
func (r *WorkflowRepository) GetByID(ctx context.Context, id int64) (*workflow.Workflow, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var (
		wfID        int64
		name        string
		description sql.NullString
		graphJSON   string
		status      string
		version     int
		userID      int64
		createdAt   time.Time
		updatedAt   time.Time
	)

	dest := []interface{}{
		&wfID, &name, &description, &graphJSON,
		&status, &version, &userID, &createdAt, &updatedAt,
	}

	err := r.db.QueryRow(ctx, dest,
		`SELECT id, name, description, graph_json, status, version, user_id, created_at, updated_at
		 FROM workflows WHERE id = ?`, id)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, fmt.Errorf("工作流 %d 不存在", id)
		}
		return nil, fmt.Errorf("查询工作流失败: %w", err)
	}

	wf, err := rowToWorkflow(wfID, name, description, graphJSON, status, version, userID, createdAt, updatedAt)
	if err != nil {
		return nil, err
	}
	return wf, nil
}

// List 列出指定用户的工作流
func (r *WorkflowRepository) List(ctx context.Context, userID int64, status workflow.Status) ([]*workflow.Workflow, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var workflows []*workflow.Workflow
	var query string
	var args []interface{}

	if status != "" {
		query = `SELECT id, name, description, graph_json, status, version, user_id, created_at, updated_at
			FROM workflows WHERE user_id = ? AND status = ? ORDER BY updated_at DESC LIMIT 100`
		args = []interface{}{userID, string(status)}
	} else {
		query = `SELECT id, name, description, graph_json, status, version, user_id, created_at, updated_at
			FROM workflows WHERE user_id = ? ORDER BY updated_at DESC LIMIT 100`
		args = []interface{}{userID}
	}

	err := r.db.Query(ctx, func(rows *sql.Rows) error {
		var (
			wfID        int64
			name        string
			description sql.NullString
			graphJSON   string
			st          string
			version     int
			uid         int64
			createdAt   time.Time
			updatedAt   time.Time
		)
		if err := rows.Scan(&wfID, &name, &description, &graphJSON, &st, &version, &uid, &createdAt, &updatedAt); err != nil {
			return err
		}
		wf, err := rowToWorkflow(wfID, name, description, graphJSON, st, version, uid, createdAt, updatedAt)
		if err != nil {
			shared.GetLogger().Warn("解析工作流行失败", zap.Int64("id", wfID), zap.Error(err))
			return nil // 跳过解析失败的行
		}
		workflows = append(workflows, wf)
		return nil
	}, query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询工作流列表失败: %w", err)
	}
	return workflows, nil
}

// Delete 删除工作流
func (r *WorkflowRepository) Delete(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := r.db.Exec(ctx, "DELETE FROM workflows WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("删除工作流失败: %w", err)
	}
	return nil
}

// UpdateStatus 更新工作流状态
func (r *WorkflowRepository) UpdateStatus(ctx context.Context, id int64, status workflow.Status) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	now := time.Now()
	_, err := r.db.Exec(ctx,
		"UPDATE workflows SET status = ?, updated_at = ? WHERE id = ?",
		string(status), now, id,
	)
	if err != nil {
		return fmt.Errorf("更新工作流状态失败: %w", err)
	}
	return nil
}

// rowToWorkflow 将数据库行数据转换为 Workflow 领域对象
func rowToWorkflow(
	id int64, name string, description sql.NullString, graphJSON string,
	status string, version int, userID int64,
	createdAt, updatedAt time.Time,
) (*workflow.Workflow, error) {
	var graphData workflow.GraphData
	if err := json.Unmarshal([]byte(graphJSON), &graphData); err != nil {
		return nil, fmt.Errorf("反序列化图数据失败: %w", err)
	}

	desc := ""
	if description.Valid {
		desc = description.String
	}

	wf := &workflow.Workflow{
		ID:          id,
		Name:        name,
		Description: desc,
		Nodes:       graphData.Nodes,
		Edges:       graphData.Edges,
		Variables:   graphData.Variables,
		Status:      workflow.Status(status),
		Version:     version,
		UserID:      userID,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
	return wf, nil
}

// ─── WorkflowRunRepository 执行记录存储 ─────────────────────────────────────

// WorkflowRunRepository MySQL 工作流执行记录存储库
type WorkflowRunRepository struct {
	db trmysql.Client
}

// NewWorkflowRunRepository 创建 MySQL 工作流执行记录存储库
func NewWorkflowRunRepository() *WorkflowRunRepository {
	return &WorkflowRunRepository{
		db: database.GetDB(),
	}
}

// Save 保存执行记录
func (r *WorkflowRunRepository) Save(ctx context.Context, run *workflow.WorkflowRun) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	inputsJSON, _ := json.Marshal(run.Inputs)
	outputsJSON, _ := json.Marshal(run.Outputs)
	nodeResultsJSON, _ := json.Marshal(run.NodeResults)

	query := `INSERT INTO workflow_runs (workflow_id, run_id, status, inputs, outputs, node_results, total_tokens, duration_ms, error_message, user_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	now := time.Now()
	run.CreatedAt = now.Format("2006-01-02 15:04:05")

	_, err := r.db.Exec(ctx, query,
		run.WorkflowID, run.RunID, string(run.Status),
		string(inputsJSON), string(outputsJSON), string(nodeResultsJSON),
		run.TotalTokens, run.DurationMs, run.ErrorMessage,
		run.UserID, now,
	)
	if err != nil {
		shared.GetLogger().Error("保存工作流执行记录失败", zap.String("run_id", run.RunID), zap.Error(err))
		return fmt.Errorf("保存执行记录失败: %w", err)
	}
	return nil
}

// GetByRunID 按执行 ID 获取记录
func (r *WorkflowRunRepository) GetByRunID(ctx context.Context, runID string) (*workflow.WorkflowRun, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var (
		id           int64
		workflowID   int64
		rid          string
		status       string
		inputsJSON   string
		outputsJSON  sql.NullString
		nodeResJSON  sql.NullString
		totalTokens  int
		durationMs   int64
		errorMessage sql.NullString
		userID       int64
		createdAt    time.Time
	)

	dest := []interface{}{
		&id, &workflowID, &rid, &status,
		&inputsJSON, &outputsJSON, &nodeResJSON,
		&totalTokens, &durationMs, &errorMessage,
		&userID, &createdAt,
	}

	err := r.db.QueryRow(ctx, dest,
		`SELECT id, workflow_id, run_id, status, inputs, outputs, node_results, total_tokens, duration_ms, error_message, user_id, created_at
		 FROM workflow_runs WHERE run_id = ?`, runID)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, fmt.Errorf("执行记录 %s 不存在", runID)
		}
		return nil, fmt.Errorf("查询执行记录失败: %w", err)
	}

	return rowToWorkflowRun(id, workflowID, rid, status, inputsJSON, outputsJSON, nodeResJSON, totalTokens, durationMs, errorMessage, userID, createdAt)
}

// ListByWorkflowID 列出指定工作流的执行记录
func (r *WorkflowRunRepository) ListByWorkflowID(ctx context.Context, workflowID int64, limit int) ([]*workflow.WorkflowRun, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var runs []*workflow.WorkflowRun
	err := r.db.Query(ctx, func(rows *sql.Rows) error {
		var (
			id           int64
			wfID         int64
			rid          string
			status       string
			inputsJSON   string
			outputsJSON  sql.NullString
			nodeResJSON  sql.NullString
			totalTokens  int
			durationMs   int64
			errorMessage sql.NullString
			userID       int64
			createdAt    time.Time
		)
		if err := rows.Scan(&id, &wfID, &rid, &status, &inputsJSON, &outputsJSON, &nodeResJSON, &totalTokens, &durationMs, &errorMessage, &userID, &createdAt); err != nil {
			return err
		}
		run, err := rowToWorkflowRun(id, wfID, rid, status, inputsJSON, outputsJSON, nodeResJSON, totalTokens, durationMs, errorMessage, userID, createdAt)
		if err != nil {
			shared.GetLogger().Warn("解析执行记录行失败", zap.Error(err))
			return nil
		}
		runs = append(runs, run)
		return nil
	}, `SELECT id, workflow_id, run_id, status, inputs, outputs, node_results, total_tokens, duration_ms, error_message, user_id, created_at
		FROM workflow_runs WHERE workflow_id = ? ORDER BY created_at DESC LIMIT ?`, workflowID, limit)
	if err != nil {
		return nil, fmt.Errorf("查询执行记录列表失败: %w", err)
	}
	return runs, nil
}

// UpdateStatus 更新执行状态
func (r *WorkflowRunRepository) UpdateStatus(ctx context.Context, runID string, status workflow.RunStatus, outputs map[string]interface{}, nodeResults map[string]interface{}, totalTokens int, durationMs int64, errMsg string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	outputsJSON, _ := json.Marshal(outputs)
	nodeResJSON, _ := json.Marshal(nodeResults)

	_, err := r.db.Exec(ctx,
		`UPDATE workflow_runs SET status = ?, outputs = ?, node_results = ?, total_tokens = ?, duration_ms = ?, error_message = ? WHERE run_id = ?`,
		string(status), string(outputsJSON), string(nodeResJSON), totalTokens, durationMs, errMsg, runID,
	)
	if err != nil {
		return fmt.Errorf("更新执行状态失败: %w", err)
	}
	return nil
}

// rowToWorkflowRun 将数据库行数据转换为 WorkflowRun 领域对象
func rowToWorkflowRun(
	id, workflowID int64, runID, status, inputsJSON string,
	outputsJSON, nodeResJSON sql.NullString,
	totalTokens int, durationMs int64, errorMessage sql.NullString,
	userID int64, createdAt time.Time,
) (*workflow.WorkflowRun, error) {
	var inputs map[string]interface{}
	if inputsJSON != "" && inputsJSON != "null" {
		if err := json.Unmarshal([]byte(inputsJSON), &inputs); err != nil {
			return nil, fmt.Errorf("反序列化 inputs 失败: %w", err)
		}
	}

	var outputs map[string]interface{}
	if outputsJSON.Valid && outputsJSON.String != "" && outputsJSON.String != "null" {
		if err := json.Unmarshal([]byte(outputsJSON.String), &outputs); err != nil {
			return nil, fmt.Errorf("反序列化 outputs 失败: %w", err)
		}
	}

	var nodeResults map[string]interface{}
	if nodeResJSON.Valid && nodeResJSON.String != "" && nodeResJSON.String != "null" {
		if err := json.Unmarshal([]byte(nodeResJSON.String), &nodeResults); err != nil {
			return nil, fmt.Errorf("反序列化 node_results 失败: %w", err)
		}
	}

	errMsg := ""
	if errorMessage.Valid {
		errMsg = errorMessage.String
	}

	return &workflow.WorkflowRun{
		ID:           id,
		WorkflowID:   workflowID,
		RunID:        runID,
		Status:       workflow.RunStatus(status),
		Inputs:       inputs,
		Outputs:      outputs,
		NodeResults:  nodeResults,
		TotalTokens:  totalTokens,
		DurationMs:   durationMs,
		ErrorMessage: errMsg,
		UserID:       userID,
		CreatedAt:    createdAt.Format("2006-01-02 15:04:05"),
	}, nil
}
