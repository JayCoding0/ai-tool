-- 为 chat_messages 表增加 token 统计字段（执行前请确认字段不存在）
ALTER TABLE chat_messages
    ADD COLUMN prompt_tokens INT NOT NULL DEFAULT 0 COMMENT '输入token数' AFTER content,
    ADD COLUMN completion_tokens INT NOT NULL DEFAULT 0 COMMENT '输出token数' AFTER prompt_tokens,
    ADD COLUMN total_tokens INT NOT NULL DEFAULT 0 COMMENT '总token数' AFTER completion_tokens;

-- 创建模型 token 消耗统计视图（方便查询）
CREATE OR REPLACE VIEW model_token_stats AS
SELECT
    COALESCE(model_name, '') AS model_name,
    COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
    COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
    COALESCE(SUM(total_tokens), 0) AS total_tokens,
    COUNT(*) AS message_count
FROM chat_messages
WHERE model_name != '' AND role = 'ai'
GROUP BY model_name;
