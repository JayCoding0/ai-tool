package mysql

import (
	"context"
	"time"

	"aiProject/internal/domain/user"
	"aiProject/internal/infrastructure/database"
	trmysql "git.code.oa.com/trpc-go/trpc-database/mysql"
)

// UserRepository MySQL用户存储库
type UserRepository struct {
	db trmysql.Client
}

// NewUserRepository 创建MySQL用户存储库
func NewUserRepository() *UserRepository {
	return &UserRepository{
		db: database.GetDB(),
	}
}

// Create 创建用户（默认角色为 user）
func (r *UserRepository) Create(ctx context.Context, username, passwordHash string) (*user.User, error) {
	query := `INSERT INTO users (username, password_hash, role) VALUES (?, ?, 'user')`
	result, err := r.db.Exec(ctx, query, username, passwordHash)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &user.User{
		ID:           id,
		Username:     username,
		PasswordHash: passwordHash,
		Role:         user.RoleUser,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}, nil
}

// FindByUsername 根据用户名查找用户
func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*user.User, error) {
	query := `SELECT id, username, password_hash, COALESCE(role,'user'), created_at, updated_at FROM users WHERE username = ?`
	u := &user.User{}
	var role string
	dest := []interface{}{&u.ID, &u.Username, &u.PasswordHash, &role, &u.CreatedAt, &u.UpdatedAt}
	err := r.db.QueryRow(ctx, dest, query, username)
	if err != nil {
		// trpc-database/mysql 将 sql.ErrNoRows 包装后返回，需判断
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	u.Role = user.Role(role)
	return u, nil
}

// FindByID 根据ID查找用户
func (r *UserRepository) FindByID(ctx context.Context, id int64) (*user.User, error) {
	query := `SELECT id, username, password_hash, COALESCE(role,'user'), created_at, updated_at FROM users WHERE id = ?`
	u := &user.User{}
	var role string
	dest := []interface{}{&u.ID, &u.Username, &u.PasswordHash, &role, &u.CreatedAt, &u.UpdatedAt}
	err := r.db.QueryRow(ctx, dest, query, id)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	u.Role = user.Role(role)
	return u, nil
}

// ExistsByUsername 检查用户名是否已存在
func (r *UserRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	var exists bool
	dest := []interface{}{&exists}
	err := r.db.QueryRow(ctx, dest, "SELECT EXISTS(SELECT 1 FROM users WHERE username = ?)", username)
	return exists, err
}

// isNotFound 判断是否为记录不存在错误
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == "sql: no rows in result set"
}