#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
execute_command 工具脚本
从 stdin 读取 JSON 参数，在白名单范围内执行 Shell 命令，结果输出到 stdout
安全措施：命令白名单、禁止管道/重定向写操作、输出大小限制、超时控制
"""
import sys
import json
import subprocess
import shlex
import re

# 允许的命令白名单（只允许只读/信息查询类命令）
ALLOWED_COMMANDS = {
    # 文件查看
    "ls", "ll", "la", "cat", "head", "tail", "less", "more", "file",
    "find", "locate", "which", "whereis", "stat", "wc", "diff",
    # 系统信息
    "ps", "top", "htop", "df", "du", "free", "uname", "uptime",
    "hostname", "whoami", "id", "date", "cal", "env", "printenv",
    "lsof", "netstat", "ss", "ifconfig", "ip", "ping", "traceroute",
    "nslookup", "dig", "curl", "wget",
    # 文本处理（只读）
    "echo", "printf", "grep", "egrep", "fgrep", "awk", "sed", "sort",
    "uniq", "cut", "tr", "paste", "join", "tee", "xargs",
    # 压缩查看（不解压）
    "tar", "zip", "unzip", "gzip", "gunzip", "zcat",
    # 其他
    "pwd", "history", "man", "help", "type", "alias",
    "python3", "python", "node", "go", "java",
}

# 危险模式：禁止包含这些字符/关键字（防止命令注入）
DANGEROUS_PATTERNS = [
    r"[;&|`]",           # 命令链接符
    r"\$\(",             # 命令替换
    r">\s*[^&]",         # 重定向写入（允许 2>&1）
    r"\brm\b",           # 删除
    r"\bchmod\b",        # 修改权限
    r"\bchown\b",        # 修改所有者
    r"\bsudo\b",         # 提权
    r"\bsu\b",           # 切换用户
    r"\bkill\b",         # 杀进程
    r"\bkillall\b",
    r"\bshutdown\b",
    r"\breboot\b",
    r"\bmkdir\b",        # 创建目录
    r"\btouch\b",        # 创建文件
    r"\bmv\b",           # 移动文件
    r"\bcp\b",           # 复制（可能覆盖）
    r"\bdd\b",           # 磁盘操作
    r"\bformat\b",
    r"\bmkfs\b",
    r"\bfdisk\b",
    r"\bnohup\b",        # 后台运行
    r"\bscreen\b",
    r"\btmux\b",
    r"\bcrontab\b",
    r"\bat\b\s",
]

MAX_OUTPUT_BYTES = 64 * 1024   # 输出最大 64KB
DEFAULT_TIMEOUT = 10
MAX_TIMEOUT = 20


def check_command_safety(command: str) -> str | None:
    """检查命令安全性，返回错误信息或 None（安全）"""
    # 检查危险模式
    for pattern in DANGEROUS_PATTERNS:
        if re.search(pattern, command, re.IGNORECASE):
            return f"命令包含禁止的操作：{pattern}"

    # 提取主命令（第一个词）
    try:
        parts = shlex.split(command)
    except ValueError as e:
        return f"命令解析失败: {str(e)}"

    if not parts:
        return "命令不能为空"

    main_cmd = os.path.basename(parts[0]) if "/" in parts[0] else parts[0]
    if main_cmd not in ALLOWED_COMMANDS:
        return (f"命令 '{main_cmd}' 不在允许列表中。"
                f"允许的命令：{', '.join(sorted(ALLOWED_COMMANDS))}")
    return None


def main():
    import os  # noqa: PLC0415

    try:
        raw = sys.stdin.read().strip()
        params = json.loads(raw) if raw else {}
    except json.JSONDecodeError as e:
        print(json.dumps({"error": f"参数解析失败: {str(e)}"}))
        sys.exit(1)

    command = params.get("command", "").strip()
    if not command:
        print(json.dumps({"error": "缺少参数 command"}))
        sys.exit(1)

    timeout = min(float(params.get("timeout", DEFAULT_TIMEOUT)), MAX_TIMEOUT)

    # 安全检查
    err = check_command_safety(command)
    if err:
        print(json.dumps({"error": err}))
        sys.exit(1)

    try:
        result = subprocess.run(
            command,
            shell=True,  # noqa: S602
            capture_output=True,
            timeout=timeout,
            text=True,
            encoding="utf-8",
            errors="replace",
        )

        stdout = result.stdout
        stderr = result.stderr
        truncated = False

        # 截断过长输出
        if len(stdout.encode("utf-8")) > MAX_OUTPUT_BYTES:
            stdout = stdout.encode("utf-8")[:MAX_OUTPUT_BYTES].decode("utf-8", errors="replace")
            truncated = True

        output = {
            "command": command,
            "exit_code": result.returncode,
            "stdout": stdout,
            "stderr": stderr[:2048] if stderr else "",
            "truncated": truncated,
            "success": result.returncode == 0,
        }
        print(json.dumps(output, ensure_ascii=False))

    except subprocess.TimeoutExpired:
        print(json.dumps({"error": f"命令执行超时（超过 {timeout} 秒）"}))
    except Exception as e:
        print(json.dumps({"error": f"命令执行失败: {str(e)}"}))


if __name__ == "__main__":
    main()
