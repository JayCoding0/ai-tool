-- 用户角色功能迁移脚本
-- 执行前请确保已备份数据库

USE ai_chat_db;

-- 1. 为 users 表添加 role 字段（兼容 MySQL 5.x）
SET @dbname = DATABASE();
SET @tablename = 'users';
SET @columnname = 'role';
SET @preparedStatement = (
    SELECT IF(
        COUNT(*) = 0,
        'ALTER TABLE users ADD COLUMN role VARCHAR(20) NOT NULL DEFAULT ''user'' COMMENT ''用户角色: admin/user'' AFTER password_hash',
        'SELECT 1'
    )
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = @dbname
      AND TABLE_NAME = @tablename
      AND COLUMN_NAME = @columnname
);
PREPARE stmt FROM @preparedStatement;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 2. 添加索引（兼容 MySQL 5.x）
SET @indexname = 'idx_role';
SET @preparedStatement2 = (
    SELECT IF(
        COUNT(*) = 0,
        'ALTER TABLE users ADD INDEX idx_role (role)',
        'SELECT 1'
    )
    FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = @dbname
      AND TABLE_NAME = @tablename
      AND INDEX_NAME = @indexname
);
PREPARE stmt2 FROM @preparedStatement2;
EXECUTE stmt2;
DEALLOCATE PREPARE stmt2;

-- 3. 创建 admin 账户（密码: admin123，请登录后立即修改）
-- bcrypt hash of "admin123" with cost=10
INSERT IGNORE INTO users (username, password_hash, role)
VALUES ('admin', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', 'admin');
