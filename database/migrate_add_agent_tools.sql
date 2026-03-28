USE ai_chat_db;

-- Agent 工具动态配置表
-- 存储每个 Agent 的工具列表，支持运行时动态修改
CREATE TABLE IF NOT EXISTS agent_tools (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键',
    agent_name  VARCHAR(100) NOT NULL COMMENT 'Agent 唯一名称（对应 AgentDefinition.Name）',
    tool_name   VARCHAR(100) NOT NULL COMMENT '工具名称',
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    UNIQUE KEY uk_agent_tool (agent_name, tool_name),
    INDEX idx_agent_name (agent_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Agent 工具动态配置表';
