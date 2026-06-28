// Package cache 提供缓存基础设施实现（Redis + 命中率统计 + 降级）。
package cache

import (
	"sort"
	"sync"
	"sync/atomic"

	domain_cache "aiProject/internal/domain/cache"
)

// statsCounter 单个类别的原子计数器。
type statsCounter struct {
	hits   atomic.Int64
	misses atomic.Int64
}

// StatsCollector 进程内命中率统计收集器（并发安全）。
// 按类别（如 "embedding"、"llm"）分别累计命中/未命中次数。
type StatsCollector struct {
	mu       sync.RWMutex
	counters map[string]*statsCounter
}

// NewStatsCollector 创建统计收集器。
func NewStatsCollector() *StatsCollector {
	return &StatsCollector{counters: make(map[string]*statsCounter)}
}

// counterFor 获取或创建指定类别的计数器。
func (c *StatsCollector) counterFor(category string) *statsCounter {
	c.mu.RLock()
	ct, ok := c.counters[category]
	c.mu.RUnlock()
	if ok {
		return ct
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if ct, ok = c.counters[category]; ok {
		return ct
	}
	ct = &statsCounter{}
	c.counters[category] = ct
	return ct
}

// RecordHit 记录一次命中。
func (c *StatsCollector) RecordHit(category string) {
	c.counterFor(category).hits.Add(1)
}

// RecordMiss 记录一次未命中。
func (c *StatsCollector) RecordMiss(category string) {
	c.counterFor(category).misses.Add(1)
}

// Snapshot 返回当前各类别统计快照（按类别名排序，保证展示稳定）。
func (c *StatsCollector) Snapshot() []domain_cache.CategoryStat {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]domain_cache.CategoryStat, 0, len(c.counters))
	for name, ct := range c.counters {
		hits := ct.hits.Load()
		misses := ct.misses.Load()
		total := hits + misses
		var rate float64
		if total > 0 {
			rate = float64(hits) / float64(total)
		}
		out = append(out, domain_cache.CategoryStat{
			Category: name,
			Hits:     hits,
			Misses:   misses,
			Total:    total,
			HitRate:  rate,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Category < out[j].Category })
	return out
}

// Reset 清零所有统计。
func (c *StatsCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counters = make(map[string]*statsCounter)
}

// 确保实现了 StatsRecorder 接口
var _ domain_cache.StatsRecorder = (*StatsCollector)(nil)
