-- ============================================================
-- 迁移脚本：eval_runs 表增加 scorer 评分器类型字段
-- judge=LLM裁判（默认） / exact=精确匹配 / semantic=语义相似度
-- ============================================================

ALTER TABLE eval_runs
    ADD COLUMN scorer VARCHAR(20) NOT NULL DEFAULT 'judge' COMMENT '评分器: judge/exact/semantic' AFTER tools;
