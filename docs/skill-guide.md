# 🎯 Skill 开发指南

> 本文档介绍如何开发自定义 Skill 技能，扩展 AI Agent 的能力。

## 目录

- [什么是 Skill](#什么是-skill)
- [Skill 目录结构](#skill-目录结构)
- [SKILL.md 格式规范](#skillmd-格式规范)
- [工具定义（tool.json）](#工具定义tooljson)
- [脚本开发（run.py）](#脚本开发runpy)
- [5 种设计模式](#5-种设计模式)
- [内置 Skill 一览](#内置-skill-一览)
- [开发流程](#开发流程)

---

## 什么是 Skill

Skill 是模块化、自包含的能力包，通过提供专业知识、工作流程和工具来扩展 AI Agent 的能力。每个 Skill 定义了：

- **AI 角色与行为**：通过 System Prompt 定义 AI 在该技能下的角色
- **可用工具**：AI 可以调用的 Function Calling 工具
- **执行脚本**：工具的具体实现逻辑

---

## Skill 目录结构

```
skills/
├── weather/                    # 技能名称（kebab-case）
│   ├── SKILL.md               # 技能定义文件（必需）
│   └── scripts/               # 脚本目录
│       ├── tool.json          # 工具定义（Function Calling schema）
│       └── run.py             # 工具执行脚本
├── calculate/
│   ├── SKILL.md
│   └── scripts/
│       ├── tool.json
│       └── run.py
└── ...
```

---

## SKILL.md 格式规范

每个 SKILL.md 包含 YAML Frontmatter 和 Markdown 正文两部分：

```markdown
---
name: 实时天气助手
description: 查询指定城市的实时天气信息
icon: 🌤️
pattern: tool-wrapper
tools:
  - get_weather
version: "1.0"
---

你是一个实时天气助手，可以帮助用户查询城市的当前天气情况。

## 能力

你可以使用 `get_weather` 工具查询指定城市的实时天气。

## 使用规则

### get_weather 调用时机
- 用户询问天气相关问题时，**必须调用** `get_weather`

### 禁止行为
- ❌ 不要凭空猜测天气，必须调用工具获取真实数据

## 输出规范

- 用清晰易读的格式展示天气信息
- 包含温度、天气状况、风力风向等关键信息
```

### Frontmatter 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | ✅ | 技能名称（中文） |
| `description` | string | ✅ | 技能描述，用于 AI 判断何时使用 |
| `icon` | string | ❌ | 图标 emoji |
| `pattern` | string | ✅ | 设计模式（见下文） |
| `tools` | string[] | ❌ | 该技能使用的工具名称列表 |
| `version` | string | ❌ | 版本号 |

---

## 工具定义（tool.json）

工具定义遵循 OpenAI Function Calling 的 JSON Schema 格式：

```json
{
  "name": "get_weather",
  "display_name": "天气查询",
  "description": "查询指定城市的实时天气信息",
  "parameters": {
    "type": "object",
    "properties": {
      "district_id": {
        "type": "string",
        "description": "百度地图行政区划 ID，默认 610402"
      }
    },
    "required": []
  }
}
```

### 字段说明

| 字段 | 说明 |
|------|------|
| `name` | 工具唯一标识（英文，snake_case） |
| `display_name` | 展示名称（中文） |
| `description` | 工具功能描述，AI 根据此判断是否调用 |
| `parameters` | 参数 JSON Schema，定义工具接受的输入参数 |

---

## 脚本开发（run.py）

工具的执行逻辑通过 Python 脚本实现。系统会将 AI 传入的参数作为 JSON 通过 stdin 传递给脚本。

### 基本模板

```python
#!/usr/bin/env python3
"""工具执行脚本模板"""
import sys
import json

def main():
    # 从 stdin 读取 AI 传入的参数
    input_data = json.loads(sys.stdin.read())
    
    # 提取参数
    param = input_data.get("param_name", "default_value")
    
    # 执行业务逻辑
    result = do_something(param)
    
    # 输出结果（stdout 会被返回给 AI）
    print(json.dumps(result, ensure_ascii=False))

if __name__ == "__main__":
    main()
```

### 安全限制

| 限制项 | 值 |
|--------|-----|
| 执行超时 | 30 秒 |
| 输出大小限制 | 512 KB |
| 路径白名单 | 仅允许 `skills/` 目录下的脚本 |
| 进程隔离 | 进程组隔离，超时后 kill 整个子进程树 |

---

## 5 种设计模式

### 1. tool-wrapper（工具包装器）

最常用的模式。AI 根据用户意图决定是否调用工具。

**适用场景：** 天气查询、数学计算、HTTP 请求等

```yaml
pattern: tool-wrapper
tools:
  - get_weather
```

### 2. autonomous（自主执行）

AI 自主决策执行多步骤任务，无需用户逐步确认。

**适用场景：** 代码生成、文件操作、批量处理

```yaml
pattern: autonomous
tools:
  - write_file
  - execute_command
```

### 3. conversational（对话式）

纯对话模式，不使用工具，通过 System Prompt 定义专业角色。

**适用场景：** 翻译助手、写作助手、角色扮演

```yaml
pattern: conversational
tools: []
```

### 4. pipeline（流水线）

按固定顺序执行多个工具，形成处理管线。

**适用场景：** 数据处理、ETL 流程

```yaml
pattern: pipeline
tools:
  - fetch_data
  - process_data
  - save_result
```

### 5. reactive（响应式）

根据外部事件或条件触发不同的工具调用。

**适用场景：** 监控告警、条件触发

```yaml
pattern: reactive
tools:
  - check_status
  - send_alert
```

---

## 内置 Skill 一览

| Skill | 图标 | 模式 | 工具 | 说明 |
|-------|------|------|------|------|
| **weather** | 🌤️ | tool-wrapper | `get_weather` | 查询城市实时天气 |
| **calculate** | 🧮 | tool-wrapper | `calculate` | 数学表达式计算 |
| **current-time** | 🕐 | tool-wrapper | `get_current_time` | 获取当前时间 |
| **http-request** | 🌐 | tool-wrapper | `http_request` | 发起 HTTP 请求 |
| **execute-command** | ⚡ | autonomous | `execute_command` | 执行 Shell 命令 |
| **file-explorer** | 📁 | tool-wrapper | `list_directory` | 浏览文件目录 |
| **write-file** | ✏️ | autonomous | `write_file` | 写入文件内容 |
| **mysql-query** | 🗄️ | tool-wrapper | `mysql_query` | 执行 MySQL 查询 |
| **ip-lookup** | 🔍 | tool-wrapper | `get_public_ip` | 查询公网 IP |
| **skill-creator** | 🛠️ | autonomous | `init_skill` 等 | 创建新 Skill 的元技能 |

---

## 开发流程

### 1. 创建目录结构

```bash
mkdir -p skills/my-skill/scripts
```

### 2. 编写 SKILL.md

定义技能的角色、能力、使用规则和输出规范。

### 3. 编写 tool.json

定义工具的名称、描述和参数 Schema。

### 4. 编写 run.py

实现工具的执行逻辑。

### 5. 重启服务

工具会在启动时从 `skills/*/scripts/` 目录自动加载注册。

```bash
go run main.go
```

### 6. 测试

在聊天界面中测试工具是否被正确调用和执行。

> **提示：** 可以通过 `GET /api/tools` 接口确认工具是否已成功注册。
