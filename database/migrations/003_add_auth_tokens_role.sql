-- ============================================================
-- 迁移脚本：为 auth_tokens 表补充 role 字段
-- 背景：旧版本数据库建表时 auth_tokens 无 role 列，而 schema.sql 使用
--       CREATE TABLE IF NOT EXISTS，重跑不会为已存在的表补列，导致注册时
--       INSERT auth_tokens 报错：Unknown column 'role' in 'field list' (1054)
-- ============================================================

-- 添加用户角色字段（与 schema.sql 中 auth_tokens 定义保持一致）
ALTER TABLE auth_tokens
    ADD COLUMN role VARCHAR(20) NOT NULL DEFAULT 'user' COMMENT '用户角色: admin/user' AFTER username;
