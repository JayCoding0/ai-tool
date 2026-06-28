package cache

import (
	"context"
	"testing"
	"time"
)

// newTestRedis 连接本地 Redis，不可用时跳过测试（CI 无 Redis 环境时安全跳过）
func newTestRedis(t *testing.T) *RedisCache {
	t.Helper()
	c := NewRedisCache(RedisOptions{Addr: "localhost:6379", DB: 15, Timeout: time.Second})
	if !c.Available() {
		t.Skip("本地 Redis 不可用，跳过集成测试")
	}
	return c
}

func TestRedisCache_SetGetDelete(t *testing.T) {
	ctx := context.Background()
	c := newTestRedis(t)
	defer c.Close()
	_ = c.Clear(ctx)

	key := "unit:k1"
	if err := c.Set(ctx, key, []byte("hello"), time.Minute); err != nil {
		t.Fatalf("Set 失败: %v", err)
	}
	v, found, err := c.Get(ctx, key)
	if err != nil || !found || string(v) != "hello" {
		t.Fatalf("Get 错误: v=%s found=%v err=%v", string(v), found, err)
	}

	if err := c.Delete(ctx, key); err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}
	if _, found, _ := c.Get(ctx, key); found {
		t.Error("删除后不应再命中")
	}
}

func TestRedisCache_Miss(t *testing.T) {
	ctx := context.Background()
	c := newTestRedis(t)
	defer c.Close()
	_ = c.Clear(ctx)

	if _, found, err := c.Get(ctx, "unit:not-exist"); err != nil || found {
		t.Errorf("不存在的 key 应未命中，found=%v err=%v", found, err)
	}
}

func TestRedisCache_SizeAndClear(t *testing.T) {
	ctx := context.Background()
	c := newTestRedis(t)
	defer c.Close()
	_ = c.Clear(ctx)

	for i := 0; i < 5; i++ {
		_ = c.Set(ctx, "unit:size:"+string(rune('a'+i)), []byte("v"), time.Minute)
	}
	n, err := c.Size(ctx)
	if err != nil || n != 5 {
		t.Fatalf("Size 期望 5，得到 %d err=%v", n, err)
	}
	if err := c.Clear(ctx); err != nil {
		t.Fatalf("Clear 失败: %v", err)
	}
	if n, _ := c.Size(ctx); n != 0 {
		t.Errorf("Clear 后 Size 应为 0，得到 %d", n)
	}
}

func TestRedisCache_TTLExpiry(t *testing.T) {
	ctx := context.Background()
	c := newTestRedis(t)
	defer c.Close()
	_ = c.Clear(ctx)

	_ = c.Set(ctx, "unit:ttl", []byte("v"), 500*time.Millisecond)
	if _, found, _ := c.Get(ctx, "unit:ttl"); !found {
		t.Fatal("写入后应立即命中")
	}
	time.Sleep(700 * time.Millisecond)
	if _, found, _ := c.Get(ctx, "unit:ttl"); found {
		t.Error("超过 TTL 后应过期未命中")
	}
}
