-- 002_add_user_memories.sql
-- 用户记忆表（向量记忆 + 结构化记忆统一存储）
-- Phase 2: 向量记忆（跨会话语义检索）

CREATE TABLE IF NOT EXISTS user_memories (
    id                BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '记忆ID',
    user_id           BIGINT NOT NULL COMMENT '用户ID',
    content           TEXT NOT NULL COMMENT '记忆内容（自然语言描述）',
    embedding         MEDIUMBLOB COMMENT '向量（复用 knowledge_chunks 的存储方式，float32数组小端序）',
    memory_type       ENUM('fact','preference','episode','summary') NOT NULL DEFAULT 'fact' COMMENT '记忆类型',
    source_session_id VARCHAR(36) COMMENT '来源会话ID',
    importance        FLOAT NOT NULL DEFAULT 0.5 COMMENT '重要性分数 0-1',
    access_count      INT NOT NULL DEFAULT 0 COMMENT '被检索命中次数',
    created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    expired_at        TIMESTAMP NULL COMMENT '过期时间（NULL=永不过期）',
    INDEX idx_user_id (user_id),
    INDEX idx_memory_type (user_id, memory_type),
    INDEX idx_importance (user_id, importance)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户记忆表（向量记忆 + 结构化记忆）';
