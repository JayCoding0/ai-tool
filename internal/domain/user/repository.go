package user

import "context"

// Repository 用户存储库接口
type Repository interface {
	// Create 创建用户
	Create(ctx context.Context, username, passwordHash string) (*User, error)
	// FindByUsername 根据用户名查找用户
	FindByUsername(ctx context.Context, username string) (*User, error)
	// FindByID 根据ID查找用户
	FindByID(ctx context.Context, id int64) (*User, error)
	// ExistsByUsername 检查用户名是否已存在
	ExistsByUsername(ctx context.Context, username string) (bool, error)
}
