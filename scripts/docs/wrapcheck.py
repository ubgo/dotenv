"""Fail on hard-wrapped markdown prose.

Usage: python3 -I scripts/docs/wrapcheck.py [file.md ...]

House rule: every paragraph, list item and blockquote paragraph is ONE
physical line; the renderer wraps it. A hard wrap is a prose line followed
directly by a line that continues the same paragraph. Not checked: fenced
code, tables, YAML frontmatter, HTML comments, HTML blocks (a line starting
with a tag, up to the next blank line), and link reference definitions.

Lines that end a paragraph line on purpose with a markdown hard break (a
trailing backslash or two trailing spaces) are allowed, since they render as
a line break, not as a reflowed paragraph.

With no arguments it checks every tracked or untracked-not-ignored .md file
outside .lore/, docsi/ and testdata. Prints file:line for the line that
continues a wrapped paragraph and exits 1 when any is found.
"""
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from repo import ROOT, md_files, read_text

FENCE_RE = re.compile(r"^\s{0,3}(`{3,}|~{3,})")
HEADING_RE = re.compile(r"^\s{0,3}#{1,6}(\s|$)")
LIST_RE = re.compile(r"^\s*([-*+]|\d{1,9}[.)])(\s|$)")
TABLE_RE = re.compile(r"^\s*\|")
HTML_BLOCK_RE = re.compile(r"^\s{0,3}</?[A-Za-z][A-Za-z0-9-]*(\s|/?>|$)")
RULE_RE = re.compile(r"^\s{0,3}([-*_])(\s*\1){2,}\s*$")
SETEXT_RE = re.compile(r"^\s{0,3}(=+|-+)\s*$")
LINK_DEF_RE = re.compile(r"^\s{0,3}\[[^\]]+\]:\s")
QUOTE_RE = re.compile(r"^\s{0,3}>\s?")


def starts_block(content):
    """True when a (quote-stripped) line opens a new block instead of continuing prose."""
    return bool(
        not content.strip()
        or FENCE_RE.match(content)
        or HEADING_RE.match(content)
        or LIST_RE.match(content)
        or TABLE_RE.match(content)
        or HTML_BLOCK_RE.match(content)
        or RULE_RE.match(content)
        or SETEXT_RE.match(content)
        or LINK_DEF_RE.match(content)
        or content.lstrip().startswith("<!--")
    )


def hard_break(line):
    return line.endswith("\\") or line.endswith("  ")


def check_text(text, path="<text>"):
    """Return 'path:line: ...' findings for one markdown document."""
    lines = text.split("\n")
    findings = []
    i = 0
    # YAML frontmatter: --- on line 1 up to the next ---.
    if lines and lines[0].strip() == "---":
        j = next((k for k in range(1, len(lines)) if lines[k].strip() in ("---", "...")), None)
        if j is not None:
            i = j + 1
    fence = None
    in_comment = False
    in_html = False
    prev_prose = None  # (quote_depth, line) of the previous line when it was prose, else None
    while i < len(lines):
        line = lines[i]
        i += 1
        if fence:
            if re.match(r"^\s*" + re.escape(fence[0]) + "{" + str(len(fence)) + r",}\s*$", line):
                fence = None
            prev_prose = None
            continue
        if in_comment:
            if "-->" in line:
                in_comment = False
            prev_prose = None
            continue
        if in_html:
            if not line.strip():
                in_html = False
            prev_prose = None
            continue
        # Strip blockquote markers so quoted prose is judged like any prose.
        depth, content = 0, line
        while QUOTE_RE.match(content):
            content = QUOTE_RE.sub("", content, count=1)
            depth += 1
        m = FENCE_RE.match(content)
        if m:
            fence = m.group(1)
            prev_prose = None
            continue
        stripped = content.lstrip()
        if stripped.startswith("<!--"):
            if "-->" not in stripped:
                in_comment = True
            prev_prose = None
            continue
        if HTML_BLOCK_RE.match(content) and depth == 0:
            in_html = True
            prev_prose = None
            continue
        if not starts_block(content) and prev_prose is not None and prev_prose[0] == depth and not hard_break(prev_prose[1]):
            findings.append(f"{path}:{i}: hard-wrapped line (continues the paragraph from line {i - 1}); join it onto one line")
        if not content.strip() or HEADING_RE.match(content) or TABLE_RE.match(content) or RULE_RE.match(content) or SETEXT_RE.match(content) or LINK_DEF_RE.match(content) or HTML_BLOCK_RE.match(content):
            prev_prose = None
        elif content.startswith("    ") and prev_prose is None and not LIST_RE.match(content):
            prev_prose = None  # indented code block, or a list item's indented paragraph after a blank line
        else:
            prev_prose = (depth, content.rstrip("\n"))
    return findings


def main(argv):
    files = argv or md_files()
    findings = []
    for f in files:
        full = f if os.path.isabs(f) else os.path.join(ROOT, f)
        findings += check_text(read_text(full), f)
    for x in findings:
        print(x)
    print(f"wrapcheck: {len(files)} files, {len(findings)} hard-wrapped lines")
    return 1 if findings else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
