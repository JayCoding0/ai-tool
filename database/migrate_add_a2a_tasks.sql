-- A2A 任务持久化迁移脚本
USE ai_chat_db;

-- 创建 a2a_tasks 表
CREATE TABLE IF NOT EXISTS a2a_tasks (
    id VARCHAR(64) NOT NULL PRIMARY KEY COMMENT '任务ID',
    session_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT '会话ID（支持多轮对话）',
    state VARCHAR(20) NOT NULL DEFAULT 'submitted' COMMENT '任务状态: submitted/working/completed/failed/canceled',
    state_message VARCHAR(500) NOT NULL DEFAULT '' COMMENT '状态描述',
    input_json JSON NOT NULL COMMENT '输入消息（TaskMessage JSON）',
    output_json JSON COMMENT '输出消息列表（[]TaskMessage JSON）',
    metadata_json JSON COMMENT '任务元数据（map JSON）',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    INDEX idx_session_id (session_id),
    INDEX idx_state (state),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='A2A 任务表';
