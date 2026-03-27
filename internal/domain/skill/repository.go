package skill

import "context"

// Repository 技能仓库接口
type Repository interface {
	// Create 创建技能
	Create(ctx context.Context, s *Skill) error

	// Update 更新技能
	Update(ctx context.Context, s *Skill) error

	// Delete 删除技能
	Delete(ctx context.Context, id SkillID) error

	// FindByID 根据ID查找技能
	FindByID(ctx context.Context, id SkillID) (*Skill, error)

	// ListByUser 列出用户可见的技能（系统预设 + 公开 + 自己的）
	ListByUser(ctx context.Context, userID int64) ([]*Skill, error)

	// ListAll 列出所有技能（admin 专用）
	ListAll(ctx context.Context) ([]*Skill, error)

	// ListSystem 列出所有系统预设技能
	ListSystem(ctx context.Context) ([]*Skill, error)
}
