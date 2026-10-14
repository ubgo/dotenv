"""Fail when published docs or examples carry a local path or a forbidden word.

Usage: python3 -I scripts/docs/words.py [file ...]

Scanned by default: every tracked or untracked-not-ignored .md file (outside
.lore/, docsi/, testdata), llms.txt, files under docs/ (svg diagrams too), and
every *_test.go file that defines a func Example (those render on
pkg.go.dev). Prints file:line: what matched, never more of the line than the
match, and exits 1 on any hit.

Why: a local path leaks one machine's layout into public docs and breaks for
every reader; the word list keeps tool attribution out of the project.
"""
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from repo import ROOT, read_text, repo_files

# Local filesystem prefixes that only exist on a maintainer's machine.
LOCAL_PATHS = ["/Users/", "/Volumes/", "/khanakia/", "/space/", "/home/", "/tmp/claude"]
# Attribution words that must never appear (case-sensitive where a common
# lowercase word would collide, e.g. "ai" inside other words).
FORBIDDEN = [
    (re.compile(r"\bClaude\b"), "Claude"),
    (re.compile(r"\bAnthropic\b", re.I), "Anthropic"),
    (re.compile(r"\bAI-generated\b", re.I), "AI-generated"),
    (re.compile(r"\bGenerated with\b"), "Generated with"),
    (re.compile(r"\bAI\b"), "AI"),
]
TEXT_SUFFIXES = (".md", ".txt", ".svg")
EXAMPLE_RE = re.compile(r"^func Example\w*\(\)", re.M)


def default_files():
    files = set(repo_files(".md"))
    for p in repo_files(""):
        if p == "llms.txt" or (p.startswith("docs/") and p.endswith(TEXT_SUFFIXES)):
            files.add(p)
        elif p.endswith("_test.go"):
            if EXAMPLE_RE.search(read_text(os.path.join(ROOT, p))):
                files.add(p)
    return sorted(files)


def check_text(text, path):
    hits = []
    for n, line in enumerate(text.split("\n"), 1):
        for prefix in LOCAL_PATHS:
            if prefix in line:
                hits.append(f"{path}:{n}: local path {prefix}")
        for rx, label in FORBIDDEN:
            if rx.search(line):
                hits.append(f"{path}:{n}: forbidden word {label!r}")
    return hits


def main(argv):
    files = argv or default_files()
    hits = []
    for f in files:
        full = f if os.path.isabs(f) else os.path.join(ROOT, f)
        hits += check_text(read_text(full), f)
    for h in hits:
        print(h)
    print(f"words: {len(files)} files, {len(hits)} hits")
    return 1 if hits else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
