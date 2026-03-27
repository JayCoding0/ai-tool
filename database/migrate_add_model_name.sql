-- 为 chat_messages 表增加 model_name 字段
ALTER TABLE chat_messages
    ADD COLUMN model_name VARCHAR(100) NOT NULL DEFAULT '' COMMENT '调用的模型名称' AFTER role;

-- 为 chat_sessions 表增加 model_name 字段（记录会话最后使用的模型）
ALTER TABLE chat_sessions
    ADD COLUMN model_name VARCHAR(100) NOT NULL DEFAULT '' COMMENT '会话使用的模型名称' AFTER title;
