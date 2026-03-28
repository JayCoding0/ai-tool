package knowledge

import (
	"strings"
	"unicode/utf8"
)

const (
	// DefaultChunkSize 默认分块大小（字符数）
	DefaultChunkSize = 500
	// DefaultChunkOverlap 相邻分块重叠字符数（保留语义连续性）
	DefaultChunkOverlap = 50
)

// TextSplitter 文本分块器
type TextSplitter struct {
	ChunkSize    int // 每块最大字符数
	ChunkOverlap int // 相邻块重叠字符数
}

// NewTextSplitter 创建文本分块器
func NewTextSplitter(chunkSize, chunkOverlap int) *TextSplitter {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	if chunkOverlap < 0 || chunkOverlap >= chunkSize {
		chunkOverlap = DefaultChunkOverlap
	}
	return &TextSplitter{
		ChunkSize:    chunkSize,
		ChunkOverlap: chunkOverlap,
	}
}

// Split 将文本分割为多个块
// 策略：优先按段落（双换行）分割，段落过长时再按句子分割，最终按字符数截断
func (s *TextSplitter) Split(text string) []string {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) == 0 {
		return nil
	}

	// 先按段落分割
	paragraphs := splitByParagraph(text)

	var chunks []string
	var current strings.Builder

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		paraLen := utf8.RuneCountInString(para)
		curLen := utf8.RuneCountInString(current.String())

		if curLen+paraLen <= s.ChunkSize {
			// 当前段落加入后不超限，直接追加
			if current.Len() > 0 {
				current.WriteString("\n\n")
			}
			current.WriteString(para)
		} else {
			// 当前缓冲区非空，先保存
			if current.Len() > 0 {
				chunks = append(chunks, current.String())
				// 保留重叠部分
				overlap := s.getOverlapText(current.String())
				current.Reset()
				current.WriteString(overlap)
			}

			// 段落本身超过 ChunkSize，需要进一步切割
			if paraLen > s.ChunkSize {
				subChunks := s.splitLongText(para)
				for i, sub := range subChunks {
					if i < len(subChunks)-1 {
						chunks = append(chunks, sub)
					} else {
						// 最后一个子块放入缓冲区继续合并
						current.Reset()
						current.WriteString(sub)
					}
				}
			} else {
				if current.Len() > 0 {
					current.WriteString("\n\n")
				}
				current.WriteString(para)
			}
		}
	}

	// 保存最后一块
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}

// splitByParagraph 按双换行分割段落
func splitByParagraph(text string) []string {
	// 统一换行符
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Split(text, "\n\n")
}

// splitLongText 将超长文本按句子或字符数切割
func (s *TextSplitter) splitLongText(text string) []string {
	// 先尝试按句子分割（。！？.!?）
	sentences := splitBySentence(text)

	var chunks []string
	var current strings.Builder

	for _, sent := range sentences {
		sentLen := utf8.RuneCountInString(sent)
		curLen := utf8.RuneCountInString(current.String())

		if curLen+sentLen <= s.ChunkSize {
			current.WriteString(sent)
		} else {
			if current.Len() > 0 {
				chunks = append(chunks, current.String())
				overlap := s.getOverlapText(current.String())
				current.Reset()
				current.WriteString(overlap)
			}
			// 单句超长，强制按字符数截断
			if sentLen > s.ChunkSize {
				runes := []rune(sent)
				for start := 0; start < len(runes); start += s.ChunkSize - s.ChunkOverlap {
					end := start + s.ChunkSize
					if end > len(runes) {
						end = len(runes)
					}
					chunks = append(chunks, string(runes[start:end]))
					if end == len(runes) {
						break
					}
				}
			} else {
				current.WriteString(sent)
			}
		}
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

// splitBySentence 按中英文句子结束符分割
func splitBySentence(text string) []string {
	var sentences []string
	var current strings.Builder
	runes := []rune(text)
	for i, r := range runes {
		current.WriteRune(r)
		// 句子结束符
		if r == '。' || r == '！' || r == '？' || r == '.' || r == '!' || r == '?' || r == '\n' {
			// 避免小数点误判（前后都是数字时不切割）
			if r == '.' && i > 0 && i < len(runes)-1 {
				prev := runes[i-1]
				next := runes[i+1]
				if prev >= '0' && prev <= '9' && next >= '0' && next <= '9' {
					continue
				}
			}
			sentences = append(sentences, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		sentences = append(sentences, current.String())
	}
	return sentences
}

// getOverlapText 从文本末尾取重叠部分（按字符数）
func (s *TextSplitter) getOverlapText(text string) string {
	if s.ChunkOverlap <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= s.ChunkOverlap {
		return text
	}
	return string(runes[len(runes)-s.ChunkOverlap:])
}
