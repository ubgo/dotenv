"""Shared helpers for the docs checks: where the repo root is and which files count.

Every docs script resolves paths from the repo root (two levels above this
file), so the checks run the same from any working directory and on any
machine. Nothing here hardcodes a local path.
"""
import os
import subprocess

# The repo root: scripts/docs/ sits two levels below it.
ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", ".."))

# Directories whose files are not public docs: lore's private store, the
# gitignored private overlay (listed in case it is ever un-ignored), and test
# fixtures, which may hold deliberately malformed markdown.
EXCLUDED_DIRS = (".lore/", "docsi/")
EXCLUDED_PARTS = ("testdata",)


def _excluded(path):
    if path.startswith(EXCLUDED_DIRS):
        return True
    return any(part in EXCLUDED_PARTS for part in path.split("/"))


def repo_files(suffix):
    """Tracked plus untracked-but-not-ignored files ending in suffix, repo-relative, sorted.

    Untracked files count so that pages a writer has not committed yet are
    checked too; ignored files (docsi/, build output) never are.
    """
    out = subprocess.run(
        ["git", "ls-files", "-co", "--exclude-standard"],
        cwd=ROOT, check=True, capture_output=True, text=True,
    ).stdout.split("\n")
    seen = set()
    files = []
    for p in out:
        if p and p.endswith(suffix) and not _excluded(p) and p not in seen:
            if os.path.exists(os.path.join(ROOT, p)):  # deleted-but-tracked files are skipped
                seen.add(p)
                files.append(p)
    return sorted(files)


def md_files():
    return repo_files(".md")


def read_text(path):
    """Read a UTF-8 file and close it (bare open().read() leaks the handle)."""
    with open(path, encoding="utf-8") as fh:
        return fh.read()
