-- ============================================================
-- 迁移脚本：为 chat_sessions 表添加 summary 字段
-- Phase 1: 会话摘要记忆 + Token 窗口管理
-- ============================================================

-- 添加会话摘要字段（长对话自动压缩）
ALTER TABLE chat_sessions ADD COLUMN summary TEXT DEFAULT NULL COMMENT '会话摘要（长对话自动压缩，用于上下文窗口管理）' AFTER model_name;
