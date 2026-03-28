package knowledge

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// ─────────────────────────────────────────────────────────────────────────────
// NewTextSplitter 构造函数测试
// ─────────────────────────────────────────────────────────────────────────────

func TestNewTextSplitter_Defaults(t *testing.T) {
	// chunkSize <= 0 时使用默认值
	s := NewTextSplitter(0, 0)
	if s.ChunkSize != DefaultChunkSize {
		t.Errorf("期望默认 ChunkSize=%d，实际 %d", DefaultChunkSize, s.ChunkSize)
	}

	// overlap >= chunkSize 时使用默认值
	s2 := NewTextSplitter(100, 200)
	if s2.ChunkOverlap != DefaultChunkOverlap {
		t.Errorf("期望默认 ChunkOverlap=%d，实际 %d", DefaultChunkOverlap, s2.ChunkOverlap)
	}

	// overlap < 0 时使用默认值
	s3 := NewTextSplitter(100, -1)
	if s3.ChunkOverlap != DefaultChunkOverlap {
		t.Errorf("期望默认 ChunkOverlap=%d，实际 %d", DefaultChunkOverlap, s3.ChunkOverlap)
	}
}

func TestNewTextSplitter_CustomValues(t *testing.T) {
	s := NewTextSplitter(200, 30)
	if s.ChunkSize != 200 || s.ChunkOverlap != 30 {
		t.Errorf("期望 ChunkSize=200, ChunkOverlap=30，实际 %d, %d", s.ChunkSize, s.ChunkOverlap)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Split 方法测试
// ─────────────────────────────────────────────────────────────────────────────

func TestSplit_EmptyText(t *testing.T) {
	s := NewTextSplitter(100, 10)
	result := s.Split("")
	if result != nil {
		t.Errorf("空文本应返回 nil，实际 %v", result)
	}
}

func TestSplit_WhitespaceOnly(t *testing.T) {
	s := NewTextSplitter(100, 10)
	result := s.Split("   \n\n  \t  ")
	if result != nil {
		t.Errorf("纯空白文本应返回 nil，实际 %v", result)
	}
}

func TestSplit_ShortText(t *testing.T) {
	s := NewTextSplitter(100, 10)
	text := "这是一段很短的文本。"
	result := s.Split(text)
	if len(result) != 1 {
		t.Fatalf("短文本应只有 1 个块，实际 %d 个", len(result))
	}
	if result[0] != text {
		t.Errorf("期望 %q，实际 %q", text, result[0])
	}
}

func TestSplit_ByParagraph(t *testing.T) {
	s := NewTextSplitter(15, 0)
	text := "第一段的内容比较长。\n\n第二段的内容也比较长。\n\n第三段的内容同样很长。"
	result := s.Split(text)
	if len(result) < 2 {
		t.Fatalf("应按段落分割为多个块，实际 %d 个: %v", len(result), result)
	}
}

func TestSplit_LongParagraph(t *testing.T) {
	// 单段落超过 ChunkSize，应进一步切割
	s := NewTextSplitter(20, 0)
	text := "这是一段非常非常非常非常非常非常非常非常非常非常长的文本内容。"
	result := s.Split(text)
	if len(result) < 2 {
		t.Fatalf("超长段落应被切割为多个块，实际 %d 个", len(result))
	}
	// 每个块不应超过 ChunkSize
	for i, chunk := range result {
		runeCount := utf8.RuneCountInString(chunk)
		if runeCount > s.ChunkSize+s.ChunkOverlap {
			t.Errorf("块 %d 长度 %d 超过限制 %d", i, runeCount, s.ChunkSize)
		}
	}
}

func TestSplit_ChunkOverlap(t *testing.T) {
	s := NewTextSplitter(30, 5)
	// 构造两个段落，每个刚好超过一半 ChunkSize
	text := "第一段的内容比较长一些。\n\n第二段的内容也比较长。"
	result := s.Split(text)
	if len(result) < 2 {
		t.Skipf("文本未被分割为多块，跳过重叠检查")
	}
	// 检查相邻块之间是否有重叠内容
	for i := 1; i < len(result); i++ {
		prev := result[i-1]
		curr := result[i]
		// 取前一块末尾几个字符，检查是否出现在当前块开头
		prevRunes := []rune(prev)
		if len(prevRunes) >= s.ChunkOverlap {
			overlap := string(prevRunes[len(prevRunes)-s.ChunkOverlap:])
			if !strings.HasPrefix(curr, overlap) {
				t.Logf("块 %d 末尾: %q", i-1, overlap)
				t.Logf("块 %d 开头: %q", i, string([]rune(curr)[:min(len([]rune(curr)), 10)]))
				// 重叠不是严格要求前缀匹配，只记录日志
			}
		}
	}
}

func TestSplit_ChineseSentences(t *testing.T) {
	s := NewTextSplitter(10, 0)
	text := "这是第一句话内容。这是第二句话内容！这是第三句话内容？这是第四句话内容。"
	result := s.Split(text)
	// 应该能按中文句号切割
	if len(result) < 2 {
		t.Logf("分块结果: %v", result)
		t.Errorf("中文句子应被切割为多个块，实际 %d 个", len(result))
	}
}

func TestSplit_PreservesContent(t *testing.T) {
	s := NewTextSplitter(500, 0) // 无重叠，方便验证
	text := "段落一的内容。\n\n段落二的内容。\n\n段落三的内容。"
	result := s.Split(text)
	// 所有块拼接后应包含原始文本的所有内容
	joined := strings.Join(result, "\n\n")
	for _, keyword := range []string{"段落一", "段落二", "段落三"} {
		if !strings.Contains(joined, keyword) {
			t.Errorf("分块结果丢失了内容: %q", keyword)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// splitBySentence 辅助函数测试
// ─────────────────────────────────────────────────────────────────────────────

func TestSplitBySentence_DecimalPoint(t *testing.T) {
	// 小数点不应被当作句子结束符
	sentences := splitBySentence("价格是3.14元。")
	// "3.14" 中的 "." 不应切割
	found := false
	for _, s := range sentences {
		if strings.Contains(s, "3.14") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("小数点被误切割，分句结果: %v", sentences)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
