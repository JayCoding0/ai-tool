#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
http_request 工具脚本
从 stdin 读取 JSON 参数，发送 HTTP 请求，结果输出到 stdout
"""
import sys
import json
import urllib.request
import urllib.error
import urllib.parse

MAX_RESPONSE_BYTES = 256 * 1024  # 响应体最大 256KB


def main():
    try:
        raw = sys.stdin.read().strip()
        params = json.loads(raw) if raw else {}
    except json.JSONDecodeError as e:
        print(json.dumps({"error": f"参数解析失败: {str(e)}"}))
        sys.exit(1)

    url = params.get("url", "").strip()
    if not url:
        print(json.dumps({"error": "缺少参数 url"}))
        sys.exit(1)

    method = params.get("method", "GET").upper()
    headers = params.get("headers") or {}
    body = params.get("body", "")
    timeout = min(float(params.get("timeout", 15)), 30)

    # 安全检查：只允许 http/https
    if not url.startswith(("http://", "https://")):
        print(json.dumps({"error": "只支持 http:// 或 https:// 协议"}))
        sys.exit(1)

    try:
        data = body.encode("utf-8") if body else None
        req = urllib.request.Request(url, data=data, method=method)

        # 设置默认 User-Agent
        req.add_header("User-Agent", "AIAgent-HttpTool/1.0")
        for k, v in headers.items():
            req.add_header(str(k), str(v))

        with urllib.request.urlopen(req, timeout=timeout) as resp:
            status_code = resp.status
            resp_headers = dict(resp.headers)
            raw_body = resp.read(MAX_RESPONSE_BYTES)
            truncated = len(raw_body) >= MAX_RESPONSE_BYTES

            # 尝试 UTF-8 解码，失败则 latin-1
            try:
                body_str = raw_body.decode("utf-8")
            except UnicodeDecodeError:
                body_str = raw_body.decode("latin-1")

            # 尝试解析 JSON 响应
            content_type = resp_headers.get("Content-Type", "")
            parsed_body = None
            if "application/json" in content_type:
                try:
                    parsed_body = json.loads(body_str)
                except Exception:
                    pass

            result = {
                "status_code": status_code,
                "url": url,
                "method": method,
                "headers": {k: v for k, v in resp_headers.items()
                            if k.lower() in ("content-type", "content-length", "server", "date")},
                "body": parsed_body if parsed_body is not None else body_str,
                "truncated": truncated,
            }
            print(json.dumps(result, ensure_ascii=False))

    except urllib.error.HTTPError as e:
        err_body = ""
        try:
            err_body = e.read(4096).decode("utf-8", errors="replace")
        except Exception:
            pass
        print(json.dumps({
            "error": f"HTTP 错误 {e.code}: {e.reason}",
            "status_code": e.code,
            "body": err_body,
        }, ensure_ascii=False))
    except urllib.error.URLError as e:
        print(json.dumps({"error": f"请求失败: {str(e.reason)}"}))
    except Exception as e:
        print(json.dumps({"error": f"未知错误: {str(e)}"}))


if __name__ == "__main__":
    main()
