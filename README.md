# AI 智能助手

一个基于 Go 语言构建的本地 AI 对话平台，支持多模型切换、Skill 技能系统、Function Calling 工具调用、多用户权限管理，提供现代化 Web 聊天界面。

## ✨ 功能特性

- 🤖 **多模型支持**：同时接入云端模型（阿里云 DashScope / OpenAI 兼容接口）与本地 Ollama 模型，运行时自由切换
- 🎯 **Skill 技能系统**：通过 `SKILL.md` 定义 AI 角色与行为，支持 5 种设计模式（工具封装、生成器、评审器、倒置、流水线）
- 🔧 **Function Calling**：ReAct 循环架构，AI 可自主调用工具（计算器、天气查询、HTTP 请求、命令执行、文件操作、MySQL 查询等）
- 📡 **SSE 流式输出**：Server-Sent Events 实时推送 AI 回复、思考过程、工具调用过程
- 👥 **多用户权限系统**：三级角色（admin / user / guest），精细化权限控制
- 💬 **多会话管理**：支持多会话并行、历史记录持久化、会话标题自动生成、会话重命名
- 📊 **Token 统计**：按用户、按模型统计 Token 消耗
- 🔌 **MCP 协议**：提供 Model Context Protocol 服务端接口
- 🤝 **多 Agent 编排**：内置 Agent 注册中心，支持主 Agent 编排多个子 Agent 协同完成复杂任务，子 Agent 思考/工具调用过程实时透传

## 🏗️ 技术架构

```
aiProject/
├── main.go                          # 程序入口
├── trpc_go.yaml.example             # 配置文件模板
├── frontend/                        # 前端（Vue 3 + Element Plus）
│   ├── index.html                   # 主聊天界面
│   ├── login.html                   # 登录/注册页面
│   ├── style.css                    # 全局样式
│   └── js/
│       ├── app.js                   # 主应用逻辑（Vue 3 组件）
│       ├── api.js                   # API 请求封装
│       ├── markdown.js              # Markdown 渲染（marked + highlight.js）
│       ├── session.js               # 会话管理逻辑
│       └── tools.js                 # 工具调用展示逻辑
├── skills/                          # 内置 Skill 技能目录
│   ├── calculate/                   # 计算器技能
│   ├── create-skill/                # Skill 生成器技能
│   ├── current-time/                # 当前时间技能
│   ├── execute-command/             # 命令执行技能
│   ├── file-explorer/               # 文件浏览技能
│   ├── http-request/                # HTTP 请求技能
│   ├── ip-lookup/                   # IP 查询技能
│   ├── mysql-query/                 # MySQL 查询技能
│   ├── weather/                     # 天气查询技能
│   └── write-file/                  # 文件写入技能
├── database/                        # 数据库迁移脚本
│   ├── schema.sql                   # 初始建表
│   └── migrate_*.sql                # 增量迁移脚本
└── internal/                        # 后端核心（DDD 分层架构）
    ├── application/                 # 应用层
    │   ├── chat_service.go          # 聊天服务（流式 + 工具调用）
    │   ├── auth_service.go          # 认证服务（JWT）
    │   └── skill_service.go         # 技能管理服务
    ├── domain/                      # 领域层
    │   ├── model/                   # AI 模型抽象
    │   ├── session/                 # 会话聚合根
    │   ├── skill/                   # Skill 实体
    │   ├── tool/                    # 工具注册表
    │   └── user/                    # 用户实体（含角色）
    ├── infrastructure/              # 基础设施层
    │   ├── model/                   # OpenAI / Ollama 实现
    │   ├── session/mysql/           # 会话 MySQL 持久化
    │   ├── skill/mysql/             # Skill MySQL 持久化
    │   ├── tools/                   # 工具加载器（扫描 skills/*/scripts/）
    │   └── user/mysql/              # 用户 MySQL 持久化
    ├── application/
    │   ├── agent_registry.go        # Agent 注册中心（多 Agent 编排）
    │   └── agent_runner.go          # ReAct 循环执行器
    └── interfaces/
        ├── http/handler.go          # HTTP API 处理器
        └── mcp/server.go            # MCP 协议服务端
```

