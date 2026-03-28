# 🚀 部署指南

> 本文档介绍如何在生产环境中部署 AI Agent Platform。

## 目录

- [前置要求](#前置要求)
- [本地开发部署](#本地开发部署)
- [生产环境部署](#生产环境部署)
- [Docker 部署](#docker-部署)
- [Nginx 反向代理](#nginx-反向代理)
- [安全加固](#安全加固)
- [监控与运维](#监控与运维)

---

## 前置要求

| 依赖 | 版本 | 必需 | 说明 |
|------|------|------|------|
| Go | 1.24+ | ✅ | 后端编译运行 |
| MySQL | 8.0+ | ✅ | 数据持久化 |
| Python 3 | 3.8+ | ❌ | Skill 脚本工具执行 |
| Ollama | 最新版 | ❌ | 本地模型推理 |

---

## 本地开发部署

### 1. 克隆项目

```bash
git clone https://github.com/JayCoding0/ai-tool.git
cd ai-tool
go mod tidy
```

### 2. 初始化数据库

```bash
# 创建数据库和表结构
mysql -u root -p < database/schema.sql

# 插入预设数据（admin 账户 + 系统预设技能）
mysql -u root -p ai_chat_db < database/seed.sql
```

### 3. 配置

```bash
cp trpc_go.yaml.example trpc_go.yaml
# 编辑 trpc_go.yaml，填写 API Key 等必要配置
```

### 4. 启动

```bash
go run main.go
```

访问 http://localhost:8080 🎉

> 默认管理员账户：`admin` / `admin123`（**请及时修改密码**）

---

## 生产环境部署

### 1. 编译

```bash
# 编译为静态二进制
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ai-agent main.go
```

### 2. 目录结构

```
/opt/ai-agent/
├── ai-agent                   # 可执行文件
├── trpc_go.yaml              # 配置文件
├── frontend/                  # 前端静态文件
├── skills/                    # Skill 技能目录
├── database/                  # 数据库脚本
└── logs/                      # 日志目录
```

### 3. Systemd 服务

创建 `/etc/systemd/system/ai-agent.service`：

```ini
[Unit]
Description=AI Agent Platform
After=network.target mysql.service

[Service]
Type=simple
User=ai-agent
WorkingDirectory=/opt/ai-agent
ExecStart=/opt/ai-agent/ai-agent
Restart=always
RestartSec=5
LimitNOFILE=65536

# 环境变量
Environment=TRPC_CONFIG_PATH=/opt/ai-agent/trpc_go.yaml

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable ai-agent
sudo systemctl start ai-agent
```

---

## Docker 部署

### Dockerfile

```dockerfile
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o ai-agent main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates python3 py3-pip tzdata
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=builder /app/ai-agent .
COPY frontend/ ./frontend/
COPY skills/ ./skills/
COPY database/ ./database/

EXPOSE 8080 8001

CMD ["./ai-agent"]
```

### docker-compose.yml

```yaml
version: '3.8'

services:
  ai-agent:
    build: .
    ports:
      - "8080:8080"
      - "8001:8001"
    volumes:
      - ./trpc_go.yaml:/app/trpc_go.yaml
      - ./logs:/app/logs
    depends_on:
      - mysql
    restart: always

  mysql:
    image: mysql:8.0
    environment:
      MYSQL_ROOT_PASSWORD: your_password
      MYSQL_DATABASE: ai_chat_db
    ports:
      - "3306:3306"
    volumes:
      - mysql_data:/var/lib/mysql
      - ./database/schema.sql:/docker-entrypoint-initdb.d/01-schema.sql
      - ./database/seed.sql:/docker-entrypoint-initdb.d/02-seed.sql
    restart: always

volumes:
  mysql_data:
```

```bash
docker-compose up -d
```

---

## Nginx 反向代理

### 配置示例

```nginx
upstream ai_agent {
    server 127.0.0.1:8080;
}

server {
    listen 80;
    server_name your-domain.com;

    # 强制 HTTPS
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate     /etc/ssl/certs/your-domain.crt;
    ssl_certificate_key /etc/ssl/private/your-domain.key;

    # SSE 流式响应配置（关键！）
    proxy_buffering off;
    proxy_cache off;

    location / {
        proxy_pass http://ai_agent;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # SSE 支持
        proxy_set_header Connection '';
        proxy_http_version 1.1;
        chunked_transfer_encoding off;
        proxy_read_timeout 300s;
    }
}
```

> ⚠️ **重要**：必须关闭 `proxy_buffering`，否则 SSE 流式响应会被缓冲导致前端无法实时接收。

---

## 安全加固

### 生产环境检查清单

- [ ] 修改默认管理员密码
- [ ] 配置具体的 CORS 允许域名（不要用 `*`）
- [ ] 启用 IP 限流
- [ ] 使用 HTTPS
- [ ] MySQL 使用强密码，限制远程访问
- [ ] API Key 不要提交到代码仓库
- [ ] 日志级别设为 `info` 或 `warn`
- [ ] 限制 `execute_command` 工具的使用范围

### 安全机制一览

| 机制 | 说明 |
|------|------|
| JWT Token 认证 | Cookie + Header 双通道 |
| 密码哈希 | bcrypt 加密存储 |
| 脚本沙箱 | 路径白名单，防路径穿越 |
| 执行超时 | 单次脚本 30 秒超时 |
| 输出限制 | 脚本输出 512 KB 上限 |
| 进程隔离 | 进程组隔离，超时 kill 子进程树 |
| IP 限流 | 令牌桶算法 |
| CORS 控制 | 配置化 Origin 白名单 |
| 游客清理 | 定期清理游客会话 |

---

## 监控与运维

### 日志

日志输出到 `./logs/app.log`，支持 JSON 格式，可接入 ELK 等日志系统。

```bash
# 实时查看日志
tail -f logs/app.log

# 查看错误日志
grep "error" logs/app.log
```

### 健康检查

```bash
# 检查服务是否正常
curl http://localhost:8080/api/models
```

### 数据库备份

```bash
# 备份
mysqldump -u root -p ai_chat_db > backup_$(date +%Y%m%d).sql

# 恢复
mysql -u root -p ai_chat_db < backup_20240101.sql
```
