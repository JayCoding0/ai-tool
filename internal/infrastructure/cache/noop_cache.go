package cache

import (
	"context"
	"time"

	domain_cache "aiProject/internal/domain/cache"
)

// NoopCache 空缓存实现：所有读取均未命中，写入均忽略。
// 用于缓存功能被禁用（cache.enabled=false）的场景，使上层代码无需分支判断。
type NoopCache struct{}

// NewNoopCache 创建空缓存。
func NewNoopCache() *NoopCache { return &NoopCache{} }

// Get 始终未命中。
func (NoopCache) Get(context.Context, string) ([]byte, bool, error) { return nil, false, nil }

// Set 忽略写入。
func (NoopCache) Set(context.Context, string, []byte, time.Duration) error { return nil }

// Delete 无操作。
func (NoopCache) Delete(context.Context, string) error { return nil }

// Size 返回 0。
func (NoopCache) Size(context.Context) (int64, error) { return 0, nil }

// Clear 无操作。
func (NoopCache) Clear(context.Context) error { return nil }

// Available 始终不可用。
func (NoopCache) Available() bool { return false }

// Backend 返回标识。
func (NoopCache) Backend() string { return "noop" }

var _ domain_cache.Cache = (*NoopCache)(nil)
