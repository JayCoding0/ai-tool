#!/bin/bash
# ============================================================
# AI Agent Platform - 停止脚本
# 用法: ./scripts/stop.sh [--force]
# ============================================================

set -e

# ─── 颜色定义 ───────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# ─── 默认配置 ───────────────────────────────────────────────
APP_NAME="ai-agent"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PID_FILE="$PROJECT_DIR/logs/${APP_NAME}.pid"
FORCE=false

# ─── 参数解析 ───────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case $1 in
        --force|-f)
            FORCE=true
            shift
            ;;
        -h|--help)
            echo "用法: $0 [选项]"
            echo ""
            echo "选项:"
            echo "  --force, -f  强制终止（SIGKILL）"
            echo "  -h, --help   显示帮助信息"
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

# ─── 停止服务 ───────────────────────────────────────────────
stop_service() {
    local pid=$1
    local name=$2

    if ! kill -0 "$pid" 2>/dev/null; then
        log_warn "进程 $pid ($name) 已不存在"
        return 0
    fi

    if [ "$FORCE" = true ]; then
        log_warn "强制终止进程 $pid ($name)..."
        kill -9 "$pid" 2>/dev/null || true
        log_info "进程已强制终止"
        return 0
    fi

    # 优雅停止：先发 SIGTERM，等待退出
    log_info "正在优雅停止 $name (PID: $pid)..."
    kill -TERM "$pid" 2>/dev/null || true

    # 等待进程退出（最多 10 秒）
    local waited=0
    while [ $waited -lt 10 ]; do
        if ! kill -0 "$pid" 2>/dev/null; then
            log_info "✅ $name 已停止"
            return 0
        fi
        sleep 1
        waited=$((waited + 1))
        printf "."
    done

    echo ""
    log_warn "优雅停止超时，强制终止..."
    kill -9 "$pid" 2>/dev/null || true
    sleep 1

    if kill -0 "$pid" 2>/dev/null; then
        log_error "无法终止进程 $pid"
        return 1
    fi

    log_info "✅ $name 已强制停止"
    return 0
}

# ─── 主流程 ─────────────────────────────────────────────────
echo -e "${BLUE}╔══════════════════════════════════════╗${NC}"
echo -e "${BLUE}║   🤖 AI Agent Platform - 停止中     ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════╝${NC}"
echo ""

STOPPED=false

# 方式一：通过 PID 文件停止
if [ -f "$PID_FILE" ]; then
    PID=$(cat "$PID_FILE")
    if stop_service "$PID" "$APP_NAME"; then
        rm -f "$PID_FILE"
        STOPPED=true
    fi
else
    log_warn "未找到 PID 文件: $PID_FILE"
fi

# 方式二：查找残留进程（兜底）
REMAINING_PIDS=$(pgrep -f "ai-agent|go run main.go" 2>/dev/null | grep -v "$$" || true)
if [ -n "$REMAINING_PIDS" ]; then
    log_warn "发现残留进程:"
    for pid in $REMAINING_PIDS; do
        # 排除当前脚本自身和 grep 进程
        if [ "$pid" != "$$" ]; then
            ps -p "$pid" -o pid,ppid,command 2>/dev/null || true
            stop_service "$pid" "残留进程"
            STOPPED=true
        fi
    done
fi

if [ "$STOPPED" = true ]; then
    echo ""
    log_info "🛑 服务已停止"
else
    log_info "没有正在运行的服务"
fi

# 清理 PID 文件
rm -f "$PID_FILE"
