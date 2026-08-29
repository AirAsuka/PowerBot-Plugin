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

import json
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
# AmongUs 小知识（与插件 plugin/amongus/dict 的 GetRoleDesc 逻辑保持一致）
# ---------------------------------------------------------------------------

# 阵营短名，与 dict/role_config.go 的 CampShortText 一致，用于过滤纯阵营词条
AMONGUS_CAMP_NAMES = {"船员", "伪装者", "中立"}

_AMONGUS_DESC_LABELS = (
    ("ShortDesc", "简介："),
    ("FullDesc", "详细介绍："),
    ("IntroDesc", "开场白："),
)


def _read_dict_file(name: str) -> str:
    with open(os.path.join(PLUGIN_DIR, "amongus", "dict", name), encoding="utf-8") as f:
        return f.read()


def generate_amongus_kb() -> str:
    """生成 AmongUs 职业小知识章节，返回 markdown 文本。"""
    info = json.loads(_read_dict_file("role_info.json"))
    maps_src = _read_dict_file("maps.go")
    cats_src = _read_dict_file("role_categories.go")

    # RoleText: 英文名 -> 中文名
    m = re.search(r"var RoleText = map\[string\]string\{(.*?)\n\}", maps_src, re.S)
    roletext = dict(
        re.findall(r'"([^"]+)"\s*:\s*"([^"]+)"', m.group(1)) if m else []
    )

    # 中文名 -> 英文名列表（等价于 RoleTextReverse）
    reverse = {}
    for en, cn in roletext.items():
        if cn in AMONGUS_CAMP_NAMES:
            continue
        reverse.setdefault(cn, []).append(en)

    # 阵营分类（等价于 RoleCategories + CategoryKeys）
    m = re.search(
        r"var RoleCategories = map\[string\]\[\]string\{(.*?)\n\}",
        cats_src,
        re.S,
    )
    categories = {}
    if m:
        for cm, rm in re.findall(r'"([^"]+)"\s*:\s*\{([^}]*)\}', m.group(1), re.S):
            categories[cm] = re.findall(r'"([A-Za-z0-9]+)"', rm)
    m = re.search(r"var CategoryKeys = \[\]string\{([^}]*)\}", cats_src)
    cat_order = re.findall(r'"([^"]+)"', m.group(1)) if m else list(categories)

    # 中文名 -> 第一个所属阵营（与 GetRoleDesc 一致，仅用于分组展示）
    cn_category = {}
    for cat in cat_order:
        for en in categories.get(cat, []):
            cn = roletext.get(en, en)
            if cn in AMONGUS_CAMP_NAMES or cn in cn_category:
                continue
            cn_category[cn] = cat

    def desc_of(cn: str) -> str:
        """等价于 dict.GetRoleDesc：拼接所有英文名对应的 简介/详细介绍/开场白。"""
        parts = []
        for en in reverse.get(cn, []):
            for suffix, label in _AMONGUS_DESC_LABELS:
                d = info.get(en + suffix, {}).get("13", "")
                if d:
                    parts.append(label + d)
        # 一个中文名对应多个英文名时，可能读到相同描述，去重
        seen = set()
        uniq = []
        for p in parts:
            if p in seen:
                continue
            seen.add(p)
            uniq.append(p)
        text = "；".join(uniq)
        # 去除 AmongUs 富文本颜色标签，如 <color=#FF1919FF>严禁</color>
        text = re.sub(r"<[^>]*>", "", text)
        # 描述内可能含字面 \n 或真实换行，统一压平成单行；行内空格保留
        text = text.replace("\\n", "；")
        text = re.sub(r"\s*\n\s*", "；", text)
        while "；；" in text:
            text = text.replace("；；", "；")
        return text.strip("；").strip()

    lines = [
        "## AmongUs 小知识（职业描述）",
        "> 当用户询问 AmongUs 职业的玩法、技能、规则或“小知识”时，可直接引用下方对应职业的描述回答，"
        "也可提示用户发送「小知识 职业名」查看。",
    ]
    for cat in cat_order + ["其他"]:
        names = (
            [cn for cn, c in cn_category.items() if c == cat]
            if cat != "其他"
            else [cn for cn in reverse if cn not in cn_category]
        )
        entries = []
        for cn in names:
            d = desc_of(cn)
            if d:
                entries.append("- " + cn + "：" + d)
        if not entries:
            continue
        lines.append("### " + cat)
        lines.extend(entries)
    return "\n".join(lines)


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
        # AmongUs 小知识内容紧随 amongus 插件章节
        if name == "amongus":
            sections.append(generate_amongus_kb())

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
