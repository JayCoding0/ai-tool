-- 为 skills 表增加 pattern 和 tools 字段（支持 Skill 设计模式和 Function Calling）
ALTER TABLE skills
    ADD COLUMN pattern VARCHAR(50) NOT NULL DEFAULT '' COMMENT 'Skill设计模式: tool-wrapper/generator/reviewer/inversion/pipeline' AFTER system_prompt,
    ADD COLUMN tools JSON COMMENT '绑定的工具名称列表，如 ["query_database","calculate_stats"]' AFTER pattern;
