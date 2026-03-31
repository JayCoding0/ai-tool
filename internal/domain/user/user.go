// Package user 定义用户领域的核心实体和角色模型
// Package user 定义用户领域的核心实体和角色模型
package user

import "time"

// Role 用户角色
type Role string

const (
	RoleAdmin Role = "admin" // 超级管理员：可查看/下载/上传所有 skill
	RoleUser  Role = "user"  // 普通用户：只能操作自己的 skill，只读预设
	RoleGuest Role = "guest" // 游客（未登录）：只能使用预设，无法创建/修改
)

// User 用户实体
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         Role
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// IsAdmin 是否为管理员
func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

// UserInfo 用户信息（用于对外展示，不含密码）
type UserInfo struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}
