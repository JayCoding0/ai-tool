package mysql

import (
	"context"
	"database/sql"
	"time"

	"aiProject/internal/domain/model"
	"aiProject/internal/domain/session"
	"aiProject/internal/infrastructure/database"
	"aiProject/internal/shared"
	trmysql "git.code.oa.com/trpc-go/trpc-database/mysql"
	"go.uber.org/zap"
)

// MySQLRepository MySQL会话存储库
type MySQLRepository struct {
	db trmysql.Client
}

// NewMySQLRepository 创建MySQL会话存储库
func NewMySQLRepository() *MySQLRepository {
	return &MySQLRepository{
		db: database.GetDB(),
	}
}

// SaveMessageWithModel 保存消息到数据库（带用户ID和模型名称）
func (r *MySQLRepository) SaveMessageWithModel(ctx context.Context, sessID session.SessionID, role, content string, userID int64, modelName string) error {
	return r.SaveMessageWithTokens(ctx, sessID, role, content, userID, modelName, model.TokenUsage{})
}

// SaveMessageWithTokens 保存消息到数据库（带用户ID、模型名称和 token 用量）
func (r *MySQLRepository) SaveMessageWithTokens(ctx context.Context, sessID session.SessionID, role, content string, userID int64, modelName string, usage model.TokenUsage) error {
	// 使用事务确保会话创建和消息插入的原子性
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		// 检查会话是否已存在
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM chat_sessions WHERE id = ?)", string(sessID)).Scan(&exists); err != nil {
			return err
		}

		if !exists {
			// 创建新会话，使用第一条消息作为标题（截断到255字符）
			title := []rune(content)
			if len(title) > 255 {
				title = title[:255]
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO chat_sessions (id, user_id, title, model_name) VALUES (?, ?, ?, ?)",
				string(sessID), userID, string(title), modelName); err != nil {
				return err
			}
		} else if modelName != "" {
			// 更新会话的模型名称
			if _, err := tx.ExecContext(ctx,
				"UPDATE chat_sessions SET model_name = ? WHERE id = ?",
				modelName, string(sessID)); err != nil {
				return err
			}
		}

		// 插入消息（含 token 用量）
		_, err := tx.ExecContext(ctx,
			"INSERT INTO chat_messages (session_id, user_id, role, model_name, content, prompt_tokens, completion_tokens, total_tokens) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			string(sessID), userID, role, modelName, content, usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
		if err != nil {
			shared.GetLogger().Error("保存消息失败",
				zap.String("session_id", string(sessID)),
				zap.String("role", role),
				zap.Error(err),
			)
			return err
		}
		return nil
	})
}


// Save 保存会话（实现Repository接口）
func (r *MySQLRepository) Save(sess *session.Session) error {
	// 对于MySQL实现，会话通过SaveMessage自动创建
	// 这里只需要确保会话存在即可
	return nil
}

// FindByID 根据ID查找会话（实现Repository接口）
func (r *MySQLRepository) FindByID(id session.SessionID) (*session.Session, error) {
	ctx := context.Background()

	// 从数据库加载历史消息，重建会话
	messages, err := r.GetSessionHistory(ctx, id)
	if err != nil {
		return nil, err
	}

	sess := session.NewSessionWithID(id)
	for _, msg := range messages {
		sess.AddMessage(msg.Role, msg.Content)
	}
	return sess, nil
}

// Delete 删除会话（实现Repository接口）
func (r *MySQLRepository) Delete(id session.SessionID) error {
	return r.DeleteSession(context.Background(), id)
}

// Exists 检查会话是否存在（实现Repository接口）
func (r *MySQLRepository) Exists(id session.SessionID) bool {
	var exists bool
	dest := []interface{}{&exists}
	err := r.db.QueryRow(context.Background(),
		dest, "SELECT EXISTS(SELECT 1 FROM chat_sessions WHERE id = ?)", string(id))
	return err == nil && exists
}

// GetSessionHistory 获取会话历史记录
func (r *MySQLRepository) GetSessionHistory(ctx context.Context, sessID session.SessionID) ([]session.Message, error) {
	query := `
		SELECT role, model_name, content, created_at 
		FROM chat_messages 
		WHERE session_id = ? 
		ORDER BY created_at ASC
	`

	var messages []session.Message
	err := r.db.Query(ctx, func(rows *sql.Rows) error {
		var role, modelName, content string
		var createdAt time.Time
		if err := rows.Scan(&role, &modelName, &content, &createdAt); err != nil {
			return err
		}
		messages = append(messages, session.Message{
			Role:      role,
			ModelName: modelName,
			Content:   content,
			Time:      createdAt,
		})
		return nil
	}, query, string(sessID))
	if err != nil {
		return nil, err
	}

	return messages, nil
}

// ListSessions 列出所有会话（兼容旧接口）
func (r *MySQLRepository) ListSessions(ctx context.Context) ([]session.SessionInfo, error) {
	return r.ListSessionsByUser(ctx, 0)
}

// ListSessionsByUser 列出指定用户的会话（userID=0时列出所有），支持分页
func (r *MySQLRepository) ListSessionsByUser(ctx context.Context, userID int64) ([]session.SessionInfo, error) {
	return r.ListSessionsByUserPaged(ctx, userID, 50, 0)
}

