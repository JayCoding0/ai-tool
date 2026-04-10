package mysql

import (
	"context"
	"database/sql"
	"time"

	"aiProject/internal/domain/memory"
	"aiProject/internal/infrastructure/database"
	trmysql "git.code.oa.com/trpc-go/trpc-database/mysql"
)

// MemoryRepository MySQL 记忆仓储实现
type MemoryRepository struct {
	db trmysql.Client
}

// NewMemoryRepository 创建记忆仓储
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{db: database.GetDB()}
}

// CreateMemory 创建记忆
func (r *MemoryRepository) CreateMemory(ctx context.Context, m *memory.Memory) error {
	result, err := r.db.Exec(ctx,
		`INSERT INTO user_memories (user_id, content, embedding, memory_type, source_session_id, importance, access_count)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.UserID, m.Content, m.Embedding, string(m.MemoryType),
		m.SourceSessionID, m.Importance, m.AccessCount,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	m.ID = id
	m.CreatedAt = time.Now()
	m.UpdatedAt = time.Now()
	return nil
}

// UpdateMemory 更新记忆内容和向量
func (r *MemoryRepository) UpdateMemory(ctx context.Context, m *memory.Memory) error {
	_, err := r.db.Exec(ctx,
		`UPDATE user_memories SET content = ?, embedding = ?, memory_type = ?, importance = ? WHERE id = ?`,
		m.Content, m.Embedding, string(m.MemoryType), m.Importance, m.ID,
	)
	return err
}

// DeleteMemory 删除记忆
func (r *MemoryRepository) DeleteMemory(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM user_memories WHERE id = ?`, id)
	return err
}

