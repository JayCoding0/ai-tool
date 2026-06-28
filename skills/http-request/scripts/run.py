#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
http_request 工具脚本
从 stdin 读取 JSON 参数，发送 HTTP 请求，结果输出到 stdout
"""
import sys
import json
import socket
import ipaddress
import urllib.request
import urllib.error
import urllib.parse

MAX_RESPONSE_BYTES = 256 * 1024  # 响应体最大 256KB


def is_blocked_ip(ip_str: str) -> bool:
    """判断 IP 是否属于禁止访问的内网/保留网段（SSRF 防护）"""
    try:
        ip = ipaddress.ip_address(ip_str)
    except ValueError:
        return True
    if (ip.is_private or ip.is_loopback or ip.is_link_local or
            ip.is_multicast or ip.is_reserved or ip.is_unspecified):
        return True
    # 按安全基线额外拦截的内部约定网段
    if ip.version == 4:
        first = int(str(ip).split(".")[0])
        if first in (9, 11, 21, 30):
            return True
    return False


def validate_url_host(url: str) -> str | None:
    """校验 URL 主机解析出的所有 IP 均为公网地址，返回错误信息或 None"""
    parsed = urllib.parse.urlparse(url)
    host = parsed.hostname
    if not host:
        return "无法解析目标主机"
    try:
        infos = socket.getaddrinfo(host, None)
    except socket.gaierror as e:
        return f"DNS 解析失败: {e}"
    for info in infos:
        ip_str = info[4][0]
        if is_blocked_ip(ip_str):
            return f"拒绝访问内网/保留地址: {ip_str}"
    return None


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

    # SSRF 防护：校验目标主机解析出的 IP 不在内网/保留网段
    ssrf_err = validate_url_host(url)
    if ssrf_err:
        print(json.dumps({"error": ssrf_err}))
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
