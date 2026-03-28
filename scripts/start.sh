#!/bin/bash
# ============================================================
# AI Agent Platform - 启动脚本
# 用法: ./scripts/start.sh [--dev|--prod] [--port PORT]
# ============================================================

set -e

# ─── 颜色定义 ───────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ─── 默认配置 ───────────────────────────────────────────────
APP_NAME="ai-agent"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PID_FILE="$PROJECT_DIR/logs/${APP_NAME}.pid"
LOG_DIR="$PROJECT_DIR/logs"
MODE="prod"
PORT=""

# ─── 参数解析 ───────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case $1 in
        --dev)
            MODE="dev"
            shift
            ;;
        --prod)
            MODE="prod"
            shift
            ;;
        --port)
            PORT="$2"
            shift 2
            ;;
        -h|--help)
            echo "用法: $0 [选项]"
            echo ""
            echo "选项:"
            echo "  --dev       开发模式（go run，支持热重载）"
            echo "  --prod      生产模式（编译后运行，默认）"
            echo "  --port PORT 指定 HTTP 端口（覆盖配置文件）"
            echo "  -h, --help  显示帮助信息"
            exit 0
            ;;
        *)
            echo -e "${RED}未知参数: $1${NC}"
            exit 1
            ;;
    esac
done

# ─── 工具函数 ───────────────────────────────────────────────
log_info()  { echo -e "${GREEN}[INFO]${NC}  $1"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# ─── 前置检查 ───────────────────────────────────────────────
check_prerequisites() {
    # 检查是否已在运行
    if [ -f "$PID_FILE" ]; then
        OLD_PID=$(cat "$PID_FILE")
        if kill -0 "$OLD_PID" 2>/dev/null; then
            log_error "服务已在运行中 (PID: $OLD_PID)"
            log_info "如需重启，请执行: ./scripts/restart.sh"
            exit 1
        else
            log_warn "发现残留 PID 文件，清理中..."
            rm -f "$PID_FILE"
        fi
    fi

    # 检查端口占用
    local check_port="${PORT:-8080}"
    if lsof -i :"$check_port" -sTCP:LISTEN >/dev/null 2>&1; then
        log_error "端口 $check_port 已被占用"
        lsof -i :"$check_port" -sTCP:LISTEN
        exit 1
    fi

    # 检查配置文件
    if [ ! -f "$PROJECT_DIR/trpc_go.yaml" ]; then
        log_error "配置文件不存在: trpc_go.yaml"
        log_info "请先执行: cp trpc_go.yaml.example trpc_go.yaml 并填写配置"
        exit 1
    fi
}

# ─── 创建日志目录 ───────────────────────────────────────────
mkdir -p "$LOG_DIR"

# ─── 主流程 ─────────────────────────────────────────────────
check_prerequisites

echo -e "${BLUE}╔══════════════════════════════════════╗${NC}"
echo -e "${BLUE}║   🤖 AI Agent Platform - 启动中     ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════╝${NC}"
echo ""

cd "$PROJECT_DIR"

if [ "$MODE" = "dev" ]; then
    # ─── 开发模式：go run ─────────────────────────────────
    log_info "启动模式: ${YELLOW}开发模式${NC} (go run)"

    if ! command -v go &>/dev/null; then
        log_error "未找到 Go 编译器，请先安装 Go 1.24+"
        exit 1
    fi

    log_info "Go 版本: $(go version)"
    log_info "启动服务..."

    nohup go run main.go > "$LOG_DIR/app.log" 2>&1 &
    APP_PID=$!

else
    # ─── 生产模式：编译后运行 ──────────────────────────────
    log_info "启动模式: ${GREEN}生产模式${NC} (编译运行)"

    BINARY="$PROJECT_DIR/output/${APP_NAME}"

    # 如果二进制不存在或源码更新了，重新编译
    if [ ! -f "$BINARY" ] || [ "$PROJECT_DIR/main.go" -nt "$BINARY" ]; then
        log_info "编译中..."
        mkdir -p "$PROJECT_DIR/output"
        CGO_ENABLED=0 go build -o "$BINARY" main.go
        log_info "编译完成: $BINARY"
    else
        log_info "使用已有二进制: $BINARY"
    fi

    nohup "$BINARY" > "$LOG_DIR/app.log" 2>&1 &
    APP_PID=$!
fi

# ─── 保存 PID ───────────────────────────────────────────────
echo "$APP_PID" > "$PID_FILE"

# ─── 等待启动 ───────────────────────────────────────────────
log_info "等待服务启动 (PID: $APP_PID)..."

HEALTH_PORT="${PORT:-8080}"
MAX_WAIT=15
WAITED=0

while [ $WAITED -lt $MAX_WAIT ]; do
    sleep 1
    WAITED=$((WAITED + 1))

    # 检查进程是否还活着
    if ! kill -0 "$APP_PID" 2>/dev/null; then
        log_error "服务启动失败！查看日志:"
        tail -20 "$LOG_DIR/app.log"
        rm -f "$PID_FILE"
        exit 1
    fi

    # 检查端口是否就绪
    if curl -s "http://localhost:${HEALTH_PORT}/api/models" >/dev/null 2>&1; then
        echo ""
        log_info "✅ 服务启动成功！"
        echo ""
        echo -e "  📍 访问地址:  ${GREEN}http://localhost:${HEALTH_PORT}${NC}"
        echo -e "  📋 进程 PID:  ${BLUE}${APP_PID}${NC}"
        echo -e "  📁 日志文件:  ${BLUE}${LOG_DIR}/app.log${NC}"
        echo -e "  🛑 停止服务:  ${YELLOW}./scripts/stop.sh${NC}"
        echo ""
        exit 0
    fi

    printf "."
done

echo ""
log_warn "服务启动超时（${MAX_WAIT}s），但进程仍在运行 (PID: $APP_PID)"
log_info "请检查日志: tail -f $LOG_DIR/app.log"
