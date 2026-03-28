# ============================================================
# AI Agent Platform - Dockerfile
# 多阶段构建：编译阶段 + 运行阶段
# ============================================================

# ─── 阶段 1：编译 ───────────────────────────────────────────
FROM golang:1.24-alpine AS builder

# 安装编译依赖
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# 先复制依赖文件，利用 Docker 缓存层
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并编译
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o ai-agent main.go

# ─── 阶段 2：运行 ───────────────────────────────────────────
FROM alpine:3.19

LABEL maintainer="AI Agent Platform"
LABEL description="AI Agent Platform - 多模型多Agent智能体平台"

# 安装运行时依赖
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    python3 \
    py3-pip \
    curl \
    && ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

# 创建非 root 用户
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

# 从编译阶段复制二进制
COPY --from=builder /app/ai-agent .

# 复制前端、技能、数据库脚本
COPY frontend/ ./frontend/
COPY skills/ ./skills/
COPY database/ ./database/

# 创建日志目录
RUN mkdir -p /app/logs && chown -R appuser:appgroup /app

# 切换到非 root 用户
USER appuser

# 暴露端口
EXPOSE 8080 8001

# 健康检查
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/api/models || exit 1

# 启动
ENTRYPOINT ["./ai-agent"]
