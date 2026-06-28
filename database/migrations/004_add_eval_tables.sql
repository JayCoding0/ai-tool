-- ============================================================
-- 迁移脚本：Agent 评估体系（Evaluation）相关表
-- 数据集 eval_datasets / 用例 eval_cases / 运行 eval_runs / 结果 eval_results
-- ============================================================

-- 评测数据集
CREATE TABLE IF NOT EXISTS eval_datasets (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    name        VARCHAR(128) NOT NULL COMMENT '数据集名称',
    description VARCHAR(512) NOT NULL DEFAULT '' COMMENT '描述',
    user_id     BIGINT NOT NULL COMMENT '创建者用户ID',
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    INDEX idx_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='评测数据集';

-- 评测用例
CREATE TABLE IF NOT EXISTS eval_cases (
    id         BIGINT AUTO_INCREMENT PRIMARY KEY,
    dataset_id BIGINT NOT NULL COMMENT '所属数据集ID',
    input      TEXT NOT NULL COMMENT '输入（用户问题）',
    expected   TEXT COMMENT '期望输出（标准答案，可空）',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    INDEX idx_dataset_id (dataset_id),
    FOREIGN KEY (dataset_id) REFERENCES eval_datasets(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='评测用例';

-- 评测运行
CREATE TABLE IF NOT EXISTS eval_runs (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY,
    dataset_id    BIGINT NOT NULL COMMENT '数据集ID',
    name          VARCHAR(128) NOT NULL DEFAULT '' COMMENT '运行名称',
    model_name    VARCHAR(100) NOT NULL DEFAULT '' COMMENT '被测Agent模型',
    system_prompt TEXT COMMENT '被测Agent System Prompt',
    tools         JSON COMMENT '被测Agent启用的工具列表',
    judge_model   VARCHAR(100) NOT NULL DEFAULT '' COMMENT '裁判模型',
    threshold     FLOAT NOT NULL DEFAULT 0.6 COMMENT '通过阈值',
    status        ENUM('running','completed','failed') NOT NULL DEFAULT 'running' COMMENT '状态',
    total_cases   INT NOT NULL DEFAULT 0 COMMENT '用例总数',
    passed_cases  INT NOT NULL DEFAULT 0 COMMENT '通过数',
    avg_score     FLOAT NOT NULL DEFAULT 0 COMMENT '平均分',
    error_message TEXT COMMENT '错误信息',
    user_id       BIGINT NOT NULL COMMENT '执行者用户ID',
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    finished_at   TIMESTAMP NULL COMMENT '完成时间',
    INDEX idx_dataset_id (dataset_id),
    INDEX idx_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='评测运行';

-- 评测结果（单条用例）
CREATE TABLE IF NOT EXISTS eval_results (
    id         BIGINT AUTO_INCREMENT PRIMARY KEY,
    run_id     BIGINT NOT NULL COMMENT '所属运行ID',
    case_id    BIGINT NOT NULL COMMENT '用例ID',
    input      TEXT NOT NULL COMMENT '输入',
    expected   TEXT COMMENT '期望输出',
    actual     TEXT COMMENT 'Agent实际输出',
    score      FLOAT NOT NULL DEFAULT 0 COMMENT '评分0-1',
    passed     TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否通过',
    reason     TEXT COMMENT '评分理由',
    latency_ms BIGINT NOT NULL DEFAULT 0 COMMENT '耗时（毫秒）',
    tokens     INT NOT NULL DEFAULT 0 COMMENT 'token消耗',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    INDEX idx_run_id (run_id),
    FOREIGN KEY (run_id) REFERENCES eval_runs(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='评测结果';
