# 🏗️ 架构设计详解

## DDD 分层架构

本项目采用 **领域驱动设计（DDD）** 分层架构，各层职责清晰，依赖方向严格从外向内。

```mermaid
graph TB
    subgraph Interfaces["接口层 (Interfaces)"]
        direction LR
        HTTP["HTTP Handler<br/>REST API"]
        MCP["MCP Server<br/>模型上下文协议"]
    end

    subgraph Application["应用层 (Application)"]
        direction LR
        ChatSvc["ChatService"]
        AuthSvc["AuthService"]
        SkillSvc["SkillService"]
        KnowledgeSvc["KnowledgeService"]
        WorkflowSvc["WorkflowService"]
        WorkflowEngine["WorkflowEngine"]
        A2ASvc["A2AService"]
        AgentReg["AgentRegistry"]
    end

    subgraph Domain["领域层 (Domain)"]
        direction LR
        Session["Session 聚合根"]
        User["User 实体"]
        Skill["Skill 实体"]
        Tool["Tool 注册表"]
        Workflow["Workflow 聚合根"]
        Knowledge["Knowledge 实体"]
        A2ATask["A2A Task"]
    end

    subgraph Infrastructure["基础设施层 (Infrastructure)"]
        direction LR
        MySQL["MySQL 持久化"]
        OpenAI["OpenAI 接口"]
        Ollama["Ollama 本地"]
        Embedder["向量化引擎"]
        ToolLoader["工具加载器"]
    end

    Interfaces --> Application
    Application --> Domain
    Infrastructure -.->|实现接口| Domain
    Application --> Infrastructure
```

### 各层职责

| 层 | 目录 | 职责 |
|----|------|------|
| **接口层** | `internal/interfaces/` | 处理 HTTP 请求、协议适配（REST、MCP、A2A），不含业务逻辑 |
| **应用层** | `internal/application/` | 编排领域对象完成用例，事务管理，不含业务规则（含 WorkflowEngine DAG 执行引擎） |
| **领域层** | `internal/domain/` | 核心业务逻辑，定义实体、值对象、聚合根、仓储接口 |
| **基础设施层** | `internal/infrastructure/` | 技术实现：数据库、外部 API、工具执行、向量化 |
| **启动编排层** | `internal/bootstrap/` | 依赖注入、组件初始化、路由注册、中间件链 |

---

## 数据库 ER 图

```mermaid
erDiagram
    users ||--o{ chat_sessions : "拥有"
    users ||--o{ skills : "创建"
    users ||--o{ knowledge_bases : "创建"
    users ||--o{ auth_tokens : "持有"
    users ||--o{ workflows : "创建"
    
    chat_sessions ||--o{ chat_messages : "包含"
    
    knowledge_bases ||--o{ knowledge_documents : "包含"
    knowledge_documents ||--o{ knowledge_chunks : "分块"
    
    workflows ||--o{ workflow_runs : "执行记录"
    
    users {
        bigint id PK
        varchar username UK
        varchar password_hash
        varchar role "admin/user"
        timestamp created_at
    }
    
    auth_tokens {
        varchar token PK
        bigint user_id FK
        varchar username
        timestamp expires_at
    }
    
    chat_sessions {
        varchar id PK "UUID"
        bigint user_id FK
        varchar title
        text system_prompt
        varchar model_name
        timestamp created_at
    }
    
    chat_messages {
        bigint id PK
        varchar session_id FK
        bigint user_id
        enum role "user/ai"
        varchar model_name
        text content
        int prompt_tokens
        int completion_tokens
        int total_tokens
    }
    
    skills {
        bigint id PK
        bigint user_id FK
        varchar name
        text system_prompt
        varchar pattern
        json tools
        boolean is_public
    }
    
    knowledge_bases {
        bigint id PK
        bigint user_id FK
        varchar name
        text description
        varchar embed_model
        int doc_count
        int chunk_count
    }
    
    knowledge_documents {
        bigint id PK
        bigint knowledge_base_id FK
        varchar name
        varchar content_type
        int char_count
        int chunk_count
        enum status "pending/processing/done/failed"
    }
    
    knowledge_chunks {
        bigint id PK
        bigint document_id FK
        bigint knowledge_base_id FK
        text content
        mediumblob embedding
        int chunk_index
        int token_count
    }
    
    a2a_tasks {
        varchar id PK
        varchar session_id
        varchar state
        json input_json
        json output_json
        json metadata_json
    }
    
    workflows {
        bigint id PK
        bigint user_id FK
        varchar name
        text description
        json graph_data "nodes + edges + variables"
        varchar status "draft/published/archived"
        int version
        timestamp created_at
        timestamp updated_at
    }
    
    workflow_runs {
        varchar id PK "UUID"
        bigint workflow_id FK
        bigint user_id FK
        varchar status "running/completed/failed"
        json inputs
        text output
        json node_outputs "各节点输出"
        int total_tokens
        int duration_ms
        timestamp created_at
    }
    
    agent_tools {
        bigint id PK
        varchar agent_name
        varchar tool_name
    }
```

