#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
write_file 工具脚本
从 stdin 读取 JSON 参数，将内容写入指定文件，结果输出到 stdout
安全限制：只允许相对路径，禁止 .. 和绝对路径
"""
import sys
import json
import os


# 允许写入的文件扩展名白名单（防止写入可执行文件）
ALLOWED_EXTENSIONS = {
    ".txt", ".md", ".json", ".yaml", ".yml", ".csv", ".log",
    ".html", ".xml", ".toml", ".ini", ".conf", ".sql",
    ".py", ".js", ".ts", ".go", ".java", ".sh", ".bash",
    ".css", ".scss", ".less", ".rst", ".tex",
}

# 单次写入最大字节数（1MB）
MAX_WRITE_BYTES = 1 * 1024 * 1024


def validate_path(path: str) -> str | None:
    """校验路径安全性，返回错误信息或 None（合法）"""
    if not path:
        return "路径不能为空"
    if os.path.isabs(path):
        return "不允许使用绝对路径"
    if ".." in path.split(os.sep) or ".." in path.split("/"):
        return "不允许使用 .. 访问上级目录"
    # 检查扩展名
    _, ext = os.path.splitext(path)
    if ext and ext.lower() not in ALLOWED_EXTENSIONS:
        return f"不允许写入扩展名为 {ext} 的文件，支持的类型：{', '.join(sorted(ALLOWED_EXTENSIONS))}"
    return None


def main():
    try:
        raw = sys.stdin.read().strip()
        params = json.loads(raw) if raw else {}
    except json.JSONDecodeError as e:
        print(json.dumps({"error": f"参数解析失败: {str(e)}"}))
        sys.exit(1)

    path = params.get("path", "").strip()
    content = params.get("content", "")
    mode = params.get("mode", "write").lower()
    encoding = params.get("encoding", "utf-8")

    # 路径安全校验
    err = validate_path(path)
    if err:
        print(json.dumps({"error": err}))
        sys.exit(1)

    # 写入模式校验
    if mode not in ("write", "append"):
        mode = "write"
    file_mode = "a" if mode == "append" else "w"

    # 内容大小限制
    content_bytes = content.encode(encoding, errors="replace")
    if len(content_bytes) > MAX_WRITE_BYTES:
        print(json.dumps({"error": f"内容超过最大限制 {MAX_WRITE_BYTES // 1024}KB"}))
        sys.exit(1)

    # 自动创建父目录
    parent_dir = os.path.dirname(path)
    if parent_dir:
        os.makedirs(parent_dir, exist_ok=True)

    try:
        with open(path, file_mode, encoding=encoding) as f:
            f.write(content)

        file_size = os.path.getsize(path)
        result = {
            "success": True,
            "path": path,
            "mode": mode,
            "bytes_written": len(content_bytes),
            "file_size": file_size,
            "message": f"文件{'追加' if mode == 'append' else '写入'}成功：{path}（{len(content_bytes)} 字节）",
        }
        print(json.dumps(result, ensure_ascii=False))
    except PermissionError:
        print(json.dumps({"error": f"没有写入权限：{path}"}))
    except Exception as e:
        print(json.dumps({"error": f"写入失败: {str(e)}"}))


if __name__ == "__main__":
    main()
