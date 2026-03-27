-- 用户账户功能迁移脚本
-- 执行前请确保已备份数据库

USE ai_chat_db;

-- 1. 创建用户表（如果不存在）
CREATE TABLE IF NOT EXISTS users (
    id BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '用户ID',
    username VARCHAR(50) NOT NULL UNIQUE COMMENT '用户名',
    password_hash VARCHAR(255) NOT NULL COMMENT '密码哈希',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    INDEX idx_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户表';

-- 2. 为 chat_sessions 表添加 user_id 字段（如果不存在）
-- user_id=0 表示游客会话，启动时会被自动清理
ALTER TABLE chat_sessions ADD COLUMN IF NOT EXISTS user_id BIGINT NOT NULL DEFAULT 0 COMMENT '用户ID，0表示游客' AFTER id;

-- 3. 添加索引（如果不存在）
ALTER TABLE chat_sessions ADD INDEX IF NOT EXISTS idx_user_id (user_id);

-- 4. 如果之前有外键约束，删除它（允许 user_id=0 的游客记录）
-- ALTER TABLE chat_sessions DROP FOREIGN KEY fk_sessions_user;

-- 5. 清理旧的游客会话（user_id=0 的记录）
DELETE FROM chat_sessions WHERE user_id = 0;