### 后端技术栈

- **语言**：Go 1.24+
- **框架**：TRPC-GO
- **数据库**：MySQL 8.0+
- **AI 接入**：OpenAI 兼容接口 / Ollama
- **架构模式**：DDD（领域驱动设计）分层架构

### 前端技术栈

- **Vue 3**（CDN 引入，无构建工具依赖）
- **Element Plus**：UI 组件库（按钮、对话框、下拉菜单等）
- **marked.js + highlight.js**：Markdown 渲染与代码高亮
- SSE（Server-Sent Events）流式接收
- 模块化 JS（`api.js` / `markdown.js` / `session.js` / `tools.js` / `app.js`）
- 响应式设计，支持移动端

## 🚀 快速开始

### 前置要求

- Go 1.24+
- MySQL 8.0+
- Python 3（用于执行 Skill 脚本工具）
- （可选）[Ollama](https://ollama.com/) 用于本地模型

### 1. 克隆项目

```bash
git clone https://github.com/JayCoding0/ai-tool.git
cd ai-tool
go mod tidy
```

### 2. 初始化数据库

```bash
# 执行完整建库脚本（包含建库 + 所有表结构）
mysql -u root -p < database/schema.sql

# 插入预设数据（admin 账户 + 系统预设技能）
mysql -u root -p ai_chat_db < database/seed.sql
```

### 3. 配置文件

复制配置模板并填写实际配置：

```bash
cp trpc_go.yaml.example trpc_go.yaml
```

编辑 `trpc_go.yaml`，重点配置以下内容：

```yaml
client:
  service:
    - name: trpc.mysql.ai_chat.db
      # 替换为你的 MySQL 连接信息
      target: dsn://YOUR_DB_USER:YOUR_DB_PASSWORD@tcp(localhost:3306)/ai_chat_db?...

custom:
  model:
    name: "qwen-plus"           # 默认模型
    type: "openai"              # openai 或 local
    openai_base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1"
    openai_api_key: "YOUR_API_KEY"   # 替换为你的 API Key
    ollama_url: "http://localhost:11434"  # 本地 Ollama 地址
```

### 4. 启动服务

```bash
go run main.go
```

访问 [http://localhost:8080](http://localhost:8080) 开始使用。

## 👥 用户权限系统

系统内置三级角色，初始化后自动创建 `admin` 账户（密码：`admin123`，**请及时修改**）。

| 角色 | 说明 | 权限 |
|------|------|------|
| **admin** | 超级管理员 | 查看/编辑/删除所有 Skill，下载/上传 Skill，管理所有用户数据 |
| **user** | 普通用户 | 创建/编辑/删除自己的 Skill，只读预设 Skill，使用所有功能 |
| **guest** | 游客（未登录） | 仅使用预设 Skill，无法创建/修改任何内容 |

## 🎯 Skill 技能系统

Skill 是 AI 的角色与行为定义，通过 `SKILL.md` 文件描述。每个 Skill 包含：

- **System Prompt**：定义 AI 的角色、能力边界和行为规范
- **绑定工具**：指定该 Skill 可调用的 Function Calling 工具
- **设计模式**：5 种内置模式

### Skill 设计模式

| 模式 | 说明 |
|------|------|
| `tool-wrapper` | 工具封装：按需加载知识，调用外部工具 |
| `generator` | 生成器：固定输出结构，批量生产内容 |
| `reviewer` | 评审器：解耦检查规则，对内容进行评估 |
| `inversion` | 倒置：先问清楚需求再执行 |
| `pipeline` | 流水线：强制分步执行复杂任务 |

### Skill 目录结构

```
skills/my-skill/
├── SKILL.md          # 技能定义（YAML front matter + System Prompt 正文）
├── scripts/
│   ├── tool.json     # 工具定义（Function Calling 参数描述）
│   └── run.py        # 工具执行脚本（通过 stdin 接收 JSON 参数）
├── references/       # 参考资料（.md 文件，自动追加到 System Prompt）
└── assets/           # 模板文件（.md 文件，自动追加到 System Prompt）
```

### SKILL.md 格式示例

```markdown
---
name: 数据分析助手
description: 帮助用户分析数据并生成报告
icon: 📊
pattern: pipeline
tools:
  - mysql_query
  - calculate
version: "1.0"
---

你是一个专业的数据分析助手...
```

### 内置 Skill 列表

| Skill | 说明 | 工具 |
|-------|------|------|
| `calculate` | 数学计算 | Python 计算器 |
| `create-skill` | 自动生成新 Skill | 文件写入 |
| `current-time` | 获取当前时间 | Python 时间脚本 |
| `execute-command` | 执行系统命令 | Shell 执行器 |
| `file-explorer` | 文件目录浏览 | Go 原生实现 |
| `http-request` | 发送 HTTP 请求 | Python requests |
| `ip-lookup` | IP 归属地查询 | Go 原生实现 |
| `mysql-query` | MySQL 数据库查询 | Python MySQL 客户端 |
| `weather` | 天气查询 | Go 原生实现（百度地图 API）|
| `write-file` | 写入文件 | Python 文件操作 |

## 🤝 多 Agent 编排

系统内置 **Agent 注册中心**（`AgentRegistry`），支持将多个专职 AI Agent 组合成协作团队：

### 工作原理

```
用户请求
    │
    ▼
主 Agent（Master）
    │  通过 call_agent 工具调用子 Agent
    ├──► 子 Agent A（专职工具调用）
    │        └── 思考/工具调用过程实时透传给主 Agent
    ├──► 子 Agent B（专职内容生成）
    │        └── 思考/工具调用过程实时透传给主 Agent
    └──► 子 Agent C（专职评审）
             └── 思考/工具调用过程实时透传给主 Agent
```

### Agent 定义示例

```go
// 注册一个子 Agent
registry.Register(application.AgentDefinition{
    Name:         "data-analyst",
    DisplayName:  "数据分析师",
    Description:  "负责查询数据库并分析数据，当需要数据查询或统计时调用",
    SystemPrompt: "你是一个专业的数据分析师...",
    EnabledTools: []string{"mysql_query", "calculate"},
    ModelName:    "qwen-plus",
    IsMaster:     false,
}, chatService)
```

### 特性

| 特性 | 说明 |
|------|------|
| **主 Agent 编排** | 主 Agent 自动获得 `call_agent` 工具，可按需调用任意子 Agent |
| **事件透传** | 子 Agent 的思考过程、工具调用、工具结果实时透传到前端展示 |
| **独立模型** | 每个子 Agent 可配置不同的模型（如主 Agent 用 GPT-4，子 Agent 用 qwen-plus）|
| **独立工具集** | 每个子 Agent 只能使用其配置的工具，职责边界清晰 |
| **并发安全** | 注册中心使用读写锁保护，支持并发访问 |

## 🔧 Function Calling 工具调用

系统采用 **ReAct（Reason + Act）循环**架构，AI 可在单次对话中自主决策调用多个工具：

1. AI 分析用户需求，决定调用哪些工具
2. 并发执行所有工具调用
3. 将工具结果反馈给 AI
4. AI 根据结果继续推理或生成最终回复
5. 最多循环 **5 轮**，超出后 AI 自动总结进度并提示用户继续

工具通过扫描 `skills/*/scripts/tool.json` 自动注册，支持 Python / Shell / Node.js 脚本，参数通过 `stdin` 以 JSON 格式传入。

## 📡 API 接口

### 认证

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/auth/register` | 注册账户 |
| POST | `/api/auth/login` | 登录（返回 JWT Token + Cookie）|
| POST | `/api/auth/logout` | 登出 |
| GET  | `/api/auth/me` | 获取当前用户信息及 Token 统计 |

### 聊天

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/chat/stream` | 流式聊天（SSE），支持工具调用 |
| POST | `/api/chat/history` | 获取会话历史 |
| GET  | `/api/chat/sessions` | 列出会话列表 |
| POST | `/api/chat/session/delete` | 删除会话 |
| POST | `/api/chat/session/rename` | 重命名会话 |
| GET  | `/api/chat/system-prompt` | 获取会话 System Prompt |
| POST | `/api/chat/system-prompt` | 更新会话 System Prompt |

### 模型 & 工具

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/api/models` | 获取可用模型列表（含 Ollama 实时拉取）|
| GET  | `/api/tools` | 获取已注册工具列表 |

### Skill 技能

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| GET  | `/api/skills` | 全部 | 列出可见 Skill |
| POST | `/api/skills/create` | user/admin | 创建 Skill |
| POST | `/api/skills/update?id=` | 仅本人/admin | 更新 Skill |
| POST | `/api/skills/delete` | 仅本人/admin | 删除 Skill |
| POST | `/api/skills/apply` | 全部 | 将 Skill 应用到会话 |
| GET  | `/api/admin/skills/download?id=` | admin | 下载 Skill JSON |
| POST | `/api/admin/skills/upload` | admin | 上传/导入 Skill |

### SSE 流式事件类型

```json
{ "type": "chunk",       "content": "...", "thinking": "..." }   // 增量内容
{ "type": "thought",     "content": "...", "step": 1 }           // AI 思考过程
{ "type": "tool_call",   "tool_name": "...", "tool_args": "..." } // 工具调用
{ "type": "tool_result", "tool_name": "...", "tool_result": "..." } // 工具结果
{ "type": "done",        "total_tokens": 123 }                   // 完成
{ "type": "error",       "error": "..." }                        // 错误
```

## ⚙️ 配置说明

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `custom.server.http_port` | HTTP 服务端口 | `8080` |
| `custom.server.mcp_port` | MCP 服务端口 | `8001` |
| `custom.model.name` | 默认模型名称 | `qwen-plus` |
| `custom.model.type` | 模型类型（openai/local）| `openai` |
| `custom.model.openai_base_url` | OpenAI 兼容接口地址 | DashScope |
| `custom.model.openai_api_key` | API Key | — |
| `custom.model.ollama_url` | Ollama 服务地址 | `http://localhost:11434` |
| `custom.model.max_context_length` | 最大上下文长度 | `4096` |

## 🛠️ 开发指南

### 添加新 Skill

1. 在 `skills/` 下创建新目录，如 `skills/my-skill/`
2. 创建 `SKILL.md`（含 YAML front matter）
3. 创建 `scripts/tool.json` 定义工具参数
4. 创建 `scripts/run.py`（或 `.sh` / `.js`）实现工具逻辑
5. 重启服务，工具自动注册，Skill 自动加载

### 接入新 AI 模型

实现 `internal/domain/model/Generator` 接口：

```go
type Generator interface {
    Generate(ctx context.Context, prompt Prompt) (*Response, error)
    GenerateStreamWithMessages(ctx context.Context, messages []Message) (<-chan StreamChunk, error)
    GenerateWithTools(ctx context.Context, messages []Message, tools []ToolDefinition) (*ToolCallResponse, error)
}
```

## 🔒 安全说明

- 脚本执行有路径白名单校验，防止路径穿越攻击
- 单次脚本执行超时限制：30 秒
- 脚本输出大小限制：512 KB
- 进程组隔离，超时后 kill 整个子进程树
- JWT Token 认证，支持 Cookie 和 Authorization Header
- 游客会话定期自动清理

## 📄 许可证

MIT License
