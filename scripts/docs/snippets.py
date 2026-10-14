"""Snippet sync: every ```go block in the docs is a tested Example, or says why not.

Usage:
  python3 -I scripts/docs/snippets.py          check, exit 1 on any failure
  python3 -I scripts/docs/snippets.py --fix    rewrite synced blocks from their regions, then check

The convention (docs batch, 2026-10-14):

  <!-- snippet: path/to/x_test.go#RegionName -->
  ```go
  ...block equal to the region...
  ```

The block must equal, after removing the common leading indentation and any
leading/trailing blank lines on both sides, the lines between `// snippet:start RegionName` and
`// snippet:end RegionName` in that *_test.go file. The region must overlap a
`func Example...()` whose body carries `// Output:` or `// Unordered output:`,
so `go test` both compiles the code and checks what it prints (or be a helper
func that such an Example in the same file calls). Region names
are unique across the whole repo.

A block that cannot be a tested example says so on the line before the fence:

  <!-- snippet: exempt — <non-empty reason> -->

Any other ```go block, a marker that is not directly followed by a ```go
fence, or a marker naming a missing file or region is a failure. Shell and
text blocks are not checked.
"""
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from repo import ROOT, md_files, read_text, repo_files

# Fence languages that count as Go code.
GO_LANGS = {"go", "golang"}
FENCE_OPEN_RE = re.compile(r"^(\s*)(`{3,}|~{3,})\s*([^\s`]*)")
SYNC_RE = re.compile(r"^\s*<!--\s*snippet:\s*(?!exempt\b)(\S+?)#([A-Za-z0-9_.-]+)\s*-->\s*$")
EXEMPT_RE = re.compile(r"^\s*<!--\s*snippet:\s*exempt\b\s*(?:[—–:-]+)?\s*(.*?)\s*-->\s*$")
ANY_MARKER_RE = re.compile(r"^\s*<!--\s*snippet:")
REGION_RE = re.compile(r"^\s*//\s*snippet:(start|end)\s+(\S+)\s*$")
EXAMPLE_FUNC_RE = re.compile(r"^func (Example\w*)\(\)")
HELPER_FUNC_RE = re.compile(r"^func (?:\([^)]*\) )?(\w+)\(", re.M)
OUTPUT_RE = re.compile(r"^\s*//\s*(Output|Unordered output):")


def dedent(lines):
    """Remove the common leading whitespace and leading/trailing blank lines.

    Whitespace-only lines become empty. Edge blank lines are dropped because
    gofmt-friendly regions usually end with a blank line before
    `// snippet:end`, which no doc block should have to copy.
    """
    lines = list(lines)
    while lines and not lines[0].strip():
        lines.pop(0)
    while lines and not lines[-1].strip():
        lines.pop()
    if not lines:
        return []
    width = min(len(l) - len(l.lstrip()) for l in lines if l.strip())
    return [l[width:] if l.strip() else "" for l in lines]


class Region:
    def __init__(self, path, name, start, end, lines):
        self.path, self.name, self.start, self.end, self.lines = path, name, start, end, lines


def parse_go(path, root):
    """Return (regions by name, problems) for one Go file. Lines are 1-based."""
    full = os.path.join(root, path)
    src = read_text(full).split("\n")
    problems, regions, open_at = [], {}, {}
    for i, line in enumerate(src, 1):
        m = REGION_RE.match(line)
        if not m:
            continue
        kind, name = m.groups()
        if kind == "start":
            if name in open_at or name in regions:
                problems.append(f"{path}:{i}: region {name} started twice in this file")
            open_at[name] = i
        else:
            if name not in open_at:
                problems.append(f"{path}:{i}: snippet:end {name} without a start")
                continue
            s = open_at.pop(name)
            # Marker lines of other (nested) regions never appear in the docs.
            body = [l for l in src[s:i - 1] if not REGION_RE.match(l)]
            regions[name] = Region(path, name, s, i, dedent(body))
    for name, s in open_at.items():
        problems.append(f"{path}:{s}: snippet:start {name} is never closed")
    # Example functions: span from the func line to the first column-0 "}".
    funcs = []
    for i, line in enumerate(src, 1):
        m = EXAMPLE_FUNC_RE.match(line)
        if not m:
            continue
        end = next((j for j in range(i, len(src) + 1) if src[j - 1] == "}"), len(src))
        has_output = any(OUTPUT_RE.match(l) for l in src[i - 1:end])
        funcs.append((m.group(1), i, end, has_output))
    for r in regions.values():
        hit = [f for f in funcs if f[1] <= r.end and f[2] >= r.start]
        if not hit:
            # A helper region (a func the Examples call) is still compiled,
            # run and output-checked when every func it declares is called
            # from an Example with an // Output: block in the same file.
            declared = HELPER_FUNC_RE.findall("\n".join(src[r.start:r.end - 1]))
            checked = ["\n".join(src[f[1]:f[2]]) for f in funcs if f[3]]
            unused = [d for d in declared if not any(re.search(r"\b" + re.escape(d) + r"\(", body) for body in checked)]
            if not declared:
                problems.append(f"{path}:{r.start}: region {r.name} is not inside a func Example... (nor a helper func an Example calls)")
            for d in unused:
                problems.append(f"{path}:{r.start}: helper {d} in region {r.name} is not called by any Example with // Output: in this file")
        for name, fstart, _, has_output in hit:
            if not has_output:
                problems.append(f"{path}:{fstart}: {name} (region {r.name}) has no // Output: comment, so go test never runs it")
    return regions, problems


