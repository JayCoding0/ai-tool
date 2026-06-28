package cache

import (
	"sync"
	"testing"
)

func TestStatsCollector_HitMiss(t *testing.T) {
	c := NewStatsCollector()
	c.RecordHit("embedding")
	c.RecordHit("embedding")
	c.RecordMiss("embedding")

	snap := c.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("期望 1 个类别，得到 %d", len(snap))
	}
	s := snap[0]
	if s.Category != "embedding" {
		t.Errorf("类别名错误: %s", s.Category)
	}
	if s.Hits != 2 || s.Misses != 1 || s.Total != 3 {
		t.Errorf("计数错误: hits=%d misses=%d total=%d", s.Hits, s.Misses, s.Total)
	}
	if got, want := s.HitRate, 2.0/3.0; got < want-1e-9 || got > want+1e-9 {
		t.Errorf("命中率错误: got=%v want=%v", got, want)
	}
}

func TestStatsCollector_EmptyHitRate(t *testing.T) {
	c := NewStatsCollector()
	c.RecordMiss("x")
	snap := c.Snapshot()
	if snap[0].HitRate != 0 {
		t.Errorf("无命中时命中率应为 0，得到 %v", snap[0].HitRate)
	}
}

func TestStatsCollector_MultiCategorySorted(t *testing.T) {
	c := NewStatsCollector()
	c.RecordHit("semantic")
	c.RecordHit("embedding")
	snap := c.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("期望 2 个类别，得到 %d", len(snap))
	}
	// Snapshot 按类别名排序，embedding 应在 semantic 之前
	if snap[0].Category != "embedding" || snap[1].Category != "semantic" {
		t.Errorf("类别未按字典序排序: %s, %s", snap[0].Category, snap[1].Category)
	}
}

func TestStatsCollector_Reset(t *testing.T) {
	c := NewStatsCollector()
	c.RecordHit("a")
	c.Reset()
	if len(c.Snapshot()) != 0 {
		t.Errorf("Reset 后应无统计")
	}
}

func TestStatsCollector_Concurrent(t *testing.T) {
	c := NewStatsCollector()
	const goroutines = 50
	const perG = 200
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				c.RecordHit("embedding")
				c.RecordMiss("embedding")
			}
		}()
	}
	wg.Wait()

	snap := c.Snapshot()
	wantEach := int64(goroutines * perG)
	if snap[0].Hits != wantEach || snap[0].Misses != wantEach {
		t.Errorf("并发计数错误: hits=%d misses=%d want=%d", snap[0].Hits, snap[0].Misses, wantEach)
	}
}
