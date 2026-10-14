"""Check every relative link and #anchor in the repo's markdown resolves.

Usage: python3 -I scripts/docs/linkcheck.py [file.md ...]
With no arguments it checks every tracked or untracked-not-ignored .md file
(see repo.md_files). External http(s)/mailto links are not fetched, and docsync
citations (`ds:block?id=…`, resolved by `ds check`, see .ds/) are skipped. Prints one
"file: problem" line per failure and exits 1 when any link is broken.
"""
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from anchors import headings  # GitHub-compatible heading slugs
from repo import ROOT, md_files, read_text

# Code fences hide link-looking text that is not a link.
FENCE_RE = re.compile(r"^\s*(`{3,}|~{3,}).*?^\s*\1", re.S | re.M)
LINK_RE = re.compile(r"\]\(([^)\s]+)(?:\s+\"[^\"]*\")?\)|href=\"([^\"]+)\"")
EXTERNAL_RE = re.compile(r"^(https?|mailto):")
# docsync citations: `ds check` (task docs:sync) verifies every id, so a
# `ds:<verb>?…` link is not a path and must not be resolved on disk.
DOCSYNC_RE = re.compile(r"^ds:[a-z]+\?")
LINE_ANCHOR_RE = re.compile(r"^L\d+(-L\d+)?$")


def check_file(path, root):
    """Return the list of problems in one markdown file (path is root-relative or absolute)."""
    problems = []
    full = path if os.path.isabs(path) else os.path.join(root, path)
    txt = FENCE_RE.sub("", read_text(full))
    for m in LINK_RE.finditer(txt):
        url = m.group(1) or m.group(2)
        if EXTERNAL_RE.match(url) or DOCSYNC_RE.match(url):
            continue
        target, _, anchor = url.partition("#")
        tgt = os.path.normpath(os.path.join(os.path.dirname(full), target)) if target else full
        if not os.path.exists(tgt):
            problems.append(f"{path}: missing {url}")
            continue
        if anchor and tgt.endswith(".md"):
            if anchor not in [h[2] for h in headings(tgt)[0]]:
                problems.append(f"{path}: bad anchor {url}")
        elif anchor and not LINE_ANCHOR_RE.match(anchor):
            problems.append(f"{path}: anchor on a non-markdown file {url}")
    return problems


def main(argv):
    files = argv or md_files()
    problems = []
    for f in files:
        problems += check_file(f, ROOT)
    for p in problems:
        print(p)
    print(f"linkcheck: {len(files)} files, {len(problems)} bad links")
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
