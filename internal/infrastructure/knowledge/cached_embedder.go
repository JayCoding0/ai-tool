package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	domain_cache "aiProject/internal/domain/cache"
	"aiProject/internal/domain/knowledge"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// embedCacheCategory Embedding 缓存的统计类别名。
const embedCacheCategory = "embedding"

// CachedEmbedder 为任意 Embedder 增加缓存能力的装饰器。
// 相同文本（同一 embedding 模型）的向量化结果是确定性的，可安全缓存，
// 在 RAG 检索、跨会话记忆检索、文档重复入库等场景显著降低 API 调用与延迟。
type CachedEmbedder struct {
	inner knowledge.Embedder      // 被装饰的底层 Embedder
	cache domain_cache.Cache      // 缓存后端
	stats domain_cache.StatsRecorder // 命中率统计（可为 nil）
	ttl   time.Duration           // 缓存过期时间，<=0 表示永不过期
}

// NewCachedEmbedder 创建带缓存的 Embedder 装饰器。
func NewCachedEmbedder(inner knowledge.Embedder, cache domain_cache.Cache, stats domain_cache.StatsRecorder, ttl time.Duration) *CachedEmbedder {
	return &CachedEmbedder{inner: inner, cache: cache, stats: stats, ttl: ttl}
}

// cacheKey 根据模型名 + 文本内容生成缓存 key（SHA256 哈希，避免超长 key 与特殊字符注入）。
func (e *CachedEmbedder) cacheKey(text string) string {
	h := sha256.New()
	h.Write([]byte(e.inner.ModelName()))
	h.Write([]byte{0}) // 分隔符，防止模型名与文本拼接歧义
	h.Write([]byte(text))
	return "embed:" + hex.EncodeToString(h.Sum(nil))
}

func (e *CachedEmbedder) recordHit() {
	if e.stats != nil {
		e.stats.RecordHit(embedCacheCategory)
	}
}

func (e *CachedEmbedder) recordMiss() {
	if e.stats != nil {
		e.stats.RecordMiss(embedCacheCategory)
	}
}

// Embed 单条向量化（带缓存）。
func (e *CachedEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	key := e.cacheKey(text)
	if data, found, err := e.cache.Get(ctx, key); err == nil && found {
		e.recordHit()
		return BytesToFloat32Slice(data), nil
	} else if err != nil {
		shared.GetLogger().Warn("Embedding 缓存读取失败，回退到底层向量化", zap.Error(err))
	}

	e.recordMiss()
	vec, err := e.inner.Embed(ctx, text)
	if err != nil {
		return nil, err
	}
	// 回填缓存（失败不影响主流程）
	if setErr := e.cache.Set(ctx, key, Float32SliceToBytes(vec), e.ttl); setErr != nil {
		shared.GetLogger().Warn("Embedding 缓存写入失败", zap.Error(setErr))
	}
	return vec, nil
}

// EmbedBatch 批量向量化（带缓存）：先逐条查缓存，未命中的部分合并为一次底层批量请求，再回填。
func (e *CachedEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	result := make([][]float32, len(texts))
	var missTexts []string  // 未命中的文本
	var missIndex []int      // 未命中文本在原始 texts 中的下标

	for i, text := range texts {
		key := e.cacheKey(text)
		if data, found, err := e.cache.Get(ctx, key); err == nil && found {
			e.recordHit()
			result[i] = BytesToFloat32Slice(data)
			continue
		} else if err != nil {
			shared.GetLogger().Warn("Embedding 缓存读取失败，回退到底层向量化", zap.Error(err))
		}
		e.recordMiss()
		missTexts = append(missTexts, text)
		missIndex = append(missIndex, i)
	}

	// 全部命中，直接返回
	if len(missTexts) == 0 {
		return result, nil
	}

	// 未命中部分批量请求底层
	vecs, err := e.inner.EmbedBatch(ctx, missTexts)
	if err != nil {
		return nil, err
	}

	// 回填结果与缓存
	for j, vec := range vecs {
		origIdx := missIndex[j]
		result[origIdx] = vec
		if setErr := e.cache.Set(ctx, e.cacheKey(missTexts[j]), Float32SliceToBytes(vec), e.ttl); setErr != nil {
			shared.GetLogger().Warn("Embedding 缓存写入失败", zap.Error(setErr))
		}
	}
	return result, nil
}

// Dimensions 透传底层维度。
func (e *CachedEmbedder) Dimensions() int { return e.inner.Dimensions() }

// ModelName 透传底层模型名。
func (e *CachedEmbedder) ModelName() string { return e.inner.ModelName() }

// 确保实现了 knowledge.Embedder 接口
var _ knowledge.Embedder = (*CachedEmbedder)(nil)
