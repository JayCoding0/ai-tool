package session

import (
	"context"

	"aiProject/internal/domain/model"
)

// ModelTokenStat 模型 token 消耗统计
type ModelTokenStat struct {
	ModelName        string `json:"model_name"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	MessageCount     int    `json:"message_count"`
}

// Repository 会话仓库接口
type Repository interface {
	// Save 保存会话
	Save(session *Session) error

	// FindByID 根据ID查找会话
	FindByID(id SessionID) (*Session, error)

	// Delete 删除会话
	Delete(id SessionID) error

	// Exists 检查会话是否存在
	Exists(id SessionID) bool

	// SaveMessageWithModel 保存消息（带用户ID和模型名称）
	SaveMessageWithModel(ctx context.Context, sessID SessionID, role, content string, userID int64, modelName string) error
	// SaveMessageWithTokens 保存消息（带用户ID、模型名称和 token 用量）
	SaveMessageWithTokens(ctx context.Context, sessID SessionID, role, content string, userID int64, modelName string, usage model.TokenUsage) error
	GetSessionHistory(ctx context.Context, sessID SessionID) ([]Message, error)
	ListSessions(ctx context.Context) ([]SessionInfo, error)
	// ListSessionsByUser 列出指定用户的会话
	ListSessionsByUser(ctx context.Context, userID int64) ([]SessionInfo, error)
	DeleteSession(ctx context.Context, sessID SessionID) error
	// GetSessionOwner 获取会话归属的用户 ID（用于越权校验）。
	// 返回 0 表示会话不存在或为匿名/内存会话。
	GetSessionOwner(ctx context.Context, sessID SessionID) (int64, error)
	// GetSessionTotalTokens 获取指定 session 的累计 token 数
	GetSessionTotalTokens(ctx context.Context, sessID SessionID) (int, error)
	// GetModelTokenStats 获取各模型 token 消耗统计（userID=0 时统计所有用户）
	GetModelTokenStats(ctx context.Context, userID int64) ([]ModelTokenStat, error)
	// GetUserTotalTokens 获取指定用户累计消耗的 token 总数
	GetUserTotalTokens(ctx context.Context, userID int64) (int, error)
	// UpdateSessionTitle 更新会话标题
	UpdateSessionTitle(ctx context.Context, sessID SessionID, title string) error
	// UpdateSessionSystemPrompt 更新会话的 System Prompt
	UpdateSessionSystemPrompt(ctx context.Context, sessID SessionID, systemPrompt string) error
	// GetSessionSystemPrompt 获取会话的 System Prompt
	GetSessionSystemPrompt(ctx context.Context, sessID SessionID) (string, error)

	// GetSessionSummary 获取会话摘要
	GetSessionSummary(ctx context.Context, sessID SessionID) (string, error)
	// UpdateSessionSummary 更新会话摘要
	UpdateSessionSummary(ctx context.Context, sessID SessionID, summary string) error
	// GetSessionMessageCount 获取会话消息数量
	GetSessionMessageCount(ctx context.Context, sessID SessionID) (int, error)
}