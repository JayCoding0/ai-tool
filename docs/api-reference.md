# 📡 API 接口文档

> 完整的 REST API 参考文档，包含请求/响应示例和 SSE 事件类型说明。

## 目录

- [认证相关](#认证相关)
- [聊天 & 流式对话](#聊天--流式对话)
- [会话管理](#会话管理)
- [模型 & 工具](#模型--工具)
- [Agent 管理](#agent-管理)
- [知识库（RAG）](#知识库rag)
- [A2A 协议](#a2a-协议)
- [Workflow 工作流](#workflow-工作流)

---

## 通用说明

### 认证方式

所有需要认证的接口支持两种方式传递 Token：

| 方式 | 格式 |
|------|------|
| **Authorization Header** | `Authorization: Bearer <token>` |
| **Cookie** | `auth_token=<token>` |

### 错误响应格式

```json
{
  "error": "错误描述信息"
}
```

---

## 认证相关

### POST /api/auth/register

注册新用户。

**请求体：**
```json
{
  "username": "testuser",
  "password": "password123"
}
```

**成功响应（200）：**
```json
{
  "user_id": 1,
  "username": "testuser",
  "role": "user",
  "token": "eyJhbGciOiJIUzI1NiIs..."
}
```

**错误响应：**
- `400` - 用户名已存在 / 参数无效
- `503` - 数据库未连接

---

### POST /api/auth/login

用户登录。

**请求体：**
```json
{
  "username": "testuser",
  "password": "password123"
}
```

**成功响应（200）：**
```json
{
  "user_id": 1,
  "username": "testuser",
  "role": "admin",
  "token": "eyJhbGciOiJIUzI1NiIs..."
}
```

**错误响应：**
- `401` - 用户名或密码错误

---

### POST /api/auth/logout

退出登录，清除 Token。

**请求头：** 需要 Authorization

**成功响应（200）：**
```json
{
  "message": "已登出"
}
```

---

### GET /api/auth/me

获取当前登录用户信息。

**请求头：** 需要 Authorization

**成功响应（200）：**
```json
{
  "user_id": 1,
  "username": "admin",
  "role": "admin",
  "total_tokens": 12580
}
```

---

## 聊天 & 流式对话

### POST /api/chat/stream

发送消息并接收 SSE 流式响应。

**请求体：**
```json
{
  "message": "今天北京天气怎么样？",
  "session_id": "uuid-xxx",
  "model_name": "qwen-plus",
  "system_prompt": "你是一个有用的助手",
  "enabled_tools": ["get_weather"],
  "knowledge_base_id": 1
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `message` | string | ✅ | 用户消息 |
| `session_id` | string | ❌ | 会话 ID，为空则创建新会话 |
| `model_name` | string | ❌ | 模型名称，为空使用默认模型 |
| `system_prompt` | string | ❌ | System Prompt |
| `enabled_tools` | string[] | ❌ | 启用的工具列表，为空使用主 Agent 默认工具 |
| `knowledge_base_id` | int | ❌ | RAG 知识库 ID，0 表示不启用 |

**响应：** `Content-Type: text/event-stream`

SSE 事件格式：
```
data: {"type":"chunk","content":"你好","session_id":"uuid-xxx","model_name":"qwen-plus"}

data: {"type":"tool_call","tool_name":"get_weather","tool_call_id":"call_xxx","tool_args":"{\"city\":\"北京\"}","step":1}

data: {"type":"tool_result","tool_name":"get_weather","tool_call_id":"call_xxx","tool_result":"晴，25°C","step":1}

data: {"type":"done","session_id":"uuid-xxx","prompt_tokens":100,"completion_tokens":50,"total_tokens":150}
```

---

## 会话管理

### GET /api/sessions

获取当前用户的会话列表。

**成功响应（200）：**
```json
{
  "sessions": [
    {
      "id": "uuid-xxx",
      "title": "天气查询",
      "user_id": 1,
      "model_name": "qwen-plus",
      "system_prompt": "",
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T01:00:00Z"
    }
  ]
}
```

---

### POST /api/history

获取指定会话的消息历史。

**请求体：**
```json
{
  "session_id": "uuid-xxx"
}
```

**成功响应（200）：**
```json
{
  "session_id": "uuid-xxx",
  "messages": [
    {
      "role": "user",
      "content": "你好",
      "model_name": "",
      "prompt_tokens": 0,
      "completion_tokens": 0,
      "total_tokens": 0
    },
    {
      "role": "ai",
      "content": "你好！有什么可以帮你的吗？",
      "model_name": "qwen-plus",
      "prompt_tokens": 10,
      "completion_tokens": 15,
      "total_tokens": 25
    }
  ]
}
```

---

### POST /api/sessions/delete

删除指定会话。

**请求体：**
```json
{
  "session_id": "uuid-xxx"
}
```

---

### POST /api/sessions/rename

重命名会话。

**请求体：**
```json
{
  "session_id": "uuid-xxx",
  "title": "新的会话名称"
}
```

---

### POST /api/sessions/system-prompt

更新会话的 System Prompt。

**请求体：**
```json
{
  "session_id": "uuid-xxx",
  "system_prompt": "你是一个专业的翻译助手"
}
```

---

### GET /api/sessions/system-prompt/get

获取会话的 System Prompt。

**查询参数：** `?session_id=uuid-xxx`

**成功响应（200）：**
```json
{
  "system_prompt": "你是一个专业的翻译助手"
}
```

---

## 模型 & 工具

### GET /api/models

获取可用模型列表（云端 + 本地 Ollama）。

**成功响应（200）：**
```json
{
  "models": [
    { "name": "qwen-plus", "label": "通义千问 Plus", "type": "cloud" },
    { "name": "qwen-max", "label": "通义千问 Max", "type": "cloud" },
    { "name": "qwen2.5:14b", "label": "qwen2.5:14b (本地)", "type": "local" }
  ],
  "default_model": "qwen-plus"
}
```

> 本地模型列表从 Ollama `/api/tags` 接口实时拉取。

---

### GET /api/tools

获取所有已注册的工具列表。

**成功响应（200）：**
```json
{
  "tools": [
    {
      "name": "get_weather",
      "display_name": "天气查询",
      "description": "查询指定城市的实时天气"
    },
    {
      "name": "calculate",
      "display_name": "数学计算",
      "description": "执行数学表达式计算"
    }
  ],
  "count": 10
}
```

---

## Agent 管理

### GET /api/agents

获取所有 Agent 及其工具信息。

**成功响应（200）：**
```json
{
  "agents": [
    {
      "name": "master_agent",
      "display_name": "主调度 Agent",
      "description": "负责理解用户意图，拆解任务，并调用合适的子 Agent",
      "is_master": true,
      "default_tools": ["call_agent"],
      "tools": [
        { "name": "call_agent", "display_name": "调用子Agent", "description": "..." }
      ]
    },
    {
      "name": "weather_agent",
      "display_name": "天气查询 Agent",
      "description": "专门负责天气查询",
      "is_master": false,
      "default_tools": ["get_weather", "get_public_ip"],
      "tools": [
        { "name": "get_weather", "display_name": "天气查询", "description": "..." }
      ]
    }
  ],
  "count": 4
}
```

---

### PUT /api/agents/{name}/tools

动态更新指定 Agent 的工具列表（热更新，无需重启，自动持久化到数据库）。

**请求体：**
```json
{
  "tools": ["get_weather", "http_request"]
}
```

**成功响应（200）：**
```json
{
  "success": true,
  "agent_name": "weather_agent",
  "tools": ["get_weather", "http_request"]
}
```

---

## 知识库（RAG）

### GET /api/knowledge/bases

获取知识库列表。

**成功响应（200）：**
```json
{
  "knowledge_bases": [
    {
      "id": 1,
      "name": "产品文档",
      "description": "公司产品相关文档",
      "embed_model": "text-embedding-3-small",
      "doc_count": 5,
      "chunk_count": 120
    }
  ],
  "count": 1
}
```

---

### POST /api/knowledge/bases

创建知识库。

**请求体：**
```json
{
  "name": "产品文档",
  "description": "公司产品相关文档"
}
```

---

### DELETE /api/knowledge/bases/delete?id=1

删除知识库（同时删除所有关联文档和分块）。

---

### GET /api/knowledge/documents?kb_id=1

获取知识库下的文档列表。

---

### POST /api/knowledge/documents/upload

上传文档到知识库。支持两种模式：

**模式一：文件上传（multipart/form-data）**
```
POST /api/knowledge/documents/upload
Content-Type: multipart/form-data

kb_id: 1
file: <文件>
```

支持的文件类型：`.txt`、`.md`、`.pdf`

**模式二：JSON 直传**
```json
{
  "kb_id": 1,
  "name": "产品介绍",
  "content": "这是产品介绍的内容...",
  "content_type": "text"
}
```

---

### DELETE /api/knowledge/documents/delete?id=1

删除文档（同时删除关联分块）。

---

### POST /api/knowledge/search

手动测试知识库语义检索。

**请求体：**
```json
{
  "kb_id": 1,
  "query": "产品有哪些功能？",
  "top_k": 5
}
```

**成功响应（200）：**
```json
{
  "results": [
    {
      "content": "产品支持多模型切换、RAG 知识库...",
      "score": 0.85,
      "doc_name": "产品介绍.md"
    }
  ],
  "count": 3
}
```

---

## A2A 协议

### GET /.well-known/agent.json

获取 AgentCard（Agent 能力发现）。

**成功响应（200）：**
```json
{
  "protocol_version": "0.1",
  "name": "AI Agent",
  "description": "一个支持多工具调用的智能对话 Agent",
  "version": "1.0.0",
  "url": "http://localhost:8080/a2a/tasks/send",
  "capabilities": {
    "streaming": true,
    "multi_turn": true,
    "tool_calling": true
  },
  "skills": [
    {
      "id": "general_chat",
      "name": "通用对话",
      "description": "支持多轮对话，可以回答各类问题"
    }
  ]
}
```

---

### POST /a2a/tasks/send

提交 A2A 任务。

**请求体：**
```json
{
  "message": {
    "role": "user",
    "parts": [
      { "type": "text", "text": "今天天气怎么样？" }
    ]
  },
  "metadata": {
    "session_id": "optional-session-id"
  }
}
```

**成功响应（202 Accepted）：**
```json
{
  "task": {
    "id": "task-uuid-xxx",
    "state": "submitted",
    "input": { ... },
    "output": null,
    "metadata": { ... }
  }
}
```

---

### GET /a2a/tasks/{id}

查询任务状态。

**成功响应（200）：**
```json
{
  "id": "task-uuid-xxx",
  "state": "completed",
  "input": { ... },
  "output": {
    "role": "agent",
    "parts": [
      { "type": "text", "text": "北京今天晴，25°C..." }
    ]
  }
}
```

---

### GET /a2a/tasks/{id}/stream

SSE 流式订阅任务事件。

**响应：** `Content-Type: text/event-stream`

```
data: {"type":"status_update","state":"working"}

data: {"type":"message_chunk","content":"北京今天"}

data: {"type":"tool_call","tool_name":"get_weather","tool_args":"..."}

data: {"type":"tool_result","tool_name":"get_weather","tool_result":"..."}

data: {"type":"completed","task":{...}}
```

> 如果任务已是终态（completed/failed），直接返回 JSON 而非 SSE。

---

## Workflow 工作流

### GET /api/workflows

获取当前用户的工作流列表。

**成功响应（200）：**
```json
{
  "workflows": [
    {
      "id": 1,
      "name": "翻译工作流",
      "description": "自动翻译文本",
      "status": "published",
      "version": 2,
      "created_at": "2026-04-01T00:00:00Z",
      "updated_at": "2026-04-07T12:00:00Z"
    }
  ],
  "count": 1
}
```

---

### POST /api/workflows

创建新工作流。

**请求体：**
```json
{
  "name": "翻译工作流",
  "description": "自动翻译文本",
  "nodes": [
    { "id": "start_1", "type": "start", "name": "开始", "config": {}, "position": { "x": 100, "y": 200 } },
    { "id": "llm_1", "type": "llm", "name": "翻译", "config": { "user_prompt": "请翻译：${query}" }, "position": { "x": 300, "y": 200 } },
    { "id": "end_1", "type": "end", "name": "结束", "config": {}, "position": { "x": 500, "y": 200 } }
  ],
  "edges": [
    { "id": "e1", "source": "start_1", "target": "llm_1" },
    { "id": "e2", "source": "llm_1", "target": "end_1" }
  ],
  "variables": [
    { "name": "query", "type": "string", "required": true, "description": "待翻译文本" }
  ]
}
```

**成功响应（200）：**
```json
{
  "id": 1,
  "name": "翻译工作流",
  "status": "draft",
  "version": 1
}
```

---

### GET /api/workflows/{id}

获取工作流详情（含完整节点/边/变量定义）。

---

### PUT /api/workflows/{id}

更新工作流定义。请求体同创建接口。

---

### DELETE /api/workflows/{id}

删除工作流。

---

### POST /api/workflows/{id}/publish

发布工作流（draft → published），版本号自动递增。

**成功响应（200）：**
```json
{
  "message": "工作流已发布",
  "version": 2
}
```

---

### POST /api/workflows/{id}/execute

执行工作流（SSE 流式推送执行事件）。

**请求体：**
```json
{
  "inputs": {
    "query": "Hello, world!"
  }
}
```

**响应：** `Content-Type: text/event-stream`

SSE 事件格式：
```
data: {"type":"node_start","node_id":"llm_1","node_name":"翻译","node_type":"llm"}

data: {"type":"node_output","node_id":"llm_1","node_name":"翻译","output":"你好，世界！","duration_ms":1200}

data: {"type":"node_done","node_id":"llm_1","node_name":"翻译","duration_ms":1200}

data: {"type":"workflow_done","output":"你好，世界！","run_id":"uuid-xxx","total_tokens":150}
```

### Workflow SSE 事件类型

| 事件类型 | 说明 | 包含字段 |
|---------|------|----------|
| `node_start` | 节点开始执行 | `node_id`, `node_name`, `node_type` |
| `node_output` | 节点输出结果 | `node_id`, `node_name`, `output`, `duration_ms` |
| `node_error` | 节点执行失败 | `node_id`, `node_name`, `error` |
| `node_done` | 节点执行完成 | `node_id`, `node_name`, `duration_ms` |
| `workflow_done` | 工作流执行完成 | `output`, `run_id`, `total_tokens` |
| `workflow_error` | 工作流执行失败 | `error`, `run_id` |

### 节点类型说明

| 节点类型 | 说明 | 关键配置 |
|---------|------|----------|
| `start` | 开始节点（入口） | 无 |
| `end` | 结束节点（出口） | 无 |
| `llm` | LLM 对话节点 | `model_name`, `system_prompt`, `user_prompt`, `temperature` |
| `tool` | 工具调用节点 | `tool_name`, `tool_args` |
| `agent` | 子 Agent 节点 | `agent_name`, `agent_message` |
| `template` | 模板转换节点 | `template` |
| `http` | HTTP 请求节点 | `url`, `method`, `headers`, `body` |

### 变量引用语法

工作流使用 `${}` 语法引用变量：

| 语法 | 说明 | 示例 |
|------|------|------|
| `${变量名}` | 引用全局变量（用户输入） | `${query}` |
| `${node_id.output}` | 引用上游节点的输出 | `${llm_1.output}` |
| `${current_time}` | 内置变量：当前时间 | — |
| `${current_date}` | 内置变量：当前日期 | — |

---

### GET /api/workflows/{id}/runs

获取工作流的执行记录列表。

**成功响应（200）：**
```json
{
  "runs": [
    {
      "id": "uuid-xxx",
      "workflow_id": 1,
      "status": "completed",
      "inputs": { "query": "Hello" },
      "output": "你好",
      "total_tokens": 150,
      "duration_ms": 2500,
      "created_at": "2026-04-07T12:00:00Z"
    }
  ],
  "count": 1
}
```

---

### GET /api/workflow-runs/{run_id}

获取单次执行记录详情（含各节点执行结果）。
