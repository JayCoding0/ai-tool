// Package eval 定义 Agent 评估体系的仓储接口
package eval

import "context"

// Repository 评估数据持久化仓储接口
type Repository interface {
	// ─── Dataset ───
	CreateDataset(ctx context.Context, d *Dataset) error
	ListDatasets(ctx context.Context, userID int64) ([]*Dataset, error)
	GetDataset(ctx context.Context, id int64) (*Dataset, error)
	DeleteDataset(ctx context.Context, id, userID int64) error

	// ─── Case ───
	CreateCase(ctx context.Context, c *Case) error
	ListCases(ctx context.Context, datasetID int64) ([]*Case, error)
	DeleteCase(ctx context.Context, id int64) error

	// ─── Run ───
	CreateRun(ctx context.Context, r *Run) error
	UpdateRun(ctx context.Context, r *Run) error
	ListRuns(ctx context.Context, datasetID, userID int64) ([]*Run, error)
	GetRun(ctx context.Context, id int64) (*Run, error)

	// ─── Result ───
	CreateResult(ctx context.Context, res *Result) error
	ListResults(ctx context.Context, runID int64) ([]*Result, error)
}
