// Package trace 提供 Trace 存储实现。
package trace

import (
	"sync"

	domain_trace "aiProject/internal/domain/trace"
)

// MemoryStore 进程内 Trace 存储（环形缓冲，保留最近 N 条）。
// 用于调试面板展示，进程重启即清空；如需持久化可另行实现 domain_trace.Store。
type MemoryStore struct {
	mu     sync.RWMutex
	traces []*domain_trace.Trace // 按写入顺序，最旧在前
	byID   map[string]*domain_trace.Trace
	cap    int
}

// NewMemoryStore 创建内存存储，cap 为最大保留条数（<=0 时默认 200）
func NewMemoryStore(cap int) *MemoryStore {
	if cap <= 0 {
		cap = 200
	}
	return &MemoryStore{
		byID: make(map[string]*domain_trace.Trace),
		cap:  cap,
	}
}

// Save 保存一条 Trace，超出容量时淘汰最旧的
func (s *MemoryStore) Save(t *domain_trace.Trace) {
	if t == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.traces = append(s.traces, t)
	s.byID[t.ID] = t
	for len(s.traces) > s.cap {
		oldest := s.traces[0]
		s.traces = s.traces[1:]
		delete(s.byID, oldest.ID)
	}
}

// List 返回最近的 limit 条 Trace（最新在前）
func (s *MemoryStore) List(limit int) []*domain_trace.Trace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := len(s.traces)
	if limit <= 0 || limit > n {
		limit = n
	}
	out := make([]*domain_trace.Trace, 0, limit)
	for i := n - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, s.traces[i])
	}
	return out
}

// Get 按 ID 获取 Trace
func (s *MemoryStore) Get(id string) (*domain_trace.Trace, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.byID[id]
	return t, ok
}

var _ domain_trace.Store = (*MemoryStore)(nil)
