package cleaner

import (
	"context"

	"aiProject/internal/infrastructure/database"
	"go.uber.org/zap"
)

// CleanGuestSessions 启动时清理过期的游客会话和消息（只清理 24 小时以上无活动的记录，而非全量清理）
func CleanGuestSessions(logger *zap.Logger) {
	db := database.GetDB()
	if db == nil {
		return
	}
	ctx := context.Background()
	// 先清理过期的游客消息（24小时以上无活动）
	msgResult, err := db.Exec(ctx,
		`DELETE m FROM chat_messages m
		 INNER JOIN chat_sessions s ON m.session_id = s.id
		 WHERE s.user_id = 0 AND s.updated_at < DATE_SUB(NOW(), INTERVAL 24 HOUR)`)
	if err != nil {
		logger.Warn("启动清理过期游客消息失败", zap.Error(err))
	} else {
		affected, _ := msgResult.RowsAffected()
		if affected > 0 {
			logger.Info("启动清理过期游客消息", zap.Int64("count", affected))
		}
	}
	// 再清理过期的游客会话
	result, err := db.Exec(ctx,
		`DELETE FROM chat_sessions WHERE user_id = 0 AND updated_at < DATE_SUB(NOW(), INTERVAL 24 HOUR)`)
	if err != nil {
		logger.Warn("启动清理过期游客会话失败", zap.Error(err))
		return
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		logger.Info("启动清理过期游客会话", zap.Int64("count", affected))
	}
}

// GuestSessionCleaner 游客会话定时清理器，实现 timer.Handler 接口
type GuestSessionCleaner struct {
	logger *zap.Logger
}

// NewGuestSessionCleaner 创建游客会话定时清理器
func NewGuestSessionCleaner(logger *zap.Logger) *GuestSessionCleaner {
	return &GuestSessionCleaner{logger: logger}
}

// Handle 每次定时触发时执行清理逻辑
// 清理 user_id=0 且 5 分钟内无活动的游客会话和消息
func (c *GuestSessionCleaner) Handle(ctx context.Context) error {
	db := database.GetDB()
	if db == nil {
		return nil
	}
	// 先清理游客消息（5分钟内无活动的游客会话关联的消息）
	msgResult, err := db.Exec(ctx,
		`DELETE m FROM chat_messages m
		 INNER JOIN chat_sessions s ON m.session_id = s.id
		 WHERE s.user_id = 0 AND s.updated_at < DATE_SUB(NOW(), INTERVAL 5 MINUTE)`)
	if err != nil {
		c.logger.Warn("定时清理游客消息失败", zap.Error(err))
	} else {
		affected, _ := msgResult.RowsAffected()
		if affected > 0 {
			c.logger.Info("定时清理游客消息", zap.Int64("count", affected))
		}
	}
	// 再清理游客会话
	sessResult, err := db.Exec(ctx,
		`DELETE FROM chat_sessions WHERE user_id = 0 AND updated_at < DATE_SUB(NOW(), INTERVAL 5 MINUTE)`)
	if err != nil {
		c.logger.Warn("定时清理游客会话失败", zap.Error(err))
	} else {
		affected, _ := sessResult.RowsAffected()
		if affected > 0 {
			c.logger.Info("定时清理游客会话", zap.Int64("count", affected))
		}
	}
	return nil
}
