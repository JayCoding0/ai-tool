package session

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// SessionID 会话ID值对象
type SessionID string

// NewSessionID 创建新的会话ID
func NewSessionID() SessionID {
	return SessionID(uuid.New().String())
}

// String 返回会话ID的字符串表示
func (id SessionID) String() string {
	return string(id)
}

// Message 消息值对象
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	ModelName string    `json:"model_name,omitempty"`
	Time      time.Time `json:"time"`
}

// SessionInfo 会话信息（用于列表展示）
type SessionInfo struct {
	ID           SessionID `json:"id"`
	Title        string    `json:"title"`
	ModelName    string    `json:"model_name,omitempty"`
	SystemPrompt string    `json:"system_prompt,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Session 会话实体
type Session struct {
	id        SessionID
	history   []Message
	createdAt time.Time
	updatedAt time.Time
	mu        sync.Mutex
}

// NewSession 创建新会话
func NewSession() *Session {
	now := time.Now()
	return &Session{
		id:        NewSessionID(),
		history:   make([]Message, 0),
		createdAt: now,
		updatedAt: now,
	}
}

// NewSessionWithID 用指定ID创建会话（用于从数据库恢复会话）
func NewSessionWithID(id SessionID) *Session {
	now := time.Now()
	return &Session{
		id:        id,
		history:   make([]Message, 0),
		createdAt: now,
		updatedAt: now,
	}
}

// ID 获取会话ID
func (s *Session) ID() SessionID {
	return s.id
}

// AddMessage 添加消息到会话
func (s *Session) AddMessage(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.history = append(s.history, Message{
		Role:    role,
		Content: content,
		Time:    time.Now(),
	})
	s.updatedAt = time.Now()
}

// GetHistory 获取对话历史
func (s *Session) GetHistory() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.history
}

// ClearHistory 清空对话历史
func (s *Session) ClearHistory() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.history = make([]Message, 0)
	s.updatedAt = time.Now()
}

// CreatedAt 获取创建时间
func (s *Session) CreatedAt() time.Time {
	return s.createdAt
}

// UpdatedAt 获取更新时间
func (s *Session) UpdatedAt() time.Time {
	return s.updatedAt
}