// ListSessionsByUserPaged 列出指定用户的会话，支持分页（limit=0时默认50条）
func (r *MySQLRepository) ListSessionsByUserPaged(ctx context.Context, userID int64, limit, offset int) ([]session.SessionInfo, error) {
	if limit <= 0 {
		limit = 50
	}
	var query string
	var args []interface{}
	if userID > 0 {
		query = `SELECT id, title, model_name, COALESCE(system_prompt, '') as system_prompt, created_at, updated_at FROM chat_sessions WHERE user_id = ? ORDER BY updated_at DESC LIMIT ? OFFSET ?`
		args = append(args, userID, limit, offset)
	} else {
		query = `SELECT id, title, model_name, COALESCE(system_prompt, '') as system_prompt, created_at, updated_at FROM chat_sessions ORDER BY updated_at DESC LIMIT ? OFFSET ?`
		args = append(args, limit, offset)
	}

	var sessions []session.SessionInfo
	err := r.db.Query(ctx, func(rows *sql.Rows) error {
		var info session.SessionInfo
		if err := rows.Scan(&info.ID, &info.Title, &info.ModelName, &info.SystemPrompt, &info.CreatedAt, &info.UpdatedAt); err != nil {
			return err
		}
		sessions = append(sessions, info)
		return nil
	}, query, args...)
	if err != nil {
		return nil, err
	}

	return sessions, nil
}

// DeleteSession 删除会话及其消息
func (r *MySQLRepository) DeleteSession(ctx context.Context, sessID session.SessionID) error {
	_, err := r.db.Exec(ctx, "DELETE FROM chat_sessions WHERE id = ?", string(sessID))
	return err
}

// CleanGuestSessions 清理游客会话（user_id = 0 的记录）
func (r *MySQLRepository) CleanGuestSessions(ctx context.Context) (int64, error) {
	result, err := r.db.Exec(ctx, "DELETE FROM chat_sessions WHERE user_id = 0")
	if err != nil {
		return 0, err
	}
	affected, _ := result.RowsAffected()
	return affected, nil
}

// GetSessionTotalTokens 获取指定 session 的累计 token 数
func (r *MySQLRepository) GetSessionTotalTokens(ctx context.Context, sessID session.SessionID) (int, error) {
	var total int
	dest := []interface{}{&total}
	err := r.db.QueryRow(ctx, dest,
		"SELECT COALESCE(SUM(total_tokens), 0) FROM chat_messages WHERE session_id = ?",
		string(sessID))
	if err != nil {
		return 0, err
	}
	return total, nil
}

// GetUserTotalTokens 获取指定用户累计消耗的 token 总数
func (r *MySQLRepository) GetUserTotalTokens(ctx context.Context, userID int64) (int, error) {
	var total int
	dest := []interface{}{&total}
	err := r.db.QueryRow(ctx, dest,
		"SELECT COALESCE(SUM(total_tokens), 0) FROM chat_messages WHERE user_id = ? AND role = 'ai'",
		userID)
	if err != nil {
		return 0, err
	}
	return total, nil
}

// UpdateSessionTitle 更新会话标题
func (r *MySQLRepository) UpdateSessionTitle(ctx context.Context, sessID session.SessionID, title string) error {
	_, err := r.db.Exec(ctx, "UPDATE chat_sessions SET title = ? WHERE id = ?", title, string(sessID))
	return err
}

// UpdateSessionSystemPrompt 更新会话的 System Prompt
func (r *MySQLRepository) UpdateSessionSystemPrompt(ctx context.Context, sessID session.SessionID, systemPrompt string) error {
	_, err := r.db.Exec(ctx, "UPDATE chat_sessions SET system_prompt = ? WHERE id = ?", systemPrompt, string(sessID))
	return err
}

// GetSessionSystemPrompt 获取会话的 System Prompt
func (r *MySQLRepository) GetSessionSystemPrompt(ctx context.Context, sessID session.SessionID) (string, error) {
	var prompt string
	dest := []interface{}{&prompt}
	err := r.db.QueryRow(ctx, dest,
		"SELECT COALESCE(system_prompt, '') FROM chat_sessions WHERE id = ?",
		string(sessID))
	if err != nil {
		return "", err
	}
	return prompt, nil
}

// GetModelTokenStats 获取各模型 token 消耗统计（userID=0 时统计所有用户）
func (r *MySQLRepository) GetModelTokenStats(ctx context.Context, userID int64) ([]session.ModelTokenStat, error) {
	var query string
	var args []interface{}
	if userID > 0 {
		query = `
			SELECT 
				COALESCE(model_name, '') as model_name,
				COALESCE(SUM(prompt_tokens), 0) as prompt_tokens,
				COALESCE(SUM(completion_tokens), 0) as completion_tokens,
				COALESCE(SUM(total_tokens), 0) as total_tokens,
				COUNT(*) as message_count
			FROM chat_messages
			WHERE model_name != '' AND role = 'ai' AND user_id = ?
			GROUP BY model_name
			ORDER BY total_tokens DESC
		`
		args = append(args, userID)
	} else {
		query = `
			SELECT 
				COALESCE(model_name, '') as model_name,
				COALESCE(SUM(prompt_tokens), 0) as prompt_tokens,
				COALESCE(SUM(completion_tokens), 0) as completion_tokens,
				COALESCE(SUM(total_tokens), 0) as total_tokens,
				COUNT(*) as message_count
			FROM chat_messages
			WHERE model_name != '' AND role = 'ai'
			GROUP BY model_name
			ORDER BY total_tokens DESC
		`
	}
	var stats []session.ModelTokenStat
	err := r.db.Query(ctx, func(rows *sql.Rows) error {
		var stat session.ModelTokenStat
		if err := rows.Scan(&stat.ModelName, &stat.PromptTokens, &stat.CompletionTokens, &stat.TotalTokens, &stat.MessageCount); err != nil {
			return err
		}
		stats = append(stats, stat)
		return nil
	}, query, args...)
	if err != nil {
		return nil, err
	}
	return stats, nil
}