---

## 组件依赖关系

```mermaid
graph LR
    main[main.go] --> bootstrap[Bootstrap]
    
    bootstrap --> ChatHandler
    bootstrap --> A2AHandler
    bootstrap --> MCPServer
    bootstrap --> AgentRegistry
    
    ChatHandler --> ChatService
    ChatHandler --> AuthService
    ChatHandler --> KnowledgeService
    ChatHandler --> WorkflowHandler
    
    WorkflowHandler --> WorkflowService
    WorkflowHandler --> WorkflowEngine
    
    A2AHandler --> A2AService
    A2AService --> ChatService
    A2AService --> AgentRegistry
    
    ChatService --> SessionRepo["SessionRepository"]
    ChatService --> ModelGenerator["Generator 接口"]
    ChatService --> ToolRegistry["ToolRegistry"]
    ChatService --> KnowledgeService
    
    AgentRegistry --> ChatService
    AgentRegistry --> SubAgentTool["call_agent 工具"]
    
    KnowledgeService --> KnowledgeRepo["KnowledgeRepository"]
    KnowledgeService --> Embedder["Embedder 接口"]
    
    AuthService --> UserRepo["UserRepository"]
    AuthService --> TokenStore["TokenStore"]
    
    WorkflowEngine --> WorkflowRepo["WorkflowRepository"]
    WorkflowEngine --> RunRepo["RunRepository"]
    WorkflowEngine --> ModelGenerator
    WorkflowEngine --> AgentRegistry
    
    MCPServer --> ChatService
```

---

## 中间件链

请求经过的中间件链（从外到内）：

```
HTTP Request
    │
    ▼
┌─────────────────────┐
│  Recovery 中间件      │  ← panic 恢复，防止单请求崩溃导致服务宕机
├─────────────────────┤
│  Logging 中间件       │  ← 记录请求日志（IP、方法、路径、状态码、耗时）
├─────────────────────┤
│  RateLimit 中间件     │  ← 基于 IP 的令牌桶限流（可配置开关）
├─────────────────────┤
│  CORS 中间件          │  ← 跨域控制（从配置读取允许的 Origin）
├─────────────────────┤
│  JWT 认证（Handler级） │  ← 在 Handler 内部校验 Token，区分角色权限
└─────────────────────┘
    │
    ▼
  Handler
```

---

## 安全机制

| 机制 | 说明 |
|------|------|
| **JWT Token 认证** | 支持 Cookie 和 Authorization Header 双通道 |
| **密码哈希** | bcrypt 加密存储 |
| **脚本沙箱** | 路径白名单校验，防止路径穿越攻击 |
| **执行超时** | 单次脚本执行超时 30 秒 |
| **输出限制** | 脚本输出大小限制 512 KB |
| **进程隔离** | 进程组隔离，超时后 kill 整个子进程树 |
| **IP 限流** | 令牌桶算法，支持配置 RPS 和突发上限 |
| **CORS 控制** | 从配置文件读取允许的 Origin，不硬编码 `*` |
| **游客清理** | 定期自动清理游客会话（user_id=0） |
