# 智能小助手

一个基于 Model Context Protocol (MCP) 的本地AI模型接入系统，提供前端问答界面。

## 功能特性

- 🤖 基于MCP协议接入本地AI模型
- 🎨 现代化前端聊天界面
- 🔌 支持RESTful API接口
- ⚡ 高性能Go语言后端
- 📱 响应式设计，支持移动端
- 🔧 可配置化设置

## 技术栈

### 后端
- Go 1.24+
- TRPC-GO框架
- MCP协议实现
- HTTP服务器

### 前端
- HTML5 + CSS3
- 原生JavaScript
- 响应式设计

## 快速开始

### 前置要求

- Go 1.24 或更高版本
- Git

### 安装依赖

```bash
# 确保依赖已安装
go mod tidy
```

### 运行系统

```bash
# 直接运行
go run main.go

# 或者编译后运行
go build -o ai-chat
./ai-chat
```

### 访问系统

1. 打开浏览器访问: http://localhost:8080
2. 在输入框中输入问题
3. 查看AI模型的回复

## API接口

### 聊天接口

```http
POST /api/chat
Content-Type: application/x-www-form-urlencoded

message=你的问题
```

**响应示例:**
```json
{
  "response": "AI回复内容"
}
```

### MCP接口

系统同时提供MCP协议接口，端口: 8000

## 配置说明

编辑 `config.yaml` 文件来自定义系统配置：

```yaml
server:
  http_port: 8080    # HTTP服务端口
  trpc_port: 8000    # TRPC服务端口

model:
  name: "local-ai-model"  # 模型名称
  timeout: 5000           # 响应超时时间

mcp:
  enabled: true           # 是否启用MCP
```

## 项目结构

```
aiProject/
├── main.go          # 主程序入口
├── go.mod           # Go模块定义
├── go.sum           # 依赖校验
├── config.yaml      # 配置文件
├── README.md        # 说明文档
└── frontend/        # 前端文件
    ├── index.html   # 主页面
    ├── style.css    # 样式文件
    └── script.js    # 交互脚本
```

## 开发指南

### 扩展模型支持

要接入真实的本地模型，修改 `LocalModelClient` 类：

```go
func (c *LocalModelClient) GenerateResponse(ctx context.Context, prompt string) (string, error) {
    // 这里实现与真实模型的连接逻辑
    // 例如: 调用本地LLM API、使用 transformers 等
}
```

### 添加新功能

1. 在后端添加新的API端点
2. 在前端添加对应的界面元素
3. 更新配置文件（如需要）

### 部署生产环境

1. 编译二进制文件: `go build -ldflags="-s -w" -o ai-chat`
2. 使用系统服务管理工具（如 systemd）部署
3. 配置反向代理（如 Nginx）
4. 设置日志轮转和监控

## 故障排除

### 常见问题

1. **端口冲突**: 修改 config.yaml 中的端口配置
2. **依赖问题**: 运行 `go mod tidy` 清理依赖
3. **前端无法访问**: 检查 frontend 目录是否存在

### 日志查看

日志输出到控制台和文件（默认: ./logs/app.log）

## 许可证

MIT License

## 贡献

欢迎提交 Issue 和 Pull Request！

## 更新日志

### v1.0.0
- 初始版本发布
- 基础MCP协议支持
- 前端聊天界面
- RESTful API接口

## Ollama 集成

系统现已集成本地Ollama模型支持，可以使用您本地的AI模型进行问答。

### 前置要求

1. 安装并运行 [Ollama](https://ollama.com/)
2. 下载所需的模型，例如：
   ```bash
   ollama pull deepseek-r1:8b
   ```

### 配置模型

编辑 `main.go` 文件中的模型名称：

```go
modelClient := NewLocalModelClient("您的模型名称")
```

支持的模型：deepseek-r1:8b、llama3、mistral 等Ollama支持的模型

### 验证集成

```bash
# 测试API接口
curl -X POST http://localhost:8081/api/chat -d "message=你好"
```