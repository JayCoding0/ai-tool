#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
execute_command 工具脚本（安全加固版）
从 stdin 读取 JSON 参数，在严格白名单范围内执行只读 Shell 命令，结果输出到 stdout。

安全设计（修复 RCE 绕过）：
  1. shell=False —— 不经过 shell 解释，杜绝 ;|&$() 等元字符注入
  2. 命令白名单仅保留"只读/信息查询"类命令，移除所有解释器（python/node/go/java）
     以及可执行任意子命令的命令（awk/find/sed/xargs/tee/curl/wget 等）
  3. 拒绝任何 shell 元字符与重定向
  4. 输出大小限制 + 超时控制
"""
import os
import sys
import json
import subprocess
import shlex

# 只读信息查询类命令白名单（不含任何可执行子命令/代码的命令）
ALLOWED_COMMANDS = {
    # 文件/目录查看（只读）
    "ls", "cat", "head", "tail", "file", "stat", "wc", "diff",
    "which", "whereis", "pwd", "realpath", "basename", "dirname",
    # 系统信息（只读）
    "ps", "df", "du", "free", "uname", "uptime", "hostname",
    "whoami", "id", "date", "cal", "uptime",
    # 文本查看/检索（只读，且不支持执行子命令的安全用法）
    "grep", "egrep", "fgrep", "sort", "uniq", "cut", "tr",
    "echo", "printf", "head", "tail",
}

# 禁止出现的 shell 元字符（即使 shell=False，也拦截以防误用与多命令拼接）
FORBIDDEN_CHARS = set(";|&`$><\n\r\\")

# 针对个别命令的危险参数拦截（避免 grep -P 之类间接能力，从严处理）
FORBIDDEN_ARG_PREFIXES = ("--exec", "-exec", "--eval", "-e")

MAX_OUTPUT_BYTES = 64 * 1024   # 输出最大 64KB
DEFAULT_TIMEOUT = 10
MAX_TIMEOUT = 20


def check_command_safety(command: str):
    """检查命令安全性，返回 (argv 列表 或 None, 错误信息 或 None)"""
    # 1. 拦截 shell 元字符
    for ch in command:
        if ch in FORBIDDEN_CHARS:
            return None, f"命令包含禁止的字符: {ch!r}"

    # 2. 解析为参数列表（POSIX 语义）
    try:
        parts = shlex.split(command)
    except ValueError as e:
        return None, f"命令解析失败: {str(e)}"
    if not parts:
        return None, "命令不能为空"

    # 3. 主命令必须在白名单内（按 basename 比较，禁止绝对/相对路径调用任意程序）
    raw_cmd = parts[0]
    if "/" in raw_cmd:
        return None, "不允许通过路径指定可执行文件，请直接使用命令名"
    if raw_cmd not in ALLOWED_COMMANDS:
        return None, (f"命令 '{raw_cmd}' 不在允许列表中。"
                      f"允许的命令：{', '.join(sorted(ALLOWED_COMMANDS))}")

    # 4. 拦截危险参数
    for arg in parts[1:]:
        low = arg.lower()
        for bad in FORBIDDEN_ARG_PREFIXES:
            if low == bad or low.startswith(bad + "="):
                return None, f"命令包含禁止的参数: {arg}"

    return parts, None


def main():
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

    try:
        timeout = min(float(params.get("timeout", DEFAULT_TIMEOUT)), MAX_TIMEOUT)
    except (TypeError, ValueError):
        timeout = DEFAULT_TIMEOUT

    argv, err = check_command_safety(command)
    if err:
        print(json.dumps({"error": err}))
        sys.exit(1)

    try:
        result = subprocess.run(
            argv,
            shell=False,  # 关键：不经过 shell，杜绝注入
            capture_output=True,
            timeout=timeout,
            text=True,
            encoding="utf-8",
            errors="replace",
            env={"PATH": os.environ.get("PATH", "/usr/bin:/bin")},
        )

        stdout = result.stdout
        stderr = result.stderr
        truncated = False
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
    except FileNotFoundError:
        print(json.dumps({"error": "命令不存在或不可执行"}))
    except Exception as e:
        print(json.dumps({"error": f"命令执行失败: {str(e)}"}))


if __name__ == "__main__":
    main()
