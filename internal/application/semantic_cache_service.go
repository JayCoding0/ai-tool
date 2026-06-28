// Package application 应用服务层，编排领域对象完成业务用例
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	domain_cache "aiProject/internal/domain/cache"
	"aiProject/internal/domain/knowledge"
	infra_knowledge "aiProject/internal/infrastructure/knowledge"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// semanticCacheCategory 语义缓存的命中率统计类别名。
const semanticCacheCategory = "semantic"

// SemanticScope 语义缓存的隔离维度。
// 只有在完全相同的 scope 下（同模型、同 System Prompt、同知识库）才允许复用历史回答，
// 避免不同配置/上下文返回错误的缓存答案。
type SemanticScope struct {
	ModelName       string
	SystemPrompt    string
	KnowledgeBaseID int64
}

// hash 生成 scope 的稳定哈希（作为缓存桶 key 的一部分）。
func (sc SemanticScope) hash() string {
	h := sha256.New()
	h.Write([]byte(sc.ModelName))
	h.Write([]byte{0})
	h.Write([]byte(sc.SystemPrompt))
	h.Write([]byte{0})
	h.Write([]byte(fmt.Sprintf("kb=%d", sc.KnowledgeBaseID)))
	return hex.EncodeToString(h.Sum(nil))
}

// semCacheEntry 单条语义缓存记录。Embedding 用 []byte 存储，JSON 序列化时自动 base64 编码，更紧凑。
type semCacheEntry struct {
	Question  string `json:"q"`
	Answer    string `json:"a"`
	Embedding []byte `json:"e"`
}

// semCacheBucket 一个 scope 下的全部缓存记录（整桶读写，复用现有 KV Cache 接口）。
type semCacheBucket struct {
	Entries []semCacheEntry `json:"entries"`
}

// SemanticCacheService LLM 语义缓存服务。
// 将用户问题向量化后，在同 scope 的历史问答中检索语义相似的问题，
// 相似度超过阈值则直接复用历史答案，省去一次完整的 LLM 调用。
type SemanticCacheService struct {
	cache      domain_cache.Cache
	embedder   knowledge.Embedder
	stats      domain_cache.StatsRecorder
	threshold  float64 // 相似度阈值 [0,1]，超过才视为命中
	maxEntries int     // 每个 scope 最多保留的记录数（超出按 FIFO 淘汰）
	mu         sync.Mutex
}

// NewSemanticCacheService 创建语义缓存服务。
func NewSemanticCacheService(cache domain_cache.Cache, embedder knowledge.Embedder, stats domain_cache.StatsRecorder, threshold float64, maxEntries int) *SemanticCacheService {
	if threshold <= 0 || threshold > 1 {
		threshold = 0.92
	}
	if maxEntries <= 0 {
		maxEntries = 50
	}
	return &SemanticCacheService{
		cache:      cache,
		embedder:   embedder,
		stats:      stats,
		threshold:  threshold,
		maxEntries: maxEntries,
	}
}

// bucketKey 根据 scope 生成缓存桶 key。
func (s *SemanticCacheService) bucketKey(scope SemanticScope) string {
	return "semantic:" + scope.hash()
}

func (s *SemanticCacheService) recordHit() {
	if s.stats != nil {
		s.stats.RecordHit(semanticCacheCategory)
	}
}

func (s *SemanticCacheService) recordMiss() {
	if s.stats != nil {
		s.stats.RecordMiss(semanticCacheCategory)
	}
}

// loadBucket 读取并反序列化指定 scope 的缓存桶。
func (s *SemanticCacheService) loadBucket(ctx context.Context, key string) (*semCacheBucket, error) {
	data, found, err := s.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	bucket := &semCacheBucket{}
	if !found {
		return bucket, nil
	}
	if err := json.Unmarshal(data, bucket); err != nil {
		// 数据损坏时丢弃重建，不影响主流程
		shared.GetLogger().Warn("语义缓存桶反序列化失败，已忽略", zap.Error(err))
		return &semCacheBucket{}, nil
	}
	return bucket, nil
}

// Lookup 查找与 question 语义相似的历史回答。
// hit=true 时返回复用的答案；否则需调用方正常生成。
func (s *SemanticCacheService) Lookup(ctx context.Context, scope SemanticScope, question string) (answer string, hit bool) {
	if s == nil || s.cache == nil || !s.cache.Available() || question == "" {
		return "", false
	}

	queryVec, err := s.embedder.Embed(ctx, question)
	if err != nil {
		shared.GetLogger().Warn("语义缓存：问题向量化失败", zap.Error(err))
		return "", false
	}

	bucket, err := s.loadBucket(ctx, s.bucketKey(scope))
	if err != nil {
		shared.GetLogger().Warn("语义缓存：读取缓存桶失败", zap.Error(err))
		return "", false
	}

	var bestScore float32
	var bestAnswer string
	for _, e := range bucket.Entries {
		entryVec := infra_knowledge.BytesToFloat32Slice(e.Embedding)
		score := infra_knowledge.CosineSimilarity(queryVec, entryVec)
		if score > bestScore {
			bestScore = score
			bestAnswer = e.Answer
		}
	}

	if float64(bestScore) >= s.threshold && bestAnswer != "" {
		s.recordHit()
		shared.GetLogger().Info("语义缓存命中",
			zap.Float64("score", float64(bestScore)),
			zap.Float64("threshold", s.threshold),
			zap.String("question", msgPreview(question, 50)),
		)
		return bestAnswer, true
	}
	s.recordMiss()
	return "", false
}

// Store 将一对问答写入语义缓存（异步调用，失败不影响主流程）。
func (s *SemanticCacheService) Store(ctx context.Context, scope SemanticScope, question, answer string) {
	if s == nil || s.cache == nil || !s.cache.Available() || question == "" || answer == "" {
		return
	}

	queryVec, err := s.embedder.Embed(ctx, question)
	if err != nil {
		shared.GetLogger().Warn("语义缓存：写入时向量化失败", zap.Error(err))
		return
	}

	key := s.bucketKey(scope)

	// 整桶读改写需串行，避免并发覆盖丢失
	s.mu.Lock()
	defer s.mu.Unlock()

	bucket, err := s.loadBucket(ctx, key)
	if err != nil {
		shared.GetLogger().Warn("语义缓存：写入时读取缓存桶失败", zap.Error(err))
		return
	}

	bucket.Entries = append(bucket.Entries, semCacheEntry{
		Question:  question,
		Answer:    answer,
		Embedding: infra_knowledge.Float32SliceToBytes(queryVec),
	})
	// 超出容量按 FIFO 淘汰最旧记录
	if len(bucket.Entries) > s.maxEntries {
		bucket.Entries = bucket.Entries[len(bucket.Entries)-s.maxEntries:]
	}

	data, err := json.Marshal(bucket)
	if err != nil {
		return
	}
	if err := s.cache.Set(ctx, key, data, 0); err != nil {
		shared.GetLogger().Warn("语义缓存：写入缓存桶失败", zap.Error(err))
	}
}
