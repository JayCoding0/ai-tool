package application

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"aiProject/internal/domain/knowledge"
	infra_knowledge "aiProject/internal/infrastructure/knowledge"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// KnowledgeService 知识库应用服务
type KnowledgeService struct {
	repo     knowledge.Repository
	embedder knowledge.Embedder
	splitter *infra_knowledge.TextSplitter
}

// NewKnowledgeService 创建知识库服务
func NewKnowledgeService(repo knowledge.Repository, embedder knowledge.Embedder) *KnowledgeService {
	return &KnowledgeService{
		repo:     repo,
		embedder: embedder,
		splitter: infra_knowledge.NewTextSplitter(500, 50),
	}
}

// ─── 知识库管理 ────────────────────────────────────────────────────────────────

// CreateKnowledgeBase 创建知识库
func (s *KnowledgeService) CreateKnowledgeBase(ctx context.Context, userID int64, name, description string) (*knowledge.KnowledgeBase, error) {
	if name == "" {
		return nil, fmt.Errorf("知识库名称不能为空")
	}
	kb := &knowledge.KnowledgeBase{
		UserID:      userID,
		Name:        name,
		Description: description,
		EmbedModel:  s.embedder.ModelName(),
	}
	if err := s.repo.CreateKnowledgeBase(ctx, kb); err != nil {
		return nil, fmt.Errorf("创建知识库失败: %w", err)
	}
	return kb, nil
}

// GetKnowledgeBase 获取知识库详情
func (s *KnowledgeService) GetKnowledgeBase(ctx context.Context, id int64) (*knowledge.KnowledgeBase, error) {
	return s.repo.GetKnowledgeBase(ctx, id)
}

// ListKnowledgeBases 列出知识库
func (s *KnowledgeService) ListKnowledgeBases(ctx context.Context, userID int64) ([]*knowledge.KnowledgeBase, error) {
	return s.repo.ListKnowledgeBases(ctx, userID)
}

// DeleteKnowledgeBase 删除知识库
func (s *KnowledgeService) DeleteKnowledgeBase(ctx context.Context, id int64) error {
	return s.repo.DeleteKnowledgeBase(ctx, id)
}

// ─── 文档管理 ──────────────────────────────────────────────────────────────────

// AddDocument 添加文档到知识库（解析 → 分块 → 向量化 → 存储）
// 异步处理，立即返回文档 ID，处理状态通过 GetDocument 查询
func (s *KnowledgeService) AddDocument(ctx context.Context, kbID int64, name, contentType, content string) (*knowledge.Document, error) {
	if content == "" {
		return nil, fmt.Errorf("文档内容不能为空")
	}

	charCount := utf8.RuneCountInString(content)
	doc := &knowledge.Document{
		KnowledgeBaseID: kbID,
		Name:            name,
		ContentType:     contentType,
		CharCount:       charCount,
		Status:          knowledge.StatusPending,
	}
	if err := s.repo.CreateDocument(ctx, doc); err != nil {
		return nil, fmt.Errorf("创建文档记录失败: %w", err)
	}

	// 异步处理：分块 + 向量化
	go s.processDocument(context.Background(), doc, content, kbID)

	return doc, nil
}

// processDocument 异步处理文档：分块 → 向量化 → 存储
func (s *KnowledgeService) processDocument(ctx context.Context, doc *knowledge.Document, content string, kbID int64) {
	logger := shared.GetLogger()

	// 更新状态为处理中
	if err := s.repo.UpdateDocumentStatus(ctx, doc.ID, knowledge.StatusProcessing, ""); err != nil {
		logger.Error("更新文档状态失败", zap.Int64("doc_id", doc.ID), zap.Error(err))
		return
	}

	// 1. 文本分块
	textChunks := s.splitter.Split(content)
	if len(textChunks) == 0 {
		s.repo.UpdateDocumentStatus(ctx, doc.ID, knowledge.StatusFailed, "文档内容为空，无法分块") //nolint:errcheck
		return
	}

	logger.Info("文档分块完成", zap.Int64("doc_id", doc.ID), zap.Int("chunks", len(textChunks)))

	// 2. 批量向量化（每批最多 20 条，避免超出 API 限制）
	const batchSize = 20
	var chunks []*knowledge.Chunk

	for i := 0; i < len(textChunks); i += batchSize {
		end := i + batchSize
		if end > len(textChunks) {
			end = len(textChunks)
		}
		batch := textChunks[i:end]

		embeddings, err := s.embedder.EmbedBatch(ctx, batch)
		if err != nil {
			logger.Error("向量化失败", zap.Int64("doc_id", doc.ID), zap.Error(err))
			s.repo.UpdateDocumentStatus(ctx, doc.ID, knowledge.StatusFailed, fmt.Sprintf("向量化失败: %v", err)) //nolint:errcheck
			return
		}

		for j, text := range batch {
			chunks = append(chunks, &knowledge.Chunk{
				DocumentID:      doc.ID,
				KnowledgeBaseID: kbID,
				Content:         text,
				Embedding:       infra_knowledge.Float32SliceToBytes(embeddings[j]),
				ChunkIndex:      i + j,
				TokenCount:      estimateTokens(text),
			})
		}
	}

	// 3. 批量存储分块
	if err := s.repo.CreateChunks(ctx, chunks); err != nil {
		logger.Error("存储分块失败", zap.Int64("doc_id", doc.ID), zap.Error(err))
		s.repo.UpdateDocumentStatus(ctx, doc.ID, knowledge.StatusFailed, fmt.Sprintf("存储分块失败: %v", err)) //nolint:errcheck
		return
	}

	// 4. 更新文档状态和分块数
	s.repo.UpdateDocumentChunkCount(ctx, doc.ID, len(chunks)) //nolint:errcheck
	s.repo.UpdateDocumentStatus(ctx, doc.ID, knowledge.StatusDone, "") //nolint:errcheck

	// 5. 更新知识库统计
	s.refreshKBStats(ctx, kbID)

	logger.Info("文档处理完成",
		zap.Int64("doc_id", doc.ID),
		zap.String("name", doc.Name),
		zap.Int("chunks", len(chunks)),
	)
}

