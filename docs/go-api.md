# Go API

Everything `dotenvctl` does is callable from Go. The CLI is a thin translator — flags in, `envkit` call, formatted output — so a program importing these packages gets the same behavior with none of the subprocess overhead.

```go
import "github.com/ubgo/dotenv/cli/envkit"
```

| Package | What it gives you |
|---|---|
| [`envkit`](#envkit--the-operations-api) | the operations: select, discover, matrix, diff, edit, run |
| [`providerkit`](#providerkit--plugins) | the plugin seam: build your own secret-store integration, or mount ours |
| [`discover`](#discover) | env-file family detection for a directory |
| [`report`](#report) | self-contained HTML reports for matrix and diff |
| [`textdiff`](#textdiff) | the minimal line diff used by `--dry-run` |
| [`outfmt`](#outfmt) | the `{ok,data|error}` envelope printer, if you want dotenvctl-compatible output |

The underlying parser/editor is a separate, dependency-free module: [`github.com/ubgo/dotenv`](../README.md). Use it directly when you only need to read or edit one file; use `envkit` when you want the higher-level operations.

## envkit — the operations API

Contract for everything here: typed options in, typed results out. No printing, no `os.Exit`, no cobra. Errors are plain wrapped errors — inspect them with `errors.Is`/`errors.As`. And, like the library beneath it, **nothing in envkit reads or writes `os.Environ`**.

### Selecting keys

The selection grammar the CLI exposes as `--prefix` / `--exclude-prefix` / `--include` / `--exclude` / `--strip-prefix`, as a struct:

```go
sel, err := envkit.SelectFile(".env.prod", envkit.SelectOptions{
    Prefix:      "GITHUB_SECRET_",   // keep only these
    StripPrefix: true,               // …renamed without the prefix
    ExcludeKeys: []string{"GITHUB_SECRET_LOCAL_ONLY"},
    Expand:      true,               // resolve ${references}
})
if err != nil {
    return err
}

for _, p := range sel.Pairs {         // dotenv.Pair{Key, Value}
    fmt.Println(p.Key, "=", p.Value)
}
for _, s := range sel.Skipped {       // guarded out, with the reason
    fmt.Println(s.Name, "skipped:", s.Reason)  // "placeholder" | "empty"
}

names := sel.Names()   // []string, selected names in order
values := sel.Map()    // map[string]string, last wins
```

Resolution order is fixed: explicit `Keys` as given → otherwise all keys → `Prefix` keeps → `ExcludePrefix` drops → `IncludeKeys` adds → `ExcludeKeys` drops → `StripPrefix` renames → the placeholder/empty guards classify. A key you *name* (`Keys` or `IncludeKeys`) that doesn't exist is an error, never a silent skip. Excluding an absent key is fine.

Guards are on by default: values matching `envkit.DefaultPlaceholderPattern` (`^__[A-Z0-9_]+__$`) and empty values land in `Skipped` rather than `Pairs`. Set `IncludePlaceholders` / `IncludeEmpty` to lift them, or `PlaceholderPattern` to use your own convention.

Already holding a parsed file? Use `envkit.Select(f, opts)` and skip the re-parse.

### Discovering environment files

```go
files, err := envkit.Discover(".")        // []envkit.EnvFile
for _, f := range files {
    fmt.Println(f.Env, f.File, f.Contract) // "prod", ".env.prod", false
}

inv, err := envkit.Inventories(envkit.InventoryOptions{Dir: "."})
for _, i := range inv {
    fmt.Printf("%s: %d keys, %d placeholders\n", i.Env, i.Keys, i.Placeholders)
}
```

Non-recursive, filename-based, deterministic order (distance-from-prod). Long forms canonicalize (`.env.production` → `prod`), template files are tagged `Contract`, editor droppings are ignored.

### The drift matrix

```go
m, err := envkit.BuildMatrix(envkit.MatrixOptions{
    Dir:          ".",                 // or Files: []string{...} for explicit paths
    ContractPath: ".env.example",      // optional CI gate
})
if err != nil {
    return err
}

for _, row := range m.Rows {
    for _, cell := range row.Cells {
        fmt.Printf("%s/%s = %s %s\n", row.Key, cell.Env, cell.State, envkit.StateSymbols[cell.State])
    }
}

if m.HasContractGaps() {
    return fmt.Errorf("env files are missing contract keys: %v", m.ContractMissing)
}
```

Cell states are the closed set `present · empty · placeholder · disabled · inherited · missing`. Values are **absent** from the result unless you pass `Reveal: true` — a matrix is the artifact most likely to be pasted into a ticket. `MatrixCell.Raw()` exposes the value for renderers that mask at display time.

Two helpers worth knowing: `envkit.DriftRows(rows)` filters to rows that aren't uniformly present, and `envkit.SparseColumn(m, rowsBefore)` diagnoses the case where one nearly-empty file makes every row look like drift — so you can explain it instead of showing an unchanged table.

### Diffing two files

```go
d, err := envkit.Diff(".env.staging", ".env.prod", envkit.DiffOptions{Expand: true})
if err != nil {
    return err
}
if d.Empty() {
    fmt.Println("configurations match")
}
for _, c := range d.Changed {
    fmt.Printf("%s: %s -> %s\n", c.Key, c.From, c.To)
}
```

Compares the *effective* view (last-wins) of both files — formatting, comments, and shadowed duplicates are invisible on purpose. Both files must exist. `envkit.DiffMaps(a, b, expanded)` gives the same comparison over maps you obtained elsewhere.

### Editing, byte-preservingly

```go
res, err := envkit.Set(".env", []dotenv.Pair{{Key: "DB_HOST", Value: "db.prod"}}, envkit.EditOptions{
    Anchor: "DB_PORT",   // place a NEW key after this one
    DryRun: false,
})
if err != nil {
    return err
}
fmt.Println(res.Changed, res.Written, res.Diff, res.Created)

envkit.Unset(".env", []string{"OLD"}, envkit.EditOptions{})              // comment out (reversible)
envkit.Unset(".env", []string{"OLD"}, envkit.EditOptions{Delete: true})  // remove the line
envkit.Restore(".env", []string{"OLD"}, envkit.EditOptions{})            // the inverse of unset
```

Every byte outside the touched entry survives — comments, blank lines, ordering, quoting style, `=` vs `:`. Setting a key to its current value is a full no-op: `Changed` is false and **the file is not rewritten**, so its mtime is untouched. `DryRun: true` fills `Diff` with the would-be `-old`/`+new` lines and writes nothing. A missing file is created at `0600` by `Set`; `Unset`/`Restore` require it to exist.

Sentinel errors to branch on: `envkit.ErrAnchorNotFound`, `envkit.ErrKeyNotFound`.

### Running a command with the file's values

```go
code, err := envkit.Run(ctx, ".env.test", []string{"go", "test", "./..."}, envkit.RunOptions{
    Base:   os.Environ(),   // nil = the child sees ONLY the file's values
    Stdout: os.Stdout,
    Stderr: os.Stderr,
})
```

`err` is non-nil only for *your* failures (unreadable file, unresolvable reference, unstartable command). A child that runs and exits non-zero returns its `code` with a nil error, so a wrapper can pass it through verbatim. Need just the environment, not the exec? `envkit.ChildEnv(path, base, sel)` returns the merged `[]string` (file wins), and `envkit.MergeEnv(base, values)` does the merge alone.

## providerkit — plugins

`providerkit` is the seam the `github` and `vercel` plugins are built on, and it's fully importable. Two ways to use it:

**Mount our plugins in your own binary:**

```go
deps := providerkit.Deps{
    Printer:     &outfmt.Printer{Out: os.Stdout},
    File:        ".env.prod",
    Source:      func(o providerkit.SelectOpts) (providerkit.Source, error) {
        return envkit.SelectFile(".env.prod", o)
    },
    Runner:      providerkit.ExecRunner{},
    Interactive: func() bool { return true },
    Ask:         myPrompt,
}
myRoot.AddCommand(githubplugin.New().Command(deps))
```

**Or build a plugin for your own backend** — implement as much of the capability set as your store honestly supports, and the kit generates the standard verbs with the shared flags, guards, confirm gate, and output vocabulary:

```go
type myStore struct{ /* … */ }

func (s *myStore) Set(ctx context.Context, name, value string) error { /* … */ }
func (s *myStore) Names(ctx context.Context) ([]string, error)       { /* … */ }
func (s *myStore) Delete(ctx context.Context, name string) error     { /* … */ }

func (p *MyPlugin) Command(deps providerkit.Deps) *cobra.Command {
    store := &myStore{}
    root := &cobra.Command{Use: "mystore"}
    root.AddCommand(
        providerkit.NewPushCmd(deps, store, providerkit.VerbConfig{Gate: store.gate}),
        providerkit.NewListCmd(deps, store),
        providerkit.NewPruneCmd(deps, store, store, providerkit.VerbConfig{Gate: store.gate}),
    )
    return root
}
```

`providerkit.SelectOpts` is an alias for `envkit.SelectOptions` and `providerkit.Source` for `*envkit.Selection` — one grammar, no translation layer. See [Plugins](plugins.md) for the full contract and the exec-plugin alternative (no Go required).

## discover

```go
files, err := discover.Scan("./deploy")     // []discover.EnvFile
ef, ok := discover.Classify(".env.production")  // {Env: "prod", ...}, true
```

## report

Self-contained HTML — inline CSS, no JavaScript, no external requests:

```go
page, err := report.MatrixHTML(report.Matrix{ /* … */ })
page, err := report.DiffHTML(report.Diff{ /* … */ })
os.WriteFile("report.html", page, 0o600)
```

**Masking is structural**: when `Revealed` is false the input structs carry no values and the templates render `report.MaskGlyph`, so a masked page contains no secret bytes at all.

## textdiff

```go
lines := textdiff.Lines(before, after)   // []string of "-old" / "+new"; nil when identical
```

## outfmt

The CLI's output seam, if you want dotenvctl-compatible machine output:

```go
p := &outfmt.Printer{Out: os.Stdout, JSON: true}
p.OK(myPayload)                              // {"ok":true,"data":{…}}
p.Fail(outfmt.CodeNotFound, "key missing")   // {"ok":false,"error":{…}}
p.Human("plain line")                        // silent in JSON mode
```

## Stability

`envkit`, `providerkit`, and the helper packages are the public Go surface — their exported signatures are API, and breaking one is a major-version event. The CLI's *flag* names and `--json` envelope are covered by the same promise. Anything under a `dotenvcmd` unexported identifier is not.