def collect_regions(root, go_files):
    """All regions in the given *_test.go files, with uniqueness enforced repo-wide."""
    by_name, problems = {}, []
    for path in go_files:
        regions, probs = parse_go(path, root)
        problems += probs
        for name, r in regions.items():
            if name in by_name:
                o = by_name[name]
                problems.append(f"{path}:{r.start}: region name {name} already used at {o.path}:{o.start}; names are unique repo-wide")
                continue
            by_name[name] = r
    return by_name, problems


def go_blocks(lines):
    """Yield (open_idx, close_idx, indent, lang) for every fenced block (0-based line indexes)."""
    i = 0
    while i < len(lines):
        m = FENCE_OPEN_RE.match(lines[i])
        if not m:
            i += 1
            continue
        indent, fence, lang = m.groups()
        close_re = re.compile(r"^\s*" + re.escape(fence[0]) + "{" + str(len(fence)) + r",}\s*$")
        j = i + 1
        while j < len(lines) and not close_re.match(lines[j]):
            j += 1
        yield i, j, indent, lang.lower()
        i = j + 1


def check_md(path, root, regions, fix=False):
    """Return (synced, exempt, problems, new_text_or_None) for one markdown file."""
    full = os.path.join(root, path)
    text = read_text(full)
    lines = text.split("\n")
    synced = exempt = 0
    problems = []
    used_markers = set()
    replacements = []  # (open_idx, close_idx, new_body_lines)
    for o, c, indent, lang in go_blocks(lines):
        prev = lines[o - 1] if o > 0 else ""
        if ANY_MARKER_RE.match(prev):
            used_markers.add(o - 1)
        if lang not in GO_LANGS:
            if ANY_MARKER_RE.match(prev):
                problems.append(f"{path}:{o}: snippet marker is followed by a ```{lang or '(no language)'} block, not ```go")
            continue
        where = f"{path}:{o + 1}"
        if c >= len(lines):
            problems.append(f"{where}: ```go fence is never closed")
            continue
        sm, em = SYNC_RE.match(prev), EXEMPT_RE.match(prev)
        if em:
            if em.group(1):
                exempt += 1
            else:
                problems.append(f"{where}: exempt snippet needs a reason: <!-- snippet: exempt — <why> -->")
            continue
        if not sm:
            problems.append(f"{where}: ```go block has no snippet marker on the line before it (sync it to an Example or mark it exempt with a reason)")
            continue
        ref, name = sm.groups()
        if not ref.endswith("_test.go"):
            problems.append(f"{where}: snippet source {ref} is not a *_test.go file")
            continue
        if not os.path.exists(os.path.join(root, ref)):
            problems.append(f"{where}: snippet source {ref} does not exist")
            continue
        r = regions.get(name)
        if r is None or r.path != ref:
            found = f" (it is in {r.path})" if r else ""
            problems.append(f"{where}: region {name} not found in {ref}{found}")
            continue
        block = dedent(lines[o + 1:c])
        if block == r.lines:
            synced += 1
            continue
        if fix:
            replacements.append((o, c, [(indent + l) if l else "" for l in r.lines]))
            synced += 1
            continue
        first = next((k for k, (a, b) in enumerate(zip(block, r.lines)) if a != b), min(len(block), len(r.lines)))
        doc_l = block[first] if first < len(block) else "<end of block>"
        reg_l = r.lines[first] if first < len(r.lines) else "<end of region>"
        problems.append(f"{where}: block differs from {ref}#{name} at block line {first + 1} (doc line {o + 2 + first}): doc {doc_l!r} vs region {reg_l!r}  (run task docs:snippets:fix)")
    # A marker that is not directly before a fence is almost always a typo or a stray blank line.
    fenced = set()
    for o, c, _, _ in go_blocks(lines):
        fenced.update(range(o, c + 1))
    for k, l in enumerate(lines):
        if k not in fenced and ANY_MARKER_RE.match(l) and k not in used_markers:
            problems.append(f"{path}:{k + 1}: snippet marker is not directly followed by a ```go fence")
    new_text = None
    if replacements:
        for o, c, body in reversed(replacements):
            lines[o + 1:c] = body
        new_text = "\n".join(lines)
    return synced, exempt, problems, new_text


def run(root, md, go, fix=False, out=print):
    """Check (or fix) the given files; return (synced, exempt, problems)."""
    regions, problems = collect_regions(root, go)
    synced = exempt = 0
    for path in md:
        s, e, probs, new_text = check_md(path, root, regions, fix=fix)
        if new_text is not None:
            with open(os.path.join(root, path), "w", encoding="utf-8") as fh:
                fh.write(new_text)
            out(f"fixed {path}")
        synced += s
        exempt += e
        problems += probs
    return synced, exempt, problems


def main(argv):
    fix = "--fix" in argv
    synced, exempt, problems = run(ROOT, md_files(), repo_files("_test.go"), fix=fix)
    for p in problems:
        print(p)
    print(f"snippets: {synced} synced, {exempt} exempt, {len(problems)} failing")
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
