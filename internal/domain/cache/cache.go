// Package cache 定义缓存领域的核心接口与命中率统计类型。
// 缓存层与具体实现（Redis / 内存）解耦，供 Embedding 缓存、LLM 响应缓存等场景复用。
package cache

import (
	"context"
	"time"
)

// Cache 通用键值缓存接口（与具体实现解耦）。
// 实现需保证并发安全；当后端不可用时应优雅降级（返回未命中而非 panic）。
type Cache interface {
	// Get 读取缓存。found=false 表示未命中（key 不存在或已过期）。
	Get(ctx context.Context, key string) (value []byte, found bool, err error)
	// Set 写入缓存。ttl<=0 表示永不过期。
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// Delete 删除指定 key。
	Delete(ctx context.Context, key string) error
	// Size 返回当前缓存的条目总数（用于监控展示，实现可返回 -1 表示不支持）。
	Size(ctx context.Context) (int64, error)
	// Clear 清空缓存（谨慎使用，仅清理本应用使用的命名空间）。
	Clear(ctx context.Context) error
	// Available 后端是否可用（如 Redis 连接是否健康）。
	Available() bool
	// Backend 返回后端类型标识（如 "redis" / "noop"），用于监控展示。
	Backend() string
}

// CategoryStat 单一缓存类别（如 embedding / llm）的命中率统计快照。
type CategoryStat struct {
	Category string  `json:"category"` // 类别名称
	Hits     int64   `json:"hits"`     // 命中次数
	Misses   int64   `json:"misses"`   // 未命中次数
	Total    int64   `json:"total"`    // 总查询次数
	HitRate  float64 `json:"hit_rate"` // 命中率 [0,1]
}

// StatsSnapshot 缓存统计的整体快照，用于 HTTP 接口与监控页面展示。
type StatsSnapshot struct {
	Backend    string         `json:"backend"`     // 后端类型（redis / noop）
	Available  bool           `json:"available"`   // 后端是否可用
	EntryCount int64          `json:"entry_count"` // 缓存条目总数（-1 表示未知）
	TotalHits  int64          `json:"total_hits"`
	TotalMiss  int64          `json:"total_miss"`
	HitRate    float64        `json:"hit_rate"` // 总体命中率
	Categories []CategoryStat `json:"categories"`
}

// StatsRecorder 命中率统计记录器接口。
type StatsRecorder interface {
	// RecordHit 记录一次命中。
	RecordHit(category string)
	// RecordMiss 记录一次未命中。
	RecordMiss(category string)
	// Snapshot 返回当前统计快照。
	Snapshot() []CategoryStat
	// Reset 清零统计。
	Reset()
}
