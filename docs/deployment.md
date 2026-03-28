# 🚀 部署指南

> 本文档介绍如何在各种环境中部署 AI Agent Platform。

## 目录

- [前置要求](#前置要求)
- [快速启动脚本](#快速启动脚本)
- [本地开发部署](#本地开发部署)
- [生产环境部署](#生产环境部署)
- [Docker 容器化部署](#docker-容器化部署)
- [Nginx 前端配置](#nginx-前端配置)
- [安全加固](#安全加固)
- [监控与运维](#监控与运维)

---

## 前置要求

| 依赖 | 版本 | 必需 | 说明 |
|------|------|------|------|
| Go | 1.24+ | ✅ | 后端编译运行 |
| MySQL | 8.0+ | ✅ | 数据持久化 |
| Python 3 | 3.8+ | ❌ | Skill 脚本工具执行 |
| Docker | 20.10+ | ❌ | 容器化部署 |
| Docker Compose | 2.0+ | ❌ | 容器编排 |
| Ollama | 最新版 | ❌ | 本地模型推理 |

---

## 快速启动脚本

项目提供了 3 个运维脚本，位于 `scripts/` 目录：

```
scripts/
├── start.sh      # 启动脚本
├── stop.sh       # 停止脚本
└── restart.sh    # 重启脚本
```

### 启动服务

```bash
# 生产模式（默认）：自动编译 + 后台运行
./scripts/start.sh

# 开发模式：go run 运行
./scripts/start.sh --dev

# 指定端口
./scripts/start.sh --port 9090
```

**启动脚本功能：**
- ✅ 自动检测端口占用
- ✅ 自动检测配置文件是否存在
- ✅ 防止重复启动（PID 文件管理）
- ✅ 生产模式自动编译（增量编译，源码未变不重新编译）
- ✅ 启动后自动健康检查（最多等待 15 秒）
- ✅ 启动失败自动输出错误日志

### 停止服务

```bash
# 优雅停止（SIGTERM，等待 10 秒后强制终止）
./scripts/stop.sh

# 强制终止（SIGKILL）
./scripts/stop.sh --force
```

**停止脚本功能：**
- ✅ 优雅停止（先 SIGTERM，超时后 SIGKILL）
- ✅ 自动清理残留进程
- ✅ 自动清理 PID 文件

### 重启服务

```bash
# 重启（透传所有参数给 start.sh）
./scripts/restart.sh

# 重启为开发模式
./scripts/restart.sh --dev
```

### 脚本参数一览

| 脚本 | 参数 | 说明 |
|------|------|------|
| `start.sh` | `--dev` | 开发模式（go run） |
| `start.sh` | `--prod` | 生产模式（编译运行，默认） |
| `start.sh` | `--port PORT` | 指定 HTTP 端口 |
| `stop.sh` | `--force` / `-f` | 强制终止 |
| `restart.sh` | 同 `start.sh` | 透传给 start.sh |

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
# 方式一：使用脚本（推荐）
./scripts/start.sh --dev

# 方式二：直接运行
go run main.go
```

访问 http://localhost:8080 🎉

> 默认管理员账户：`admin` / `admin123`（**请及时修改密码**）

---

## 生产环境部署

### 1. 编译

```bash
# 方式一：使用启动脚本自动编译
./scripts/start.sh --prod

# 方式二：手动编译
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o output/ai-agent main.go
```

### 2. 目录结构

```
/opt/ai-agent/
├── ai-agent                   # 可执行文件（或 output/ai-agent）
├── trpc_go.yaml              # 配置文件
├── frontend/                  # 前端静态文件
├── skills/                    # Skill 技能目录
├── scripts/                   # 运维脚本
├── database/                  # 数据库脚本
└── logs/                      # 日志目录（自动创建）
```

### 3. 使用脚本管理

```bash
# 启动
./scripts/start.sh

# 查看状态
cat logs/ai-agent.pid

# 停止
./scripts/stop.sh

# 重启
./scripts/restart.sh
```

### 4. Systemd 服务（可选）

如果需要开机自启，创建 `/etc/systemd/system/ai-agent.service`：

```ini
[Unit]
Description=AI Agent Platform
After=network.target mysql.service

[Service]
Type=simple
User=ai-agent
WorkingDirectory=/opt/ai-agent
ExecStart=/opt/ai-agent/output/ai-agent
ExecStop=/opt/ai-agent/scripts/stop.sh
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

## Docker 容器化部署

### 快速开始（推荐）

只需 3 步即可通过 Docker 启动完整服务（Nginx + 应用 + MySQL）：

```bash
# 1. 准备配置文件
cp trpc_go.yaml.example trpc_go.yaml
cp .env.example .env
# 编辑 trpc_go.yaml 填写 API Key
# 编辑 .env 修改密码等配置（可选）
```

```bash
# 2. 启动所有服务
docker-compose up -d
```

```bash
# 3. 查看状态
docker-compose ps
```

访问 http://localhost 🎉（默认 80 端口，通过 Nginx 代理）

> ⚠️ **注意**：`trpc_go.yaml` 中的 MySQL 连接地址需要改为 Docker 内部网络地址：
> ```yaml
> # 将 localhost 改为 mysql（Docker 服务名）
> target: dsn://root:ai_agent_2024@tcp(mysql:3306)/ai_chat_db?charset=utf8mb4&parseTime=true&loc=Local&timeout=5s
> ```

### 环境变量配置

通过 `.env` 文件配置 Docker Compose 参数：

```bash
# .env
HTTP_PORT=80                # Nginx 对外端口（浏览器访问）
MCP_PORT=8001               # MCP 服务端口（直连后端）
MYSQL_PORT=3306             # MySQL 端口映射
MYSQL_ROOT_PASSWORD=ai_agent_2024  # MySQL root 密码
# HTTPS_PORT=443            # HTTPS 端口（生产环境取消注释）
```

### 常用命令

```bash
# 启动所有服务（后台运行）
docker-compose up -d

# 查看服务状态
docker-compose ps

# 查看应用日志
docker-compose logs -f ai-agent

# 查看 Nginx 日志
docker-compose logs -f nginx

# 查看 MySQL 日志
docker-compose logs -f mysql

# 停止所有服务
docker-compose down

# 停止并删除数据卷（⚠️ 会清除数据库数据）
docker-compose down -v

# 重新构建并启动
docker-compose up -d --build

# 仅重启应用（不重启 MySQL）
docker-compose restart ai-agent

# 进入应用容器
docker-compose exec ai-agent sh

# 进入 MySQL 容器
docker-compose exec mysql mysql -u root -p ai_chat_db
```

### 架构说明

```
                    ┌─────────────────────────────────────────────────────────┐
                    │                    Docker Network                       │
                    │                                                         │
  浏览器 ──:80──▶  │  ┌─────────┐    ┌──────────────┐    ┌──────────────┐   │
                    │  │  nginx  │───▶│  ai-agent    │───▶│  mysql       │   │
                    │  │  :80    │    │  :8080       │    │  :3306       │   │
  MCP ────:8001──▶  │  └─────────┘    │  :8001 MCP   │    └──────────────┘   │
                    │       │         └──────────────┘           │            │
                    └───────┼────────────────┼──────────────────┼────────────┘
                            │                │                  │
                       ┌────┴────┐      ┌────┴────┐       ┌────┴─────┐
                       │ volumes │      │ volumes │       │  volumes │
                       │ frontend│      │ logs/   │       │ mysql_data│
                       │ nginx.  │      │ skills/ │       └──────────┘
                       │  conf   │      │ config  │
                       └─────────┘      └─────────┘
```

### 数据持久化

| 服务 | 挂载项 | 容器路径 | 说明 |
|------|--------|---------|------|
| nginx | `./nginx/nginx.conf` | `/etc/nginx/conf.d/default.conf` | Nginx 配置（只读） |
| nginx | `./frontend` | `/usr/share/nginx/html` | 前端静态文件（只读） |
| ai-agent | `./trpc_go.yaml` | `/app/trpc_go.yaml` | 应用配置文件（只读） |
| ai-agent | `./logs` | `/app/logs` | 应用日志 |
| ai-agent | `./skills` | `/app/skills` | Skill 技能目录（支持热更新） |
| mysql | `mysql_data` | `/var/lib/mysql` | MySQL 数据（Docker Volume） |

### Dockerfile 特性

| 特性 | 说明 |
|------|------|
| **多阶段构建** | 编译阶段 + 运行阶段，最终镜像更小 |
| **非 root 用户** | 使用 `appuser` 运行，提升安全性 |
| **健康检查** | 内置 `HEALTHCHECK`，30 秒间隔检测 |
| **时区设置** | 默认 `Asia/Shanghai` |
| **Python 支持** | 预装 Python 3，支持 Skill 脚本执行 |
| **构建缓存** | 先复制 `go.mod`，利用 Docker 层缓存加速 |

### 仅构建镜像（不使用 Compose）

```bash
# 构建镜像
docker build -t ai-agent:latest .

# 运行容器（需要自行准备 MySQL）
docker run -d \
  --name ai-agent \
  -p 8080:8080 \
  -p 8001:8001 \
  -v $(pwd)/trpc_go.yaml:/app/trpc_go.yaml:ro \
  -v $(pwd)/logs:/app/logs \
  -v $(pwd)/skills:/app/skills \
  ai-agent:latest
```

---

## Nginx 前端配置

项目已内置完整的 Nginx 配置文件 `nginx/nginx.conf`，Docker Compose 部署时自动使用。

### 配置文件位置

```
nginx/
└── nginx.conf          # Nginx 配置（前端静态文件 + API 反向代理）
```

### 配置特性

| 特性 | 说明 |
|------|------|
| **前端静态文件服务** | 直接服务 `index.html`、`login.html`、`knowledge.html` |
| **API 反向代理** | `/api/*` → 后端 8080 端口 |
| **A2A 协议代理** | `/a2a/*`、`/.well-known/agent.json` → 后端 |
| **SSE 流式支持** | 关闭 `proxy_buffering`，确保实时推送 |
| **Gzip 压缩** | JS/CSS/JSON 等自动压缩，减少传输体积 |
| **缓存策略** | HTML 不缓存（即时更新），JS/CSS 缓存 7 天 |
| **安全头** | X-Frame-Options、X-Content-Type-Options 等 |
| **上传限制** | `client_max_body_size 50m`（知识库文档上传） |
| **HTTPS 模板** | 配置文件底部提供 HTTPS 配置模板，取消注释即可启用 |

### 裸机部署使用 Nginx

如果不使用 Docker，可以手动配置 Nginx：

```bash
# 复制配置文件
sudo cp nginx/nginx.conf /etc/nginx/conf.d/ai-agent.conf

# 修改 upstream 地址（裸机部署改为 127.0.0.1）
# upstream ai_agent_backend { server 127.0.0.1:8080; }

# 修改前端文件路径
# root /opt/ai-agent/frontend;

# 测试配置
sudo nginx -t

# 重载
sudo nginx -s reload
```

### HTTPS 配置

编辑 `nginx/nginx.conf`，取消底部 HTTPS 配置的注释，并配置 SSL 证书路径：

```nginx
server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate     /etc/ssl/certs/your-domain.crt;
    ssl_certificate_key /etc/ssl/private/your-domain.key;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    # ... 其余配置同 HTTP ...
}
```

Docker 部署时，取消 `docker-compose.yml` 中 HTTPS 端口和 SSL 证书挂载的注释：

```yaml
nginx:
  ports:
    - "443:443"           # 取消注释
  volumes:
    - ./nginx/ssl:/etc/ssl:ro  # 取消注释，放入证书文件
```

> ⚠️ **重要**：必须关闭 `proxy_buffering`，否则 SSE 流式响应会被缓冲导致前端无法实时接收。配置文件中已默认关闭。

---

## 安全加固

### 生产环境检查清单

- [ ] 修改默认管理员密码
- [ ] 修改 MySQL 默认密码
- [ ] 配置具体的 CORS 允许域名（不要用 `*`）
- [ ] 启用 IP 限流
- [ ] 使用 HTTPS
- [ ] MySQL 使用强密码，限制远程访问
- [ ] API Key 不要提交到代码仓库
- [ ] `.env` 文件不要提交到代码仓库
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
| Docker 非 root | 容器内使用非 root 用户运行 |

---

## 监控与运维

### 日志

日志输出到 `./logs/app.log`，支持 JSON 格式，可接入 ELK 等日志系统。

```bash
# 实时查看日志（裸机部署）
tail -f logs/app.log

# 实时查看日志（Docker 部署）
docker-compose logs -f ai-agent

# 查看错误日志
grep "error" logs/app.log
```

### 健康检查

```bash
# 检查服务是否正常
curl http://localhost:8080/api/models

# Docker 健康状态
docker-compose ps
```

### 数据库备份

```bash
# 裸机部署备份
mysqldump -u root -p ai_chat_db > backup_$(date +%Y%m%d).sql

# Docker 部署备份
docker-compose exec mysql mysqldump -u root -pai_agent_2024 ai_chat_db > backup_$(date +%Y%m%d).sql

# 恢复
mysql -u root -p ai_chat_db < backup_20240101.sql
```
