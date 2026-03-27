#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
mysql_query 工具脚本
从 stdin 读取 JSON 参数，执行只读 SQL 查询，结果输出到 stdout
连接信息通过环境变量读取：DB_HOST、DB_PORT、DB_USER、DB_PASSWORD、DB_NAME
"""
import sys
import json
import os
import re

MAX_LIMIT = 200
DEFAULT_LIMIT = 50


def get_db_config() -> dict:
    """从环境变量读取数据库连接配置，回退到默认值"""
    return {
        "host": os.environ.get("DB_HOST", "localhost"),
        "port": int(os.environ.get("DB_PORT", "3306")),
        "user": os.environ.get("DB_USER", "root"),
        "password": os.environ.get("DB_PASSWORD", "123456"),
        "database": os.environ.get("DB_NAME", "ai_chat_db"),
    }


def is_readonly_sql(sql: str) -> bool:
    """检查 SQL 是否为只读查询（仅允许 SELECT / SHOW / DESCRIBE / EXPLAIN）"""
    stripped = sql.strip().lstrip("(")
    pattern = re.compile(r"^\s*(SELECT|SHOW|DESCRIBE|DESC|EXPLAIN)\b", re.IGNORECASE)
    return bool(pattern.match(stripped))


def execute_query(sql: str, limit: int) -> dict:
    """执行 SQL 查询并返回结果"""
    try:
        import pymysql  # type: ignore
        import pymysql.cursors  # type: ignore
    except ImportError:
        return {"error": "缺少依赖 pymysql，请执行：pip3 install pymysql"}

    cfg = get_db_config()
    try:
        conn = pymysql.connect(
            host=cfg["host"],
            port=cfg["port"],
            user=cfg["user"],
            password=cfg["password"],
            database=cfg["database"],
            charset="utf8mb4",
            cursorclass=pymysql.cursors.DictCursor,
            connect_timeout=10,
            read_timeout=15,
        )
    except Exception as e:
        return {"error": f"数据库连接失败: {str(e)}"}

    try:
        with conn.cursor() as cursor:
            # 注入 LIMIT 保护（如果 SQL 中没有 LIMIT 子句）
            sql_upper = sql.upper()
            if "LIMIT" not in sql_upper and sql_upper.strip().startswith("SELECT"):
                safe_sql = f"{sql.rstrip(';')} LIMIT {limit}"
            else:
                safe_sql = sql

            cursor.execute(safe_sql)
            rows = cursor.fetchmany(limit)

            # 将所有值转为 JSON 可序列化类型
            serializable_rows = []
            for row in rows:
                clean_row = {}
                for k, v in row.items():
                    if hasattr(v, "isoformat"):  # datetime/date
                        clean_row[k] = v.isoformat()
                    elif isinstance(v, bytes):
                        clean_row[k] = v.decode("utf-8", errors="replace")
                    else:
                        clean_row[k] = v
                serializable_rows.append(clean_row)

            return {
                "sql": safe_sql,
                "row_count": len(serializable_rows),
                "columns": list(serializable_rows[0].keys()) if serializable_rows else [],
                "rows": serializable_rows,
            }
    except Exception as e:
        return {"error": f"SQL 执行失败: {str(e)}"}
    finally:
        conn.close()


def main():
    try:
        raw = sys.stdin.read().strip()
        params = json.loads(raw) if raw else {}
    except json.JSONDecodeError as e:
        print(json.dumps({"error": f"参数解析失败: {str(e)}"}))
        sys.exit(1)

    sql = params.get("sql", "").strip()
    if not sql:
        print(json.dumps({"error": "缺少参数 sql"}))
        sys.exit(1)

    # 安全检查：只允许只读 SQL
    if not is_readonly_sql(sql):
        print(json.dumps({"error": "只允许执行 SELECT/SHOW/DESCRIBE/EXPLAIN 语句，禁止写操作"}))
        sys.exit(1)

    limit = min(int(params.get("limit", DEFAULT_LIMIT)), MAX_LIMIT)
    result = execute_query(sql, limit)
    print(json.dumps(result, ensure_ascii=False, default=str))


if __name__ == "__main__":
    main()
