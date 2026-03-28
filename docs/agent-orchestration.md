# 🤖 多 Agent 编排指南

> 本文档介绍如何配置和使用多 Agent 编排系统，实现复杂任务的协同处理。

## 目录

- [架构概述](#架构概述)
- [Agent 注册中心](#agent-注册中心)
- [主 Agent 与子 Agent](#主-agent-与子-agent)
- [call_agent 工具](#call_agent-工具)
- [动态工具管理](#动态工具管理)
- [事件透传机制](#事件透传机制)
- [自定义 Agent](#自定义-agent)

---

## 架构概述

多 Agent 编排采用 **主/子 Agent 模式**：

- **主 Agent（Master）**：负责理解用户意图、拆解任务、调度子 Agent
- **子 Agent（Sub）**：专注于特定领域任务，拥有独立的工具集和 System Prompt

```
用户请求 → 主 Agent 分析意图 → 调用子 Agent → 汇总结果 → 返回用户
```

每个 Agent 拥有独立的 `ChatService` 实例，确保上下文隔离。

---

## Agent 注册中心

`AgentRegistry` 是全局单例，管理所有 Agent 实例的生命周期。

### 核心 API

| 方法 | 说明 |
|------|------|
| `Register(def, chatService)` | 注册一个 Agent |
| `Get(name)` | 按名称获取 Agent |
| `GetMaster()` | 获取主 Agent |
| `ListSubAgents()` | 列出所有子 Agent |
| `ListAll()` | 列出所有 Agent |
| `UpdateTools(name, tools)` | 热更新 Agent 工具列表 |
| `CallSubAgent(name, msg, sid, cb)` | 调用子 Agent 执行任务 |

### AgentDefinition 结构

```go
type AgentDefinition struct {
    Name         string   // Agent 唯一名称（英文）
    DisplayName  string   // 展示名称（中文）
    Description  string   // 职责描述
    SystemPrompt string   // System Prompt
    EnabledTools []string // 可用工具列表
    ModelName    string   // 使用的模型（空=默认）
    IsMaster     bool     // 是否为主 Agent
}
```

---

## 主 Agent 与子 Agent

### 预置 Agent 配置

系统预置了 4 个 Agent：

| Agent | 名称 | 角色 | 工具 |
|-------|------|------|------|
| `master_agent` | 主调度 Agent | 理解意图、拆解任务、编排子 Agent | `call_agent` |
| `weather_agent` | 天气查询 Agent | 天气相关查询 | `get_weather`, `get_public_ip` |
| `search_agent` | 搜索 Agent | 信息搜索、景点推荐 | `http_request` |
| `code_agent` | 代码工具 Agent | 计算、文件、命令、数据库 | `calculate`, `write_file`, `execute_command`, `mysql_query` |

### 主 Agent 调度规则

主 Agent 的 System Prompt 中定义了强制调度规则：

- 天气相关 → 必须调用 `weather_agent`
- 搜索/推荐 → 必须调用 `search_agent`
- 代码/计算/文件/数据库 → 必须调用 `code_agent`
- 纯问候/闲聊 → 直接回复

---

## call_agent 工具

`call_agent` 是主 Agent 专用的编排工具，用于调用子 Agent。

### 工具参数

```json
{
  "agent_name": "weather_agent",
  "message": "查询北京今天的天气"
}
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `agent_name` | string | 子 Agent 名称 |
| `message` | string | 传递给子 Agent 的消息 |

### 执行流程

1. 主 Agent 决定调用 `call_agent`
2. `AgentRegistry.CallSubAgent()` 被触发
3. 子 Agent 使用独立的 `ChatService` 处理消息
4. 子 Agent 可以调用自己的工具（ReAct 循环）
5. 子 Agent 的思考/工具调用事件实时透传给前端
6. 子 Agent 返回文本结果给主 Agent
7. 主 Agent 汇总所有子 Agent 结果后回复用户

---

## 动态工具管理

支持在运行时动态修改 Agent 的工具列表，无需重启服务。

### API 接口

```
PUT /api/agents/{agent_name}/tools
```

```json
{
  "tools": ["get_weather", "http_request", "calculate"]
}
```

### 特性

- **热更新**：修改立即生效，无需重启
- **自动持久化**：工具配置保存到 `agent_tools` 数据库表
- **启动恢复**：服务重启时自动从数据库恢复配置
- **描述刷新**：更新子 Agent 工具后，`call_agent` 的描述自动刷新，确保主 Agent 感知最新能力

### 前端管理

在聊天界面的工具抽屉中，可以可视化管理每个 Agent 的工具配置。

---

## 事件透传机制

子 Agent 执行过程中的所有中间事件（思考、工具调用、工具结果）都会实时透传给前端。

### 透传事件类型

| 事件 | 说明 |
|------|------|
| `thought` | 子 Agent 的思考过程 |
| `tool_call` | 子 Agent 调用工具 |
| `tool_result` | 子 Agent 工具执行结果 |

### 事件归属标识

每个透传事件都携带 `parent_tool_call_id` 字段，标识该事件属于哪次 `call_agent` 调用，前端据此进行层级展示。

```json
{
  "type": "tool_call",
  "tool_name": "get_weather",
  "tool_call_id": "call_sub_001",
  "parent_tool_call_id": "call_master_001",
  "step": 1
}
```

---

## 自定义 Agent

### 添加新的子 Agent

在 `internal/bootstrap/bootstrap.go` 的 `registerAgents` 函数中添加：

```go
registry.Register(application.AgentDefinition{
    Name:        "my_agent",
    DisplayName: "我的自定义 Agent",
    Description: "负责处理特定领域的任务",
    SystemPrompt: "你是一个专业的 XXX 助手...",
    EnabledTools: []string{"tool_a", "tool_b"},
    IsMaster:     false,
}, newChatSvc())
```

### 注意事项

1. Agent 名称必须唯一（英文，snake_case）
2. 子 Agent 的 `IsMaster` 必须为 `false`
3. 添加新子 Agent 后，`call_agent` 工具描述会自动更新
4. 主 Agent 的 System Prompt 需要同步更新调度规则
5. 确保子 Agent 使用的工具已在 `skills/` 目录中注册
