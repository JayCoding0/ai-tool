// Package http 提供 HTTP 接口层实现
package http

import (
	"encoding/json"
	"net/http"

	domain_cache "aiProject/internal/domain/cache"
	"go.uber.org/zap"
)

// ─── 缓存监控接口 ──────────────────────────────────────────────────────────────

// HandleCacheStats 返回缓存命中率统计与后端状态
// GET /api/cache/stats
func (h *ChatHandler) HandleCacheStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.cache == nil || h.cacheStats == nil {
		writeJSONError(w, "缓存功能未启用", http.StatusServiceUnavailable)
		return
	}

	categories := h.cacheStats.Snapshot()

	// 汇总总命中/未命中
	var totalHits, totalMiss int64
	for _, c := range categories {
		totalHits += c.Hits
		totalMiss += c.Misses
	}
	var hitRate float64
	if total := totalHits + totalMiss; total > 0 {
		hitRate = float64(totalHits) / float64(total)
	}

	// 查询缓存条目数（后端不可用时返回 -1）
	entryCount := int64(-1)
	if h.cache.Available() {
		if n, err := h.cache.Size(r.Context()); err == nil {
			entryCount = n
		} else {
			h.logger.Warn("获取缓存条目数失败", zap.Error(err))
		}
	}

	snapshot := domain_cache.StatsSnapshot{
		Backend:    h.cache.Backend(),
		Available:  h.cache.Available(),
		EntryCount: entryCount,
		TotalHits:  totalHits,
		TotalMiss:  totalMiss,
		HitRate:    hitRate,
		Categories: categories,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snapshot) //nolint:errcheck
}

// HandleCacheClear 清空缓存并重置统计（需登录）
// POST /api/cache/clear
func (h *ChatHandler) HandleCacheClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := h.requireLogin(w, r); !ok {
		return
	}
	if h.cache == nil || h.cacheStats == nil {
		writeJSONError(w, "缓存功能未启用", http.StatusServiceUnavailable)
		return
	}

	if err := h.cache.Clear(r.Context()); err != nil {
		h.logger.Error("清空缓存失败", zap.Error(err))
		writeJSONError(w, "清空缓存失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.cacheStats.Reset()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "缓存已清空，统计已重置"}) //nolint:errcheck
}
