# 🔄 核心流程图集

> 本文档集中展示系统各核心流程的时序图和流程图，帮助理解系统运行机制。

## 目录

- [聊天 + 工具调用时序图](#1-聊天--工具调用时序图)
- [多 Agent 编排流程](#2-多-agent-编排流程)
- [RAG 知识库处理流程](#3-rag-知识库处理流程)
- [A2A 协议任务流程](#4-a2a-协议任务流程)
- [用户认证流程](#5-用户认证流程)
- [请求处理全链路](#6-请求处理全链路)

---

## 1. 聊天 + 工具调用时序图

展示一次完整的聊天请求从前端发起到最终响应的全过程，包括 RAG 注入和 ReAct 工具调用循环。

```mermaid
sequenceDiagram
    actor User as 用户
    participant FE as 前端
    participant GW as 网关层
    participant Chat as ChatService
    participant Agent as AgentRunner
    participant LLM as AI 模型
    participant Tool as 工具执行器

    User->>FE: 发送消息
    FE->>GW: POST /api/chat/stream (SSE)
    GW->>GW: JWT 认证 + 限流
    GW->>Chat: 转发请求

    Chat->>Chat: 加载会话历史 + System Prompt
    
    opt RAG 知识库已绑定
        Chat->>Chat: 语义检索相关文档
        Chat->>Chat: 注入检索结果到 Prompt
    end

    Chat->>Agent: 启动 ReAct 循环
    
    loop 最多 5 轮
        Agent->>LLM: 发送消息 + 工具定义
        LLM-->>FE: SSE: thinking (思考过程)
        LLM-->>Agent: 返回工具调用决策
        
        alt AI 决定调用工具
            Agent-->>FE: SSE: tool_call (工具名 + 参数)
            Agent->>Tool: 并发执行工具
            Tool-->>Agent: 返回工具结果
            Agent-->>FE: SSE: tool_result (执行结果)
            Agent->>Agent: 将结果加入上下文，继续循环
        else AI 直接回复
            LLM-->>FE: SSE: chunk (流式文本)
            Agent->>Agent: 退出循环
        end
    end

    Agent-->>FE: SSE: done (完成 + Token 统计)
    Chat->>Chat: 持久化消息到 MySQL
    FE->>User: 渲染完整回复
```

### SSE 事件类型说明

| 事件类型 | 说明 | 包含字段 |
|---------|------|---------|
| `chunk` | 流式文本片段 | `content`, `thinking`, `session_id`, `model_name` |
| `thought` | AI 思考过程 | `content`, `step`, `parent_tool_call_id` |
| `tool_call` | 工具调用请求 | `tool_name`, `tool_display_name`, `tool_call_id`, `tool_args`, `step` |
| `tool_result` | 工具执行结果 | `tool_name`, `tool_call_id`, `tool_result`, `step` |
| `done` | 完成信号 | `session_id`, `model_name`, `prompt_tokens`, `completion_tokens`, `total_tokens` |
| `error` | 错误信息 | `error` |

---

## 2. 多 Agent 编排流程

展示主 Agent 如何编排多个子 Agent 协同完成复杂任务，以及事件透传机制。

```mermaid
sequenceDiagram
    actor User as 用户
    participant Master as 主 Agent (调度)
    participant Weather as 天气 Agent
    participant Search as 搜索 Agent
    participant Code as 代码 Agent

    User->>Master: "北京今天天气怎么样？推荐去哪玩？"
    
    Master->>Master: 分析意图：天气查询 + 景点推荐
    Master->>Weather: call_agent(weather_agent, "北京天气")
    
    activate Weather
    Weather->>Weather: 调用 get_weather 工具
    Weather-->>Master: "北京今天晴，25°C..."
    deactivate Weather
    
    Master-->>User: SSE: 实时透传天气 Agent 的思考过程
    
    Master->>Search: call_agent(search_agent, "北京25°C晴天推荐景点")
    
    activate Search
    Search->>Search: 调用 http_request 工具
    Search-->>Master: "推荐：故宫、颐和园..."
    deactivate Search
    
    Master-->>User: SSE: 实时透传搜索 Agent 的思考过程
    Master->>Master: 汇总所有子 Agent 结果
    Master-->>User: 最终回复：天气 + 景点推荐
```

### Agent 注册架构

```mermaid
graph TB
    subgraph Registry["Agent 注册中心"]
        Master["🎯 master_agent<br/>主调度 Agent<br/>工具: call_agent"]
        Weather["🌤️ weather_agent<br/>天气查询 Agent<br/>工具: get_weather, get_public_ip"]
        Search["🔍 search_agent<br/>搜索 Agent<br/>工具: http_request"]
        Code["💻 code_agent<br/>代码工具 Agent<br/>工具: calculate, write_file,<br/>execute_command, mysql_query"]
    end

    Master -->|call_agent| Weather
    Master -->|call_agent| Search
    Master -->|call_agent| Code

    subgraph Features["动态能力"]
        HotUpdate["🔥 热更新工具列表"]
        DBPersist["💾 数据库持久化配置"]
        EventRelay["📡 子Agent事件透传"]
    end
```

---

## 3. RAG 知识库处理流程

展示文档从上传到向量化存储，再到语义检索增强 AI 回答的完整流程。

```mermaid
flowchart LR
    subgraph Upload["📤 文档上传"]
        A[上传文档] --> B[创建文档记录]
        B --> C[状态: pending]
    end

    subgraph Process["⚙️ 异步处理"]
        C --> D[文本分块<br/>500字/块, 50字重叠]
        D --> E[批量向量化<br/>OpenAI Embedding]
        E --> F[存储到 MySQL<br/>向量 + 原文]
        F --> G[状态: done]
    end

    subgraph Query["🔍 语义检索"]
        H[用户提问] --> I[问题向量化]
        I --> J[余弦相似度计算]
        J --> K[TopK 过滤<br/>阈值 ≥ 0.3]
        K --> L[注入 System Prompt]
        L --> M[AI 基于知识回答]
    end

    Upload --> Process
    Process -.->|知识就绪| Query
```

### 知识库数据流

```mermaid
flowchart TB
    subgraph Input["输入支持"]
        TXT[纯文本 .txt]
        MD[Markdown .md]
        PDF[PDF .pdf]
        JSON_INPUT[JSON 直传]
    end

    subgraph Processing["处理管线"]
        Parse[文本解析] --> Chunk[智能分块<br/>500字/块]
        Chunk --> Embed[向量化<br/>text-embedding-3-small]
        Embed --> Store[存储<br/>MySQL mediumblob]
    end

    subgraph Retrieval["检索增强"]
        Query[用户问题] --> QEmbed[问题向量化]
        QEmbed --> CosSim[余弦相似度]
        CosSim --> TopK[Top-K 筛选]
        TopK --> Inject[注入 Prompt]
    end

    Input --> Processing
    Processing --> Retrieval
```

---

## 4. A2A 协议任务流程

展示外部 Agent/客户端通过 A2A 协议与本系统交互的完整流程。

```mermaid
sequenceDiagram
    participant Client as 外部 Agent/客户端
    participant A2A as A2A Service
    participant Store as TaskStore
    participant Chat as ChatService
    participant Hub as StreamHub

    Client->>A2A: POST /a2a/tasks/send
    A2A->>Store: 创建任务 (submitted)
    A2A-->>Client: 返回 Task{id, status}
    
    A2A->>A2A: 异步执行任务
    A2A->>Store: 更新状态 (working)
    A2A->>Hub: 推送 status_update
    
    A2A->>Chat: 调用 ChatService 处理
    Chat-->>Hub: 推送 message_chunk / tool_call / tool_result
    
    par 客户端订阅 SSE
        Client->>A2A: GET /a2a/tasks/{id}/stream
        Hub-->>Client: SSE 实时推送事件流
    end
    
    Chat-->>A2A: 处理完成
    A2A->>Store: 更新状态 (completed) + 持久化
    A2A->>Hub: 推送 completed 事件
    Hub-->>Client: SSE: completed + 完整任务
    
    Note over Client,Hub: 也可通过 GET /a2a/tasks/{id} 轮询状态
```

### A2A 任务状态机

```mermaid
stateDiagram-v2
    [*] --> submitted: POST /a2a/tasks/send
    submitted --> working: 开始处理
    working --> completed: 处理成功
    working --> failed: 处理失败
    completed --> [*]
    failed --> [*]
    
    note right of working
        SSE 实时推送：
        - status_update
        - message_chunk
        - tool_call
        - tool_result
    end note
```

---

## 5. 用户认证流程

```mermaid
sequenceDiagram
    actor User as 用户
    participant FE as 前端
    participant Auth as AuthService
    participant DB as MySQL

    alt 注册
        User->>FE: 填写用户名/密码
        FE->>Auth: POST /api/auth/register
        Auth->>Auth: 校验用户名唯一性
        Auth->>Auth: bcrypt 加密密码
        Auth->>DB: 插入用户记录
        Auth->>Auth: 生成 JWT Token
        Auth-->>FE: {user_id, username, role, token}
        FE->>FE: 存储 token 到 localStorage + Cookie
    end

    alt 登录
        User->>FE: 填写用户名/密码
        FE->>Auth: POST /api/auth/login
        Auth->>DB: 查询用户
        Auth->>Auth: bcrypt 验证密码
        Auth->>Auth: 生成 JWT Token
        Auth-->>FE: {user_id, username, role, token}
        FE->>FE: 存储 token
    end

    alt 请求认证
        FE->>Auth: 请求头 Authorization: Bearer {token}
        Auth->>Auth: 验证 JWT 签名 + 过期时间
        Auth-->>FE: {user_id, username, role}
    end
```

---

## 6. 请求处理全链路

展示一个 HTTP 请求从进入到响应的完整中间件链路。

```mermaid
flowchart TB
    Request[HTTP 请求] --> Recovery[Recovery 中间件<br/>panic 恢复]
    Recovery --> Logging[Logging 中间件<br/>请求日志记录]
    Logging --> RateLimit[RateLimit 中间件<br/>IP 令牌桶限流]
    RateLimit --> CORS[CORS 中间件<br/>跨域控制]
    CORS --> Router[路由匹配]
    
    Router --> AuthCheck{需要认证?}
    AuthCheck -->|是| JWT[JWT Token 校验<br/>Cookie / Header]
    AuthCheck -->|否| Handler[Handler 处理]
    JWT --> Handler
    
    Handler --> Response[HTTP 响应]

    style Recovery fill:#fef3c7
    style Logging fill:#dbeafe
    style RateLimit fill:#fce7f3
    style CORS fill:#d1fae5
    style JWT fill:#e0e7ff
```
