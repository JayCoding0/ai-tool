# 🤖 AI Agent 平台

> 对标 Dify / Coze（扣子）/ FastGPT 的企业级 AI Agent 平台 —— 多 Agent 编排 · 可视化 Workflow · RAG 知识库 · A2A / MCP 双协议

基于 **Go (TRPC-GO)** + **Vue 3** 构建，采用 DDD 分层架构，提供多模型切换、ReAct 工具调用、可视化 DAG 工作流、RAG 知识库与长期记忆等完整 Agent 基础能力。

---

## 📖 目录

- [功能概览](#-功能概览)
- [系统架构](#-系统架构)
- [快速开始](#-快速开始)
- [技术栈](#-技术栈)
- [项目结构](#-项目结构)
- [详细文档](#-详细文档)
- [Roadmap](#-roadmap)
- [Contributing](#-contributing)
- [许可证](#-许可证)

---

## ✨ 功能概览

### 🧠 AI 核心能力


| 功能                 | 说明                                                                                     |
| -------------------- | ---------------------------------------------------------------------------------------- |
| **多模型切换**       | 同时接入云端模型（阿里云 DashScope / OpenAI 兼容接口）与本地 Ollama 模型，运行时自由切换 |
| **Function Calling** | ReAct 循环架构，AI 自主决策调用工具（计算器、天气、HTTP、命令执行、文件操作、MySQL 等）  |
| **多 Agent 编排**    | 主 Agent 编排多个子 Agent 协同完成复杂任务，思考/工具调用过程实时透传                    |
| **Workflow 编排**    | 可视化 DAG 工作流引擎，7 种节点类型，拓扑排序执行，SSE 流式推送执行事件                  |
| **RAG 知识库**       | 文档上传 → 自动分块 → 向量化 → 语义检索，增强 AI 回答的准确性                         |
| **Prompt 模板变量**  | `${变量名}` 模板语法，三级变量优先级（用户级 → 会话级 → 请求级），数据库持久化         |
| **Skill 技能系统**   | 通过`SKILL.md` 定义 AI 角色与行为，支持 5 种设计模式                                     |
| **SSE 流式输出**     | 实时推送 AI 回复、思考过程、工具调用过程                                                 |

### 🔗 协议支持


| 协议                              | 说明                                                                 |
| --------------------------------- | -------------------------------------------------------------------- |
| **A2A（Agent-to-Agent）**         | Google 提出的 Agent 间通信协议，支持任务提交、状态查询、SSE 流式订阅 |
| **MCP（Model Context Protocol）** | Anthropic 提出的模型上下文协议，提供标准化工具接口                   |

### 👥 平台功能


| 功能           | 说明                                                 |
| -------------- | ---------------------------------------------------- |
| **多用户权限** | 三级角色（admin / user / guest），精细化权限控制     |
| **多会话管理** | 多会话并行、历史记录持久化、标题自动生成、会话重命名 |
| **Token 统计** | 按用户、按模型统计 Token 消耗                        |
| **暗色主题**   | 支持亮色/暗色主题切换，自动记忆用户偏好              |
| **知识库管理** | 可视化知识库管理页面，支持文档上传、删除、语义搜索   |
| **IP 限流**    | 基于令牌桶的 IP 级限流，防止恶意请求                 |

---

## 🏗️ 系统架构

```mermaid
graph TB
    subgraph Frontend["🖥️ 前端 (Vue 3 + Element Plus)"]
        UI[聊天界面]
        WF_UI[工作流编辑器]
        KB_UI[知识库管理]
        Login[登录/注册]
        Theme[主题切换]
    end

    subgraph Gateway["🔒 网关层"]
        CORS[CORS 中间件]
        RateLimit[IP 限流]
        Auth[JWT 认证]
        Recovery[Panic 恢复]
    end

    subgraph Application["⚙️ 应用层"]
        ChatSvc[ChatService<br/>聊天服务]
        AuthSvc[AuthService<br/>认证服务]
        SkillSvc[SkillService<br/>技能服务]
        KnowledgeSvc[KnowledgeService<br/>知识库服务]
        WorkflowSvc[WorkflowService<br/>工作流服务]
        WorkflowEngine[WorkflowEngine<br/>DAG 执行引擎]
        A2ASvc[A2AService<br/>A2A 任务调度]
        AgentRegistry[AgentRegistry<br/>Agent 注册中心]
    end

    subgraph Domain["🎯 领域层"]
        Session[会话聚合根]
        User[用户实体]
        Skill[Skill 实体]
        Tool[工具注册表]
        Workflow[Workflow 聚合根]
        Task[A2A 任务]
        KnowledgeBase[知识库实体]
    end

    subgraph Infrastructure["🔧 基础设施层"]
        OpenAI[OpenAI 兼容接口]
        Ollama[Ollama 本地模型]
        MySQL[(MySQL 8.0)]
        Embedder[向量化引擎]
        ToolLoader[工具加载器]
        MCP_Server[MCP Server]
    end

    Frontend -->|HTTP/SSE| Gateway
    Gateway --> Application
    Application --> Domain
    Application --> Infrastructure
    A2ASvc -->|A2A 协议| ChatSvc
    AgentRegistry -->|编排| ChatSvc
    WorkflowEngine -->|调用| AgentRegistry
    WorkflowEngine -->|调用| Tool
    KnowledgeSvc -->|RAG 注入| ChatSvc
```

> 📊 更多流程图（聊天时序图、多 Agent 编排、RAG 处理流程、Workflow 执行流程、A2A 任务流程等）请查看 **[核心流程图集](docs/flowcharts.md)**

---

## 🚀 快速开始

### 前置要求


| 依赖     | 版本  | 说明               |
| -------- | ----- | ------------------ |
| Go       | 1.25+ | 后端运行时         |
| MySQL    | 8.0+  | 数据持久化         |
| Python 3 | 3.8+  | Skill 脚本工具执行 |
| Ollama   | 可选  | 本地模型推理       |

### 1. 克隆 & 安装依赖

```bash
git clone https://github.com/JayCoding0/ai-tool.git
cd ai-tool
go mod tidy
```

### 2. 初始化数据库

```bash
# 建库 + 建表
mysql -u root -p < database/schema.sql
# 插入预设数据（admin 账户 + 系统预设技能）
mysql -u root -p ai_chat_db < database/seed.sql
```

### 3. 配置

```bash
cp trpc_go.yaml.example trpc_go.yaml
```

编辑 `trpc_go.yaml`，填写关键配置：

```yaml
custom:
  model:
    openai_api_key: "YOUR_API_KEY"        # ⚠️ 必填：AI 模型 API Key
    openai_base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1"
  server:
    http_port: 8080                        # HTTP 服务端口
    mcp_port: 8001                         # MCP 服务端口
  rag:
    enabled: true                          # 启用 RAG 知识库
```

> 完整配置说明见 [docs/configuration.md](docs/configuration.md)

### 4. 启动

```bash
# 方式一：脚本启动（推荐）
./scripts/start.sh          # 生产模式
./scripts/start.sh --dev    # 开发模式

# 方式二：直接运行
go run main.go

# 方式三：Docker 一键部署（Nginx + 应用 + MySQL）
cp .env.example .env
docker-compose up -d
```

访问 [http://localhost:8080](http://localhost:8080) 🎉

> 默认管理员账户：`admin` / `admin123`（**请及时修改密码**）

### 运维脚本

```bash
./scripts/start.sh          # 启动（--dev 开发模式 / --prod 生产模式）
./scripts/stop.sh           # 停止（--force 强制终止）
./scripts/restart.sh        # 重启（透传所有参数）
```

> 详细部署方案（Docker、Systemd、Nginx 反代等）见 [docs/deployment.md](docs/deployment.md)

---

## 🛠️ 技术栈

### 后端


| 技术             | 用途       |
| ---------------- | ---------- |
| **Go 1.25+**     | 主语言     |
| **TRPC-GO**      | 服务框架   |
| **MySQL 8.0**    | 数据持久化 |
| **JWT**          | 用户认证（不透明随机 Token，服务端持久化存储） |
| **SSE**          | 流式推送   |
| **DDD 分层架构** | 代码组织   |

### 前端


| 技术             | 用途                        |
| ---------------- | --------------------------- |
| **Vue 3**        | UI 框架（CDN 引入，零构建） |
| **Element Plus** | UI 组件库                   |
| **marked.js**    | Markdown 渲染               |
| **highlight.js** | 代码高亮                    |
| **SSE**          | 流式接收                    |

### AI & 协议


| 技术                | 用途                     |
| ------------------- | ------------------------ |
| **OpenAI 兼容接口** | 云端模型（DashScope 等） |
| **Ollama**          | 本地模型推理             |
| **A2A 协议**        | Agent 间通信             |
| **MCP 协议**        | 模型上下文协议           |
| **ReAct 循环**      | 工具调用决策             |
| **RAG**             | 检索增强生成             |

---

## 📁 项目结构

```
aiProject/
├── main.go                              # 程序入口
├── trpc_go.yaml.example                 # 配置文件模板
├── frontend/                            # 🖥️ 前端（Vue 3 CDN，零构建）
│   ├── index.html                       # 主聊天界面
│   ├── login.html                       # 登录/注册页面
│   ├── knowledge.html                   # 知识库管理页面
│   ├── workflow.html                    # 工作流可视化编辑器
│   ├── style.css                        # 全局样式（含暗色主题）
│   └── js/                              # 模块化 JS（10 个模块）
├── internal/                            # ⚙️ 后端核心（DDD 分层）
│   ├── bootstrap/                       # 启动编排（路由、中间件、Agent 注册）
│   ├── application/                     # 应用层（ChatService、WorkflowEngine、A2A 等）
│   ├── domain/                          # 领域层（实体、聚合根、仓储接口）
│   │   ├── workflow/                    # 工作流领域（Workflow 聚合根、DAG 拓扑排序）
│   │   ├── session/                     # 会话领域
│   │   ├── knowledge/                   # 知识库领域
│   │   └── ...                          # 其他领域
│   ├── infrastructure/                  # 基础设施层（MySQL、OpenAI、Ollama、向量化）
│   ├── interfaces/                      # 接口层（HTTP Handler、MCP Server）
│   ├── config/                          # 配置管理
│   └── shared/                          # 共享工具（日志等）
├── skills/                              # 🎯 内置 Skill 技能（10 个）
│   ├── weather/                         # 天气查询
│   ├── calculate/                       # 数学计算
│   ├── execute-command/                 # 命令执行
│   ├── http-request/                    # HTTP 请求
│   ├── mysql-query/                     # MySQL 查询
│   ├── write-file/                      # 文件写入
│   ├── file-explorer/                   # 文件浏览
│   ├── current-time/                    # 当前时间
│   ├── ip-lookup/                       # IP 查询
│   └── skill-creator/                   # Skill 生成器（元技能）
├── nginx/                               # 🌐 Nginx 配置（前端 + 反向代理）
├── scripts/                             # 🔧 运维脚本（启动/停止/重启）
├── database/                            # 🗄️ 数据库脚本（schema + seed）
├── Dockerfile                           # 🐳 Docker 多阶段构建
├── docker-compose.yml                   # 🐳 Docker Compose 编排
└── docs/                                # 📚 详细文档（9 篇）
```

> 完整目录结构见 [docs/architecture.md](docs/architecture.md)

---

## 📚 详细文档

> 为保持主 README 精简，详细文档已拆分到 `docs/` 目录：


| 文档                                                | 说明                                                                               |
| --------------------------------------------------- | ---------------------------------------------------------------------------------- |
| [🏗️ 架构设计详解](docs/architecture.md)           | DDD 分层架构、数据库 ER 图、组件依赖关系、中间件链、安全机制                       |
| [🔄 核心流程图集](docs/flowcharts.md)               | 聊天时序图、多 Agent 编排、RAG 处理流程、Workflow 执行流程、A2A 任务流程、认证流程 |
| [📡 API 接口文档](docs/api-reference.md)            | 完整 REST API 参考，含请求/响应示例、SSE 事件类型、Workflow API                    |
| [🎯 Skill 开发指南](docs/skill-guide.md)            | Skill 目录结构、SKILL.md 格式、5 种设计模式、工具开发                              |
| [🤖 多 Agent 编排指南](docs/agent-orchestration.md) | Agent 注册、主/子 Agent 配置、call_agent 工具、动态工具管理                        |
| [⚙️ 配置说明](docs/configuration.md)              | 完整配置项说明、模型类型判断规则、环境变量                                         |
| [🚀 部署指南](docs/deployment.md)                   | 生产环境部署、Docker、Nginx 前端配置、安全加固                                     |
| [🖥️ 前端功能说明](docs/frontend-guide.md)         | 页面功能模块、模块化设计、主题系统                                                 |

---

## 🗺️ Roadmap

- [X]  多模型切换（云端 + 本地 Ollama）
- [X]  ReAct 工具调用循环
- [X]  多 Agent 编排（主/子 Agent 模式）
- [X]  RAG 知识库（文档上传 + 语义检索）
- [X]  A2A 协议支持
- [X]  MCP 协议支持
- [X]  多用户权限系统
- [X]  暗色主题
- [X]  Prompt 模板变量系统（三级优先级 + 数据库持久化）
- [X]  可视化 Workflow / DAG 编排（7 种节点类型 + SSE 流式执行）
- [ ]  对话导出（Markdown / PDF）
- [ ]  插件市场（Skill 在线安装）
- [ ]  多模态支持（图片理解 / 生成）
- [ ]  WebSocket 替代 SSE
- [ ]  更多向量数据库支持（Milvus / Qdrant）
- [ ]  长期记忆 / Memory 系统

---

## 🤝 Contributing

欢迎贡献代码！请遵循以下步骤：

1. Fork 本仓库
2. 创建特性分支：`git checkout -b feat/amazing-feature`
3. 提交更改：`git commit -m 'feat(scope): add amazing feature'`
4. 推送分支：`git push origin feat/amazing-feature`
5. 提交 Pull Request

> 💡 Commit Message 请遵循 [Conventional Commits](https://www.conventionalcommits.org/) 规范。

---

## 📄 许可证

[MIT License](LICENSE)