// refreshKBStats 重新统计知识库的文档数和分块数
func (s *KnowledgeService) refreshKBStats(ctx context.Context, kbID int64) {
	docs, err := s.repo.ListDocuments(ctx, kbID)
	if err != nil {
		return
	}
	docCount := len(docs)
	chunkCount := 0
	for _, d := range docs {
		chunkCount += d.ChunkCount
	}
	s.repo.UpdateKnowledgeBaseStats(ctx, kbID, docCount, chunkCount) //nolint:errcheck
}

// GetDocument 获取文档详情（含处理状态）
func (s *KnowledgeService) GetDocument(ctx context.Context, id int64) (*knowledge.Document, error) {
	return s.repo.GetDocument(ctx, id)
}

// ListDocuments 列出知识库下的文档
func (s *KnowledgeService) ListDocuments(ctx context.Context, kbID int64) ([]*knowledge.Document, error) {
	return s.repo.ListDocuments(ctx, kbID)
}

// DeleteDocument 删除文档及其分块
func (s *KnowledgeService) DeleteDocument(ctx context.Context, docID int64) error {
	doc, err := s.repo.GetDocument(ctx, docID)
	if err != nil {
		return err
	}
	// 先删分块，再删文档
	if err := s.repo.DeleteChunksByDocument(ctx, docID); err != nil {
		return err
	}
	if err := s.repo.DeleteDocument(ctx, docID); err != nil {
		return err
	}
	// 更新知识库统计
	s.refreshKBStats(context.Background(), doc.KnowledgeBaseID)
	return nil
}

// ─── 语义检索 ──────────────────────────────────────────────────────────────────

// Search 语义检索，返回最相关的 topK 个分块
func (s *KnowledgeService) Search(ctx context.Context, kbID int64, query string, topK int) ([]knowledge.ScoredChunk, error) {
	if query == "" {
		return nil, nil
	}
	if topK <= 0 {
		topK = 5
	}

	// 1. 问题向量化
	queryVec, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("问题向量化失败: %w", err)
	}

	// 2. 读取所有分块
	chunks, err := s.repo.ListChunks(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("读取分块失败: %w", err)
	}
	if len(chunks) == 0 {
		return nil, nil
	}

	// 3. 计算余弦相似度
	type scored struct {
		chunk *knowledge.Chunk
		score float32
	}
	results := make([]scored, 0, len(chunks))
	for _, chunk := range chunks {
		chunkVec := infra_knowledge.BytesToFloat32Slice(chunk.Embedding)
		score := infra_knowledge.CosineSimilarity(queryVec, chunkVec)
		results = append(results, scored{chunk: chunk, score: score})
	}

	// 4. 按相似度降序排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// 5. 取 TopK，过滤低相似度结果（阈值 0.3）
	const minScore = 0.3
	if topK > len(results) {
		topK = len(results)
	}
	out := make([]knowledge.ScoredChunk, 0, topK)
	for i := 0; i < topK; i++ {
		if results[i].score < minScore {
			break
		}
		out = append(out, knowledge.ScoredChunk{
			Chunk: results[i].chunk,
			Score: results[i].score,
		})
	}
	return out, nil
}

// BuildRAGContext 将检索结果格式化为注入 System Prompt 的文本
func BuildRAGContext(chunks []knowledge.ScoredChunk) string {
	if len(chunks) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, sc := range chunks {
		sb.WriteString(fmt.Sprintf("[%d] %s\n\n", i+1, sc.Chunk.Content))
	}
	return strings.TrimSpace(sb.String())
}

// estimateTokens 粗略估算 token 数（中文约 1.5 字/token，英文约 4 字/token）
func estimateTokens(text string) int {
	return utf8.RuneCountInString(text) * 2 / 3
}
