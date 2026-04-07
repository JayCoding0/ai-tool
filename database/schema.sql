-- ============================================================
-- AI Chat Platform - 完整数据库初始化脚本
-- 包含所有表结构，新用户只需执行此文件即可完成建库
-- ============================================================

CREATE DATABASE IF NOT EXISTS ai_chat_db CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE ai_chat_db;

-- -----------------------------------------------------------
-- 1. 用户表
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '用户ID',
    username      VARCHAR(50) NOT NULL UNIQUE COMMENT '用户名',
    password_hash VARCHAR(255) NOT NULL COMMENT '密码哈希',
    role          VARCHAR(20) NOT NULL DEFAULT 'user' COMMENT '用户角色: admin/user',
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    INDEX idx_username (username),
    INDEX idx_role (role)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户表';

-- -----------------------------------------------------------
-- 2. 认证 Token 表（支持多实例部署下的 Token 持久化）
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS auth_tokens (
    token      VARCHAR(64) NOT NULL PRIMARY KEY COMMENT 'Token值',
    user_id    BIGINT NOT NULL COMMENT '用户ID',
    username   VARCHAR(50) NOT NULL COMMENT '用户名',
    expires_at TIMESTAMP NOT NULL COMMENT '过期时间',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    INDEX idx_user_id (user_id),
    INDEX idx_expires_at (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='认证Token表';

-- -----------------------------------------------------------
-- 3. 聊天会话表
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS chat_sessions (
    id            VARCHAR(36) PRIMARY KEY COMMENT '会话ID',
    user_id       BIGINT NOT NULL DEFAULT 0 COMMENT '用户ID，0表示游客',
    title         VARCHAR(255) COMMENT '会话标题（第一条消息）',
    system_prompt TEXT DEFAULT NULL COMMENT 'System Prompt（会话级别的系统提示词）',
    model_name    VARCHAR(100) NOT NULL DEFAULT '' COMMENT '会话使用的模型名称',
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    INDEX idx_created_at (created_at),
    INDEX idx_updated_at (updated_at),
    INDEX idx_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='聊天会话表';

-- -----------------------------------------------------------
-- 4. 聊天消息表
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS chat_messages (
    id                BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '消息ID',
    session_id        VARCHAR(36) NOT NULL COMMENT '会话ID',
    user_id           BIGINT NOT NULL DEFAULT 0 COMMENT '用户ID，0表示游客',
    role              ENUM('user', 'ai') NOT NULL COMMENT '消息角色',
    model_name        VARCHAR(100) NOT NULL DEFAULT '' COMMENT '调用的模型名称',
    content           TEXT NOT NULL COMMENT '消息内容',
    prompt_tokens     INT NOT NULL DEFAULT 0 COMMENT '输入token数',
    completion_tokens INT NOT NULL DEFAULT 0 COMMENT '输出token数',
    total_tokens      INT NOT NULL DEFAULT 0 COMMENT '总token数',
    created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    INDEX idx_session_id (session_id),
    INDEX idx_created_at (created_at),
    INDEX idx_user_id (user_id),
    FOREIGN KEY (session_id) REFERENCES chat_sessions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='聊天消息表';

-- -----------------------------------------------------------
-- 5. 模型 Token 消耗统计视图
-- -----------------------------------------------------------
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

-- -----------------------------------------------------------
-- 6. Skills 技能表
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS skills (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '技能ID',
    user_id       BIGINT NOT NULL DEFAULT 0 COMMENT '用户ID，0=系统预设，>0=用户自定义',
    name          VARCHAR(100) NOT NULL COMMENT '技能名称',
    description   VARCHAR(500) NOT NULL DEFAULT '' COMMENT '技能描述',
    icon          VARCHAR(20) NOT NULL DEFAULT '🤖' COMMENT '技能图标（emoji）',
    system_prompt TEXT NOT NULL COMMENT '技能的 System Prompt',
    pattern       VARCHAR(50) NOT NULL DEFAULT '' COMMENT 'Skill设计模式: tool-wrapper/generator/reviewer/inversion/pipeline',
    tools         JSON COMMENT '绑定的工具名称列表，如 ["query_database","calculate_stats"]',
    is_public     TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否公开（1=公开，0=私有）',
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    INDEX idx_user_id (user_id),
    INDEX idx_is_public (is_public)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='AI 技能表（Skills）';

-- -----------------------------------------------------------
-- 7. A2A 任务表
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS a2a_tasks (
    id            VARCHAR(64) NOT NULL PRIMARY KEY COMMENT '任务ID',
    session_id    VARCHAR(64) NOT NULL DEFAULT '' COMMENT '会话ID（支持多轮对话）',
    state         VARCHAR(20) NOT NULL DEFAULT 'submitted' COMMENT '任务状态: submitted/working/completed/failed/canceled',
    state_message VARCHAR(500) NOT NULL DEFAULT '' COMMENT '状态描述',
    input_json    JSON NOT NULL COMMENT '输入消息（TaskMessage JSON）',
    output_json   JSON COMMENT '输出消息列表（[]TaskMessage JSON）',
    metadata_json JSON COMMENT '任务元数据（map JSON）',
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    INDEX idx_session_id (session_id),
    INDEX idx_state (state),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='A2A 任务表';

-- -----------------------------------------------------------
-- 8. Agent 工具动态配置表
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS agent_tools (
    id         BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键',
    agent_name VARCHAR(100) NOT NULL COMMENT 'Agent 唯一名称（对应 AgentDefinition.Name）',
    tool_name  VARCHAR(100) NOT NULL COMMENT '工具名称',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    UNIQUE KEY uk_agent_tool (agent_name, tool_name),
    INDEX idx_agent_name (agent_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Agent 工具动态配置表';

-- -----------------------------------------------------------
-- 9. 知识库表
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS knowledge_bases (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '知识库ID',
    user_id     BIGINT NOT NULL DEFAULT 0 COMMENT '创建者用户ID，0表示公共',
    name        VARCHAR(100) NOT NULL COMMENT '知识库名称',
    description TEXT COMMENT '知识库描述',
    embed_model VARCHAR(100) NOT NULL DEFAULT 'text-embedding-3-small' COMMENT '使用的Embedding模型',
    doc_count   INT NOT NULL DEFAULT 0 COMMENT '文档数量',
    chunk_count INT NOT NULL DEFAULT 0 COMMENT '分块数量',
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    INDEX idx_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='知识库表';

-- -----------------------------------------------------------
-- 10. 知识库文档表
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS knowledge_documents (
    id                BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '文档ID',
    knowledge_base_id BIGINT NOT NULL COMMENT '所属知识库ID',
    name              VARCHAR(255) NOT NULL COMMENT '文件名',
    content_type      VARCHAR(50) NOT NULL DEFAULT 'text' COMMENT '内容类型：text/markdown/pdf',
    char_count        INT NOT NULL DEFAULT 0 COMMENT '字符数',
    chunk_count       INT NOT NULL DEFAULT 0 COMMENT '分块数量',
    status            ENUM('pending','processing','done','failed') NOT NULL DEFAULT 'pending' COMMENT '处理状态',
    error_msg         TEXT COMMENT '错误信息',
    created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    INDEX idx_kb_id (knowledge_base_id),
    FOREIGN KEY (knowledge_base_id) REFERENCES knowledge_bases(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='知识库文档表';

-- -----------------------------------------------------------
-- 11. 文档分块表（存储向量 + 原文）
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS knowledge_chunks (
    id                BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '分块ID',
    document_id       BIGINT NOT NULL COMMENT '所属文档ID',
    knowledge_base_id BIGINT NOT NULL COMMENT '所属知识库ID（冗余，加速检索）',
    content           TEXT NOT NULL COMMENT '原始文本片段',
    embedding         MEDIUMBLOB NOT NULL COMMENT '向量二进制（float32数组，小端序）',
    chunk_index       INT NOT NULL DEFAULT 0 COMMENT '在文档中的顺序',
    token_count       INT NOT NULL DEFAULT 0 COMMENT '估算token数',
    created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    INDEX idx_doc_id (document_id),
    INDEX idx_kb_id (knowledge_base_id),
    FOREIGN KEY (document_id) REFERENCES knowledge_documents(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='文档分块表（含向量）';

-- -----------------------------------------------------------
-- 12. Prompt 模板变量 - 用户级
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS prompt_vars_user (
    id         BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键',
    user_id    BIGINT NOT NULL COMMENT '用户ID',
    var_key    VARCHAR(100) NOT NULL COMMENT '变量名',
    var_value  TEXT NOT NULL COMMENT '变量值',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    UNIQUE KEY uk_user_var (user_id, var_key),
    INDEX idx_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Prompt 模板变量 - 用户级';

-- -----------------------------------------------------------
-- 13. Prompt 模板变量 - 会话级
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS prompt_vars_session (
    id         BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键',
    session_id VARCHAR(36) NOT NULL COMMENT '会话ID',
    var_key    VARCHAR(100) NOT NULL COMMENT '变量名',
    var_value  TEXT NOT NULL COMMENT '变量值',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    UNIQUE KEY uk_session_var (session_id, var_key),
    INDEX idx_session_id (session_id),
    FOREIGN KEY (session_id) REFERENCES chat_sessions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Prompt 模板变量 - 会话级';

-- -----------------------------------------------------------
-- 14. 工作流定义表（Workflow DAG 编排）
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS workflows (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '工作流ID',
    name        VARCHAR(128) NOT NULL COMMENT '工作流名称',
    description TEXT COMMENT '工作流描述',
    graph_json  JSON NOT NULL COMMENT '完整的 DAG 定义（nodes + edges + variables）',
    status      ENUM('draft', 'published', 'archived') NOT NULL DEFAULT 'draft' COMMENT '状态',
    version     INT NOT NULL DEFAULT 1 COMMENT '版本号',
    user_id     BIGINT NOT NULL COMMENT '创建者用户ID',
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    INDEX idx_user_status (user_id, status),
    INDEX idx_updated_at (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='工作流定义表（DAG 编排）';

-- -----------------------------------------------------------
-- 15. 工作流执行记录表
-- -----------------------------------------------------------
CREATE TABLE IF NOT EXISTS workflow_runs (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '记录ID',
    workflow_id   BIGINT NOT NULL COMMENT '工作流ID',
    run_id        VARCHAR(64) NOT NULL UNIQUE COMMENT '执行唯一ID（UUID）',
    status        ENUM('running', 'completed', 'failed', 'cancelled') NOT NULL DEFAULT 'running' COMMENT '执行状态',
    inputs        JSON COMMENT '输入变量',
    outputs       JSON COMMENT '最终输出',
    node_results  JSON COMMENT '各节点执行结果快照',
    total_tokens  INT NOT NULL DEFAULT 0 COMMENT '总 token 消耗',
    duration_ms   BIGINT NOT NULL DEFAULT 0 COMMENT '总耗时（毫秒）',
    error_message TEXT COMMENT '错误信息',
    user_id       BIGINT NOT NULL COMMENT '执行者用户ID',
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    INDEX idx_workflow_id (workflow_id),
    INDEX idx_user_id (user_id),
    INDEX idx_run_id (run_id),
    INDEX idx_created_at (created_at),
    FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='工作流执行记录表';