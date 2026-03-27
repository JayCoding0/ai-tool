package token

import (
	"context"
	"database/sql"

	"aiProject/internal/application"
	"aiProject/internal/infrastructure/database"
	trmysql "git.code.oa.com/trpc-go/trpc-database/mysql"
)

// 确保 DBTokenStore 实现了 application.TokenStore 接口
var _ application.TokenStore = (*DBTokenStore)(nil)

// DBTokenStore 基于MySQL的Token持久化存储
type DBTokenStore struct {
	db trmysql.Client
}

// NewDBTokenStore 创建数据库Token存储
func NewDBTokenStore() *DBTokenStore {
	return &DBTokenStore{db: database.GetDB()}
}

// Set 保存Token到数据库
func (s *DBTokenStore) Set(ctx context.Context, token string, info *application.TokenInfo) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO auth_tokens (token, user_id, username, expires_at)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE user_id=VALUES(user_id), username=VALUES(username), expires_at=VALUES(expires_at)`,
		token, info.UserID, info.Username, info.ExpiresAt,
	)
	return err
}

// Get 从数据库获取Token信息
func (s *DBTokenStore) Get(ctx context.Context, token string) (*application.TokenInfo, bool) {
	var info application.TokenInfo
	dest := []interface{}{&info.UserID, &info.Username, &info.ExpiresAt}
	err := s.db.QueryRow(ctx, dest,
		`SELECT user_id, username, expires_at FROM auth_tokens WHERE token = ? AND expires_at > NOW()`,
		token,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false
		}
		return nil, false
	}
	return &info, true
}

// Delete 从数据库删除Token
func (s *DBTokenStore) Delete(ctx context.Context, token string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM auth_tokens WHERE token = ?`, token)
	return err
}

// CleanExpired 清理过期Token（可由定时任务调用）
func (s *DBTokenStore) CleanExpired(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `DELETE FROM auth_tokens WHERE expires_at <= NOW()`)
	return err
}
