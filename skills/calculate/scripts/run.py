#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
calculate 工具脚本
从 stdin 读取 JSON 参数，安全计算数学表达式，结果输出到 stdout
"""
import sys
import json
import math

def safe_calculate(expression: str) -> dict:
    """安全计算数学表达式，只允许白名单函数和操作符"""
    # 白名单：允许的名称
    allowed_names = {
        "sqrt": math.sqrt,
        "abs": abs,
        "round": round,
        "floor": math.floor,
        "ceil": math.ceil,
        "log": math.log,
        "log2": math.log2,
        "log10": math.log10,
        "sin": math.sin,
        "cos": math.cos,
        "tan": math.tan,
        "pi": math.pi,
        "e": math.e,
        "pow": math.pow,
        "max": max,
        "min": min,
    }

    # 安全检查：禁止包含危险关键字
    forbidden = ["import", "exec", "eval", "open", "os", "sys", "__", "compile",
                 "globals", "locals", "vars", "dir", "getattr", "setattr", "delattr"]
    expr_lower = expression.lower()
    for kw in forbidden:
        if kw in expr_lower:
            return {"error": f"表达式包含禁止的关键字: {kw}"}

    try:
        result = eval(expression, {"__builtins__": {}}, allowed_names)  # noqa: S307
        # 格式化结果：整数不显示小数点，浮点数最多保留 10 位有效数字
        if isinstance(result, float):
            if result == int(result) and abs(result) < 1e15:
                formatted = str(int(result))
            else:
                formatted = f"{result:.10g}"
        else:
            formatted = str(result)
        return {
            "expression": expression,
            "result": result,
            "result_str": formatted,
        }
    except ZeroDivisionError:
        return {"error": "除数不能为零"}
    except Exception as e:
        return {"error": f"计算失败: {str(e)}"}


def main():
    try:
        raw = sys.stdin.read().strip()
        params = json.loads(raw) if raw else {}
    except json.JSONDecodeError as e:
        print(json.dumps({"error": f"参数解析失败: {str(e)}"}))
        sys.exit(1)

    expression = params.get("expression", "").strip()
    if not expression:
        print(json.dumps({"error": "缺少参数 expression"}))
        sys.exit(1)

    result = safe_calculate(expression)
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
