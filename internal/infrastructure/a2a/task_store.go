package a2a

import (
	"fmt"
	"sync"
	"time"

	domain_a2a "aiProject/internal/domain/a2a"
)

// TaskStore 任务内存存储（线程安全）
type TaskStore struct {
	mu    sync.RWMutex
	tasks map[string]*domain_a2a.Task
}

// NewTaskStore 创建任务存储
func NewTaskStore() *TaskStore {
	store := &TaskStore{
		tasks: make(map[string]*domain_a2a.Task),
	}
	// 启动定期清理过期任务的 goroutine（超过 1 小时的终态任务）
	go store.cleanupLoop()
	return store
}

// Save 保存或更新任务
func (s *TaskStore) Save(task *domain_a2a.Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[task.ID] = task
}

// Get 获取任务（不存在返回 nil, false）
func (s *TaskStore) Get(id string) (*domain_a2a.Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task, ok := s.tasks[id]
	return task, ok
}

// Delete 删除任务
func (s *TaskStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tasks, id)
}

// List 列出所有任务（返回快照）
func (s *TaskStore) List() []*domain_a2a.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tasks := make([]*domain_a2a.Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, t)
	}
	return tasks
}

// cleanupLoop 定期清理超过 1 小时的终态任务，防止内存泄漏
func (s *TaskStore) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for id, task := range s.tasks {
			if task.IsTerminal() && now.Sub(task.UpdatedAt) > time.Hour {
				delete(s.tasks, id)
			}
		}
		s.mu.Unlock()
	}
}

// StreamHub 流式推送中心，管理每个任务的 SSE 订阅者
type StreamHub struct {
	mu          sync.RWMutex
	subscribers map[string][]chan domain_a2a.TaskStreamEvent
}

// NewStreamHub 创建流式推送中心
func NewStreamHub() *StreamHub {
	return &StreamHub{
		subscribers: make(map[string][]chan domain_a2a.TaskStreamEvent),
	}
}

// Subscribe 订阅任务的流式事件，返回事件 channel
func (h *StreamHub) Subscribe(taskID string) (<-chan domain_a2a.TaskStreamEvent, func()) {
	ch := make(chan domain_a2a.TaskStreamEvent, 32)
	h.mu.Lock()
	h.subscribers[taskID] = append(h.subscribers[taskID], ch)
	h.mu.Unlock()

	// 返回取消订阅函数
	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		subs := h.subscribers[taskID]
		for i, sub := range subs {
			if sub == ch {
				h.subscribers[taskID] = append(subs[:i], subs[i+1:]...)
				close(ch)
				break
			}
		}
		if len(h.subscribers[taskID]) == 0 {
			delete(h.subscribers, taskID)
		}
	}
	return ch, cancel
}

// Publish 向所有订阅者推送事件
func (h *StreamHub) Publish(taskID string, event domain_a2a.TaskStreamEvent) {
	h.mu.RLock()
	subs := make([]chan domain_a2a.TaskStreamEvent, len(h.subscribers[taskID]))
	copy(subs, h.subscribers[taskID])
	h.mu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// 订阅者消费太慢，跳过（避免阻塞）
		}
	}
}

// Close 关闭并清理某个任务的所有订阅者
func (h *StreamHub) Close(taskID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subscribers[taskID] {
		close(ch)
	}
	delete(h.subscribers, taskID)
}

// GenerateTaskID 生成唯一任务 ID
func GenerateTaskID() string {
	return fmt.Sprintf("task-%d", time.Now().UnixNano())
}
