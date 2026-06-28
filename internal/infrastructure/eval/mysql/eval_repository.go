// Package mysql 提供 Agent 评估体系的 MySQL 持久化实现
package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"aiProject/internal/domain/eval"
	"aiProject/internal/infrastructure/database"
	trmysql "git.code.oa.com/trpc-go/trpc-database/mysql"
)

// 确保实现了领域层仓储接口
var _ eval.Repository = (*EvalRepository)(nil)

// EvalRepository MySQL 实现的评估仓储
type EvalRepository struct {
	db trmysql.Client
}

// NewEvalRepository 创建 MySQL 评估仓储
func NewEvalRepository() *EvalRepository {
	return &EvalRepository{db: database.GetDB()}
}

// ─── Dataset ───────────────────────────────────────────────────────────────

// CreateDataset 创建数据集
func (r *EvalRepository) CreateDataset(ctx context.Context, d *eval.Dataset) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	d.CreatedAt = time.Now()
	res, err := r.db.Exec(ctx,
		`INSERT INTO eval_datasets (name, description, user_id, created_at) VALUES (?, ?, ?, ?)`,
		d.Name, d.Description, d.UserID, d.CreatedAt)
	if err != nil {
		return fmt.Errorf("创建数据集失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("获取数据集 ID 失败: %w", err)
	}
	d.ID = id
	return nil
}

