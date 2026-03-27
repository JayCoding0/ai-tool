-- 为 chat_sessions 表添加 system_prompt 字段
USE ai_chat_db;

ALTER TABLE chat_sessions 
    ADD COLUMN system_prompt TEXT DEFAULT NULL COMMENT 'System Prompt（会话级别的系统提示词）' 
    AFTER title;
