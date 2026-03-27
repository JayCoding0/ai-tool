package session

import (
	"context"
	"sync"

	"aiProject/internal/domain/model"
	"aiProject/internal/domain/session"
)

// MemoryRepository 内存会话仓库
type MemoryRepository struct {
	sessions map[session.SessionID]*session.Session
	mu       sync.RWMutex
}

// 确保MemoryRepository实现了session.Repository接口
var _ session.Repository = (*MemoryRepository)(nil)

// NewMemoryRepository 创建内存会话仓库
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		sessions: make(map[session.SessionID]*session.Session),
	}
}

// Save 保存会话
func (r *MemoryRepository) Save(sess *session.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.sessions[sess.ID()] = sess
	return nil
}

// FindByID 根据ID查找会话
func (r *MemoryRepository) FindByID(id session.SessionID) (*session.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sess, exists := r.sessions[id]
	if !exists {
		return nil, nil
	}
	return sess, nil
}

// Delete 删除会话
func (r *MemoryRepository) Delete(id session.SessionID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.sessions, id)
	return nil
}

// Exists 检查会话是否存在
func (r *MemoryRepository) Exists(id session.SessionID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.sessions[id]
	return exists
}

// SaveMessageWithModel 保存消息（内存实现忽略userID和modelName）
func (r *MemoryRepository) SaveMessageWithModel(ctx context.Context, sessID session.SessionID, role, content string, userID int64, modelName string) error {
	return nil
}

// SaveMessageWithTokens 保存消息（内存实现忽略token信息）
func (r *MemoryRepository) SaveMessageWithTokens(ctx context.Context, sessID session.SessionID, role, content string, userID int64, modelName string, usage model.TokenUsage) error {
	return nil
}

// GetSessionHistory 获取会话历史记录
func (r *MemoryRepository) GetSessionHistory(ctx context.Context, sessID session.SessionID) ([]session.Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sess, exists := r.sessions[sessID]
	if !exists {
		return nil, nil
	}

	return sess.GetHistory(), nil
}

// ListSessions 列出所有会话
func (r *MemoryRepository) ListSessions(ctx context.Context) ([]session.SessionInfo, error) {
	return r.ListSessionsByUser(ctx, 0)
}

// ListSessionsByUser 列出指定用户的会话（内存实现忽略userID）
func (r *MemoryRepository) ListSessionsByUser(ctx context.Context, userID int64) ([]session.SessionInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sessions []session.SessionInfo
	for _, sess := range r.sessions {
		sessions = append(sessions, session.SessionInfo{
			ID:        sess.ID(),
			Title:     "内存会话", // 内存会话没有标题，使用默认值
			CreatedAt: sess.CreatedAt(),
			UpdatedAt: sess.UpdatedAt(),
		})
	}

	return sessions, nil
}

// DeleteSession 删除会话
func (r *MemoryRepository) DeleteSession(ctx context.Context, sessID session.SessionID) error {
	return r.Delete(sessID)
}

// GetSessionTotalTokens 内存实现返回0
func (r *MemoryRepository) GetSessionTotalTokens(ctx context.Context, sessID session.SessionID) (int, error) {
	return 0, nil
}

// GetModelTokenStats 内存实现返回空列表
func (r *MemoryRepository) GetModelTokenStats(ctx context.Context, userID int64) ([]session.ModelTokenStat, error) {
	return nil, nil
}

// GetUserTotalTokens 内存实现返回0
func (r *MemoryRepository) GetUserTotalTokens(ctx context.Context, userID int64) (int, error) {
	return 0, nil
}

// UpdateSessionTitle 内存实现（无持久化，忽略）
func (r *MemoryRepository) UpdateSessionTitle(ctx context.Context, sessID session.SessionID, title string) error {
	return nil
}

// UpdateSessionSystemPrompt 内存实现（无持久化，忽略）
func (r *MemoryRepository) UpdateSessionSystemPrompt(ctx context.Context, sessID session.SessionID, systemPrompt string) error {
	return nil
}

// GetSessionSystemPrompt 内存实现返回空字符串
func (r *MemoryRepository) GetSessionSystemPrompt(ctx context.Context, sessID session.SessionID) (string, error) {
	return "", nil
}