#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
create_skill 工具脚本
从 stdin 读取 JSON 参数，在 skills/ 目录下自动生成完整的 skill 目录结构：
  skills/<skill_name>/
    SKILL.md
    scripts/
      run.py
      tool.json
"""
import sys
import json
import os
import re


# skills 根目录（相对于项目根目录）
SKILLS_BASE_DIR = "skills"


def slugify(name: str) -> str:
    """将名称转换为合法的目录名（小写、连字符）"""
    name = name.lower().strip()
    name = re.sub(r"[^\w\s-]", "", name)
    name = re.sub(r"[\s_]+", "-", name)
    name = re.sub(r"-+", "-", name)
    return name.strip("-")


def validate_params(params: dict) -> str | None:
    """校验必要参数，返回错误信息或 None"""
    required = ["skill_name", "display_name", "description", "tool_description",
                 "parameters", "script_code", "system_prompt"]
    for field in required:
        if not params.get(field):
            return f"缺少必要参数: {field}"

    # 校验 parameters 是否为合法 JSON 对象
    params_val = params["parameters"]
    if isinstance(params_val, str):
        try:
            parsed = json.loads(params_val)
        except json.JSONDecodeError as e:
            return f"parameters 不是合法的 JSON: {e}"
        if not isinstance(parsed, dict):
            return "parameters 必须是 JSON 对象"
    elif not isinstance(params_val, dict):
        return "parameters 必须是 JSON 对象或 JSON 字符串"

    return None


def build_tool_json(params: dict) -> dict:
    """构建 tool.json 内容"""
    parameters = params["parameters"]
    if isinstance(parameters, str):
        parameters = json.loads(parameters)

    return {
        "name": slugify(params["skill_name"]),
        "display_name": params["display_name"],
        "description": params["tool_description"],
        "script": "run.py",
        "parameters": parameters,
    }


def build_skill_md(params: dict) -> str:
    """构建 SKILL.md 内容"""
    slug = slugify(params["skill_name"])
    icon = params.get("icon", "🔧")
    tools_line = params.get("extra_tools", "")
    tools_list = f"  - {slug}"
    if tools_line:
        for t in tools_line.split(","):
            t = t.strip()
            if t and t != slug:
                tools_list += f"\n  - {t}"

    system_prompt = params["system_prompt"]

    return f"""---
name: {params["display_name"]}
description: {params["description"]}
icon: {icon}
pattern: tool-wrapper
tools:
{tools_list}
version: "1.0"
---

{system_prompt}
"""


def main():
    try:
        raw = sys.stdin.read().strip()
        params = json.loads(raw) if raw else {}
    except json.JSONDecodeError as e:
        print(json.dumps({"error": f"参数解析失败: {str(e)}"}))
        sys.exit(1)

    # 参数校验
    err = validate_params(params)
    if err:
        print(json.dumps({"error": err}))
        sys.exit(1)

    slug = slugify(params["skill_name"])
    if not slug:
        print(json.dumps({"error": "skill_name 转换后为空，请使用合法名称"}))
        sys.exit(1)

    # 构建目标路径
    skill_dir = os.path.join(SKILLS_BASE_DIR, slug)
    scripts_dir = os.path.join(skill_dir, "scripts")

    # 检查是否已存在
    if os.path.exists(skill_dir) and not params.get("overwrite", False):
        print(json.dumps({
            "error": f"skill '{slug}' 已存在，如需覆盖请设置 overwrite=true",
            "path": skill_dir,
        }))
        sys.exit(1)

    # 创建目录
    os.makedirs(scripts_dir, exist_ok=True)

    created_files = []

    # 写入 scripts/tool.json
    tool_json_path = os.path.join(scripts_dir, "tool.json")
    tool_json_content = build_tool_json(params)
    with open(tool_json_path, "w", encoding="utf-8") as f:
        json.dump(tool_json_content, f, ensure_ascii=False, indent=2)
    created_files.append(tool_json_path)

    # 写入 scripts/run.py
    run_py_path = os.path.join(scripts_dir, "run.py")
    script_code = params["script_code"]
    # 确保脚本有 shebang
    if not script_code.startswith("#!"):
        script_code = "#!/usr/bin/env python3\n# -*- coding: utf-8 -*-\n" + script_code
    with open(run_py_path, "w", encoding="utf-8") as f:
        f.write(script_code)
    # 赋予执行权限
    os.chmod(run_py_path, 0o755)
    created_files.append(run_py_path)

    # 写入 SKILL.md
    skill_md_path = os.path.join(skill_dir, "SKILL.md")
    skill_md_content = build_skill_md(params)
    with open(skill_md_path, "w", encoding="utf-8") as f:
        f.write(skill_md_content)
    created_files.append(skill_md_path)

    result = {
        "success": True,
        "skill_name": slug,
        "display_name": params["display_name"],
        "skill_dir": skill_dir,
        "created_files": created_files,
        "message": f"✅ Skill '{params['display_name']}' 创建成功！目录：{skill_dir}",
        "next_steps": [
            f"1. 检查并测试脚本：{run_py_path}",
            f"2. 确认工具定义：{tool_json_path}",
            f"3. 在平台重启或刷新后即可在 Skills 列表中看到新技能",
        ],
    }
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