// ListDatasets 列出用户的数据集（含用例数量）
func (r *EvalRepository) ListDatasets(ctx context.Context, userID int64) ([]*eval.Dataset, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var list []*eval.Dataset
	err := r.db.Query(ctx, func(rows *sql.Rows) error {
		var d eval.Dataset
		if err := rows.Scan(&d.ID, &d.Name, &d.Description, &d.UserID, &d.CreatedAt, &d.CaseCount); err != nil {
			return err
		}
		list = append(list, &d)
		return nil
	}, `SELECT d.id, d.name, d.description, d.user_id, d.created_at,
	           (SELECT COUNT(*) FROM eval_cases c WHERE c.dataset_id = d.id) AS case_count
	    FROM eval_datasets d WHERE d.user_id = ? ORDER BY d.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	return list, nil
}

// GetDataset 获取单个数据集
func (r *EvalRepository) GetDataset(ctx context.Context, id int64) (*eval.Dataset, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var d eval.Dataset
	dest := []interface{}{&d.ID, &d.Name, &d.Description, &d.UserID, &d.CreatedAt}
	err := r.db.QueryRow(ctx, dest,
		`SELECT id, name, description, user_id, created_at FROM eval_datasets WHERE id = ?`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("数据集不存在")
		}
		return nil, err
	}
	return &d, nil
}

// DeleteDataset 删除数据集（级联删除用例由外键 ON DELETE CASCADE 处理）
func (r *EvalRepository) DeleteDataset(ctx context.Context, id, userID int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := r.db.Exec(ctx, `DELETE FROM eval_datasets WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

// ─── Case ──────────────────────────────────────────────────────────────────

// CreateCase 创建用例
func (r *EvalRepository) CreateCase(ctx context.Context, c *eval.Case) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	c.CreatedAt = time.Now()
	res, err := r.db.Exec(ctx,
		`INSERT INTO eval_cases (dataset_id, input, expected, created_at) VALUES (?, ?, ?, ?)`,
		c.DatasetID, c.Input, c.Expected, c.CreatedAt)
	if err != nil {
		return fmt.Errorf("创建用例失败: %w", err)
	}
	id, _ := res.LastInsertId()
	c.ID = id
	return nil
}

// ListCases 列出数据集的所有用例
func (r *EvalRepository) ListCases(ctx context.Context, datasetID int64) ([]*eval.Case, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var list []*eval.Case
	err := r.db.Query(ctx, func(rows *sql.Rows) error {
		var c eval.Case
		if err := rows.Scan(&c.ID, &c.DatasetID, &c.Input, &c.Expected, &c.CreatedAt); err != nil {
			return err
		}
		list = append(list, &c)
		return nil
	}, `SELECT id, dataset_id, input, expected, created_at FROM eval_cases WHERE dataset_id = ? ORDER BY id ASC`, datasetID)
	if err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteCase 删除用例
func (r *EvalRepository) DeleteCase(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := r.db.Exec(ctx, `DELETE FROM eval_cases WHERE id = ?`, id)
	return err
}

// ─── Run ───────────────────────────────────────────────────────────────────

// CreateRun 创建评测运行
func (r *EvalRepository) CreateRun(ctx context.Context, run *eval.Run) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	toolsJSON, _ := json.Marshal(run.Tools)
	run.CreatedAt = time.Now()
	res, err := r.db.Exec(ctx,
		`INSERT INTO eval_runs (dataset_id, name, model_name, system_prompt, tools, scorer, judge_model, threshold, status, total_cases, passed_cases, avg_score, error_message, user_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.DatasetID, run.Name, run.ModelName, run.SystemPrompt, string(toolsJSON), string(run.Scorer), run.JudgeModel, run.Threshold,
		string(run.Status), run.TotalCases, run.PassedCases, run.AvgScore, run.ErrorMessage, run.UserID, run.CreatedAt)
	if err != nil {
		return fmt.Errorf("创建评测运行失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("获取运行 ID 失败: %w", err)
	}
	run.ID = id
	return nil
}

// UpdateRun 更新评测运行（状态、聚合指标）
func (r *EvalRepository) UpdateRun(ctx context.Context, run *eval.Run) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var finishedAt interface{}
	if run.FinishedAt != nil {
		finishedAt = *run.FinishedAt
	}
	_, err := r.db.Exec(ctx,
		`UPDATE eval_runs SET status = ?, passed_cases = ?, avg_score = ?, error_message = ?, finished_at = ? WHERE id = ?`,
		string(run.Status), run.PassedCases, run.AvgScore, run.ErrorMessage, finishedAt, run.ID)
	return err
}

// ListRuns 列出运行记录（按数据集过滤，datasetID<=0 表示该用户全部）
func (r *EvalRepository) ListRuns(ctx context.Context, datasetID, userID int64) ([]*eval.Run, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	query := `SELECT id, dataset_id, name, model_name, judge_model, scorer, threshold, status, total_cases, passed_cases, avg_score, error_message, user_id, created_at, finished_at
	          FROM eval_runs WHERE user_id = ?`
	args := []interface{}{userID}
	if datasetID > 0 {
		query += " AND dataset_id = ?"
		args = append(args, datasetID)
	}
	query += " ORDER BY id DESC"

	var list []*eval.Run
	err := r.db.Query(ctx, func(rows *sql.Rows) error {
		run, err := scanRunBrief(rows)
		if err != nil {
			return err
		}
		list = append(list, run)
		return nil
	}, query, args...)
	if err != nil {
		return nil, err
	}
	return list, nil
}

// GetRun 获取单个运行（含完整配置）
func (r *EvalRepository) GetRun(ctx context.Context, id int64) (*eval.Run, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var run eval.Run
	var toolsJSON string
	var errMsg sql.NullString
	var finishedAt sql.NullTime
	dest := []interface{}{
		&run.ID, &run.DatasetID, &run.Name, &run.ModelName, &run.SystemPrompt, &toolsJSON, &run.Scorer, &run.JudgeModel,
		&run.Threshold, &run.Status, &run.TotalCases, &run.PassedCases, &run.AvgScore, &errMsg, &run.UserID, &run.CreatedAt, &finishedAt,
	}
	err := r.db.QueryRow(ctx, dest,
		`SELECT id, dataset_id, name, model_name, system_prompt, tools, scorer, judge_model, threshold, status, total_cases, passed_cases, avg_score, error_message, user_id, created_at, finished_at
		 FROM eval_runs WHERE id = ?`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("评测运行不存在")
		}
		return nil, err
	}
	if toolsJSON != "" {
		json.Unmarshal([]byte(toolsJSON), &run.Tools) //nolint:errcheck
	}
	if errMsg.Valid {
		run.ErrorMessage = errMsg.String
	}
	if finishedAt.Valid {
		run.FinishedAt = &finishedAt.Time
	}
	return &run, nil
}

// scanRunBrief 扫描运行记录的简要字段（列表用，不含 system_prompt/tools）
func scanRunBrief(rows *sql.Rows) (*eval.Run, error) {
	var run eval.Run
	var errMsg sql.NullString
	var finishedAt sql.NullTime
	if err := rows.Scan(&run.ID, &run.DatasetID, &run.Name, &run.ModelName, &run.JudgeModel, &run.Scorer, &run.Threshold,
		&run.Status, &run.TotalCases, &run.PassedCases, &run.AvgScore, &errMsg, &run.UserID, &run.CreatedAt, &finishedAt); err != nil {
		return nil, err
	}
	if errMsg.Valid {
		run.ErrorMessage = errMsg.String
	}
	if finishedAt.Valid {
		run.FinishedAt = &finishedAt.Time
	}
	return &run, nil
}

// ─── Result ────────────────────────────────────────────────────────────────

// CreateResult 保存单条评测结果
func (r *EvalRepository) CreateResult(ctx context.Context, res *eval.Result) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res.CreatedAt = time.Now()
	result, err := r.db.Exec(ctx,
		`INSERT INTO eval_results (run_id, case_id, input, expected, actual, score, passed, reason, latency_ms, tokens, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		res.RunID, res.CaseID, res.Input, res.Expected, res.Actual, res.Score, res.Passed, res.Reason, res.LatencyMs, res.Tokens, res.CreatedAt)
	if err != nil {
		return fmt.Errorf("保存评测结果失败: %w", err)
	}
	id, _ := result.LastInsertId()
	res.ID = id
	return nil
}

// ListResults 列出某次运行的所有结果
func (r *EvalRepository) ListResults(ctx context.Context, runID int64) ([]*eval.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var list []*eval.Result
	err := r.db.Query(ctx, func(rows *sql.Rows) error {
		var res eval.Result
		if err := rows.Scan(&res.ID, &res.RunID, &res.CaseID, &res.Input, &res.Expected, &res.Actual,
			&res.Score, &res.Passed, &res.Reason, &res.LatencyMs, &res.Tokens, &res.CreatedAt); err != nil {
			return err
		}
		list = append(list, &res)
		return nil
	}, `SELECT id, run_id, case_id, input, expected, actual, score, passed, reason, latency_ms, tokens, created_at
	    FROM eval_results WHERE run_id = ? ORDER BY id ASC`, runID)
	if err != nil {
		return nil, err
	}
	return list, nil
}
