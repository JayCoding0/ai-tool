package application

import (
	"context"
	"sync"
	"testing"
	"time"

	domain_cache "aiProject/internal/domain/cache"
	"aiProject/internal/domain/knowledge"
)

// semMemCache 测试用进程内缓存
type semMemCache struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newSemMemCache() *semMemCache { return &semMemCache{data: map[string][]byte{}} }

func (m *semMemCache) Get(_ context.Context, k string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[k]
	return v, ok, nil
}
func (m *semMemCache) Set(_ context.Context, k string, v []byte, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(v))
	copy(cp, v)
	m.data[k] = cp
	return nil
}
func (m *semMemCache) Delete(_ context.Context, k string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, k)
	return nil
}
func (m *semMemCache) Size(context.Context) (int64, error) { return int64(len(m.data)), nil }
func (m *semMemCache) Clear(context.Context) error         { m.data = map[string][]byte{}; return nil }
func (m *semMemCache) Available() bool                     { return true }
func (m *semMemCache) Backend() string                     { return "mem" }

var _ domain_cache.Cache = (*semMemCache)(nil)

// vecEmbedder 可控向量 Embedder：优先用 fixed 中预设的向量，否则按文本分配唯一 one-hot 向量
type vecEmbedder struct {
	mu    sync.Mutex
	fixed map[string][]float32
	idx   map[string]int
	next  int
}

func newVecEmbedder() *vecEmbedder {
	return &vecEmbedder{fixed: map[string][]float32{}, idx: map[string]int{}}
}
func (e *vecEmbedder) set(text string, v []float32) { e.fixed[text] = v }
func (e *vecEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if v, ok := e.fixed[text]; ok {
		return v, nil
	}
	i, ok := e.idx[text]
	if !ok {
		i = e.next
		e.idx[text] = i
		e.next++
	}
	v := make([]float32, 64)
	v[i%64] = 1
	return v, nil
}
func (e *vecEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i], _ = e.Embed(ctx, t)
	}
	return out, nil
}
func (e *vecEmbedder) Dimensions() int   { return 64 }
func (e *vecEmbedder) ModelName() string { return "vec-test" }

var _ knowledge.Embedder = (*vecEmbedder)(nil)

func TestSemanticScope_HashStableAndDistinct(t *testing.T) {
	a := SemanticScope{ModelName: "m", SystemPrompt: "p", KnowledgeBaseID: 1}
	b := SemanticScope{ModelName: "m", SystemPrompt: "p", KnowledgeBaseID: 1}
	c := SemanticScope{ModelName: "m", SystemPrompt: "p", KnowledgeBaseID: 2}
	if a.hash() != b.hash() {
		t.Error("相同 scope 哈希应一致")
	}
	if a.hash() == c.hash() {
		t.Error("不同 KnowledgeBaseID 哈希应不同")
	}
}

func TestSemanticCache_StoreThenHit(t *testing.T) {
	ctx := context.Background()
	svc := NewSemanticCacheService(newSemMemCache(), newVecEmbedder(), nil, 0.92, 50)
	scope := SemanticScope{ModelName: "m", SystemPrompt: "p"}

	if _, hit := svc.Lookup(ctx, scope, "你好"); hit {
		t.Error("空缓存不应命中")
	}
	svc.Store(ctx, scope, "你好", "你好，有什么可以帮你？")

	ans, hit := svc.Lookup(ctx, scope, "你好")
	if !hit {
		t.Fatal("相同问题应命中")
	}
	if ans != "你好，有什么可以帮你？" {
		t.Errorf("返回答案错误: %s", ans)
	}
}

func TestSemanticCache_DifferentQuestionMiss(t *testing.T) {
	ctx := context.Background()
	svc := NewSemanticCacheService(newSemMemCache(), newVecEmbedder(), nil, 0.92, 50)
	scope := SemanticScope{ModelName: "m"}
	svc.Store(ctx, scope, "今天天气", "晴天")
	if _, hit := svc.Lookup(ctx, scope, "股票走势"); hit {
		t.Error("语义不相关的问题不应命中")
	}
}

func TestSemanticCache_ScopeIsolation(t *testing.T) {
	ctx := context.Background()
	svc := NewSemanticCacheService(newSemMemCache(), newVecEmbedder(), nil, 0.92, 50)
	scopeA := SemanticScope{ModelName: "m", KnowledgeBaseID: 1}
	scopeB := SemanticScope{ModelName: "m", KnowledgeBaseID: 2}
	svc.Store(ctx, scopeA, "问题", "答案A")
	if _, hit := svc.Lookup(ctx, scopeB, "问题"); hit {
		t.Error("不同 scope 不应命中")
	}
}

func TestSemanticCache_Threshold(t *testing.T) {
	ctx := context.Background()
	emb := newVecEmbedder()
	// 构造可控相似度：stored=[1,0]，near≈cos0.96，far=cos0.5
	emb.set("stored", []float32{1, 0})
	emb.set("near", []float32{0.96, 0.28})  // cos ≈ 0.96 > 0.92 → 命中
	emb.set("far", []float32{0.5, 0.866})   // cos = 0.5 < 0.92 → 未命中
	svc := NewSemanticCacheService(newSemMemCache(), emb, nil, 0.92, 50)
	scope := SemanticScope{ModelName: "m"}
	svc.Store(ctx, scope, "stored", "缓存答案")

	if _, hit := svc.Lookup(ctx, scope, "near"); !hit {
		t.Error("相似度高于阈值应命中")
	}
	if _, hit := svc.Lookup(ctx, scope, "far"); hit {
		t.Error("相似度低于阈值不应命中")
	}
}

func TestSemanticCache_FIFOEviction(t *testing.T) {
	ctx := context.Background()
	svc := NewSemanticCacheService(newSemMemCache(), newVecEmbedder(), nil, 0.92, 2)
	scope := SemanticScope{ModelName: "m"}
	svc.Store(ctx, scope, "q1", "a1")
	svc.Store(ctx, scope, "q2", "a2")
	svc.Store(ctx, scope, "q3", "a3") // 超过容量 2，q1 应被淘汰

	if _, hit := svc.Lookup(ctx, scope, "q1"); hit {
		t.Error("最旧记录 q1 应被 FIFO 淘汰")
	}
	if _, hit := svc.Lookup(ctx, scope, "q3"); !hit {
		t.Error("最新记录 q3 应命中")
	}
}
