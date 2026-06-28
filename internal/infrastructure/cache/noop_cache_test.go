package cache

import (
	"context"
	"testing"
	"time"
)

func TestNoopCache(t *testing.T) {
	ctx := context.Background()
	c := NewNoopCache()

	if c.Available() {
		t.Error("NoopCache 应始终不可用")
	}
	if c.Backend() != "noop" {
		t.Errorf("Backend 应为 noop，得到 %s", c.Backend())
	}

	if err := c.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Errorf("Set 不应报错: %v", err)
	}
	if _, found, err := c.Get(ctx, "k"); err != nil || found {
		t.Errorf("NoopCache Get 应始终未命中，found=%v err=%v", found, err)
	}
	if n, err := c.Size(ctx); err != nil || n != 0 {
		t.Errorf("Size 应为 0，得到 %d err=%v", n, err)
	}
	if err := c.Delete(ctx, "k"); err != nil {
		t.Errorf("Delete 不应报错: %v", err)
	}
	if err := c.Clear(ctx); err != nil {
		t.Errorf("Clear 不应报错: %v", err)
	}
}
