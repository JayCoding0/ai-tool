-- RAG 知识库相关表迁移脚本
-- 执行前请确保已连接到 ai_chat_db 数据库

USE ai_chat_db;

-- 知识库表
CREATE TABLE IF NOT EXISTS knowledge_bases (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '知识库ID',
    user_id     BIGINT NOT NULL DEFAULT 0 COMMENT '创建者用户ID，0表示公共',
    name        VARCHAR(100) NOT NULL COMMENT '知识库名称',
    description TEXT COMMENT '知识库描述',
    embed_model VARCHAR(100) NOT NULL DEFAULT 'text-embedding-3-small' COMMENT '使用的Embedding模型',
    doc_count   INT NOT NULL DEFAULT 0 COMMENT '文档数量',
    chunk_count INT NOT NULL DEFAULT 0 COMMENT '分块数量',
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    INDEX idx_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='知识库表';

-- 文档表
CREATE TABLE IF NOT EXISTS knowledge_documents (
    id                BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '文档ID',
    knowledge_base_id BIGINT NOT NULL COMMENT '所属知识库ID',
    name              VARCHAR(255) NOT NULL COMMENT '文件名',
    content_type      VARCHAR(50) NOT NULL DEFAULT 'text' COMMENT '内容类型：text/markdown/pdf',
    char_count        INT NOT NULL DEFAULT 0 COMMENT '字符数',
    chunk_count       INT NOT NULL DEFAULT 0 COMMENT '分块数量',
    status            ENUM('pending','processing','done','failed') NOT NULL DEFAULT 'pending' COMMENT '处理状态',
    error_msg         TEXT COMMENT '错误信息',
    created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    INDEX idx_kb_id (knowledge_base_id),
    FOREIGN KEY (knowledge_base_id) REFERENCES knowledge_bases(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='知识库文档表';

-- 文档分块表（存储向量 + 原文）
CREATE TABLE IF NOT EXISTS knowledge_chunks (
    id                BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '分块ID',
    document_id       BIGINT NOT NULL COMMENT '所属文档ID',
    knowledge_base_id BIGINT NOT NULL COMMENT '所属知识库ID（冗余，加速检索）',
    content           TEXT NOT NULL COMMENT '原始文本片段',
    embedding         MEDIUMBLOB NOT NULL COMMENT '向量二进制（float32数组，小端序）',
    chunk_index       INT NOT NULL DEFAULT 0 COMMENT '在文档中的顺序',
    token_count       INT NOT NULL DEFAULT 0 COMMENT '估算token数',
    created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    INDEX idx_doc_id (document_id),
    INDEX idx_kb_id (knowledge_base_id),
    FOREIGN KEY (document_id) REFERENCES knowledge_documents(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='文档分块表（含向量）';
