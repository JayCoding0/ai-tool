package mysql

import (
	"context"
	"database/sql"
	"time"

	"aiProject/internal/domain/knowledge"
	"aiProject/internal/infrastructure/database"
	trmysql "git.code.oa.com/trpc-go/trpc-database/mysql"
)

// KnowledgeRepository MySQL 知识库仓储实现
type KnowledgeRepository struct {
	db trmysql.Client
}

// NewKnowledgeRepository 创建知识库仓储
func NewKnowledgeRepository() *KnowledgeRepository {
	return &KnowledgeRepository{db: database.GetDB()}
}

// ─── 知识库 CRUD ───────────────────────────────────────────────────────────────

// CreateKnowledgeBase 创建知识库
func (r *KnowledgeRepository) CreateKnowledgeBase(ctx context.Context, kb *knowledge.KnowledgeBase) error {
	result, err := r.db.Exec(ctx,
		`INSERT INTO knowledge_bases (user_id, name, description, embed_model) VALUES (?, ?, ?, ?)`,
		kb.UserID, kb.Name, kb.Description, kb.EmbedModel,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	kb.ID = id
	kb.CreatedAt = time.Now()
	kb.UpdatedAt = time.Now()
	return nil
}

// GetKnowledgeBase 根据 ID 获取知识库
func (r *KnowledgeRepository) GetKnowledgeBase(ctx context.Context, id int64) (*knowledge.KnowledgeBase, error) {
	var kb knowledge.KnowledgeBase
	dest := []interface{}{
		&kb.ID, &kb.UserID, &kb.Name, &kb.Description,
		&kb.EmbedModel, &kb.DocCount, &kb.ChunkCount,
		&kb.CreatedAt, &kb.UpdatedAt,
	}
	err := r.db.QueryRow(ctx, dest,
		`SELECT id, user_id, name, description, embed_model, doc_count, chunk_count, created_at, updated_at
		 FROM knowledge_bases WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	return &kb, nil
}

// ListKnowledgeBases 列出知识库（userID=0 时列出所有）
func (r *KnowledgeRepository) ListKnowledgeBases(ctx context.Context, userID int64) ([]*knowledge.KnowledgeBase, error) {
	var query string
	var args []interface{}
	if userID > 0 {
		query = `SELECT id, user_id, name, description, embed_model, doc_count, chunk_count, created_at, updated_at
				 FROM knowledge_bases WHERE user_id = ? ORDER BY created_at DESC`
		args = append(args, userID)
	} else {
		query = `SELECT id, user_id, name, description, embed_model, doc_count, chunk_count, created_at, updated_at
				 FROM knowledge_bases ORDER BY created_at DESC`
	}

	var list []*knowledge.KnowledgeBase
	err := r.db.Query(ctx, func(rows *sql.Rows) error {
		var kb knowledge.KnowledgeBase
		if err := rows.Scan(&kb.ID, &kb.UserID, &kb.Name, &kb.Description,
			&kb.EmbedModel, &kb.DocCount, &kb.ChunkCount, &kb.CreatedAt, &kb.UpdatedAt); err != nil {
			return err
		}
		list = append(list, &kb)
		return nil
	}, query, args...)
	return list, err
}

// DeleteKnowledgeBase 删除知识库（级联删除文档和分块）
func (r *KnowledgeRepository) DeleteKnowledgeBase(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM knowledge_bases WHERE id = ?`, id)
	return err
}

// UpdateKnowledgeBaseStats 更新知识库统计数据
func (r *KnowledgeRepository) UpdateKnowledgeBaseStats(ctx context.Context, id int64, docCount, chunkCount int) error {
	_, err := r.db.Exec(ctx,
		`UPDATE knowledge_bases SET doc_count = ?, chunk_count = ? WHERE id = ?`,
		docCount, chunkCount, id)
	return err
}

// ─── 文档 CRUD ─────────────────────────────────────────────────────────────────

// CreateDocument 创建文档记录
func (r *KnowledgeRepository) CreateDocument(ctx context.Context, doc *knowledge.Document) error {
	result, err := r.db.Exec(ctx,
		`INSERT INTO knowledge_documents (knowledge_base_id, name, content_type, char_count, status) VALUES (?, ?, ?, ?, ?)`,
		doc.KnowledgeBaseID, doc.Name, doc.ContentType, doc.CharCount, string(doc.Status),
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	doc.ID = id
	doc.CreatedAt = time.Now()
	return nil
}

// GetDocument 根据 ID 获取文档
func (r *KnowledgeRepository) GetDocument(ctx context.Context, id int64) (*knowledge.Document, error) {
	var doc knowledge.Document
	var status string
	var errMsg sql.NullString
	dest := []interface{}{
		&doc.ID, &doc.KnowledgeBaseID, &doc.Name, &doc.ContentType,
		&doc.CharCount, &doc.ChunkCount, &status, &errMsg, &doc.CreatedAt,
	}
	err := r.db.QueryRow(ctx, dest,
		`SELECT id, knowledge_base_id, name, content_type, char_count, chunk_count, status, error_msg, created_at
		 FROM knowledge_documents WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	doc.Status = knowledge.DocumentStatus(status)
	if errMsg.Valid {
		doc.ErrorMsg = errMsg.String
	}
	return &doc, nil
}

// ListDocuments 列出知识库下的所有文档
func (r *KnowledgeRepository) ListDocuments(ctx context.Context, kbID int64) ([]*knowledge.Document, error) {
	var list []*knowledge.Document
	err := r.db.Query(ctx, func(rows *sql.Rows) error {
		var doc knowledge.Document
		var status string
		var errMsg sql.NullString
		if err := rows.Scan(&doc.ID, &doc.KnowledgeBaseID, &doc.Name, &doc.ContentType,
			&doc.CharCount, &doc.ChunkCount, &status, &errMsg, &doc.CreatedAt); err != nil {
			return err
		}
		doc.Status = knowledge.DocumentStatus(status)
		if errMsg.Valid {
			doc.ErrorMsg = errMsg.String
		}
		list = append(list, &doc)
		return nil
	}, `SELECT id, knowledge_base_id, name, content_type, char_count, chunk_count, status, error_msg, created_at
		FROM knowledge_documents WHERE knowledge_base_id = ? ORDER BY created_at DESC`, kbID)
	return list, err
}

// UpdateDocumentStatus 更新文档处理状态
func (r *KnowledgeRepository) UpdateDocumentStatus(ctx context.Context, id int64, status knowledge.DocumentStatus, errMsg string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE knowledge_documents SET status = ?, error_msg = ? WHERE id = ?`,
		string(status), errMsg, id)
	return err
}

// UpdateDocumentChunkCount 更新文档分块数量
func (r *KnowledgeRepository) UpdateDocumentChunkCount(ctx context.Context, id int64, chunkCount int) error {
	_, err := r.db.Exec(ctx,
		`UPDATE knowledge_documents SET chunk_count = ? WHERE id = ?`,
		chunkCount, id)
	return err
}

// DeleteDocument 删除文档（级联删除分块）
func (r *KnowledgeRepository) DeleteDocument(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM knowledge_documents WHERE id = ?`, id)
	return err
}

// ─── 分块 CRUD ─────────────────────────────────────────────────────────────────

// CreateChunks 批量创建分块（含向量）
func (r *KnowledgeRepository) CreateChunks(ctx context.Context, chunks []*knowledge.Chunk) error {
	if len(chunks) == 0 {
		return nil
	}
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx,
			`INSERT INTO knowledge_chunks (document_id, knowledge_base_id, content, embedding, chunk_index, token_count) VALUES (?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, chunk := range chunks {
			result, err := stmt.ExecContext(ctx,
				chunk.DocumentID, chunk.KnowledgeBaseID,
				chunk.Content, chunk.Embedding,
				chunk.ChunkIndex, chunk.TokenCount,
			)
			if err != nil {
				return err
			}
			id, _ := result.LastInsertId()
			chunk.ID = id
		}
		return nil
	})
}

// ListChunks 获取知识库下所有分块（含向量，用于检索）
func (r *KnowledgeRepository) ListChunks(ctx context.Context, kbID int64) ([]*knowledge.Chunk, error) {
	var list []*knowledge.Chunk
	err := r.db.Query(ctx, func(rows *sql.Rows) error {
		var chunk knowledge.Chunk
		if err := rows.Scan(&chunk.ID, &chunk.DocumentID, &chunk.KnowledgeBaseID,
			&chunk.Content, &chunk.Embedding, &chunk.ChunkIndex, &chunk.TokenCount, &chunk.CreatedAt); err != nil {
			return err
		}
		list = append(list, &chunk)
		return nil
	}, `SELECT id, document_id, knowledge_base_id, content, embedding, chunk_index, token_count, created_at
		FROM knowledge_chunks WHERE knowledge_base_id = ? ORDER BY document_id, chunk_index`, kbID)
	return list, err
}

// DeleteChunksByDocument 删除指定文档的所有分块
func (r *KnowledgeRepository) DeleteChunksByDocument(ctx context.Context, docID int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM knowledge_chunks WHERE document_id = ?`, docID)
	return err
}

// 确保实现了 knowledge.Repository 接口
var _ knowledge.Repository = (*KnowledgeRepository)(nil)
