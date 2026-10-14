import os, re, sys, collections
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from anchors import headings, ROOT
from repo import read_text
from features import F
import glob
def rel(path):  # from docs/reference/
    return os.path.relpath(os.path.join(ROOT, path), os.path.join(ROOT, "docs/reference"))
# Index every Example func in both modules: name -> [(repo-relative file, first line)].
EXAMPLES = collections.defaultdict(list)
for _path in sorted(glob.glob(os.path.join(ROOT, "**", "*_test.go"), recursive=True)):
    _rel = os.path.relpath(_path, ROOT)
    for _i, _l in enumerate(read_text(_path).split("\n")):
        _m = re.match(r"^func (Example\w*)\(\)", _l)
        if _m: EXAMPLES[_m.group(1)].append((_rel, _i))
errors = []
def ex_body(path, start):
    src = read_text(os.path.join(ROOT, path)).split("\n")
    body = []
    for l in src[start:]:
        body.append(l)
        if l == "}": break
    return "\n".join(body)
def ex_cell(keys, tok):
    """Render the Example cell: each name links to its file; the first must contain tok."""
    if not keys: return "—"
    cells = []
    for n, key in enumerate(keys.split(",")):
        where, _, name = key.rpartition(":")
        hits = [h for h in EXAMPLES.get(name, []) if not where or os.path.dirname(h[0]) == where]
        if len(hits) != 1:
            errors.append(f"example {key}: {len(hits)} matches"); continue
        path, start = hits[0]
        if n == 0 and tok and tok not in ex_body(path, start): errors.append(f"example {key} lacks {tok}")
        cells.append(f"[`{name}`]({rel(path)})")
    return ", ".join(cells)
def check(guide):
    path, anc = guide.split("#")
    full = os.path.join(ROOT, path)
    if not os.path.exists(full): errors.append(f"missing file {path}"); return
    hs, _ = headings(full)
    if anc not in [h[2] for h in hs]: errors.append(f"bad anchor {guide}")
groups = collections.OrderedDict()
for row in F: groups.setdefault(row[0], []).append(row)
out = ["# Feature index", "",
"Every feature of the `github.com/ubgo/dotenv` library and the `dotenvctl` CLI, one row each, with the guide section that explains it and a runnable example where one exists. The list is built from the exported Go API and the CLI's command and flag definitions, not from other docs.", "",
"- **Package** — the Go package (or `dotenvctl` for a command-line feature).",
"- **Guide** — the section that documents the feature. A row marked *(brief)* would be covered only by a table row or a one-line mention; none is today.",
"- **Example** — runnable `Example` functions (run with `go test -run Example` in the module) whose code uses the feature; `go test` compiles each one and checks its output. A dash means the feature is a CLI or process behaviour shown in the guide with real output rather than an `Example`.", "",
"**Areas:** " + " · ".join(f"[{g}](#{re.sub(r'[^\w\- ]','',g.lower()).replace(' ','-')})" for g in groups), ""]
for g, rows in groups.items():
    out += [f"## {g}", "", "| Feature | Package | Guide | Example |", "|---|---|---|---|"]
    for area, feat, pkg, guide, depth, ex, tok in rows:
        if guide:
            check(guide)
            p, anc = guide.split("#")
            hs, _ = headings(os.path.join(ROOT, p))
            title = next(h[1] for h in hs if h[2] == anc)
            title = title.replace("`", "")
            name = {"README.md": "README", "cli/README.md": "CLI README"}.get(p, os.path.basename(p)[:-3])
            gc = f"[{name} › {title}]({rel(p)}#{anc})" + (" *(brief)*" if depth in "MT" else "")
        else:
            gc = "— *(not documented)*"
        out.append(f"| {feat} | {pkg} | {gc} | {ex_cell(ex, tok)} |")
    out.append("")
# verify the area index anchors resolve in the generated page itself
with open(os.path.join(ROOT, "docs/reference/features.md"), "w", encoding="utf-8") as fh:
    fh.write("\n".join(out))
hs, _ = headings(os.path.join(ROOT, "docs/reference/features.md"))
for g in groups:
    if re.sub(r'[^\w\- ]','',g.lower()).replace(' ','-') not in [h[2] for h in hs]: errors.append("area anchor "+g)
c = collections.Counter(r[4] for r in F)
print("features", len(F), dict(c), "with example", sum(1 for r in F if r[5]))
for g, rows in groups.items():
    cc = collections.Counter(r[4] for r in rows); print(f"  {g}: {len(rows)} {dict(cc)}")
print("ERRORS", errors or "none")
