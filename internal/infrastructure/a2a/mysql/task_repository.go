package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	domain_a2a "aiProject/internal/domain/a2a"
	"aiProject/internal/infrastructure/database"
	"aiProject/internal/shared"
	trmysql "git.code.oa.com/trpc-go/trpc-database/mysql"
	"go.uber.org/zap"
)

// TaskRepository MySQL 任务存储库，实现 infra_a2a.TaskRepository 接口
type TaskRepository struct {
	db trmysql.Client
}

// NewTaskRepository 创建 MySQL 任务存储库
func NewTaskRepository() *TaskRepository {
	return &TaskRepository{
		db: database.GetDB(),
	}
}

// Save 保存或更新任务（UPSERT）
func (r *TaskRepository) Save(task *domain_a2a.Task) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	inputJSON, err := json.Marshal(task.Input)
	if err != nil {
		return fmt.Errorf("序列化 input 失败: %w", err)
	}

	outputJSON, err := json.Marshal(task.Output)
	if err != nil {
		return fmt.Errorf("序列化 output 失败: %w", err)
	}

	metadataJSON, err := json.Marshal(task.Metadata)
	if err != nil {
		return fmt.Errorf("序列化 metadata 失败: %w", err)
	}

	query := `
		INSERT INTO a2a_tasks (id, session_id, state, state_message, input_json, output_json, metadata_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			state = VALUES(state),
			state_message = VALUES(state_message),
			output_json = VALUES(output_json),
			metadata_json = VALUES(metadata_json),
			updated_at = VALUES(updated_at)
	`
	_, err = r.db.Exec(ctx, query,
		task.ID,
		task.SessionID,
		string(task.Status.State),
		task.Status.Message,
		string(inputJSON),
		string(outputJSON),
		string(metadataJSON),
		task.CreatedAt,
		task.UpdatedAt,
	)
	if err != nil {
		shared.GetLogger().Error("保存 A2A 任务失败",
			zap.String("task_id", task.ID),
			zap.Error(err),
		)
		return fmt.Errorf("保存任务失败: %w", err)
	}
	return nil
}

// Get 根据 ID 获取任务
func (r *TaskRepository) Get(id string) (*domain_a2a.Task, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		taskID       string
		sessionID    string
		state        string
		stateMessage string
		inputJSON    string
		outputJSON   sql.NullString
		metadataJSON sql.NullString
		createdAt    time.Time
		updatedAt    time.Time
	)

	dest := []interface{}{
		&taskID, &sessionID, &state, &stateMessage,
		&inputJSON, &outputJSON, &metadataJSON,
		&createdAt, &updatedAt,
	}

	err := r.db.QueryRow(ctx, dest,
		`SELECT id, session_id, state, state_message, input_json, output_json, metadata_json, created_at, updated_at
		 FROM a2a_tasks WHERE id = ?`, id)
	if err != nil {
		// 记录非"未找到"的错误
		if err.Error() != "sql: no rows in result set" {
			shared.GetLogger().Error("查询 A2A 任务失败",
				zap.String("task_id", id),
				zap.Error(err),
			)
		}
		return nil, false
	}

	task, err := rowToTask(taskID, sessionID, state, stateMessage, inputJSON, outputJSON, metadataJSON, createdAt, updatedAt)
	if err != nil {
		shared.GetLogger().Error("解析 A2A 任务失败",
			zap.String("task_id", id),
			zap.Error(err),
		)
		return nil, false
	}
	return task, true
}

// Delete 删除任务
func (r *TaskRepository) Delete(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.db.Exec(ctx, "DELETE FROM a2a_tasks WHERE id = ?", id)
	return err
}

// List 列出所有任务（按创建时间倒序，最多返回 100 条）
func (r *TaskRepository) List() ([]*domain_a2a.Task, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var tasks []*domain_a2a.Task
	err := r.db.Query(ctx, func(rows *sql.Rows) error {
		var (
			taskID       string
			sessionID    string
			state        string
			stateMessage string
			inputJSON    string
			outputJSON   sql.NullString
			metadataJSON sql.NullString
			createdAt    time.Time
			updatedAt    time.Time
		)
		if err := rows.Scan(&taskID, &sessionID, &state, &stateMessage,
			&inputJSON, &outputJSON, &metadataJSON, &createdAt, &updatedAt); err != nil {
			return err
		}
		task, err := rowToTask(taskID, sessionID, state, stateMessage, inputJSON, outputJSON, metadataJSON, createdAt, updatedAt)
		if err != nil {
			shared.GetLogger().Warn("解析任务行失败", zap.Error(err))
			return nil // 跳过解析失败的行，不中断整体查询
		}
		tasks = append(tasks, task)
		return nil
	}, `SELECT id, session_id, state, state_message, input_json, output_json, metadata_json, created_at, updated_at
		FROM a2a_tasks ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return nil, fmt.Errorf("查询任务列表失败: %w", err)
	}
	return tasks, nil
}

// rowToTask 将数据库行数据转换为 Task 领域对象
func rowToTask(
	id, sessionID, state, stateMessage string,
	inputJSON string,
	outputJSON, metadataJSON sql.NullString,
	createdAt, updatedAt time.Time,
) (*domain_a2a.Task, error) {
	var input domain_a2a.TaskMessage
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		return nil, fmt.Errorf("反序列化 input 失败: %w", err)
	}

	var output []domain_a2a.TaskMessage
	if outputJSON.Valid && outputJSON.String != "" && outputJSON.String != "null" {
		if err := json.Unmarshal([]byte(outputJSON.String), &output); err != nil {
			return nil, fmt.Errorf("反序列化 output 失败: %w", err)
		}
	}
	if output == nil {
		output = []domain_a2a.TaskMessage{}
	}

	var metadata map[string]interface{}
	if metadataJSON.Valid && metadataJSON.String != "" && metadataJSON.String != "null" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err != nil {
			return nil, fmt.Errorf("反序列化 metadata 失败: %w", err)
		}
	}

	return &domain_a2a.Task{
		ID:        id,
		SessionID: sessionID,
		Status: domain_a2a.TaskStatus{
			State:     domain_a2a.TaskState(state),
			Message:   stateMessage,
			Timestamp: updatedAt,
		},
		Input:     input,
		Output:    output,
		Metadata:  metadata,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}