// GetMemory 根据 ID 获取记忆
func (r *MemoryRepository) GetMemory(ctx context.Context, id int64) (*memory.Memory, error) {
	var m memory.Memory
	var memType string
	var expiredAt sql.NullTime
	var sourceSessionID sql.NullString

	dest := []interface{}{
		&m.ID, &m.UserID, &m.Content, &m.Embedding, &memType,
		&sourceSessionID, &m.Importance, &m.AccessCount,
		&m.CreatedAt, &m.UpdatedAt, &expiredAt,
	}
	err := r.db.QueryRow(ctx, dest,
		`SELECT id, user_id, content, embedding, memory_type, source_session_id,
		        importance, access_count, created_at, updated_at, expired_at
		 FROM user_memories WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	m.MemoryType = memory.MemoryType(memType)
	if sourceSessionID.Valid {
		m.SourceSessionID = sourceSessionID.String
	}
	if expiredAt.Valid {
		m.ExpiredAt = &expiredAt.Time
	}
	return &m, nil
}

// ListByUser 列出用户的所有记忆（分页，按重要性降序）
func (r *MemoryRepository) ListByUser(ctx context.Context, userID int64, offset, limit int) ([]*memory.Memory, error) {
	if limit <= 0 {
		limit = 20
	}
	var list []*memory.Memory
	err := r.db.Query(ctx, func(rows *sql.Rows) error {
		var m memory.Memory
		var memType string
		var expiredAt sql.NullTime
		var sourceSessionID sql.NullString
		if err := rows.Scan(&m.ID, &m.UserID, &m.Content, &memType,
			&sourceSessionID, &m.Importance, &m.AccessCount,
			&m.CreatedAt, &m.UpdatedAt, &expiredAt); err != nil {
			return err
		}
		m.MemoryType = memory.MemoryType(memType)
		if sourceSessionID.Valid {
			m.SourceSessionID = sourceSessionID.String
		}
		if expiredAt.Valid {
			m.ExpiredAt = &expiredAt.Time
		}
		list = append(list, &m)
		return nil
	}, `SELECT id, user_id, content, memory_type, source_session_id,
	           importance, access_count, created_at, updated_at, expired_at
	    FROM user_memories WHERE user_id = ?
	    ORDER BY importance DESC, updated_at DESC
	    LIMIT ? OFFSET ?`, userID, limit, offset)
	return list, err
}

// ListByUserAndType 按类型列出用户记忆
func (r *MemoryRepository) ListByUserAndType(ctx context.Context, userID int64, memoryType memory.MemoryType) ([]*memory.Memory, error) {
	var list []*memory.Memory
	err := r.db.Query(ctx, func(rows *sql.Rows) error {
		var m memory.Memory
		var memType string
		var expiredAt sql.NullTime
		var sourceSessionID sql.NullString
		if err := rows.Scan(&m.ID, &m.UserID, &m.Content, &memType,
			&sourceSessionID, &m.Importance, &m.AccessCount,
			&m.CreatedAt, &m.UpdatedAt, &expiredAt); err != nil {
			return err
		}
		m.MemoryType = memory.MemoryType(memType)
		if sourceSessionID.Valid {
			m.SourceSessionID = sourceSessionID.String
		}
		if expiredAt.Valid {
			m.ExpiredAt = &expiredAt.Time
		}
		list = append(list, &m)
		return nil
	}, `SELECT id, user_id, content, memory_type, source_session_id,
	           importance, access_count, created_at, updated_at, expired_at
	    FROM user_memories WHERE user_id = ? AND memory_type = ?
	    ORDER BY importance DESC, updated_at DESC`, userID, string(memoryType))
	return list, err
}

// CountByUser 统计用户记忆总数
func (r *MemoryRepository) CountByUser(ctx context.Context, userID int64) (int, error) {
	var count int
	dest := []interface{}{&count}
	err := r.db.QueryRow(ctx, dest, `SELECT COUNT(*) FROM user_memories WHERE user_id = ?`, userID)
	return count, err
}

// ListAllEmbeddings 加载用户所有记忆（含向量，用于相似度比对）
func (r *MemoryRepository) ListAllEmbeddings(ctx context.Context, userID int64) ([]*memory.Memory, error) {
	var list []*memory.Memory
	err := r.db.Query(ctx, func(rows *sql.Rows) error {
		var m memory.Memory
		var memType string
		if err := rows.Scan(&m.ID, &m.Content, &m.Embedding, &memType, &m.Importance, &m.AccessCount); err != nil {
			return err
		}
		m.MemoryType = memory.MemoryType(memType)
		m.UserID = userID
		list = append(list, &m)
		return nil
	}, `SELECT id, content, embedding, memory_type, importance, access_count
	    FROM user_memories WHERE user_id = ? AND embedding IS NOT NULL
	    ORDER BY importance DESC`, userID)
	return list, err
}

// IncrementAccessCount 增加记忆的检索命中次数
func (r *MemoryRepository) IncrementAccessCount(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE user_memories SET access_count = access_count + 1 WHERE id = ?`, id)
	return err
}

// BatchDecayImportance 批量衰减长期未访问的记忆重要性
func (r *MemoryRepository) BatchDecayImportance(ctx context.Context, userID int64, cutoff string, decayAmount float64) (int64, error) {
	result, err := r.db.Exec(ctx,
		`UPDATE user_memories
		 SET importance = GREATEST(importance - ?, 0), access_count = 0
		 WHERE user_id = ? AND access_count = 0 AND updated_at < ?`,
		decayAmount, userID, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DeleteExpiredMemories 删除 importance 低于阈值的过期记忆
func (r *MemoryRepository) DeleteExpiredMemories(ctx context.Context, userID int64, minImportance float64) (int64, error) {
	result, err := r.db.Exec(ctx,
		`DELETE FROM user_memories WHERE user_id = ? AND importance < ?`,
		userID, minImportance)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DeleteLowestImportance 当记忆数超限时，删除重要性最低的记忆
func (r *MemoryRepository) DeleteLowestImportance(ctx context.Context, userID int64, deleteCount int) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM user_memories WHERE user_id = ? ORDER BY importance ASC, updated_at ASC LIMIT ?`,
		userID, deleteCount)
	return err
}

// 确保实现了 memory.Repository 接口
var _ memory.Repository = (*MemoryRepository)(nil)
