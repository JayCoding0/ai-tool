package application

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"aiProject/internal/domain/user"
)

// ─────────────────────────────────────────────────────────────────────────────
// Mock UserRepository（内存实现，用于测试）
// ─────────────────────────────────────────────────────────────────────────────

type mockUserRepo struct {
	mu    sync.RWMutex
	users map[string]*user.User
	nextID int64
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{
		users:  make(map[string]*user.User),
		nextID: 1,
	}
}

func (r *mockUserRepo) Create(_ context.Context, username, passwordHash string) (*user.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.users[username]; exists {
		return nil, fmt.Errorf("用户名已存在")
	}
	u := &user.User{
		ID:           r.nextID,
		Username:     username,
		PasswordHash: passwordHash,
		Role:         user.RoleUser,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	r.nextID++
	r.users[username] = u
	return u, nil
}

func (r *mockUserRepo) FindByUsername(_ context.Context, username string) (*user.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.users[username]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (r *mockUserRepo) FindByID(_ context.Context, id int64) (*user.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, nil
}

func (r *mockUserRepo) ExistsByUsername(_ context.Context, username string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.users[username]
	return exists, nil
}

var _ user.Repository = (*mockUserRepo)(nil)

// ─────────────────────────────────────────────────────────────────────────────
// 密码哈希测试
// ─────────────────────────────────────────────────────────────────────────────

func TestHashPassword_And_CheckPassword(t *testing.T) {
	password := "mySecurePass123"
	hash, err := hashPassword(password)
	if err != nil {
		t.Fatalf("哈希密码失败: %v", err)
	}
	if hash == password {
		t.Error("哈希后不应与原密码相同")
	}
	if !checkPassword(password, hash) {
		t.Error("正确密码应验证通过")
	}
	if checkPassword("wrongPassword", hash) {
		t.Error("错误密码不应验证通过")
	}
}

func TestHashPassword_DifferentSalts(t *testing.T) {
	// 同一密码两次哈希结果应不同（bcrypt 自动加盐）
	h1, _ := hashPassword("samePassword")
	h2, _ := hashPassword("samePassword")
	if h1 == h2 {
		t.Error("同一密码两次哈希结果不应相同（bcrypt 自动加盐）")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// generateToken 测试
// ─────────────────────────────────────────────────────────────────────────────

func TestGenerateToken_Unique(t *testing.T) {
	t1, err := generateToken()
	if err != nil {
		t.Fatalf("生成 token 失败: %v", err)
	}
	t2, err := generateToken()
	if err != nil {
		t.Fatalf("生成 token 失败: %v", err)
	}
	if t1 == t2 {
		t.Error("两次生成的 token 不应相同")
	}
	if len(t1) == 0 {
		t.Error("token 不应为空")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 注册测试
// ─────────────────────────────────────────────────────────────────────────────

func TestRegister_Success(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo)

	resp, err := svc.Register(context.Background(), RegisterRequest{
		Username: "testuser",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	if resp.Username != "testuser" {
		t.Errorf("期望用户名 testuser，实际 %s", resp.Username)
	}
	if resp.Token == "" {
		t.Error("注册后应返回 token")
	}
	if resp.UserID == 0 {
		t.Error("注册后应返回 UserID")
	}
}

func TestRegister_EmptyFields(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo)

	_, err := svc.Register(context.Background(), RegisterRequest{
		Username: "",
		Password: "password123",
	})
	if err == nil {
		t.Error("空用户名应返回错误")
	}

	_, err = svc.Register(context.Background(), RegisterRequest{
		Username: "testuser",
		Password: "",
	})
	if err == nil {
		t.Error("空密码应返回错误")
	}
}

func TestRegister_ShortUsername(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo)

	_, err := svc.Register(context.Background(), RegisterRequest{
		Username: "ab",
		Password: "password123",
	})
	if err == nil {
		t.Error("用户名少于3字符应返回错误")
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo)

	_, err := svc.Register(context.Background(), RegisterRequest{
		Username: "testuser",
		Password: "12345",
	})
	if err == nil {
		t.Error("密码少于6位应返回错误")
	}
}

func TestRegister_DuplicateUsername(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo)

	_, err := svc.Register(context.Background(), RegisterRequest{
		Username: "testuser",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("首次注册失败: %v", err)
	}

	_, err = svc.Register(context.Background(), RegisterRequest{
		Username: "testuser",
		Password: "password456",
	})
	if err == nil {
		t.Error("重复用户名应返回错误")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 登录测试
// ─────────────────────────────────────────────────────────────────────────────

func TestLogin_Success(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo)

	// 先注册
	_, err := svc.Register(context.Background(), RegisterRequest{
		Username: "loginuser",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}

	// 再登录
	resp, err := svc.Login(context.Background(), LoginRequest{
		Username: "loginuser",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	if resp.Username != "loginuser" {
		t.Errorf("期望用户名 loginuser，实际 %s", resp.Username)
	}
	if resp.Token == "" {
		t.Error("登录后应返回 token")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo)

	svc.Register(context.Background(), RegisterRequest{
		Username: "loginuser",
		Password: "password123",
	})

	_, err := svc.Login(context.Background(), LoginRequest{
		Username: "loginuser",
		Password: "wrongpassword",
	})
	if err == nil {
		t.Error("错误密码应返回错误")
	}
}

func TestLogin_NonexistentUser(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo)

	_, err := svc.Login(context.Background(), LoginRequest{
		Username: "nobody",
		Password: "password123",
	})
	if err == nil {
		t.Error("不存在的用户应返回错误")
	}
}

func TestLogin_EmptyFields(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo)

	_, err := svc.Login(context.Background(), LoginRequest{
		Username: "",
		Password: "password123",
	})
	if err == nil {
		t.Error("空用户名应返回错误")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Token 验证与登出测试
// ─────────────────────────────────────────────────────────────────────────────

func TestValidateToken_Success(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo)

	resp, _ := svc.Register(context.Background(), RegisterRequest{
		Username: "tokenuser",
		Password: "password123",
	})

	userID, username, role, err := svc.ValidateToken(resp.Token)
	if err != nil {
		t.Fatalf("验证 token 失败: %v", err)
	}
	if userID != resp.UserID {
		t.Errorf("期望 UserID=%d，实际 %d", resp.UserID, userID)
	}
	if username != "tokenuser" {
		t.Errorf("期望用户名 tokenuser，实际 %s", username)
	}
	if role != string(user.RoleUser) {
		t.Errorf("期望角色 user，实际 %s", role)
	}
}

func TestValidateToken_InvalidToken(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo)

	_, _, _, err := svc.ValidateToken("invalid-token-xxx")
	if err == nil {
		t.Error("无效 token 应返回错误")
	}
}

func TestLogout(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo)

	resp, _ := svc.Register(context.Background(), RegisterRequest{
		Username: "logoutuser",
		Password: "password123",
	})

	// 登出
	svc.Logout(resp.Token)

	// 登出后 token 应失效
	_, _, _, err := svc.ValidateToken(resp.Token)
	if err == nil {
		t.Error("登出后 token 应失效")
	}
}
