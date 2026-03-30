// Package application 应用服务层，编排领域对象完成业务用例
package application

import (
	"context"
	"fmt"

	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// PromptVarsRepository Prompt 变量持久化仓储接口
type PromptVarsRepository interface {
	// GetUserVars 获取用户级变量
	GetUserVars(ctx context.Context, userID int64) (map[string]string, error)
	// SetUserVar 设置用户级变量（upsert）
	SetUserVar(ctx context.Context, userID int64, key, value string) error
	// DeleteUserVar 删除用户级变量
	DeleteUserVar(ctx context.Context, userID int64, key string) error
	// GetSessionVars 获取会话级变量
	GetSessionVars(ctx context.Context, sessionID string) (map[string]string, error)
	// SetSessionVar 设置会话级变量（upsert）
	SetSessionVar(ctx context.Context, sessionID, key, value string) error
	// DeleteSessionVar 删除会话级变量
	DeleteSessionVar(ctx context.Context, sessionID, key string) error
}

// PromptVarsService Prompt 变量管理服务
type PromptVarsService struct {
	repo PromptVarsRepository
}

// NewPromptVarsService 创建 Prompt 变量管理服务
func NewPromptVarsService(repo PromptVarsRepository) *PromptVarsService {
	return &PromptVarsService{repo: repo}
}

// GetUserVars 获取用户级变量
func (s *PromptVarsService) GetUserVars(ctx context.Context, userID int64) (map[string]string, error) {
	vars, err := s.repo.GetUserVars(ctx, userID)
	if err != nil {
		shared.GetLogger().Error("获取用户级变量失败", zap.Int64("user_id", userID), zap.Error(err))
		return nil, fmt.Errorf("获取用户级变量失败: %w", err)
	}
	return vars, nil
}

// SetUserVar 设置用户级变量
func (s *PromptVarsService) SetUserVar(ctx context.Context, userID int64, key, value string) error {
	if key == "" {
		return fmt.Errorf("变量名不能为空")
	}
	if err := s.repo.SetUserVar(ctx, userID, key, value); err != nil {
		shared.GetLogger().Error("设置用户级变量失败",
			zap.Int64("user_id", userID), zap.String("key", key), zap.Error(err))
		return fmt.Errorf("设置用户级变量失败: %w", err)
	}
	shared.GetLogger().Info("用户级变量已设置",
		zap.Int64("user_id", userID), zap.String("key", key))
	return nil
}

// DeleteUserVar 删除用户级变量
func (s *PromptVarsService) DeleteUserVar(ctx context.Context, userID int64, key string) error {
	if err := s.repo.DeleteUserVar(ctx, userID, key); err != nil {
		return fmt.Errorf("删除用户级变量失败: %w", err)
	}
	return nil
}

// GetSessionVars 获取会话级变量
func (s *PromptVarsService) GetSessionVars(ctx context.Context, sessionID string) (map[string]string, error) {
	vars, err := s.repo.GetSessionVars(ctx, sessionID)
	if err != nil {
		shared.GetLogger().Error("获取会话级变量失败", zap.String("session_id", sessionID), zap.Error(err))
		return nil, fmt.Errorf("获取会话级变量失败: %w", err)
	}
	return vars, nil
}

// SetSessionVar 设置会话级变量
func (s *PromptVarsService) SetSessionVar(ctx context.Context, sessionID, key, value string) error {
	if key == "" {
		return fmt.Errorf("变量名不能为空")
	}
	if sessionID == "" {
		return fmt.Errorf("会话ID不能为空")
	}
	if err := s.repo.SetSessionVar(ctx, sessionID, key, value); err != nil {
		shared.GetLogger().Error("设置会话级变量失败",
			zap.String("session_id", sessionID), zap.String("key", key), zap.Error(err))
		return fmt.Errorf("设置会话级变量失败: %w", err)
	}
	shared.GetLogger().Info("会话级变量已设置",
		zap.String("session_id", sessionID), zap.String("key", key))
	return nil
}

// DeleteSessionVar 删除会话级变量
func (s *PromptVarsService) DeleteSessionVar(ctx context.Context, sessionID, key string) error {
	if err := s.repo.DeleteSessionVar(ctx, sessionID, key); err != nil {
		return fmt.Errorf("删除会话级变量失败: %w", err)
	}
	return nil
}

// BuildPromptContext 构建完整的 PromptContext（合并用户级 + 会话级 + 请求级变量）
// 优先级：用户级 → 会话级 → 请求级（后面覆盖前面）
func (s *PromptVarsService) BuildPromptContext(ctx context.Context, userID int64, userName, sessionID, modelName string, requestVars map[string]string) PromptContext {
	var userVars, sessionVars map[string]string

	// 获取用户级变量（忽略错误，降级为空）
	if userID > 0 {
		userVars, _ = s.repo.GetUserVars(ctx, userID)
	}

	// 获取会话级变量（忽略错误，降级为空）
	if sessionID != "" {
		sessionVars, _ = s.repo.GetSessionVars(ctx, sessionID)
	}

	return PromptContext{
		UserName:   userName,
		UserID:     userID,
		SessionID:  sessionID,
		ModelName:  modelName,
		CustomVars: MergePromptVars(userVars, sessionVars, requestVars),
	}
}
