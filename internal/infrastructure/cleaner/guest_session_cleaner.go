// Package cleaner 提供游客会话的清理功能
// 包括启动时一次性清理和定时清理两种模式
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
