// Package mysql 提供 Prompt 变量的 MySQL 持久化实现
package mysql

import (
	"context"
	"database/sql"

	"aiProject/internal/infrastructure/database"
	trmysql "git.code.oa.com/trpc-go/trpc-database/mysql"
)

// PromptVarsRepository MySQL 实现的 Prompt 变量仓储
type PromptVarsRepository struct {
	db trmysql.Client
}

// NewPromptVarsRepository 创建 MySQL Prompt 变量仓储
func NewPromptVarsRepository() *PromptVarsRepository {
	return &PromptVarsRepository{
		db: database.GetDB(),
	}
}

// GetUserVars 获取用户级变量
func (r *PromptVarsRepository) GetUserVars(ctx context.Context, userID int64) (map[string]string, error) {
	vars := make(map[string]string)
	err := r.db.Query(ctx, func(rows *sql.Rows) error {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return err
		}
		vars[key] = value
		return nil
	}, "SELECT var_key, var_value FROM prompt_vars_user WHERE user_id = ?", userID)
	if err != nil {
		return nil, err
	}
	return vars, nil
}

// SetUserVar 设置用户级变量（upsert）
func (r *PromptVarsRepository) SetUserVar(ctx context.Context, userID int64, key, value string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO prompt_vars_user (user_id, var_key, var_value) VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE var_value = VALUES(var_value), updated_at = NOW()`,
		userID, key, value)
	return err
}

// DeleteUserVar 删除用户级变量
func (r *PromptVarsRepository) DeleteUserVar(ctx context.Context, userID int64, key string) error {
	_, err := r.db.Exec(ctx,
		"DELETE FROM prompt_vars_user WHERE user_id = ? AND var_key = ?",
		userID, key)
	return err
}

// GetSessionVars 获取会话级变量
func (r *PromptVarsRepository) GetSessionVars(ctx context.Context, sessionID string) (map[string]string, error) {
	vars := make(map[string]string)
	err := r.db.Query(ctx, func(rows *sql.Rows) error {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return err
		}
		vars[key] = value
		return nil
	}, "SELECT var_key, var_value FROM prompt_vars_session WHERE session_id = ?", sessionID)
	if err != nil {
		return nil, err
	}
	return vars, nil
}

// SetSessionVar 设置会话级变量（upsert）
func (r *PromptVarsRepository) SetSessionVar(ctx context.Context, sessionID, key, value string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO prompt_vars_session (session_id, var_key, var_value) VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE var_value = VALUES(var_value), updated_at = NOW()`,
		sessionID, key, value)
	return err
}

// DeleteSessionVar 删除会话级变量
func (r *PromptVarsRepository) DeleteSessionVar(ctx context.Context, sessionID, key string) error {
	_, err := r.db.Exec(ctx,
		"DELETE FROM prompt_vars_session WHERE session_id = ? AND var_key = ?",
		sessionID, key)
	return err
}
