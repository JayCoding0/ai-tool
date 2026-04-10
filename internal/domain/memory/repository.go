package memory

import "context"

// Repository 记忆仓储接口
type Repository interface {
	// CreateMemory 创建记忆
	CreateMemory(ctx context.Context, m *Memory) error
	// UpdateMemory 更新记忆内容和向量
	UpdateMemory(ctx context.Context, m *Memory) error
	// DeleteMemory 删除记忆
	DeleteMemory(ctx context.Context, id int64) error
	// GetMemory 根据 ID 获取记忆
	GetMemory(ctx context.Context, id int64) (*Memory, error)
	// ListByUser 列出用户的所有记忆（分页）
	ListByUser(ctx context.Context, userID int64, offset, limit int) ([]*Memory, error)
	// ListByUserAndType 按类型列出用户记忆
	ListByUserAndType(ctx context.Context, userID int64, memoryType MemoryType) ([]*Memory, error)
	// CountByUser 统计用户记忆总数
	CountByUser(ctx context.Context, userID int64) (int, error)
	// ListAllEmbeddings 加载用户所有记忆的向量（用于相似度比对）
	ListAllEmbeddings(ctx context.Context, userID int64) ([]*Memory, error)
	// IncrementAccessCount 增加记忆的检索命中次数
	IncrementAccessCount(ctx context.Context, id int64) error
	// BatchDecayImportance 批量衰减长期未访问的记忆重要性
	// 将 access_count=0 且 updated_at 早于 cutoff 的记忆 importance 减少 decayAmount
	BatchDecayImportance(ctx context.Context, userID int64, cutoff string, decayAmount float64) (int64, error)
	// DeleteExpiredMemories 删除 importance 低于阈值的过期记忆
	DeleteExpiredMemories(ctx context.Context, userID int64, minImportance float64) (int64, error)
	// DeleteLowestImportance 当记忆数超限时，删除重要性最低的记忆
	DeleteLowestImportance(ctx context.Context, userID int64, deleteCount int) error
}
