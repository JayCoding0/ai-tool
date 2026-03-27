#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
get_current_time 工具脚本
从 stdin 读取 JSON 参数，返回当前时间信息
"""
import sys
import json
import time
from datetime import datetime

# 星期映射
WEEKDAY_CN = ["星期一", "星期二", "星期三", "星期四", "星期五", "星期六", "星期日"]


def get_time_with_tz(timezone: str) -> dict:
    """获取指定时区的当前时间"""
    try:
        import zoneinfo
        tz = zoneinfo.ZoneInfo(timezone)
        now = datetime.now(tz)
    except Exception:
        # 回退到系统本地时间
        now = datetime.now()
        timezone = "local"

    weekday_cn = WEEKDAY_CN[now.weekday()]
    unix_ts = int(time.time())

    return {
        "timezone": timezone,
        "datetime": now.strftime("%Y-%m-%d %H:%M:%S"),
        "date": now.strftime("%Y-%m-%d"),
        "time": now.strftime("%H:%M:%S"),
        "year": now.year,
        "month": now.month,
        "day": now.day,
        "hour": now.hour,
        "minute": now.minute,
        "second": now.second,
        "weekday": weekday_cn,
        "weekday_num": now.weekday() + 1,  # 1=周一, 7=周日
        "unix_timestamp": unix_ts,
        "utc_offset": now.strftime("%z") or "+0000",
    }


def main():
    try:
        raw = sys.stdin.read().strip()
        params = json.loads(raw) if raw else {}
    except json.JSONDecodeError as e:
        print(json.dumps({"error": f"参数解析失败: {str(e)}"}))
        sys.exit(1)

    timezone = params.get("timezone", "Asia/Shanghai").strip()
    result = get_time_with_tz(timezone)
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
