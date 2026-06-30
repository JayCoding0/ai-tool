# ⚙️ 配置说明

> 本文档详细说明所有配置项及其默认值。

## 配置文件

配置文件为 `trpc_go.yaml`，应用配置位于 `custom` 块下。

```bash
# 从模板创建配置文件
cp trpc_go.yaml.example trpc_go.yaml
```

### 配置文件查找顺序

1. 环境变量 `TRPC_CONFIG_PATH` 指定的路径
2. `./trpc_go.yaml`（当前目录）
3. `../trpc_go.yaml`（上级目录）
4. `../../trpc_go.yaml`（上上级目录）

---

## 完整配置参考

```yaml
custom:
  # ─── 服务器配置 ───────────────────────────────
  server:
    http_port: "8080"          # HTTP 服务端口
    mcp_port: "8001"           # MCP 服务端口
    host: "localhost"          # 服务主机地址

  # ─── 模型配置 ─────────────────────────────────
  model:
    name: "qwen-plus"          # 默认模型名称
    type: "openai"             # 模型类型：openai（云端）/ local（本地）
    timeout: 5000              # 请求超时（毫秒）
    max_context_length: 4096   # 最大上下文长度
    ollama_url: "http://localhost:11434"  # Ollama 地址
    openai_base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1"  # OpenAI 兼容接口地址
    openai_api_key: "YOUR_API_KEY"  # ⚠️ 必填：API Key
    available_models:          # 可用模型列表（前端切换用）
      - name: "qwen-plus"
        label: "通义千问 Plus"
        type: "cloud"
      - name: "qwen-max"
        label: "通义千问 Max"
        type: "cloud"
      - name: "qwen-turbo"
        label: "通义千问 Turbo"
        type: "cloud"

  # ─── MCP 配置 ──────────────────────────────────
  mcp:
    version: "1.0"             # MCP 协议版本
    enabled: true              # 是否启用 MCP Server
    service_name: "ai-chat-service"  # MCP 服务名称

  # ─── 日志配置 ──────────────────────────────────
  log:
    level: "info"              # 日志级别：debug / info / warn / error
    file_path: "./logs/app.log"  # 日志文件路径
    console: true              # 是否同时输出到控制台

  # ─── 数据库配置 ─────────────────────────────────
  database:
    mysql:
      host: "localhost"        # MySQL 主机
      port: 3306               # MySQL 端口
      username: "root"         # 用户名
      password: "123456"       # 密码
      database: "ai_chat_db"   # 数据库名
      max_idle_conns: 10       # 最大空闲连接数
      max_open_conns: 100      # 最大打开连接数
      conn_max_lifetime: 3600  # 连接最大生命周期（秒）

  # ─── 安全配置 ──────────────────────────────────
  security:
    allowed_origins:           # CORS 允许的来源
      - "http://localhost:8080"
      - "http://127.0.0.1:8080"
    rate_limit:
      enabled: true            # 是否启用 IP 限流
      requests_per_second: 10  # 每个 IP 每秒请求数
      burst: 30                # 令牌桶容量（突发上限）

  # ─── 工具配置 ──────────────────────────────────
  tools:
    baidu_ak: ""               # 百度地图 API Key（天气查询用）

  # ─── RAG 知识库配置 ─────────────────────────────
  rag:
    enabled: true              # 是否启用 RAG 功能
    embed_model: "text-embedding-3-small"  # Embedding 模型名称
```

---

## 配置项详解

### 模型配置

#### 模型类型判断规则

系统根据以下规则自动判断模型类型：

| 条件 | 类型 | 说明 |
|------|------|------|
| `type: "openai"` | 云端 | 强制使用 OpenAI 兼容接口 |
| `type: "local"` | 本地 | 强制使用 Ollama |
| 模型名包含 `:` | 本地 | 如 `qwen2.5:14b` → Ollama |
| 其他 | 云端 | 默认走 OpenAI 兼容接口 |

#### 支持的云端服务

| 服务 | `openai_base_url` |
|------|-------------------|
| 阿里云 DashScope | `https://dashscope.aliyuncs.com/compatible-mode/v1` |
| DeepSeek 官方 API | `https://api.deepseek.com` |
| OpenAI | `https://api.openai.com/v1` |
| 其他 OpenAI 兼容接口 | 对应的 Base URL |

#### 按模型独立凭证（多厂商混用）

全局的 `openai_base_url` / `openai_api_key` 对所有云端模型生效。如果某个模型来自**不同厂商**（如同时使用 DashScope 的通义千问和 DeepSeek 官方 API），可在 `available_models` 中为该模型单独指定 `base_url` 和 `api_key`，**覆盖全局配置**；未指定时回退到全局值。

```yaml
available_models:
  - name: "qwen-plus"          # 使用全局 DashScope 配置
    label: "通义千问 Plus"
    type: "cloud"
  - name: "deepseek-v4-pro"    # 使用 DeepSeek 官方 API（独立凭证）
    label: "DeepSeek V4 Pro"
    type: "cloud"
    base_url: "https://api.deepseek.com"   # 覆盖全局 openai_base_url
    api_key: "YOUR_DEEPSEEK_API_KEY"       # 覆盖全局 openai_api_key
    thinking: true                          # 开启思考模式（见下）
    reasoning_effort: "high"                # 思考强度：low / medium / high
```

| 字段 | 说明 |
|------|------|
| `base_url` | 该模型专属的 OpenAI 兼容地址，留空回退全局 `openai_base_url` |
| `api_key` | 该模型专属的 API Key，留空回退全局 `openai_api_key` |
| `thinking` | 是否开启思考模式（DeepSeek V4 等），开启后请求注入专有字段 `thinking: {"type":"enabled"}` |
| `reasoning_effort` | 思考强度 `low` / `medium` / `high`，仅在 `thinking: true` 时生效，留空则不传 |

> 思考过程通过 SSE 的 `thinking` 事件流式返回（解析模型响应中的 `reasoning_content` 字段）。思考模式仅对配置了该字段的模型生效，不影响其他模型。

#### 本地模型（Ollama）

本地模型列表从 Ollama 实时拉取，无需在 `available_models` 中配置。确保 Ollama 服务已启动：

```bash
# 启动 Ollama
ollama serve

# 拉取模型
ollama pull qwen2.5:14b
```

### 安全配置

#### CORS

- 生产环境请配置具体的域名，不要使用 `*`
- 支持多个 Origin

#### 限流

- 基于 IP 的令牌桶算法
- `requests_per_second`：令牌生成速率
- `burst`：令牌桶容量，允许的突发请求数

### RAG 配置

- `enabled: false` 可完全关闭 RAG 功能
- `embed_model` 需要与 `openai_api_key` 对应的服务支持的 Embedding 模型一致

---

## 环境变量

| 环境变量 | 说明 |
|---------|------|
| `TRPC_CONFIG_PATH` | 指定配置文件路径 |

---

## 最小化配置

只需配置 API Key 即可启动：

```yaml
custom:
  model:
    openai_api_key: "YOUR_API_KEY"
    openai_base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1"
```

其他所有配置项均有合理的默认值。
