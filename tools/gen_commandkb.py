#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
生成 aichat 指令知识库 plugin/aichat/commands.md。

扫描 plugin/<name>/ 下所有 .go 文件中 control.AutoRegister(&ctrl.Options[*zero.Ctx]{...})
里的 Brief/Help 字段，并解析 main.go 中启用/禁用插件的导入列表，最终输出一份
按 main.go 注册顺序排列的指令知识库 markdown。

用法:
    python3 tools/gen_commandkb.py

依赖: 仅 Python 标准库。
"""

import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
MAIN_GO = os.path.join(ROOT, "main.go")
PLUGIN_DIR = os.path.join(ROOT, "plugin")
OUT_FILE = os.path.join(ROOT, "plugin", "aichat", "commands.md")

# ---------------------------------------------------------------------------
# Go 字符串字面量解析
# ---------------------------------------------------------------------------

ESCAPES = {
    "a": "\a", "b": "\b", "f": "\f", "n": "\n", "r": "\r", "t": "\t",
    "v": "\v", "\\": "\\", '"': '"', "'": "'", "0": "\0",
}


def decode_go_string(s: str) -> str:
    """解码 Go 双引号字符串字面量（不含首尾引号）。"""
    out = []
    i = 0
    n = len(s)
    while i < n:
        c = s[i]
        if c != "\\":
            out.append(c)
            i += 1
            continue
        i += 1
        if i >= n:
            break
        e = s[i]
        if e in ESCAPES:
            out.append(ESCAPES[e])
            i += 1
        elif e == "x":
            out.append(chr(int(s[i + 1:i + 3], 16)))
            i += 3
        elif e == "u":
            out.append(chr(int(s[i + 1:i + 5], 16)))
            i += 5
        elif e == "U":
            out.append(chr(int(s[i + 1:i + 9], 16)))
            i += 9
        else:
            out.append("\\" + e)
            i += 1
    return "".join(out)


def read_resolved_expr(text: str, start: int, consts: dict):
    """
    从 start 处读取一个 Go 字符串拼接表达式（形如 "a" + "b" + ident + ...），
    返回 (求值结果, 结束位置)。表达式可跨行（行尾以 + 续接）。

    终止条件:
      - 结构体字段值以 ',' 或 '}' 结束;
      - const/var 语句以换行结束（前一个 token 不是 + 时）。
    字符串内的逗号/花括号/换行不会被误判。
    """
    i, n = start, len(text)
    parts = []
    prev_plus = False
    while i < n:
        c = text[i]
        if c in " \t\r":
            i += 1
            continue
        if c == "\n":
            if prev_plus:
                i += 1
                continue
            break
        if c == '"':
            j = i + 1
            while j < n:
                if text[j] == "\\":
                    j += 2
                    continue
                if text[j] == '"':
                    break
                j += 1
            if j >= n:
                break
            parts.append(decode_go_string(text[i + 1:j]))
            prev_plus = False
            i = j + 1
            continue
        if c == "+":
            prev_plus = True
            i += 1
            continue
        if c in ",}":
            break
        m = re.match(r"[A-Za-z_][A-Za-z0-9_]*", text[i:])
        if m:
            ident = m.group(0)
            if ident in consts:
                parts.append(consts[ident])
            prev_plus = False
            i += len(ident)
            continue
        break
    return "".join(parts), i


# ---------------------------------------------------------------------------
# 文件扫描
# ---------------------------------------------------------------------------

def find_consts(src: str) -> dict:
    """提取文件内 const/var 的字符串值，供 Help 表达式引用解析。"""
    consts = {}
    for m in re.finditer(
        r"\b(?:const|var)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*", src
    ):
        name = m.group(1)
        if name in consts:
            continue
        val, _ = read_resolved_expr(src, m.end(), consts)
        consts[name] = val
    return consts


def find_options_blocks(src: str):
    """找出所有 control.AutoRegister(&ctrl.Options[*zero.Ctx]{ ... }) 的块文本。"""
    blocks = []
    start_pat = re.compile(
        r"control\.AutoRegister\(\s*&ctrl\.Options\[\*zero\.Ctx\]\{"
    )
    pos = 0
    while True:
        m = start_pat.search(src, pos)
        if not m:
            break
        brace_start = src.index("{", m.end() - 1)
        depth = 0
        i = brace_start
        in_str = False
        while i < len(src):
            c = src[i]
            if in_str:
                if c == "\\":
                    i += 2
                    continue
                if c == '"':
                    in_str = False
            elif c == '"':
                in_str = True
            elif c == "{":
                depth += 1
            elif c == "}":
                depth -= 1
                if depth == 0:
                    break
            i += 1
        blocks.append(src[brace_start:i + 1])
        pos = i + 1
    return blocks


def extract_field(block: str, name: str, consts: dict):
    """
    提取 Options 块中指定字段（Brief/Help）的字符串值。
    字段名必须位于行首（仅前导空白），避免误匹配字符串内容。
    返回 None 表示没有该字段。
    """
    m = re.search(
        r"(?m)^[ \t]*" + name + r"[ \t]*:[ \t]*", block
    )
    if not m:
        return None
    val, _ = read_resolved_expr(block, m.end(), consts)
    return val.strip()


def collect_plugin(name: str) -> dict:
    """收集单个插件目录的 Brief 列表与 Help 文本列表。"""
    d = os.path.join(PLUGIN_DIR, name)
    briefs = []
    helps = []
    for root, _dirs, files in os.walk(d):
        for fn in files:
            if not fn.endswith(".go") or fn.endswith("_test.go"):
                continue
            path = os.path.join(root, fn)
            with open(path, encoding="utf-8") as f:
                src = f.read()
            consts = find_consts(src)
            for block in find_options_blocks(src):
                brief = extract_field(block, "Brief", consts)
                help_expr = extract_field(block, "Help", consts)
                if brief:
                    briefs.append(brief)
                if help_expr:
                    helps.append(help_expr)
    return {"briefs": briefs, "helps": helps}


def find_triggers(name: str) -> list:
    """在插件目录里扫描 OnPrefix/OnFullMatch/... 的触发词（Help 为空时的兜底）。"""
    triggers = []
    single = r'(`[^`]*`|"(?:[^"\\]|\\.)*")'
    pats = [
        re.compile(r"\.OnPrefix\(\s*" + single + r"\s*,"),
        re.compile(r"\.OnFullMatch\(\s*" + single + r"\s*[,)]"),
        re.compile(r"\.OnKeyword\(\s*" + single + r"\s*[,)]"),
        re.compile(r"\.OnSuffix\(\s*" + single + r"\s*,"),
        re.compile(r"\.OnPrefixGroup\(\s*\[\]string\{(.*?)\}\s*[,)]", re.DOTALL),
        re.compile(r"\.OnFullMatchGroup\(\s*\[\]string\{(.*?)\}\s*[,)]", re.DOTALL),
    ]
    d = os.path.join(PLUGIN_DIR, name)
    for root, _dirs, files in os.walk(d):
        for fn in files:
            if not fn.endswith(".go") or fn.endswith("_test.go"):
                continue
            with open(os.path.join(root, fn), encoding="utf-8") as f:
                src = f.read()
            for pat in pats:
                for m in pat.finditer(src):
                    arg = m.group(1)
                    if "[]string" in m.group(0):
                        vals = re.findall(single, arg)
                        val = "|".join(
                            v1 if v1 else decode_go_string(v2)
                            for v1, v2 in vals
                        )
                    elif arg.startswith("`"):
                        val = arg[1:-1]
                    else:
                        val = decode_go_string(arg[1:-1])
                    val = val.strip()
                    if val and val not in triggers:
                        triggers.append(val)
    return triggers


# ---------------------------------------------------------------------------
# main.go 启用/禁用列表
# ---------------------------------------------------------------------------

def parse_main() -> tuple:
    """返回 (enabled, disabled)，每一项为 (目录名, 注释)。"""
    with open(MAIN_GO, encoding="utf-8") as f:
        src = f.read()
    enabled = []
    disabled = []
    for line in src.splitlines():
        m = re.match(
            r'^\s*_ "github\.com/FloatTech/ZeroBot-Plugin/plugin/([\w-]+)"(?:\s*//\s*(.*))?$',
            line,
        )
        if m:
            enabled.append((m.group(1), (m.group(2) or "").strip()))
            continue
        m = re.match(
            r'^\s*//+\s*_ "github\.com/FloatTech/ZeroBot-Plugin/plugin/([\w-]+)"(?:\s*//\s*(.*))?$',
            line,
        )
        if m:
            disabled.append((m.group(1), (m.group(2) or "").strip()))
    return enabled, disabled


# ---------------------------------------------------------------------------
# 输出
# ---------------------------------------------------------------------------

def clean_help_lines(help_text: str, name: str) -> list:
    """把 Help 文本拆成行，做去空行、去首行插件名等清理。"""
    lines = [ln.strip() for ln in help_text.splitlines()]
    # 去掉首行与目录名相同的行（如 chat 的 "chat"）
    while lines and lines[0] == name:
        lines.pop(0)
    while lines and not lines[0]:
        lines.pop(0)
    while lines and not lines[-1]:
        lines.pop()
    return lines


def main() -> int:
    enabled, disabled = parse_main()

    sections = []
    for name, comment in enabled:
        info = collect_plugin(name)
        if not info["briefs"] and not info["helps"]:
            # 无任何注册信息的目录（如 custom）跳过
            continue
        heading = " / ".join(dict.fromkeys(b for b in info["briefs"] if b))
        if not heading:
            heading = comment or name
        lines = []
        for h in info["helps"]:
            lines.extend(clean_help_lines(h, name))
        # 行级去重（保留首次出现顺序）
        seen = set()
        uniq = []
        for ln in lines:
            if not ln or ln in seen:
                continue
            seen.add(ln)
            uniq.append(ln)
        lines = uniq
        if not lines:
            for t in find_triggers(name):
                lines.append(t)
        if not lines:
            lines.append("（该插件没有可展示的指令）")
        section = ["## " + heading]
        for ln in lines:
            section.append("- " + ln if not ln.startswith("- ") else ln)
        sections.append("\n".join(section))

    if disabled:
        appendix = ["## 未启用插件（当前不可用，请勿向用户推荐）"]
        for name, comment in disabled:
            appendix.append("- " + (comment or name) + "（" + name + "）")
        sections.append("\n".join(appendix))

    content = (
        "# 机器人指令知识库\n\n"
        "> 本文件由 `tools/gen_commandkb.py` 自动生成，请勿手改；修改插件后重新运行该脚本。\n\n"
        "---\n\n"
        + "\n\n---\n\n".join(sections)
        + "\n"
    )

    os.makedirs(os.path.dirname(OUT_FILE), exist_ok=True)
    with open(OUT_FILE, "w", encoding="utf-8") as f:
        f.write(content)

    n_enabled = len(sections) - (1 if disabled else 0)
    print(f"已生成 {OUT_FILE}")
    print(f"  启用插件章节: {n_enabled}, 未启用插件: {len(disabled)}")
    print(f"  文件大小: {len(content.encode('utf-8'))} bytes / {len(content)} chars")
    return 0


if __name__ == "__main__":
    sys.exit(main())
