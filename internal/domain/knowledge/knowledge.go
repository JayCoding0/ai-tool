package knowledge

import (
	"context"
	"time"
)

// KnowledgeBase 知识库聚合根
type KnowledgeBase struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	EmbedModel  string    `json:"embed_model"` // 使用的 embedding 模型名称
	DocCount    int       `json:"doc_count"`
	ChunkCount  int       `json:"chunk_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// DocumentStatus 文档处理状态
type DocumentStatus string

const (
	StatusPending    DocumentStatus = "pending"
	StatusProcessing DocumentStatus = "processing"
	StatusDone       DocumentStatus = "done"
	StatusFailed     DocumentStatus = "failed"
)

// Document 知识库文档实体
type Document struct {
	ID              int64          `json:"id"`
	KnowledgeBaseID int64          `json:"knowledge_base_id"`
	Name            string         `json:"name"`         // 文件名
	ContentType     string         `json:"content_type"` // text/markdown/pdf
	CharCount       int            `json:"char_count"`
	ChunkCount      int            `json:"chunk_count"`
	Status          DocumentStatus `json:"status"`
	ErrorMsg        string         `json:"error_msg,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
}

// Chunk 文档分块值对象（含向量）
type Chunk struct {
	ID              int64     `json:"id"`
	DocumentID      int64     `json:"document_id"`
	KnowledgeBaseID int64     `json:"knowledge_base_id"`
	Content         string    `json:"content"`     // 原始文本片段
	Embedding       []byte    `json:"-"`           // 向量二进制（BLOB）
	ChunkIndex      int       `json:"chunk_index"` // 在文档中的顺序
	TokenCount      int       `json:"token_count"`
	CreatedAt       time.Time `json:"created_at"`
}

// ScoredChunk 带相似度分数的分块（检索结果）
type ScoredChunk struct {
	Chunk    *Chunk
	Score    float32
	DocName  string // 来源文档名称
}

// Embedder 向量化接口（与具体模型解耦）
type Embedder interface {
	// Embed 将单条文本转为向量
	Embed(ctx context.Context, text string) ([]float32, error)
	// EmbedBatch 批量向量化（减少 API 调用次数）
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	// Dimensions 返回向量维度
	Dimensions() int
	// ModelName 返回模型名称
	ModelName() string
}

// Repository 知识库仓储接口
type Repository interface {
	// 知识库 CRUD
	CreateKnowledgeBase(ctx context.Context, kb *KnowledgeBase) error
	GetKnowledgeBase(ctx context.Context, id int64) (*KnowledgeBase, error)
	ListKnowledgeBases(ctx context.Context, userID int64) ([]*KnowledgeBase, error)
	DeleteKnowledgeBase(ctx context.Context, id int64) error
	UpdateKnowledgeBaseStats(ctx context.Context, id int64, docCount, chunkCount int) error

	// 文档 CRUD
	CreateDocument(ctx context.Context, doc *Document) error
	GetDocument(ctx context.Context, id int64) (*Document, error)
	ListDocuments(ctx context.Context, kbID int64) ([]*Document, error)
	UpdateDocumentStatus(ctx context.Context, id int64, status DocumentStatus, errMsg string) error
	UpdateDocumentChunkCount(ctx context.Context, id int64, chunkCount int) error
	DeleteDocument(ctx context.Context, id int64) error

	// 分块 CRUD
	CreateChunks(ctx context.Context, chunks []*Chunk) error
	ListChunks(ctx context.Context, kbID int64) ([]*Chunk, error)
	DeleteChunksByDocument(ctx context.Context, docID int64) error
}
