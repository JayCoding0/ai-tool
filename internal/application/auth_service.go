package application

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"aiProject/internal/domain/user"
	"golang.org/x/crypto/bcrypt"
)

// TokenInfo Token信息
type TokenInfo struct {
	UserID    int64
	Username  string
	Role      string // 用户角色: admin/user
	ExpiresAt time.Time
}

// TokenStore Token存储接口，支持内存和数据库两种实现
type TokenStore interface {
	Set(ctx context.Context, token string, info *TokenInfo) error
	Get(ctx context.Context, token string) (*TokenInfo, bool)
	Delete(ctx context.Context, token string) error
}

// ─────────────────────────────────────────────────────────────────────────────
// 内存 TokenStore 实现（单机降级方案）
// ─────────────────────────────────────────────────────────────────────────────

type memoryTokenStore struct {
	mu     sync.RWMutex
	tokens map[string]*TokenInfo
}

func newMemoryTokenStore() *memoryTokenStore {
	ts := &memoryTokenStore{
		tokens: make(map[string]*TokenInfo),
	}
	go ts.cleanupLoop()
	return ts
}

func (ts *memoryTokenStore) Set(_ context.Context, token string, info *TokenInfo) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.tokens[token] = info
	return nil
}

func (ts *memoryTokenStore) Get(_ context.Context, token string) (*TokenInfo, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	info, ok := ts.tokens[token]
	if !ok || time.Now().After(info.ExpiresAt) {
		return nil, false
	}
	return info, true
}

func (ts *memoryTokenStore) Delete(_ context.Context, token string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	delete(ts.tokens, token)
	return nil
}

func (ts *memoryTokenStore) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		ts.mu.Lock()
		now := time.Now()
		for token, info := range ts.tokens {
			if now.After(info.ExpiresAt) {
				delete(ts.tokens, token)
			}
		}
		ts.mu.Unlock()
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AuthService
// ─────────────────────────────────────────────────────────────────────────────

// AuthService 认证应用服务
type AuthService struct {
	userRepo   user.Repository
	tokenStore TokenStore
}

// NewAuthService 创建认证服务（使用内存Token存储，适合单机部署）
func NewAuthService(userRepo user.Repository) *AuthService {
	return &AuthService{
		userRepo:   userRepo,
		tokenStore: newMemoryTokenStore(),
	}
}

// NewAuthServiceWithTokenStore 创建认证服务（使用指定Token存储，适合多实例部署）
func NewAuthServiceWithTokenStore(userRepo user.Repository, store TokenStore) *AuthService {
	return &AuthService{
		userRepo:   userRepo,
		tokenStore: store,
	}
}

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username string
	Password string
}

// RegisterResponse 注册响应
type RegisterResponse struct {
	UserID   int64
	Username string
	Token    string
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string
	Password string
}

// LoginResponse 登录响应
type LoginResponse struct {
	UserID   int64
	Username string
	Role     string
	Token    string
}

// hashPassword 使用 bcrypt 对密码进行加盐哈希
func hashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// checkPassword 验证密码是否与哈希匹配
func checkPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// generateToken 生成随机token
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// Register 用户注册
func (s *AuthService) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	if req.Username == "" || req.Password == "" {
		return nil, errors.New("用户名和密码不能为空")
	}
	if len(req.Username) < 3 || len(req.Username) > 50 {
		return nil, errors.New("用户名长度须在3-50字符之间")
	}
	if len(req.Password) < 6 {
		return nil, errors.New("密码长度不能少于6位")
	}

	// 检查用户名是否已存在
	exists, err := s.userRepo.ExistsByUsername(ctx, req.Username)
	if err != nil {
		return nil, fmt.Errorf("检查用户名失败: %w", err)
	}
	if exists {
		return nil, errors.New("用户名已存在")
	}

	// 创建用户
	passwordHash, err := hashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("密码加密失败: %w", err)
	}
	u, err := s.userRepo.Create(ctx, req.Username, passwordHash)
	if err != nil {
		return nil, fmt.Errorf("创建用户失败: %w", err)
	}

	// 生成token
	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("生成token失败: %w", err)
	}

	if err := s.tokenStore.Set(ctx, token, &TokenInfo{
		UserID:    u.ID,
		Username:  u.Username,
		Role:      string(user.RoleUser), // 新注册用户默认为普通用户
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}); err != nil {
		return nil, fmt.Errorf("保存token失败: %w", err)
	}

	return &RegisterResponse{
		UserID:   u.ID,
		Username: u.Username,
		Token:    token,
	}, nil
}

// Login 用户登录
func (s *AuthService) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	if req.Username == "" || req.Password == "" {
		return nil, errors.New("用户名和密码不能为空")
	}

	u, err := s.userRepo.FindByUsername(ctx, req.Username)
	if err != nil {
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}
	if u == nil {
		return nil, errors.New("用户名或密码错误")
	}

	if !checkPassword(req.Password, u.PasswordHash) {
		return nil, errors.New("用户名或密码错误")
	}

	// 生成token
	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("生成token失败: %w", err)
	}

	if err := s.tokenStore.Set(ctx, token, &TokenInfo{
		UserID:    u.ID,
		Username:  u.Username,
		Role:      string(u.Role),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}); err != nil {
		return nil, fmt.Errorf("保存token失败: %w", err)
	}

	return &LoginResponse{
		UserID:   u.ID,
		Username: u.Username,
		Role:     string(u.Role),
		Token:    token,
	}, nil
}

// ValidateToken 验证token，返回用户ID、用户名和角色
func (s *AuthService) ValidateToken(token string) (int64, string, string, error) {
	ctx := context.Background()
	info, ok := s.tokenStore.Get(ctx, token)
	if !ok {
		return 0, "", "", errors.New("token无效或已过期")
	}
	return info.UserID, info.Username, info.Role, nil
}

// Logout 登出，删除token
func (s *AuthService) Logout(token string) {
	s.tokenStore.Delete(context.Background(), token) //nolint:errcheck
}