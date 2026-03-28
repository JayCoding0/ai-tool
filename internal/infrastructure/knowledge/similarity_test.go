package knowledge

import (
	"math"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// Float32SliceToBytes / BytesToFloat32Slice 互转测试
// ─────────────────────────────────────────────────────────────────────────────

func TestFloat32SliceToBytes_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		vec  []float32
	}{
		{"空向量", nil},
		{"单元素", []float32{3.14}},
		{"多元素", []float32{1.0, -2.5, 0, 100.123, -0.001}},
		{"零向量", []float32{0, 0, 0}},
		{"极值", []float32{math.MaxFloat32, math.SmallestNonzeroFloat32, -math.MaxFloat32}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bytes := Float32SliceToBytes(tc.vec)
			got := BytesToFloat32Slice(bytes)

			if len(tc.vec) == 0 && len(got) == 0 {
				return // 空向量，两者都为空即可
			}
			if len(got) != len(tc.vec) {
				t.Fatalf("长度不匹配: 期望 %d, 实际 %d", len(tc.vec), len(got))
			}
			for i := range tc.vec {
				if got[i] != tc.vec[i] {
					t.Errorf("索引 %d: 期望 %v, 实际 %v", i, tc.vec[i], got[i])
				}
			}
		})
	}
}

func TestBytesToFloat32Slice_InvalidLength(t *testing.T) {
	// 长度不是 4 的倍数，应返回 nil
	result := BytesToFloat32Slice([]byte{1, 2, 3})
	if result != nil {
		t.Errorf("期望 nil，实际 %v", result)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// CosineSimilarity 测试
// ─────────────────────────────────────────────────────────────────────────────

func TestCosineSimilarity_IdenticalVectors(t *testing.T) {
	// 完全相同的向量，相似度应为 1
	a := []float32{1, 2, 3}
	got := CosineSimilarity(a, a)
	if math.Abs(float64(got)-1.0) > 1e-6 {
		t.Errorf("相同向量相似度应为 1，实际 %v", got)
	}
}

func TestCosineSimilarity_OrthogonalVectors(t *testing.T) {
	// 正交向量，相似度应为 0
	a := []float32{1, 0}
	b := []float32{0, 1}
	got := CosineSimilarity(a, b)
	if math.Abs(float64(got)) > 1e-6 {
		t.Errorf("正交向量相似度应为 0，实际 %v", got)
	}
}

func TestCosineSimilarity_OppositeVectors(t *testing.T) {
	// 反向向量，相似度应为 -1
	a := []float32{1, 2, 3}
	b := []float32{-1, -2, -3}
	got := CosineSimilarity(a, b)
	if math.Abs(float64(got)+1.0) > 1e-6 {
		t.Errorf("反向向量相似度应为 -1，实际 %v", got)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	// 零向量，应返回 0
	a := []float32{0, 0, 0}
	b := []float32{1, 2, 3}
	got := CosineSimilarity(a, b)
	if got != 0 {
		t.Errorf("零向量相似度应为 0，实际 %v", got)
	}
}

func TestCosineSimilarity_DifferentLength(t *testing.T) {
	// 不等长向量，应返回 0
	a := []float32{1, 2}
	b := []float32{1, 2, 3}
	got := CosineSimilarity(a, b)
	if got != 0 {
		t.Errorf("不等长向量相似度应为 0，实际 %v", got)
	}
}

func TestCosineSimilarity_EmptyVectors(t *testing.T) {
	got := CosineSimilarity(nil, nil)
	if got != 0 {
		t.Errorf("空向量相似度应为 0，实际 %v", got)
	}
}
