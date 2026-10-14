"""GitHub heading anchors for markdown files (slugs, duplicates numbered -1, -2, ...)."""
import re, sys, os
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from repo import ROOT, read_text  # repo root, resolved from this file's location
def slug(h):
    h = re.sub(r'<[^>]+>', '', h).strip().lower()
    h = re.sub(r'[^\w\- ]', '', h, flags=re.UNICODE)
    return h.replace(' ', '-')
def headings(path):
    """Return list of (level, text, anchor, lineno, body_lines) skipping fenced code."""
    out, seen, fence = [], {}, False
    lines = read_text(path).split('\n')
    for i, l in enumerate(lines):
        if re.match(r'^\s*(```|~~~)', l): fence = not fence; continue
        if fence: continue
        m = re.match(r'^(#{1,6})\s+(.*?)\s*#*\s*$', l)
        if m:
            a = slug(m.group(2)); n = seen.get(a, 0); seen[a] = n + 1
            out.append([len(m.group(1)), m.group(2), a if n == 0 else f"{a}-{n}", i + 1])
    return out, lines
def sections(path):
    hs, lines = headings(path)
    res = []
    for j, h in enumerate(hs):
        end = hs[j+1][3]-1 if j+1 < len(hs) else len(lines)
        res.append((h[2], h[1], "\n".join(lines[h[3]:end])))
    return res
if __name__ == "__main__":
    for f in sys.argv[1:]:
        for a, t, body in sections(os.path.join(ROOT, f)):
            print(f"{f}#{a}\t{len(body.split())}w\t{body.count('```')//2}code")
