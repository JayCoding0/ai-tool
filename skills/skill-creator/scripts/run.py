#!/usr/bin/env python3
"""
skill-creator 工具入口脚本

从 stdin 读取 JSON 参数，根据调用的工具名分发到对应的功能函数。
支持三个工具：init_skill、validate_skill、package_skill
"""

import sys
import json
from pathlib import Path

# 将当前脚本所在目录加入 sys.path，以便导入同目录下的模块
SCRIPT_DIR = Path(__file__).parent
sys.path.insert(0, str(SCRIPT_DIR))

from init_skill import init_skill
from quick_validate import validate_skill
from package_skill import package_skill


def main():
    # 从 stdin 读取 JSON 参数
    try:
        raw = sys.stdin.read()
        args = json.loads(raw) if raw.strip() else {}
    except json.JSONDecodeError as e:
        print(json.dumps({"error": f"JSON 解析失败: {e}"}))
        sys.exit(1)

    # 根据参数中的字段判断调用哪个工具
    # init_skill: 有 skill_name 和 path
    # validate_skill: 有 skill_path，无 output_dir
    # package_skill: 有 skill_path 和可选的 output_dir
    if "skill_name" in args:
        # init_skill 工具
        skill_name = args.get("skill_name", "")
        path = args.get("path", "skills")
        if not skill_name:
            print(json.dumps({"error": "skill_name 不能为空"}))
            sys.exit(1)
        result = init_skill(skill_name, path)
        if result:
            print(json.dumps({"success": True, "message": f"技能 '{skill_name}' 已初始化", "path": str(result)}))
        else:
            print(json.dumps({"error": f"初始化技能 '{skill_name}' 失败"}))
            sys.exit(1)

    elif "skill_path" in args and "output_dir" in args:
        # package_skill 工具
        skill_path = args.get("skill_path", "")
        output_dir = args.get("output_dir")
        if not skill_path:
            print(json.dumps({"error": "skill_path 不能为空"}))
            sys.exit(1)
        result = package_skill(skill_path, output_dir)
        if result:
            print(json.dumps({"success": True, "message": "技能打包成功", "file": str(result)}))
        else:
            print(json.dumps({"error": "技能打包失败"}))
            sys.exit(1)

    elif "skill_path" in args:
        # validate_skill 工具
        skill_path = args.get("skill_path", "")
        if not skill_path:
            print(json.dumps({"error": "skill_path 不能为空"}))
            sys.exit(1)
        valid, message = validate_skill(skill_path)
        print(json.dumps({"success": valid, "message": message}))
        if not valid:
            sys.exit(1)

    else:
        print(json.dumps({"error": "无法识别的参数，请提供 skill_name 或 skill_path"}))
        sys.exit(1)


if __name__ == "__main__":
    main()
