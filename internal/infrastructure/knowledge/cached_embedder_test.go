package knowledge

import (
	"context"
	"sync"
	"testing"
	"time"

	domain_cache "aiProject/internal/domain/cache"
)

// memCache 测试用的进程内缓存（实现 domain_cache.Cache）
type memCache struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemCache() *memCache { return &memCache{data: make(map[string][]byte)} }

func (m *memCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[key]
	return v, ok, nil
}
func (m *memCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(value))
	copy(cp, value)
	m.data[key] = cp
	return nil
}
func (m *memCache) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}
func (m *memCache) Size(context.Context) (int64, error) { return int64(len(m.data)), nil }
func (m *memCache) Clear(context.Context) error         { m.data = make(map[string][]byte); return nil }
func (m *memCache) Available() bool                     { return true }
func (m *memCache) Backend() string                     { return "mem" }

var _ domain_cache.Cache = (*memCache)(nil)

// fakeEmbedder 测试用 Embedder，记录底层调用次数；相同文本返回相同向量
type fakeEmbedder struct {
	mu        sync.Mutex
	embedN    int // Embed 单条调用计入的文本数
	model     string
}

func (f *fakeEmbedder) vectorFor(text string) []float32 {
	// 基于字符的确定性向量（同文本同向量）
	var sum float32
	for _, r := range text {
		sum += float32(r)
	}
	return []float32{sum, float32(len(text)), 1.0}
}
func (f *fakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	f.mu.Lock()
	f.embedN++
	f.mu.Unlock()
	return f.vectorFor(text), nil
}
func (f *fakeEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	f.mu.Lock()
	f.embedN += len(texts)
	f.mu.Unlock()
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = f.vectorFor(t)
	}
	return out, nil
}
func (f *fakeEmbedder) Dimensions() int  { return 3 }
func (f *fakeEmbedder) ModelName() string { return f.model }

func TestCachedEmbedder_Embed_CacheHit(t *testing.T) {
	ctx := context.Background()
	inner := &fakeEmbedder{model: "test-model"}
	stats := newStatsRecorder()
	ce := NewCachedEmbedder(inner, newMemCache(), stats, 0)

	v1, _ := ce.Embed(ctx, "hello")
	v2, _ := ce.Embed(ctx, "hello") // 应命中缓存

	if inner.embedN != 1 {
		t.Errorf("底层 Embed 应只被调用 1 次，实际 %d", inner.embedN)
	}
	if len(v1) != len(v2) || v1[0] != v2[0] {
		t.Errorf("两次返回向量应一致")
	}
	if stats.hits["embedding"] != 1 || stats.misses["embedding"] != 1 {
		t.Errorf("命中统计错误: hits=%d misses=%d", stats.hits["embedding"], stats.misses["embedding"])
	}
}

func TestCachedEmbedder_EmbedBatch_PartialHit(t *testing.T) {
	ctx := context.Background()
	inner := &fakeEmbedder{model: "test-model"}
	ce := NewCachedEmbedder(inner, newMemCache(), nil, 0)

	// 预热 "a"
	_, _ = ce.Embed(ctx, "a")
	if inner.embedN != 1 {
		t.Fatalf("预热后底层调用应为 1，实际 %d", inner.embedN)
	}

	// 批量 [a,b,c]：a 命中，b/c 未命中 → 底层只新增 2
	vecs, err := ce.EmbedBatch(ctx, []string{"a", "b", "c"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 3 {
		t.Fatalf("应返回 3 个向量，实际 %d", len(vecs))
	}
	if inner.embedN != 3 { // 1(预热) + 2(b,c)
		t.Errorf("底层累计调用应为 3，实际 %d", inner.embedN)
	}
	// 校验顺序正确（a 的向量与单独 Embed 一致）
	expectA := inner.vectorFor("a")
	if vecs[0][0] != expectA[0] {
		t.Errorf("批量结果顺序错乱，a 向量不匹配")
	}
}

func TestCachedEmbedder_DifferentModelDifferentKey(t *testing.T) {
	ctx := context.Background()
	shared := newMemCache()
	e1 := NewCachedEmbedder(&fakeEmbedder{model: "m1"}, shared, nil, 0)
	e2 := NewCachedEmbedder(&fakeEmbedder{model: "m2"}, shared, nil, 0)

	_, _ = e1.Embed(ctx, "same-text")
	_, _ = e2.Embed(ctx, "same-text")

	// 不同模型应使用不同 key，缓存中应有 2 条
	if n, _ := shared.Size(ctx); n != 2 {
		t.Errorf("不同模型应产生不同缓存 key，期望 2 条，实际 %d", n)
	}
}

// statsRecorder 测试用统计记录器
type statsRecorder struct {
	mu     sync.Mutex
	hits   map[string]int
	misses map[string]int
}

func newStatsRecorder() *statsRecorder {
	return &statsRecorder{hits: map[string]int{}, misses: map[string]int{}}
}
func (s *statsRecorder) RecordHit(c string)  { s.mu.Lock(); s.hits[c]++; s.mu.Unlock() }
func (s *statsRecorder) RecordMiss(c string) { s.mu.Lock(); s.misses[c]++; s.mu.Unlock() }
func (s *statsRecorder) Snapshot() []domain_cache.CategoryStat { return nil }
func (s *statsRecorder) Reset()                                {}

var _ domain_cache.StatsRecorder = (*statsRecorder)(nil)
