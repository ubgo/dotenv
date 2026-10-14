# Go API

Everything `dotenvctl` does is callable from Go: import the `cli` module's packages and get the same behaviour with no subprocess.

**On this page:** [Before you start](#before-you-start) · [Packages](#packages) · [envkit — the operations API](#envkit--the-operations-api) · [providerkit — plugins](#providerkit--plugins) · [Mounting the github and vercel plugins](#mounting-the-github-and-vercel-plugins) · [Embedding the CLI](#embedding-the-cli) · [discover](#discover) · [report](#report) · [textdiff](#textdiff) · [outfmt](#outfmt) · [Complete API reference](#complete-api-reference) · [CLI ↔ Go equivalence](#cli--go-equivalence) · [Stability](#stability) · [Related](#related)

The CLI is a thin translator: flags in, an `envkit` call, formatted output. A program that imports these packages skips the translation and gets typed results instead of text.

## Before you start

You need Go 1.26 or newer (the `cli` module's `go` line). Add the module:

```sh
go get github.com/ubgo/dotenv/cli@latest
```

The parser and editor underneath are a separate, dependency-free module: [`github.com/ubgo/dotenv`](../README.md). Use it directly when you only need to read or edit one file. Use `envkit` when you want the higher-level operations: selection, discovery, the drift matrix, diff, edit, run.

Every snippet on this page is a Go `Example` function, compiled and output-checked by `go test`. Each one ends with its real `// Output:` block. To run them yourself from a clone:

```sh
cd cli && go test -run Example ./envkit/ ./providerkit/ ./discover/ ./report/ ./textdiff/ ./outfmt/ ./plugins/...
```

```text
ok  	github.com/ubgo/dotenv/cli/envkit	0.012s
ok  	github.com/ubgo/dotenv/cli/providerkit	0.008s
ok  	github.com/ubgo/dotenv/cli/discover	0.006s
ok  	github.com/ubgo/dotenv/cli/report	0.007s
ok  	github.com/ubgo/dotenv/cli/textdiff	0.004s
ok  	github.com/ubgo/dotenv/cli/outfmt	0.004s
ok  	github.com/ubgo/dotenv/cli/plugins/githubplugin	0.007s
ok  	github.com/ubgo/dotenv/cli/plugins/vercelplugin	0.006s
```

## Packages

| Package | What it gives you |
|---|---|
| [`envkit`](#envkit--the-operations-api) | the operations: select, discover, matrix, diff, edit, run |
| [`providerkit`](#providerkit--plugins) | the plugin seam: build your own secret-store integration, or mount ours |
| [`plugins/githubplugin`, `plugins/vercelplugin`](#mounting-the-github-and-vercel-plugins) | the two built-in plugins, mountable under any cobra root |
| [`dotenvcmd`](#embedding-the-cli) | the whole CLI as one function, `Execute` |
| [`discover`](#discover) | env-file family detection for a directory |
| [`report`](#report) | self-contained HTML reports for matrix and diff |
| [`textdiff`](#textdiff) | the minimal line diff used by `--dry-run` |
| [`outfmt`](#outfmt) | the `{ok,data|error}` envelope printer, for dotenvctl-compatible output |

## envkit — the operations API

The contract for everything in `envkit`: typed options in, typed results out. No printing, no `os.Exit`, no cobra. Errors are plain wrapped errors, so inspect them with `errors.Is` / `errors.As`. Like the library beneath it, **nothing in envkit reads or writes `os.Environ`**.

### Selecting keys

`SelectFile` applies the selection grammar the CLI exposes as `--prefix` / `--exclude-prefix` / `--include` / `--exclude` / `--strip-prefix`. You want it whenever you push a subset of a file somewhere else. It returns the kept pairs and, separately, the keys a guard held back with the reason.

<!-- snippet: cli/envkit/example_test.go#api_envkit_SelectFile -->
```go
// ExampleSelectFile picks the deploy secrets out of a file: keep one prefix,
// rename without it, drop one key by name, resolve ${references}. Guarded
// values (placeholders, empties) land in Skipped with the reason.
func ExampleSelectFile() {
	dir, err := apiEnvkitDir(map[string]string{".env.prod": `HOST=db.internal
GITHUB_SECRET_DB_URL=postgres://${HOST}/app
GITHUB_SECRET_API_KEY=__FILL_ME__
GITHUB_SECRET_SENTRY=
GITHUB_SECRET_LOCAL_ONLY=dev-token
LOG_LEVEL=info
`})
	if err != nil {
		fmt.Println("setup:", err)
		return
	}
	defer func() {
		// Cleanup is best-effort; a failure goes to stderr so the example's
		// checked stdout output is unaffected.
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
		}
	}()

	sel, err := envkit.SelectFile(filepath.Join(dir, ".env.prod"), envkit.SelectOptions{
		Prefix:      "GITHUB_SECRET_",
		StripPrefix: true,
		ExcludeKeys: []string{"GITHUB_SECRET_LOCAL_ONLY"}, // original name, before the strip
		Expand:      true,
	})
	if err != nil {
		fmt.Println("select:", err)
		return
	}
	for _, p := range sel.Pairs {
		fmt.Printf("pair    %s=%s\n", p.Key, p.Value)
	}
	for _, s := range sel.Skipped {
		fmt.Printf("skipped %s (%s)\n", s.Name, s.Reason)
	}
	fmt.Println("names:", sel.Names())
	// Output:
	// pair    DB_URL=postgres://db.internal/app
	// skipped API_KEY (placeholder)
	// skipped SENTRY (empty)
	// names: [DB_URL]
}
```

The resolution order is fixed: explicit `Keys` as given → otherwise all keys → `Prefix` keeps → `ExcludePrefix` drops → `IncludeKeys` adds → `ExcludeKeys` drops → `StripPrefix` renames → the placeholder and empty guards classify.

The guards are on by default. Values matching `envkit.DefaultPlaceholderPattern` (`^__[A-Z0-9_]+__$`) and empty values land in `Skipped`, not `Pairs`. Set `IncludePlaceholders` / `IncludeEmpty` to lift them, or `PlaceholderPattern` to use your own convention.

Already holding a parsed file? `Select` takes the `*dotenv.File` and skips the re-parse:

<!-- snippet: cli/envkit/example_test.go#api_envkit_Select -->
```go
// ExampleSelect runs the same grammar over a file you already parsed, so it
// is not read twice. A key you NAME that does not exist is an error, never a
// silent skip.
func ExampleSelect() {
	f := dotenv.Parse("A=1\nB=2\nC=3\n")

	sel, err := envkit.Select(f, envkit.SelectOptions{Keys: []string{"C", "A"}})
	if err != nil {
		fmt.Println("select:", err)
		return
	}
	fmt.Println(sel.Names(), sel.Map()["C"])

	_, err = envkit.Select(f, envkit.SelectOptions{Keys: []string{"MISSING"}})
	fmt.Println(err)
	// Output:
	// [C A] 3
	// envkit: key "MISSING" not found
}
```

**Pitfalls**

- A key you *name* (`Keys` or `IncludeKeys`) that does not exist is an error, never a silent skip. Excluding an absent key is fine.
- `ExcludeKeys` matches the original names, before `StripPrefix` renames them.
- Explicit `Keys` bypass `Prefix` and `ExcludePrefix`, but `StripPrefix` still renames any of them that carry the prefix.
- `StripPrefix` does nothing without `Prefix`.
- Stripping can produce two pairs with one name: `Prefix: "APP_"` with `StripPrefix` and `IncludeKeys: []string{"DB"}` turns both `APP_DB` and `DB` into `DB`. `Pairs` holds both; `Map()` keeps the last. Pick names that cannot collide.
- `Expand` is false in the zero value. The CLI's push verbs default `--expand` to on; a Go caller must set it.

Examples: [`ExampleSelectFile`](../cli/envkit/example_test.go), [`ExampleSelect`](../cli/envkit/example_test.go).

### Discovering environment files

`Discover` lists the env-file family of one directory. `Inventories` does the same and summarizes each file: active keys, commented-out settings, name-only declarations and placeholder values. This is the `dotenvctl envs` view as data.

<!-- snippet: cli/envkit/example_test.go#api_envkit_Discover -->
```go
// ExampleDiscover lists a directory's env-file family in distance-from-prod
// order. Long names collapse (.env.production is "prod"), templates are
// tagged Contract, and editor droppings like .env.bak are ignored.
func ExampleDiscover() {
	dir, err := apiEnvkitDir(map[string]string{
		".env":            "A=1\n",
		".env.production": "A=1\n",
		".env.staging":    "A=1\n",
		".env.example":    "A=\n",
		".env.bak":        "A=old\n",
	})
	if err != nil {
		fmt.Println("setup:", err)
		return
	}
	defer func() {
		// Cleanup is best-effort; a failure goes to stderr so the example's
		// checked stdout output is unaffected.
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
		}
	}()

	files, err := envkit.Discover(dir)
	if err != nil {
		fmt.Println("discover:", err)
		return
	}
	for _, f := range files {
		fmt.Printf("%-16s env=%-8s contract=%t\n", f.File, f.Env, f.Contract)
	}
	// Output:
	// .env             env=default  contract=false
	// .env.staging     env=stag     contract=false
	// .env.production  env=prod     contract=false
	// .env.example     env=example  contract=true
}
```

<!-- snippet: cli/envkit/example_test.go#api_envkit_Inventories -->
```go
// ExampleInventories summarizes each discovered file: active keys,
// commented-out settings, name-only declarations, and placeholder values.
func ExampleInventories() {
	dir, err := apiEnvkitDir(map[string]string{
		".env":      "A=1\nB=__TODO__\n# C=3\nPATH\n",
		".env.prod": "A=1\nB=real\nC=3\n",
	})
	if err != nil {
		fmt.Println("setup:", err)
		return
	}
	defer func() {
		// Cleanup is best-effort; a failure goes to stderr so the example's
		// checked stdout output is unaffected.
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
		}
	}()

	inv, err := envkit.Inventories(envkit.InventoryOptions{Dir: dir})
	if err != nil {
		fmt.Println("inventories:", err)
		return
	}
	for _, i := range inv {
		fmt.Printf("%s: %d keys, %d disabled, %d inherited, %d placeholders\n",
			i.Env, i.Keys, i.Disabled, i.Inherited, i.Placeholders)
	}
	// Output:
	// default: 2 keys, 1 disabled, 1 inherited, 1 placeholders
	// prod: 3 keys, 0 disabled, 0 inherited, 0 placeholders
}
```

`EnvNameForPath` labels any path with the same vocabulary, and falls back to the basename for files outside the `.env` family:

<!-- snippet: cli/envkit/example_test.go#api_envkit_EnvNameForPath -->
```go
// ExampleEnvNameForPath labels an explicit path with discovery's vocabulary,
// falling back to the basename for files outside the .env family.
func ExampleEnvNameForPath() {
	for _, p := range []string{"ops/.env.production", ".env", "ops/.env.example", "config/app.env"} {
		fmt.Printf("%s -> %s\n", p, envkit.EnvNameForPath(p))
	}
	// Output:
	// ops/.env.production -> prod
	// .env -> default
	// ops/.env.example -> example
	// config/app.env -> app.env
}
```

**Pitfalls**

- Discovery is non-recursive and reads filenames only. A `deploy/` subfolder is not scanned; pass it as the directory.
- An empty directory gives an empty slice and no error. A missing directory is an error.
- `Inventory.Keys` counts distinct active keys, so a key set twice counts once.

Examples: [`ExampleDiscover`](../cli/envkit/example_test.go), [`ExampleInventories`](../cli/envkit/example_test.go), [`ExampleEnvNameForPath`](../cli/envkit/example_test.go).

### The drift matrix

`BuildMatrix` compares env files key by key: one row per key, one column per environment. Pass `Dir` to discover the columns, or `Files` for explicit paths. Add `ContractPath` to check every environment against a template such as `.env.example`; that is the CI-gate question `HasContractGaps` answers.

<!-- snippet: cli/envkit/example_test.go#api_envkit_BuildMatrix -->
```go
// ExampleBuildMatrix compares every env file in a directory key by key and
// checks them against a contract. Values stay out of the result unless you
// pass Reveal.
func ExampleBuildMatrix() {
	dir, err := apiEnvkitDir(map[string]string{
		".env.example": "DB_URL=\nAPI_KEY=\nSENTRY_DSN=\n",
		".env.stag":    "DB_URL=postgres://stag\nAPI_KEY=__FILL_ME__\n# SENTRY_DSN=x\n",
		".env.prod":    "DB_URL=postgres://prod\nAPI_KEY=k-123\n",
	})
	if err != nil {
		fmt.Println("setup:", err)
		return
	}
	defer func() {
		// Cleanup is best-effort; a failure goes to stderr so the example's
		// checked stdout output is unaffected.
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
		}
	}()

	m, err := envkit.BuildMatrix(envkit.MatrixOptions{
		Dir:          dir, // contract files are never columns
		ContractPath: filepath.Join(dir, ".env.example"),
	})
	if err != nil {
		fmt.Println("matrix:", err)
		return
	}
	fmt.Println("columns:", m.Envs)
	for _, row := range m.Rows {
		fmt.Printf("%-10s", row.Key)
		for _, c := range row.Cells {
			fmt.Printf(" %s=%s%s", c.Env, envkit.StateSymbols[c.State], c.State)
		}
		fmt.Println()
	}
	fmt.Println("value hidden:", m.Rows[1].Cells[0].Value == "")
	fmt.Println("contract gaps:", m.HasContractGaps(), m.ContractMissing["prod"])
	// Output:
	// columns: [stag prod]
	// API_KEY    stag=!placeholder prod=✓present
	// DB_URL     stag=✓present prod=✓present
	// SENTRY_DSN stag=#disabled prod=—missing
	// value hidden: true
	// contract gaps: true [SENTRY_DSN]
}
```

Cell states are the closed set `present · empty · placeholder · disabled · inherited · missing`, and `envkit.StateSymbols` maps each to the glyph every renderer uses. Values are **absent** from the result unless you pass `Reveal: true`, because a matrix is the artifact most likely to be pasted into a ticket. `MatrixCell.Raw()` returns the value anyway, for renderers that mask at display time; treat it as a secret.

#### Filtering to drift

`DriftRows` keeps the rows that are not `present` everywhere. `MatrixOptions.OnlyDrift` applies it for you.

<!-- snippet: cli/envkit/example_test.go#api_envkit_DriftRows -->
```go
// ExampleDriftRows keeps only rows with at least one cell that is not
// "present". It filters IN PLACE, so assign the result back and do not keep
// using the old slice.
func ExampleDriftRows() {
	rows := []envkit.MatrixRow{
		{Key: "SAME", Cells: []envkit.MatrixCell{{Env: "stag", State: envkit.StatePresent}, {Env: "prod", State: envkit.StatePresent}}},
		{Key: "GAP", Cells: []envkit.MatrixCell{{Env: "stag", State: envkit.StatePresent}, {Env: "prod", State: envkit.StateMissing}}},
	}
	rows = envkit.DriftRows(rows)
	for _, r := range rows {
		fmt.Println(r.Key)
	}
	// Output:
	// GAP
}
```

A missing cell counts as drift. So one nearly empty file (a bare `.env` with one key, say) makes every row look like drift, and the filter seems to do nothing. `SparseColumn` names the column to blame, so you can explain it instead of showing an unchanged table:

<!-- snippet: cli/envkit/example_test.go#api_envkit_SparseColumn -->
```go
// ExampleSparseColumn shows why a drift filter can look broken: one nearly
// empty file makes every row "drift". Build the full matrix, remember the
// row count, filter with DriftRows, then ask SparseColumn which column is
// to blame.
func ExampleSparseColumn() {
	dir, err := apiEnvkitDir(map[string]string{
		".env":      "A=1\n",
		".env.stag": "A=1\nB=2\nC=3\nD=4\nE=5\nF=6\nG=7\nH=8\nI=9\nJ=10\n",
		".env.prod": "A=1\nB=2\nC=3\nD=4\nE=5\nF=6\nG=7\nH=8\nI=9\nJ=10\n",
	})
	if err != nil {
		fmt.Println("setup:", err)
		return
	}
	defer func() {
		// Cleanup is best-effort; a failure goes to stderr so the example's
		// checked stdout output is unaffected.
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
		}
	}()

	m, err := envkit.BuildMatrix(envkit.MatrixOptions{Dir: dir})
	if err != nil {
		fmt.Println("matrix:", err)
		return
	}
	rowsBefore := len(m.Rows)
	m.Rows = envkit.DriftRows(m.Rows) // reuses m.Rows' backing array
	fmt.Printf("rows: %d before, %d after the drift filter\n", rowsBefore, len(m.Rows))

	if env, missing, ok := envkit.SparseColumn(m, rowsBefore); ok {
		fmt.Printf("column %q is missing %d of %d keys\n", env, missing, len(m.Rows))
	}
	// Output:
	// rows: 10 before, 9 after the drift filter
	// column "default" is missing 9 of 9 keys
}
```

#### Unique column labels

`.env.stag` and `.env.staging` both classify as `stag`, and two folders' `.env` files both classify as `default`. A lookup keyed on that label would silently show one file's data under both columns. `DisambiguateColumns` makes every label unique: a collided label falls back to the filename, then to the full path. `BuildMatrix` already calls it; call it yourself when you build your own column list.

<!-- snippet: cli/envkit/example_test.go#api_envkit_DisambiguateColumns -->
```go
// ExampleDisambiguateColumns makes column labels unique. .env.stag and
// .env.staging both classify as "stag"; two directories' .env files both
// classify as "default". Labels fall back to the filename, then the path.
func ExampleDisambiguateColumns() {
	paths := []string{"deploy/.env.stag", "deploy/.env.staging", "api/.env", "web/.env", "deploy/.env.prod"}
	cols := make([]discover.EnvFile, 0, len(paths))
	for _, p := range paths {
		cols = append(cols, discover.EnvFile{Path: p, File: filepath.Base(p), Env: envkit.EnvNameForPath(p)})
	}
	for _, c := range envkit.DisambiguateColumns(cols) {
		fmt.Println(c.Env)
	}
	// Output:
	// .env.stag
	// .env.staging
	// api/.env
	// web/.env
	// prod
}
```

**Pitfalls**

- Fewer than two columns is an error (`envkit: matrix needs at least two env files, found 1`). A one-column matrix is a list.
- With `Dir`, contract files (`.env.example` and kin) are never columns. Pass them as `ContractPath`.
- `DriftRows` filters **in place**: it reuses the input slice's backing array. Assign the result back (`m.Rows = envkit.DriftRows(m.Rows)`) and do not read the old slice afterwards.
- `SparseColumn` returns `ok=false` when the filter hid more than 10% of rows, because then the filter worked. It needs the row count from *before* filtering, so build without `OnlyDrift`, record `len(m.Rows)`, then filter.
- After disambiguation, `Matrix.Envs` may hold filenames or paths (`.env.stag`, `api/.env`) instead of environment names. Code that looks columns up by `"stag"` must allow for that.

Examples: [`ExampleBuildMatrix`](../cli/envkit/example_test.go), [`ExampleDriftRows`](../cli/envkit/example_test.go), [`ExampleSparseColumn`](../cli/envkit/example_test.go), [`ExampleDisambiguateColumns`](../cli/envkit/example_test.go).

### Diffing two files

`Diff` compares the *effective* view (last-wins) of two files: the configuration a program would see. Formatting, comments and shadowed duplicates are invisible on purpose; `diff(1)` is for bytes. With `Expand: true`, `${HOST}` and the literal host compare equal.

<!-- snippet: cli/envkit/example_test.go#api_envkit_Diff -->
```go
// ExampleDiff compares the effective (last-wins) view of two files. With
// Expand, `${HOST}` and the literal host compare equal.
func ExampleDiff() {
	dir, err := apiEnvkitDir(map[string]string{
		".env.stag": "HOST=db\nDB_URL=postgres://${HOST}/app\nDEBUG=true\nPOOL=5\n",
		".env.prod": "# production\nDB_URL=postgres://db/app\nPOOL=5\nPOOL=20\nSENTRY=on\n",
	})
	if err != nil {
		fmt.Println("setup:", err)
		return
	}
	defer func() {
		// Cleanup is best-effort; a failure goes to stderr so the example's
		// checked stdout output is unaffected.
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
		}
	}()

	d, err := envkit.Diff(filepath.Join(dir, ".env.stag"), filepath.Join(dir, ".env.prod"), envkit.DiffOptions{Expand: true})
	if err != nil {
		fmt.Println("diff:", err)
		return
	}
	fmt.Println("empty:", d.Empty())
	for _, p := range d.Added {
		fmt.Println("+", p.Key)
	}
	for _, p := range d.Removed {
		fmt.Println("-", p.Key)
	}
	for _, c := range d.Changed {
		fmt.Printf("~ %s: %s -> %s\n", c.Key, c.From, c.To)
	}
	// Output:
	// empty: false
	// + SENTRY
	// - DEBUG
	// - HOST
	// ~ POOL: 5 -> 20
}
```

`DiffMaps` gives the same comparison over maps you got elsewhere, such as a remote store or a template:

<!-- snippet: cli/envkit/example_test.go#api_envkit_DiffMaps -->
```go
// ExampleDiffMaps runs the same comparison over maps you got elsewhere —
// a remote store, a template. The three slices are never nil.
func ExampleDiffMaps() {
	local := map[string]string{"A": "1", "B": "2"}
	remote := map[string]string{"A": "1", "B": "2"}

	d := envkit.DiffMaps(local, remote, false)
	fmt.Println(d.Empty(), d.Added != nil, len(d.Changed))
	// Output:
	// true true 0
}
```

**Pitfalls**

- Both files must exist. A missing file would read as "everything was removed", so it is an error instead.
- `DiffResult` carries the real values (`Added[].Value`, `Changed[].From` / `.To`). Unlike the matrix there is no reveal switch; mask them yourself before you print or log a diff.
- `Added`, `Removed` and `Changed` are never nil and are sorted by key, so JSON output never shows `null` and is itself diffable.

Examples: [`ExampleDiff`](../cli/envkit/example_test.go), [`ExampleDiffMaps`](../cli/envkit/example_test.go).

### Editing, byte-preservingly

`Set`, `Unset` and `Restore` change one entry and keep every other byte of the file: comments, blank lines, order, quoting style, `=` vs `:`.

<!-- snippet: cli/envkit/example_test.go#api_envkit_Set -->
```go
// ExampleSet upserts a key without disturbing the rest of the file. A dry
// run returns the diff and writes nothing; setting a key to its current value
// is a full no-op.
func ExampleSet() {
	dir, err := apiEnvkitDir(map[string]string{".env": "# database\nDB_PORT=5432\n"})
	if err != nil {
		fmt.Println("setup:", err)
		return
	}
	defer func() {
		// Cleanup is best-effort; a failure goes to stderr so the example's
		// checked stdout output is unaffected.
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
		}
	}()
	path := filepath.Join(dir, ".env")
	pairs := []dotenv.Pair{{Key: "DB_HOST", Value: "db.prod"}}

	preview, err := envkit.Set(path, pairs, envkit.EditOptions{Anchor: "DB_PORT", DryRun: true})
	if err != nil {
		fmt.Println("set:", err)
		return
	}
	fmt.Println("dry run:", preview.Diff, "written:", preview.Written)

	res, err := envkit.Set(path, pairs, envkit.EditOptions{Anchor: "DB_PORT"})
	if err != nil {
		fmt.Println("set:", err)
		return
	}
	fmt.Println("created:", res.Created, "written:", res.Written)

	again, err := envkit.Set(path, pairs, envkit.EditOptions{})
	if err != nil {
		fmt.Println("set:", err)
		return
	}
	fmt.Println("second call changed:", again.Changed)

	_, err = envkit.Set(path, []dotenv.Pair{{Key: "X", Value: "1"}}, envkit.EditOptions{Anchor: "NOPE"})
	fmt.Println("bad anchor:", errors.Is(err, envkit.ErrAnchorNotFound))

	body, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("read:", err)
		return
	}
	fmt.Print(string(body))
	// Output:
	// dry run: [+DB_HOST=db.prod] written: false
	// created: [DB_HOST] written: true
	// second call changed: false
	// bad anchor: true
	// # database
	// DB_PORT=5432
	// DB_HOST=db.prod
}
```

`Unset` comments a key out by default, which `Restore` can undo; `Delete: true` removes the line instead.

<!-- snippet: cli/envkit/example_test.go#api_envkit_Unset -->
```go
// ExampleUnset comments a key out by default (reversible with Restore), or
// removes the line with Delete. Naming an absent key is ErrKeyNotFound.
func ExampleUnset() {
	dir, err := apiEnvkitDir(map[string]string{".env": "A=1\nOLD=x\nGONE=y\n"})
	if err != nil {
		fmt.Println("setup:", err)
		return
	}
	defer func() {
		// Cleanup is best-effort; a failure goes to stderr so the example's
		// checked stdout output is unaffected.
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
		}
	}()
	path := filepath.Join(dir, ".env")

	if _, err := envkit.Unset(path, []string{"OLD"}, envkit.EditOptions{}); err != nil {
		fmt.Println("unset:", err)
		return
	}
	if _, err := envkit.Unset(path, []string{"GONE"}, envkit.EditOptions{Delete: true}); err != nil {
		fmt.Println("unset:", err)
		return
	}
	_, err = envkit.Unset(path, []string{"NEVER"}, envkit.EditOptions{})
	fmt.Println("absent key:", errors.Is(err, envkit.ErrKeyNotFound))

	body, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("read:", err)
		return
	}
	fmt.Print(string(body))
	// Output:
	// absent key: true
	// A=1
	// # OLD=x
}
```

<!-- snippet: cli/envkit/example_test.go#api_envkit_Restore -->
```go
// ExampleRestore re-activates a commented-out setting, removing exactly the
// marker that was there.
func ExampleRestore() {
	dir, err := apiEnvkitDir(map[string]string{".env": "A=1\n#OLD=x\n"})
	if err != nil {
		fmt.Println("setup:", err)
		return
	}
	defer func() {
		// Cleanup is best-effort; a failure goes to stderr so the example's
		// checked stdout output is unaffected.
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
		}
	}()
	path := filepath.Join(dir, ".env")

	res, err := envkit.Restore(path, []string{"OLD"}, envkit.EditOptions{})
	if err != nil {
		fmt.Println("restore:", err)
		return
	}
	fmt.Println("diff:", res.Diff)
	// Output:
	// diff: [-#OLD=x +OLD=x]
}
```

**Pitfalls**

- Setting a key to its current value is a full no-op: `Changed` is false and **the file is not rewritten**, so its mtime does not move.
- `Anchor` places only a *new* key. An existing key is updated where it is and never moved.
- A missing file is created at mode `0600` by `Set`. `Unset` and `Restore` need the file to exist.
- `DryRun: true` fills `Diff` and writes nothing; the real run computes the same diff.
- Branch on `envkit.ErrAnchorNotFound` and `envkit.ErrKeyNotFound` with `errors.Is`.

Examples: [`ExampleSet`](../cli/envkit/example_test.go), [`ExampleUnset`](../cli/envkit/example_test.go), [`ExampleRestore`](../cli/envkit/example_test.go).

### Running a command with the file's values

`Run` executes a command with the file's values in its environment and returns the child's exit code. It is `dotenvctl run` as a function.

<!-- snippet: cli/envkit/example_test.go#api_envkit_Run -->
```go
// ExampleRun executes a command with the file's values. A nil Base gives a
// hermetic child that sees ONLY the file. A child that exits non-zero returns
// its code with a nil error, so a wrapper can pass it through.
func ExampleRun() {
	dir, err := apiEnvkitDir(map[string]string{".env.test": "NAME=world\nGREETING=hello ${NAME}\n"})
	if err != nil {
		fmt.Println("setup:", err)
		return
	}
	defer func() {
		// Cleanup is best-effort; a failure goes to stderr so the example's
		// checked stdout output is unaffected.
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
		}
	}()
	path := filepath.Join(dir, ".env.test")
	ctx := context.Background()

	code, err := envkit.Run(ctx, path, []string{"sh", "-c", `echo "$GREETING"`}, envkit.RunOptions{Stdout: os.Stdout})
	fmt.Println("exit:", code, "err:", err)

	code, err = envkit.Run(ctx, path, []string{"sh", "-c", "exit 3"}, envkit.RunOptions{})
	fmt.Println("exit:", code, "err:", err)
	// Output:
	// hello world
	// exit: 0 err: <nil>
	// exit: 3 err: <nil>
}
```

Need only the environment, not the exec? `ChildEnv` returns the merged `[]string` (file wins), and `MergeEnv` does the merge alone:

<!-- snippet: cli/envkit/example_test.go#api_envkit_ChildEnv -->
```go
// ExampleChildEnv builds the environment a child would get without running
// anything. The file wins over the base, and the overridden base entry is
// removed rather than duplicated.
func ExampleChildEnv() {
	dir, err := apiEnvkitDir(map[string]string{".env": "HOME_DIR=/srv\nMODE=prod\n"})
	if err != nil {
		fmt.Println("setup:", err)
		return
	}
	defer func() {
		// Cleanup is best-effort; a failure goes to stderr so the example's
		// checked stdout output is unaffected.
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
		}
	}()

	env, err := envkit.ChildEnv(filepath.Join(dir, ".env"), []string{"MODE=dev", "TERM=xterm"}, nil)
	if err != nil {
		fmt.Println("child env:", err)
		return
	}
	slices.Sort(env) // the file's values are appended in map order
	fmt.Println(strings.Join(env, " "))
	// Output:
	// HOME_DIR=/srv MODE=prod TERM=xterm
}
```

<!-- snippet: cli/envkit/example_test.go#api_envkit_MergeEnv -->
```go
// ExampleMergeEnv is the merge alone, for values you already hold.
func ExampleMergeEnv() {
	env := envkit.MergeEnv([]string{"A=base", "B=keep"}, map[string]string{"A": "file"})
	fmt.Println(env)
	// Output:
	// [B=keep A=file]
}
```

**Pitfalls**

- `err` is non-nil only for *your* failures: an unreadable file, an unresolvable reference, a command that cannot start. A child that runs and exits non-zero returns its code with a nil error.
- A nil `Base` means the child sees **only** the file's values, with no `PATH` or `HOME`. The command itself is still found through your process's `PATH`, but anything the child starts may not be. Pass `os.Environ()` for the usual inherit behaviour.
- A nil `Select` means "every key, expanded". A non-nil `Select` is used as given, and its zero `Expand` is false, so `${refs}` then reach the child unresolved. Set `Expand: true` when you narrow the selection.
- `MergeEnv` and `ChildEnv` append the file's values in map order. Sort the slice if you compare or print it.
- Nil `Stdin` / `Stdout` / `Stderr` mean the parent's streams.

Examples: [`ExampleRun`](../cli/envkit/example_test.go), [`ExampleChildEnv`](../cli/envkit/example_test.go), [`ExampleMergeEnv`](../cli/envkit/example_test.go).

## providerkit — plugins

`providerkit` is the seam the `github` and `vercel` plugins are built on, and you can build your own on it. A plugin is one method, `Command(Deps) *cobra.Command`. The kit then generates the standard verbs (`push`, `list`, `prune`) with the shared flags, the placeholder and empty guards, the confirm gate and the output vocabulary. Your code only talks to the backend.

`providerkit.SelectOpts` is an alias for `envkit.SelectOptions` and `providerkit.Source` for `*envkit.Selection`: one grammar, no translation layer. See [Plugins](plugins.md) for the full contract and the exec-plugin alternative (no Go needed).

### Step 1 — implement the capabilities your store has

A backend implements only what it truly supports: `SecretWriter` (`Set`), `SecretLister` (`Names`, names only) and `SecretDeleter` (`Delete`). The examples on this page use this in-memory store:

<!-- snippet: cli/providerkit/example_test.go#api_providerkit_store -->
```go
// apiKitStore is an in-memory secret store implementing all three capability
// interfaces — the shape a real backend adapter takes, minus the network.
type apiKitStore struct {
	values map[string]string
}

// Set implements providerkit.SecretWriter.
func (s *apiKitStore) Set(_ context.Context, name, value string) error {
	s.values[name] = value
	return nil
}

// Names implements providerkit.SecretLister: names only, never values.
func (s *apiKitStore) Names(context.Context) ([]string, error) {
	names := make([]string, 0, len(s.values))
	for n := range s.values {
		names = append(names, n)
	}
	slices.Sort(names)
	return names, nil
}

// Delete implements providerkit.SecretDeleter.
func (s *apiKitStore) Delete(_ context.Context, name string) error {
	delete(s.values, name)
	return nil
}
```

Driving it through the interfaces directly is what the kit's verbs do for you:

<!-- snippet: cli/providerkit/example_test.go#api_providerkit_SecretWriter -->
```go
// ExampleSecretWriter drives a store through the three capability
// interfaces directly — what the kit's verbs do for you. Implement only the
// interfaces your backend truly supports; the kit builds verbs for those.
func ExampleSecretWriter() {
	store := &apiKitStore{values: map[string]string{}}
	ctx := context.Background()

	var w providerkit.SecretWriter = store
	var l providerkit.SecretLister = store
	var d providerkit.SecretDeleter = store

	for _, name := range []string{"B_KEY", "A_KEY"} {
		if err := w.Set(ctx, name, "value"); err != nil {
			fmt.Println("set:", err)
			return
		}
	}
	if err := d.Delete(ctx, "B_KEY"); err != nil {
		fmt.Println("delete:", err)
		return
	}
	names, err := l.Names(ctx)
	if err != nil {
		fmt.Println("names:", err)
		return
	}
	fmt.Println(names)
	// Output:
	// [A_KEY]
}
```

### Step 2 — generate the verbs

`NewPushCmd` builds `push [KEY...]` over a `SecretWriter`. Run non-interactively without `--yes` and it refuses; with `--dry-run` it reports and writes nothing.

<!-- snippet: cli/providerkit/example_test.go#api_providerkit_NewPushCmd -->
```go
// ExampleNewPushCmd generates a standard `push` verb over your store. The kit
// owns selection, the placeholder/empty guards, the confirm gate and the
// output; your store only writes. Non-interactive runs need --yes.
func ExampleNewPushCmd() {
	store := &apiKitStore{values: map[string]string{}}
	src := &envkit.Selection{
		Pairs:   []dotenv.Pair{{Key: "DB_URL", Value: "postgres://db/app"}},
		Skipped: []envkit.Skip{{Name: "API_KEY", Reason: envkit.SkipPlaceholder}},
	}
	push := func() *cobra.Command {
		return providerkit.NewPushCmd(apiKitDeps(src), store, providerkit.VerbConfig{})
	}

	fmt.Println("exit:", apiKitRun(push()))
	fmt.Println("exit:", apiKitRun(push(), "--dry-run"))
	fmt.Println("exit:", apiKitRun(push(), "--yes"))
	fmt.Println("stored:", store.values)
	// Output:
	// dotenvctl: refusing to write without --yes in non-interactive mode
	// exit: 2
	// API_KEY: skipped-placeholder
	// DB_URL: would-push
	// exit: 0
	// API_KEY: skipped-placeholder
	// DB_URL: pushed
	// exit: 0
	// stored: map[DB_URL:postgres://db/app]
}
```

`NewListCmd` lists remote names, sorted:

<!-- snippet: cli/providerkit/example_test.go#api_providerkit_NewListCmd -->
```go
// ExampleNewListCmd lists remote NAMES, sorted. The kit never asks a backend
// for values.
func ExampleNewListCmd() {
	store := &apiKitStore{values: map[string]string{"B_KEY": "x", "A_KEY": "y"}}
	fmt.Println("exit:", apiKitRun(providerkit.NewListCmd(apiKitDeps(&envkit.Selection{}), store)))
	// Output:
	// A_KEY
	// B_KEY
	// exit: 0
}
```

`NewPruneCmd` deletes remote names that are not in the local selection. It needs both a lister and a deleter. A key the selection guarded out (a placeholder) and a name passed to `--keep` are spared, and the output says why:

<!-- snippet: cli/providerkit/example_test.go#api_providerkit_NewPruneCmd -->
```go
// ExampleNewPruneCmd deletes remote names that are not in the local
// selection. A name the selection guarded out (a placeholder) and a name
// passed to --keep are spared, and the output says why.
func ExampleNewPruneCmd() {
	store := &apiKitStore{values: map[string]string{
		"DB_URL": "live", "API_KEY": "live", "LEGACY": "live", "BUNDLE": "live",
	}}
	src := &envkit.Selection{
		Pairs:   []dotenv.Pair{{Key: "DB_URL", Value: "postgres://db/app"}},
		Skipped: []envkit.Skip{{Name: "API_KEY", Reason: envkit.SkipPlaceholder}},
	}
	prune := providerkit.NewPruneCmd(apiKitDeps(src), store, store, providerkit.VerbConfig{})
	fmt.Println("exit:", apiKitRun(prune, "--keep", "BUNDLE", "--yes"))

	remaining, err := store.Names(context.Background())
	if err != nil {
		fmt.Println("names:", err)
		return
	}
	fmt.Println("remaining:", remaining)
	// Output:
	// API_KEY: kept-skipped
	// BUNDLE: kept-keep-flag
	// LEGACY: deleted
	// exit: 0
	// remaining: [API_KEY BUNDLE DB_URL]
}
```

The per-name outcomes are the closed `Action` set: `pushed`, `would-push`, `skipped-placeholder`, `skipped-empty`, `deleted`, `would-delete`, `kept-skipped`, `kept-keep-flag`. `SkipAction` maps a selection guard's reason onto it:

<!-- snippet: cli/providerkit/example_test.go#api_providerkit_SkipAction -->
```go
// ExampleSkipAction maps why the selection held a key back onto what a push
// reports for it.
func ExampleSkipAction() {
	fmt.Println(providerkit.SkipAction(envkit.SkipPlaceholder))
	fmt.Println(providerkit.SkipAction(envkit.SkipEmpty))
	// Output:
	// skipped-placeholder
	// skipped-empty
}
```

### Step 3 — add a gate and metadata with VerbConfig

`VerbConfig.Gate` runs after selection and before any write, dry runs included. Print where the write will land (target, account), then call `Confirm`. `VerbConfig.Meta` adds that context to the `--json` payload, so scripts see where actions landed.

<!-- snippet: cli/providerkit/example_test.go#api_providerkit_VerbConfig -->
```go
// ExampleVerbConfig adds a Gate (print where the write lands, then apply the
// standard Confirm contract) and Meta (resolution context for the --json
// payload) to a kit verb.
func ExampleVerbConfig() {
	store := &apiKitStore{values: map[string]string{}}
	src := &envkit.Selection{Pairs: []dotenv.Pair{{Key: "TOKEN", Value: "t-1"}}}

	cfg := providerkit.VerbConfig{
		Gate: func(_ context.Context, deps providerkit.Deps, info providerkit.GateInfo) error {
			deps.Printer.Humanf("target : memory store")
			deps.Printer.Humanf("keys   : %s (dry run: %t)", strings.Join(info.Names, ","), info.DryRun)
			return providerkit.Confirm(deps, info)
		},
		Meta: func(context.Context) (map[string]string, error) {
			return map[string]string{"target": "memory"}, nil
		},
	}

	human := apiKitDeps(src)
	fmt.Println("exit:", apiKitRun(providerkit.NewPushCmd(human, store, cfg), "--dry-run"))

	machine := apiKitDeps(src)
	machine.Printer.JSON = true
	fmt.Println("exit:", apiKitRun(providerkit.NewPushCmd(machine, store, cfg), "--yes"))
	// Output:
	// target : memory store
	// keys   : TOKEN (dry run: true)
	// TOKEN: would-push
	// exit: 0
	// {"ok":true,"data":{"meta":{"target":"memory"},"results":[{"name":"TOKEN","action":"pushed"}],"dry_run":false}}
	// exit: 0
}
```

`Confirm` is the proceed contract every mutating verb follows: dry run and `--yes` proceed, an interactive human is asked, anything else is refused with a usage error:

<!-- snippet: cli/providerkit/example_test.go#api_providerkit_Confirm -->
```go
// ExampleConfirm shows the proceed contract every mutating verb follows:
// dry-run and --yes proceed, an interactive human is asked, and anything else
// is refused with a usage error (exit code 2).
func ExampleConfirm() {
	deps := apiKitDeps(&envkit.Selection{})
	names := []string{"DB_URL"}

	fmt.Println(providerkit.Confirm(deps, providerkit.GateInfo{Names: names, DryRun: true}))
	fmt.Println(providerkit.Confirm(deps, providerkit.GateInfo{Names: names, Yes: true}))

	err := providerkit.Confirm(deps, providerkit.GateInfo{Names: names})
	if ce, ok := errors.AsType[*providerkit.CmdError](err); ok {
		fmt.Println("code:", ce.Code, "quiet:", ce.Quiet)
	}

	deps.Interactive = func() bool { return true }
	deps.Ask = func(prompt string) bool {
		fmt.Printf("asked %q\n", prompt)
		return true
	}
	fmt.Println(providerkit.Confirm(deps, providerkit.GateInfo{Names: names}))
	// Output:
	// <nil>
	// <nil>
	// dotenvctl: refusing to write without --yes in non-interactive mode
	// code: 2 quiet: true
	// asked "proceed? [y/N] "
	// <nil>
}
```

### Step 4 — mount the plugin

Put the verbs under one namespace command, and add that to your cobra root. `Deps.Source` is where the env file is read; using `envkit.SelectFile` there gives your plugin the same `--prefix` and guard semantics as dotenvctl.

<!-- snippet: cli/providerkit/example_test.go#api_providerkit_Plugin -->
```go
// ExamplePlugin mounts a plugin under your own cobra root. Deps.Source reads
// the env file through envkit, so --prefix and the guards mean the same thing
// here as in dotenvctl.
func ExamplePlugin() {
	dir, err := os.MkdirTemp("", "providerkit-example-")
	if err != nil {
		fmt.Println("setup:", err)
		return
	}
	defer func() {
		// Cleanup is best-effort; a failure goes to stderr so the example's
		// checked stdout output is unaffected.
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
		}
	}()
	envPath := filepath.Join(dir, ".env.prod")
	if err := os.WriteFile(envPath, []byte("APP_DB=postgres://db\nAPP_KEY=__TODO__\nOTHER=x\n"), apiKitFileMode); err != nil {
		fmt.Println("setup:", err)
		return
	}

	deps := apiKitDeps(nil)
	deps.Source = func(o providerkit.SelectOpts) (providerkit.Source, error) {
		return envkit.SelectFile(envPath, o)
	}

	var plugin providerkit.Plugin = &apiKitPlugin{store: &apiKitStore{values: map[string]string{}}}
	root := &cobra.Command{Use: "mytool"}
	root.AddCommand(plugin.Command(deps))

	fmt.Println("exit:", apiKitRun(root, "memstore", "push", "--prefix", "APP_", "--strip-prefix", "--yes"))
	fmt.Println("exit:", apiKitRun(root, "memstore", "list"))
	// Output:
	// KEY: skipped-placeholder
	// DB: pushed
	// exit: 0
	// DB
	// exit: 0
}
```

The plugin type in that example is a complete third-party plugin:

<!-- snippet: cli/providerkit/example_test.go#api_providerkit_pluginType -->
```go
// apiKitPlugin is a complete third-party plugin: one namespace with the
// three kit verbs over its own store.
type apiKitPlugin struct {
	store *apiKitStore
}

// Command implements providerkit.Plugin.
func (p *apiKitPlugin) Command(deps providerkit.Deps) *cobra.Command {
	root := &cobra.Command{Use: "memstore", Short: "Sync values to an in-memory store"}
	root.AddCommand(
		providerkit.NewPushCmd(deps, p.store, providerkit.VerbConfig{}),
		providerkit.NewListCmd(deps, p.store),
		providerkit.NewPruneCmd(deps, p.store, p.store, providerkit.VerbConfig{}),
	)
	return root
}
```

### Talking to a backend CLI: Runner

Plugins that drive a CLI (`gh`, `vercel`) call it through `Deps.Runner`. Production uses `providerkit.ExecRunner{}`; tests inject a fake that records argv and stdin and never starts a process:

<!-- snippet: cli/providerkit/example_test.go#api_providerkit_Runner -->
```go
// ExampleRunner swaps the exec seam for a recording fake. A store passes the
// secret on stdin, never in argv, because argv is visible in ps.
func ExampleRunner() {
	runner := &apiKitRunner{}
	var r providerkit.Runner = runner

	if _, err := r.Run(context.Background(), "s3cr3t", "mycli", "secret", "set", "DB_URL"); err != nil {
		fmt.Println("run:", err)
		return
	}
	fmt.Println(runner.calls[0])
	// Output:
	// mycli secret set DB_URL <stdin "s3cr3t">
}
```

### Failing with the right exit code: Fail and CmdError

`Fail` prints a failure in the active mode (`dotenvctl: <msg>` for humans, the error envelope under `--json`) and returns a `*CmdError` carrying the exit code: `CodeUsage` gives 2, every other code gives 1. `Quiet` is true because the message is already printed.

<!-- snippet: cli/providerkit/example_test.go#api_providerkit_Fail -->
```go
// ExampleFail prints a failure in the active mode and returns a *CmdError
// carrying the exit code: CodeUsage maps to 2, every other code to 1. Quiet
// is true because the message is already printed.
func ExampleFail() {
	human := &outfmt.Printer{Out: os.Stdout}
	err := providerkit.Fail(human, outfmt.CodeExec, "push %s: %s", "DB_URL", "HTTP 403")
	if ce, ok := errors.AsType[*providerkit.CmdError](err); ok {
		fmt.Println("code:", ce.Code, "quiet:", ce.Quiet, "error:", ce)
	}

	machine := &outfmt.Printer{Out: os.Stdout, JSON: true}
	err = providerkit.Fail(machine, outfmt.CodeUsage, "--org is mutually exclusive with --repo")
	if ce, ok := errors.AsType[*providerkit.CmdError](err); ok {
		fmt.Println("code:", ce.Code)
	}
	// Output:
	// dotenvctl: push DB_URL: HTTP 403
	// code: 1 quiet: true error: push DB_URL: HTTP 403
	// {"ok":false,"error":{"code":"usage","message":"--org is mutually exclusive with --repo"}}
	// code: 2
}
```

On the host side, map whatever a command returned to a process exit code. Print the message only when `Quiet` is false:

<!-- snippet: cli/providerkit/example_test.go#api_providerkit_CmdError -->
```go
// ExampleCmdError is the host side: map whatever a plugin command returned to
// a process exit code, and print the message only when the plugin has not
// already printed it (Quiet false).
func ExampleCmdError() {
	exitCode := func(err error) int {
		if err == nil {
			return 0
		}
		if ce, ok := errors.AsType[*providerkit.CmdError](err); ok {
			if !ce.Quiet && ce.Err != nil {
				fmt.Println("dotenvctl:", ce.Err)
			}
			return ce.Code
		}
		fmt.Println("dotenvctl:", err)
		return apiKitExitUsage // anything else out of cobra is a usage error
	}

	fmt.Println(exitCode(nil))
	fmt.Println(exitCode(&providerkit.CmdError{Code: 1, Err: errors.New("backend unreachable")}))
	fmt.Println(exitCode(&providerkit.CmdError{Code: 1, Quiet: true, Err: errors.New("already printed")}))
	fmt.Println(exitCode(errors.New(`unknown flag: --tagret`)))
	// Output:
	// 0
	// dotenvctl: backend unreachable
	// 1
	// 1
	// dotenvctl: unknown flag: --tagret
	// 2
}
```

### Selection flags on your own commands

`AddSelectionFlags` gives any command the five shared selection flags (`--prefix`, `--exclude-prefix`, `--include`, `--exclude`, `--strip-prefix`). `SelectionActive` tells you whether the user selected anything, so a read verb can keep its plain output for bare calls.

<!-- snippet: cli/providerkit/example_test.go#api_providerkit_AddSelectionFlags -->
```go
// ExampleAddSelectionFlags gives your own command the shared selection flags
// (--prefix, --exclude-prefix, --include, --exclude, --strip-prefix), and
// SelectionActive tells you whether the user actually selected anything.
func ExampleAddSelectionFlags() {
	run := func(args ...string) {
		var sel providerkit.SelectOpts
		c := &cobra.Command{
			Use: "show",
			RunE: func(*cobra.Command, []string) error {
				fmt.Printf("active=%t prefix=%q strip=%t exclude=%v\n",
					providerkit.SelectionActive(sel), sel.Prefix, sel.StripPrefix, sel.ExcludeKeys)
				return nil
			},
		}
		providerkit.AddSelectionFlags(c, &sel)
		c.SetArgs(args)
		if err := c.Execute(); err != nil {
			fmt.Println("error:", err)
		}
	}

	run()
	run("--prefix", "APP_", "--strip-prefix", "--exclude", "APP_DEBUG")
	// Output:
	// active=false prefix="" strip=false exclude=[]
	// active=true prefix="APP_" strip=true exclude=[APP_DEBUG]
}
```

**Pitfalls**

- The kit's commands do not silence cobra. Mount them under a root with `SilenceErrors` and `SilenceUsage` set, or cobra prints `Error: …` and the usage text after the kit has already printed the failure.
- `Deps.Interactive` and `Deps.Ask` must be non-nil: `Confirm` calls `Interactive()` whenever a write is neither a dry run nor `--yes` and the printer is not in JSON mode.
- JSON mode never prompts. `--json` without `--yes` is always refused, even at a terminal, so the envelope cannot be mixed with a prompt.
- A nil `Gate` still runs `Confirm`. No mutating kit verb runs ungated.
- `Meta` runs at the end of every push and prune, in human mode too, so a backend lookup inside it costs one extra call per run. A `Meta` error drops the metadata and keeps the results.
- When a write fails half-way, the kit prints the results so far, then fails with exit 1. Re-running is safe as long as your `Set` is an upsert.
- `NewListCmd` with an empty store prints `no secrets found for this scope (repo/environment/org secrets are listed separately — try --environment or --org)`. That hint names GitHub's scopes for every plugin, including yours.
- `AddSelectionFlags` adds only the five selection flags. `--expand`, `--include-placeholders` and `--include-empty` exist only on the kit's store verbs, where they default to expand-on and guards-on.
- Cobra keeps flag values between `Execute` calls on the same tree. Build a fresh tree with `Plugin.Command` for each invocation in a long-running process.

Examples: [`ExampleSecretWriter`](../cli/providerkit/example_test.go), [`ExampleNewPushCmd`](../cli/providerkit/example_test.go), [`ExampleNewListCmd`](../cli/providerkit/example_test.go), [`ExampleNewPruneCmd`](../cli/providerkit/example_test.go), [`ExampleVerbConfig`](../cli/providerkit/example_test.go), [`ExampleConfirm`](../cli/providerkit/example_test.go), [`ExamplePlugin`](../cli/providerkit/example_test.go), [`ExampleRunner`](../cli/providerkit/example_test.go), [`ExampleFail`](../cli/providerkit/example_test.go), [`ExampleCmdError`](../cli/providerkit/example_test.go), [`ExampleAddSelectionFlags`](../cli/providerkit/example_test.go), [`ExampleSkipAction`](../cli/providerkit/example_test.go).

## Mounting the github and vercel plugins

`githubplugin.New()` and `vercelplugin.New()` return a `*Plugin` whose `Command(deps)` is the same tree `dotenvctl github …` and `dotenvctl vercel …` run. Mount it under your own root and you get the push, list and prune verbs, the confirm banner and the safety rules, without shelling out to dotenvctl. Both drive their backend CLI through `Deps.Runner`; the examples use a fake runner, so no `gh`, no `vercel` and no network is involved.

<!-- snippet: cli/plugins/githubplugin/example_test.go#api_githubplugin_New -->
```go
// ExampleNew mounts the github plugin under your own cobra root. All gh calls
// go through Deps.Runner — here a fake — and the secret value reaches gh on
// stdin, never in argv.
func ExampleNew() {
	var out bytes.Buffer
	runner := &apiGhRunner{}
	src := &envkit.Selection{Pairs: []dotenv.Pair{{Key: "DB_URL", Value: "postgres://db/app"}}}

	deps := providerkit.Deps{
		Printer:     &outfmt.Printer{Out: &out},
		File:        ".env.prod",
		Source:      func(providerkit.SelectOpts) (providerkit.Source, error) { return src, nil },
		Runner:      runner, // providerkit.ExecRunner{} in production
		Interactive: func() bool { return false },
		Ask:         func(string) bool { return false },
	}

	root := &cobra.Command{Use: "mytool", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	root.AddCommand(githubplugin.New().Command(deps))

	apiGhRun(root, &out, "github", "push", "--repo", "acme/api", "--yes")
	for _, c := range runner.calls {
		fmt.Println(c)
	}
	// Output:
	// target : acme/api
	// account: octocat
	// keys   : 1
	// DB_URL: pushed
	// exit: 0
	// gh api user --jq .login  stdin=""
	// gh secret set DB_URL --repo acme/api  stdin="postgres://db/app"
	// gh api user --jq .login  stdin=""
}
```

The gate checks every selected name against GitHub's grammar before any write and refuses the whole push if one is invalid:

<!-- snippet: cli/plugins/githubplugin/example_test.go#api_githubplugin_Plugin_Command -->
```go
// ExamplePlugin_Command shows the gate refusing a whole push when any
// selected name is not a legal GitHub secret name — nothing is written, and
// the exit code is 2 (usage).
func ExamplePlugin_Command() {
	var out bytes.Buffer
	runner := &apiGhRunner{}
	src := &envkit.Selection{Pairs: []dotenv.Pair{
		{Key: "DB_URL", Value: "postgres://db/app"},
		{Key: "app.name", Value: "api"},
	}}

	cmd := githubplugin.New().Command(providerkit.Deps{
		Printer:     &outfmt.Printer{Out: &out},
		File:        ".env.prod",
		Source:      func(providerkit.SelectOpts) (providerkit.Source, error) { return src, nil },
		Runner:      runner,
		Interactive: func() bool { return false },
		Ask:         func(string) bool { return false },
	})
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetErr(io.Discard)

	apiGhRun(cmd, &out, "push", "--repo", "acme/api", "--yes")
	fmt.Println("gh calls:", len(runner.calls))
	// Output:
	// dotenvctl: key(s) [app.name] are not valid GitHub secret names ([A-Za-z_][A-Za-z0-9_]*) — rename them or select others; refusing a partial push
	// exit: 2
	// gh calls: 0
}
```

The vercel plugin writes once per `--target`, with the value on stdin:

<!-- snippet: cli/plugins/vercelplugin/example_test.go#api_vercelplugin_New -->
```go
// ExampleNew mounts the vercel plugin under your own cobra root and pushes one
// value to two Vercel environments. Each target is one `vercel env add`, the
// value on stdin. The project line reads .vercel/project.json in the working
// directory; there is none here.
func ExampleNew() {
	var out bytes.Buffer
	runner := &apiVercelRunner{}
	src := &envkit.Selection{Pairs: []dotenv.Pair{{Key: "DB_URL", Value: "postgres://db/app"}}}

	root := &cobra.Command{Use: "mytool", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	root.AddCommand(vercelplugin.New().Command(apiVercelDeps(&out, runner, src)))

	apiVercelRun(root, &out, "vercel", "push", "--target", "production", "--target", "preview", "--yes")
	for _, c := range runner.calls {
		if strings.HasPrefix(c, "vercel env add") {
			fmt.Println(c)
		}
	}
	// Output:
	// project: unlinked directory (vercel CLI will resolve or prompt)
	// account: acme-bot
	// targets: production, preview
	// keys   : 1
	// DB_URL: pushed
	// exit: 0
	// vercel env add DB_URL production --force  stdin="postgres://db/app"
	// vercel env add DB_URL preview --force  stdin="postgres://db/app"
}
```

<!-- snippet: cli/plugins/vercelplugin/example_test.go#api_vercelplugin_Plugin_Command -->
```go
// ExamplePlugin_Command previews a prune: OLD exists in Vercel but not in the
// local selection, so a dry run reports it as would-delete and removes
// nothing.
func ExamplePlugin_Command() {
	var out bytes.Buffer
	runner := &apiVercelRunner{}
	src := &envkit.Selection{Pairs: []dotenv.Pair{{Key: "DB_URL", Value: "postgres://db/app"}}}

	cmd := vercelplugin.New().Command(apiVercelDeps(&out, runner, src))
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetErr(io.Discard)

	apiVercelRun(cmd, &out, "prune", "--target", "production", "--dry-run")
	// Output:
	// project: unlinked directory (vercel CLI will resolve or prompt)
	// account: acme-bot
	// targets: production
	// keys   : 1
	// OLD: would-delete
	// exit: 0
}
```

The examples share a small fake runner and a run helper, `apiGhRun` / `apiVercelRun`, defined in the same files. The helpers cut the auth-source label off the `account:` line, because that label comes from the real process environment.

**Pitfalls**

- In production pass `providerkit.ExecRunner{}`; `gh` or `vercel` must be installed and logged in. A missing binary fails the gate with `resolve account: …` before anything is written.
- The `account:` banner line reads `GH_TOKEN` / `GITHUB_TOKEN` (github) or `VERCEL_TOKEN` (vercel) from your process environment to label the auth source. The token itself is passed to the backend CLI only through the inherited environment.
- The vercel plugin reads `.vercel/project.json` relative to your process's working directory, not to `Deps.File`. In any other directory the banner says `unlinked directory`.
- The github plugin resolves the account again for the `--json` metadata, so a push calls `gh api user` twice (see the last line of `ExampleNew`).
- Use `--repo` (github) when your process does not run inside the target repository; without it the plugin asks `gh repo view` for the working directory's repo.

Examples: [`githubplugin.ExampleNew`](../cli/plugins/githubplugin/example_test.go), [`githubplugin.ExamplePlugin_Command`](../cli/plugins/githubplugin/example_test.go), [`vercelplugin.ExampleNew`](../cli/plugins/vercelplugin/example_test.go), [`vercelplugin.ExamplePlugin_Command`](../cli/plugins/vercelplugin/example_test.go). Flag-level docs: [github plugin](plugins/github.md), [vercel plugin](plugins/vercel.md).

## Embedding the CLI

`dotenvcmd.Execute` is the whole of `dotenvctl` as one function: it parses `args`, runs the verb, writes to the two writers you pass, and returns the exit code. It never calls `os.Exit`. `cmd/dotenvctl/main.go` is one line around it. Use it when you want every verb, flag and output format inside your own binary, or to drive the CLI in-process from tests with buffers.

<!-- snippet: exempt — signature excerpt from cli/dotenvcmd; the runnable in-process examples live in cli/dotenvcmd/example_test.go -->
```go
// Execute runs dotenvctl in-process and returns its exit code:
// 0 ok, 1 operation failed, 2 usage error.
func Execute(args []string, stdout, stderr io.Writer) int

// Exit codes, frozen API.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// Context variables an exec plugin (dotenvctl-<name> on PATH) receives.
// Facts only, never secret values.
const (
	EnvPluginFile        = "DOTENVCTL_FILE"        // absolute path of the -f file
	EnvPluginJSON        = "DOTENVCTL_JSON"        // "1" with --json, else "0"
	EnvPluginPlaceholder = "DOTENVCTL_PLACEHOLDER" // the active placeholder regexp
	EnvPluginBin         = "DOTENVCTL_BIN"         // path of the running binary, for callbacks
	EnvPluginVersion     = "DOTENVCTL_VERSION"     // the host version
)
```

A complete call, from the CLI examples:

<!-- snippet: cli/dotenvcmd/example_test.go#cli_Execute -->
```go
// ExampleExecute embeds the whole dotenvctl CLI in a Go program: Execute
// takes the argument list (without the program name) and two writers, and
// returns the exit code instead of calling os.Exit.
func ExampleExecute() {
	dir, err := os.MkdirTemp("", "dotenvctl-")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() {
		// Cleanup is best-effort; a failure goes to stderr so the example's
		// checked stdout output is unaffected.
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
		}
	}()

	// -f takes any path; the file holds one plain and one derived value.
	envFile := filepath.Join(dir, ".env")
	// 0600 because env files hold credentials.
	if err := os.WriteFile(envFile, []byte("HOST=localhost\nURL=http://${HOST}:8080\n"), 0o600); err != nil {
		fmt.Println(err)
		return
	}

	code := dotenvcmd.Execute([]string{"-f", envFile, "get", "URL", "--expand"}, os.Stdout, os.Stderr)
	fmt.Println("exit code:", code)

	// A missing key is an operation failure: exit code 1. Its message names
	// the temp path, so both writers are discarded here.
	code = dotenvcmd.Execute([]string{"-f", envFile, "get", "PORT"}, io.Discard, io.Discard)
	fmt.Println("exit code:", code)
	// Output:
	// http://localhost:8080
	// exit code: 0
	// exit code: 1
}
```

There is one in-process example of `Execute` per verb in [`cli/dotenvcmd/example_test.go`](../cli/dotenvcmd/example_test.go) (`ExampleExecute_get`, `ExampleExecute_set`, …).

The `EnvPlugin*` constants are the names of the variables an exec plugin gets. A Go program that *is* an exec plugin can read them with `os.Getenv(dotenvcmd.EnvPluginFile)` instead of repeating the strings.

**Try it.** An exec plugin is any executable named `dotenvctl-<name>` on `PATH`. This one prints its context and calls the host back:

```sh
#!/bin/sh
# dotenvctl-ctx: prints the context dotenvctl hands an exec plugin, then calls the host back.
echo "args: $*"
echo "json: $DOTENVCTL_JSON"
echo "file: $(basename "$DOTENVCTL_FILE")"
echo "placeholder: $DOTENVCTL_PLACEHOLDER"
"$DOTENVCTL_BIN" -f "$DOTENVCTL_FILE" keys
```

```console
$ printf 'DB_URL=postgres://db\nAPI_KEY=__TODO__\n' > .env.prod
$ dotenvctl -f .env.prod ctx --flag value
args: --flag value
json: 0
file: .env.prod
placeholder: ^__[A-Z0-9_]+__$
DB_URL
API_KEY
```

**Pitfalls**

- `Execute` builds a fresh command tree per call, so calls do not share state. It does use process-wide things: confirmation prompts read `os.Stdin`, interactivity is a terminal check on `os.Stdin`, and exec plugins inherit `os.Environ()` and `os.Stdin`.
- Any first word that is not a built-in verb is looked up on `PATH` as `dotenvctl-<word>` and run. Built-ins always win, so a PATH binary cannot shadow `github`.
- `DOTENVCTL_BIN` is `os.Executable()`: inside your program that is *your* binary. A plugin's callback (`"$DOTENVCTL_BIN" -f … list --json`) then runs your program, so route those arguments to `Execute`.
- `DOTENVCTL_VERSION` and `--version` come from the main module's build info. Inside your program they report your module's version, not the dotenv CLI's.
- Messages say `dotenvctl:` no matter what your binary is called.

Related: [Plugins](plugins.md) for the exec-plugin protocol, [Commands](commands.md) for every verb.

## discover

`discover` finds and classifies the env-file family of one directory without opening any file. `envkit.Discover` is a thin wrapper over it; import `discover` directly when you only need names and order.

<!-- snippet: cli/discover/example_test.go#api_discover_Scan -->
```go
// ExampleScan lists one directory's env-file family without opening any
// file. Order is distance from prod: default, local, dev, test, ci, stag,
// prod, unknown names, then contracts. Directories, editor droppings and
// files outside the family are skipped.
func ExampleScan() {
	dir, err := os.MkdirTemp("", "discover-example-")
	if err != nil {
		fmt.Println("setup:", err)
		return
	}
	defer func() {
		// Cleanup is best-effort; a failure goes to stderr so the example's
		// checked stdout output is unaffected.
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
		}
	}()
	for _, name := range []string{".env.example", ".env.production", ".env.preview", ".env.local", ".env", ".env.swp", "app.env", ".envrc"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("A=1\n"), apiDiscoverFileMode); err != nil {
			fmt.Println("setup:", err)
			return
		}
	}

	files, err := discover.Scan(dir)
	if err != nil {
		fmt.Println("scan:", err)
		return
	}
	for _, f := range files {
		fmt.Printf("%-16s %-8s contract=%t\n", f.File, f.Env, f.Contract)
	}

	_, err = discover.Scan(filepath.Join(dir, "missing"))
	fmt.Println("missing dir is an error:", err != nil)
	// Output:
	// .env             default  contract=false
	// .env.local       local    contract=false
	// .env.production  prod     contract=false
	// .env.preview     preview  contract=false
	// .env.example     example  contract=true
	// missing dir is an error: true
}
```

`Classify` labels one filename. Use it for explicit file arguments, so they get the same labels discovery gives:

<!-- snippet: cli/discover/example_test.go#api_discover_Classify -->
```go
// ExampleClassify labels one filename with the same vocabulary Scan uses.
// Suffixes are matched case-insensitively and long forms collapse; only a
// whole-suffix junk name is ignored, so .env.prod.bak is a new, unknown
// environment called "prod.bak".
func ExampleClassify() {
	for _, name := range []string{".env", ".env.Staging", ".env.dist", ".env.bak", ".env.prod.bak", "config.env"} {
		ef, ok := discover.Classify(name)
		fmt.Printf("%-14s ok=%-5t env=%q contract=%t\n", name, ok, ef.Env, ef.Contract)
	}
	fmt.Println(discover.EnvDefault)
	// Output:
	// .env           ok=true  env="default" contract=false
	// .env.Staging   ok=true  env="stag" contract=false
	// .env.dist      ok=true  env="dist" contract=true
	// .env.bak       ok=false env="" contract=false
	// .env.prod.bak  ok=true  env="prod.bak" contract=false
	// config.env     ok=false env="" contract=false
	// default
}
```

**Pitfalls**

- The order is fixed: `default`, `local`, `dev`, `test`, `ci`, `stag`, `prod`, then unknown names, then contracts; ties break by filename.
- Only `.env` and `.env.<suffix>` are candidates. `app.env` and `.envrc` are not.
- Junk is matched on the *whole* suffix (`bak`, `tmp`, `old`, `orig`, `swp`, `swo`, `lock`). `.env.prod.bak` is not junk: it becomes an unknown environment named `prod.bak`.
- Suffixes are lowercased, so `.env.Staging` is `stag`; `Env` for unknown names is lowercased too.
- `Path` joins the directory as you gave it, without cleaning, so `./` stays in printed paths.

Examples: [`ExampleScan`](../cli/discover/example_test.go), [`ExampleClassify`](../cli/discover/example_test.go).

## report

`report` renders a matrix or a diff as one self-contained HTML page: inline CSS, no JavaScript, no external requests, light and dark themes. Write the bytes with `os.WriteFile`; this is what `dotenvctl matrix --format html -o FILE` does.

<!-- snippet: cli/report/example_test.go#api_report_MatrixHTML -->
```go
// ExampleMatrixHTML renders a drift matrix as one self-contained HTML page.
// Fill Value only when Revealed is true; a masked page shows each cell's
// state symbol, never a value. The page embeds a generation timestamp, so
// two renders are not byte-identical.
func ExampleMatrixHTML() {
	m := report.Matrix{
		Envs:    []string{"stag", "prod"},
		Sources: []string{".env.stag", ".env.prod"},
		Rows: []report.MatrixRow{{
			Key: "DB_URL",
			Cells: []report.MatrixCell{
				{State: string(envkit.StatePresent), Symbol: envkit.StateSymbols[envkit.StatePresent]},
				{State: string(envkit.StateMissing), Symbol: envkit.StateSymbols[envkit.StateMissing]},
			},
		}},
	}
	page, err := report.MatrixHTML(m)
	if err != nil {
		fmt.Println("render:", err)
		return
	}
	fmt.Println(bytes.HasPrefix(page, []byte("<!doctype html>")))
	fmt.Println(bytes.Contains(page, []byte("<script")), bytes.Contains(page, []byte("CONTAINS SECRETS")))
	// Output:
	// true
	// false false
}
```

<!-- snippet: cli/report/example_test.go#api_report_DiffHTML -->
```go
// ExampleDiffHTML renders a diff page. With Revealed false every value cell
// is report.MaskGlyph, even if a value was passed in; with Revealed true the
// values appear and the page carries a "contains secrets" banner.
func ExampleDiffHTML() {
	d := report.Diff{
		FileA:   ".env.stag",
		FileB:   ".env.prod",
		Changed: []report.DiffChange{{Key: "DB_URL", From: "postgres://stag", To: apiReportSecret}},
	}
	for _, reveal := range []bool{false, true} {
		d.Revealed = reveal
		page, err := report.DiffHTML(d)
		if err != nil {
			fmt.Println("render:", err)
			return
		}
		fmt.Printf("revealed=%t secret=%t glyph=%t banner=%t\n", reveal,
			bytes.Contains(page, []byte(apiReportSecret)),
			bytes.Contains(page, []byte(report.MaskGlyph)),
			bytes.Contains(page, []byte("CONTAINS SECRETS")))
	}
	// Output:
	// revealed=false secret=false glyph=true banner=false
	// revealed=true secret=true glyph=false banner=true
}
```

**Masking is structural.** With `Revealed` false the templates never print a value: matrix cells show their state symbol, and diff cells show `report.MaskGlyph` (`••••••`, fixed width, so value lengths leak nothing too). With `Revealed` true, the page shows values and a "contains secrets" banner.

**Pitfalls**

- The page includes a generation timestamp, so two renders of the same input differ. Do not compare report files byte for byte.
- The report types are separate from envkit's. Copy `Envs`, `Sources`, rows, `State` (as a string) and `Symbol` (from `envkit.StateSymbols`) across yourself, and leave `Value` empty unless you set `Revealed`.
- In a revealed matrix only `present` and `placeholder` cells show a value; the other states show their symbol.

Examples: [`ExampleMatrixHTML`](../cli/report/example_test.go), [`ExampleDiffHTML`](../cli/report/example_test.go).

## textdiff

`textdiff.Lines` is the preview behind every `--dry-run`: `-old` / `+new` lines for the one region where two renderings differ, or nil when they are identical.

<!-- snippet: cli/textdiff/example_test.go#api_textdiff_Lines -->
```go
// ExampleLines previews an edit as "-old"/"+new" lines for the one region
// that changed. Identical input returns nil, which callers read as "no-op".
// It trims the common prefix and suffix; it is not a general diff.
func ExampleLines() {
	before := "# db\nDB_HOST=localhost\nDB_PORT=5432\n"
	after := "# db\nDB_HOST=db.prod\nDB_PORT=5432\n"

	for _, l := range textdiff.Lines(before, after) {
		fmt.Println(l)
	}
	fmt.Println(textdiff.Lines(before, before) == nil)

	// Two separate edits come back as one block spanning both.
	fmt.Println(textdiff.Lines("A=1\nB=2\nC=3", "A=9\nB=2\nC=8"))
	// Output:
	// -DB_HOST=localhost
	// +DB_HOST=db.prod
	// true
	// [-A=1 -B=2 -C=3 +A=9 +B=2 +C=8]
}
```

**Pitfalls**

- It trims the common prefix and suffix and reports everything between as one block. Two edits far apart come back as one block spanning both. That is right for single-entry edits, which is what it is for; it is not a general diff.
- Nil means "no change". An empty, non-nil slice is never returned.

Examples: [`ExampleLines`](../cli/textdiff/example_test.go).

## outfmt

`outfmt.Printer` is dotenvctl's single output seam. Use it when your tool should print the same `{ok,data|error}` envelope scripts already parse from dotenvctl. Human mode prints lines and `OK` is silent; JSON mode prints exactly one envelope and `Human` is silent, so you can call both without checking the mode.

<!-- snippet: cli/outfmt/example_test.go#api_outfmt_Printer -->
```go
// ExamplePrinter prints the same verb in both modes. Human mode prints lines
// and OK is silent; JSON mode prints exactly one envelope per call and Human
// is silent, so --json output stays parseable.
func ExamplePrinter() {
	for _, jsonMode := range []bool{false, true} {
		p := &outfmt.Printer{Out: os.Stdout, JSON: jsonMode}
		p.Human("DB_URL")
		p.Humanf("%d keys", 1)
		if err := p.OK(apiOutfmtPayload{Keys: []string{"DB_URL"}}); err != nil {
			fmt.Println("write:", err)
		}
	}
	// Output:
	// DB_URL
	// 1 keys
	// {"ok":true,"data":{"keys":["DB_URL"]}}
}
```

<!-- snippet: cli/outfmt/example_test.go#api_outfmt_Printer_Fail -->
```go
// ExamplePrinter_Fail reports a failure. Human mode prints "dotenvctl: <msg>"
// to Out (not stderr); JSON mode prints the error envelope. Fail only prints:
// the exit code is yours to return.
func ExamplePrinter_Fail() {
	human := &outfmt.Printer{Out: os.Stdout}
	machine := &outfmt.Printer{Out: os.Stdout, JSON: true}
	for _, p := range []*outfmt.Printer{human, machine} {
		if err := p.Fail(outfmt.CodeNotFound, `key "DB_URL" not found`); err != nil {
			fmt.Println("write:", err)
		}
	}
	// Output:
	// dotenvctl: key "DB_URL" not found
	// {"ok":false,"error":{"code":"not_found","message":"key \"DB_URL\" not found"}}
}
```

The error codes are a closed set that scripts branch on:

<!-- snippet: cli/outfmt/example_test.go#api_outfmt_ErrCode -->
```go
// ExampleErrCode lists the closed set of error codes scripts branch on.
func ExampleErrCode() {
	for _, c := range []outfmt.ErrCode{outfmt.CodeNotFound, outfmt.CodeIO, outfmt.CodeRequired, outfmt.CodeUsage, outfmt.CodeExec, outfmt.CodeDiff} {
		fmt.Println(c)
	}
	// Output:
	// not_found
	// io
	// required
	// usage
	// exec
	// diff
}
```

**Pitfalls**

- `Fail` writes to `Out`, not to stderr, in both modes, and the human line always starts with `dotenvctl:`.
- `Fail` only prints. Return an exit code yourself, or use `providerkit.Fail`, which also builds the `*CmdError`.
- Pass a named struct to `OK`, not a map, so the JSON shape is visible where it is produced.
- `Human` / `Humanf` drop write errors on purpose; `OK` and `Fail` return them.

Examples: [`ExamplePrinter`](../cli/outfmt/example_test.go), [`ExamplePrinter_Fail`](../cli/outfmt/example_test.go), [`ExampleErrCode`](../cli/outfmt/example_test.go).

## Complete API reference

Every exported identifier, by package. Signatures live in `go doc`; this table is the map.

### envkit

| Kind | Identifier | Purpose |
|---|---|---|
| func | `Select(f, opts)` / `SelectFile(path, opts)` | apply the selection grammar to a parsed file / a path |
| func | `Discover(dir)` / `Inventories(opts)` | find the env-file family / summarize each file |
| func | `BuildMatrix(opts)` | the keys × environments drift table |
| func | `DriftRows(rows)` | keep only rows that aren't uniformly present (filters in place) |
| func | `SparseColumn(m, rowsBefore)` | diagnose an ineffective drift filter (returns env, missing count, ok) |
| func | `DisambiguateColumns(cols)` | make column labels unique when two files classify alike |
| func | `EnvNameForPath(path)` | label an explicit path with discovery's vocabulary |
| func | `Diff(a, b, opts)` / `DiffMaps(a, b, expanded)` | effective-config comparison of two files / two maps |
| func | `Set` / `Unset` / `Restore` | byte-preserving edits |
| func | `Run(ctx, path, argv, opts)` | execute with the file's values; returns the child's exit code |
| func | `ChildEnv(path, base, sel)` / `MergeEnv(base, values)` | build a child environment (file wins) |
| type | `SelectOptions`, `Selection`, `Skip`, `SkipReason` | selection inputs and results |
| type | `Matrix`, `MatrixRow`, `MatrixCell`, `MatrixOptions`, `CellState` | matrix types |
| type | `DiffResult`, `DiffPair`, `DiffChange`, `DiffOptions` | diff types |
| type | `EditOptions`, `EditResult` | edit types |
| type | `Inventory`, `InventoryOptions`, `EnvFile` | discovery types (`EnvFile` = `discover.EnvFile`) |
| type | `RunOptions` | execution options |
| const | `DefaultPlaceholderPattern`, `SparseColumnThreshold` | the `__LIKE_THIS__` pattern; the sparse-column trigger share (0.6) |
| const | `SkipPlaceholder`, `SkipEmpty` | skip reasons |
| const | `StatePresent`, `StateEmpty`, `StatePlaceholder`, `StateDisabled`, `StateInherited`, `StateMissing` | cell states |
| var | `StateSymbols` | state → glyph, shared by every renderer |
| var | `ErrAnchorNotFound`, `ErrKeyNotFound` | sentinels for `errors.Is` |

Methods worth knowing: `Selection.Names()` / `SkippedNames()` / `Map()`, `Matrix.HasContractGaps()`, `MatrixCell.Raw()`, `DiffResult.Empty()`.

### providerkit

| Kind | Identifier | Purpose |
|---|---|---|
| type | `Plugin` | the one mandatory contract: `Command(Deps) *cobra.Command` |
| type | `Deps` | everything a plugin receives from the host: `Printer`, `File`, `Source`, `Runner`, `Interactive`, `Ask` |
| type | `SelectOpts` = `envkit.SelectOptions`, `Source` = `*envkit.Selection`, `Skip` = `envkit.Skip` | aliases — one grammar, no translation |
| type | `SecretWriter`, `SecretLister`, `SecretDeleter` | capability interfaces; implement what your backend truly supports |
| type | `Runner`, `ExecRunner` | the injectable exec seam and its production implementation |
| type | `VerbConfig`, `Gate`, `GateInfo` | per-verb customization: gate (banner + confirm) and `--json` metadata |
| type | `Action`, `Result`, `CmdError` | the outcome vocabulary and the exit-code carrier |
| func | `NewPushCmd`, `NewListCmd`, `NewPruneCmd` | generate the standard verbs for your capabilities |
| func | `Confirm(deps, info)` | the standard prompt/`--yes` contract |
| func | `Fail(printer, code, format, …)` | emit a failure and return the right exit code |
| func | `AddSelectionFlags(cmd, sel)` | bind the five shared selection flags to any command |
| func | `SelectionActive(sel)` | did the user actually select something? |
| func | `SkipAction(reason)` | map a selection guard reason to a push action |
| const | `ActionPushed`, `ActionWouldPush`, `ActionSkippedPlaceholder`, `ActionSkippedEmpty`, `ActionDeleted`, `ActionWouldDelete`, `ActionKeptSkipped`, `ActionKeptFlag` | the closed action set |

### plugins · dotenvcmd · discover · report · textdiff · outfmt

| Package | Surface |
|---|---|
| `plugins/githubplugin` | `New() *Plugin`, `(*Plugin).Command(providerkit.Deps) *cobra.Command` |
| `plugins/vercelplugin` | `New() *Plugin`, `(*Plugin).Command(providerkit.Deps) *cobra.Command` |
| `dotenvcmd` | `Execute(args, stdout, stderr) int`, `ExitOK`/`ExitFailure`/`ExitUsage`, `EnvPluginFile`/`EnvPluginJSON`/`EnvPluginPlaceholder`/`EnvPluginBin`/`EnvPluginVersion` |
| `discover` | `Scan(dir)`, `Classify(filename)`, `EnvFile`, `EnvDefault` |
| `report` | `MatrixHTML(Matrix)`, `DiffHTML(Diff)`, types `Matrix`/`MatrixRow`/`MatrixCell`/`Diff`/`DiffPair`/`DiffChange`, `MaskGlyph` |
| `textdiff` | `Lines(before, after)` |
| `outfmt` | `Printer` with `OK`/`Fail`/`Human`/`Humanf`, `ErrCode` and its constants (`CodeNotFound`, `CodeIO`, `CodeRequired`, `CodeUsage`, `CodeExec`, `CodeDiff`) |

## CLI ↔ Go equivalence

Every command maps to a call, so a script can graduate to a program without changing behaviour:

| CLI | Go |
|---|---|
| `dotenvctl get KEY --expand` | `dotenv.Open` + `f.GetExpanded(key)` |
| `dotenvctl list --prefix P --strip-prefix` | `envkit.SelectFile(path, envkit.SelectOptions{Prefix: "P", StripPrefix: true})` |
| `dotenvctl keys --exclude-prefix P` | same, reading `Selection.Names()` |
| `dotenvctl set K=V --after ANCHOR --dry-run` | `envkit.Set(path, pairs, envkit.EditOptions{Anchor: "ANCHOR", DryRun: true})` |
| `dotenvctl unset K` / `--delete` | `envkit.Unset(path, keys, envkit.EditOptions{Delete: …})` |
| `dotenvctl restore K` | `envkit.Restore(path, keys, envkit.EditOptions{})` |
| `dotenvctl envs --dir D` | `envkit.Inventories(envkit.InventoryOptions{Dir: "D"})` |
| `dotenvctl matrix --contract C --only-drift` | `envkit.BuildMatrix(…{ContractPath: "C", OnlyDrift: true})` |
| `dotenvctl diff A B --expand` | `envkit.Diff("A", "B", envkit.DiffOptions{Expand: true})` |
| `dotenvctl matrix --format html -o F` | `report.MatrixHTML(...)` + `os.WriteFile` |
| `dotenvctl run -- cmd args` | `envkit.Run(ctx, path, argv, envkit.RunOptions{Base: os.Environ()})` |
| `dotenvctl github push …` | mount `githubplugin.New().Command(deps)`, or call `providerkit.NewPushCmd` with your own store |
| `dotenvctl vercel push …` | mount `vercelplugin.New().Command(deps)` |
| any of the above, verbatim | `dotenvcmd.Execute(args, stdout, stderr)` |

Two deliberate differences: the CLI's `--expand` default differs by verb (off for reads, on for pushes) while `SelectOptions.Expand` is explicit, and the CLI maps errors to exit codes while the Go API returns them.

## Stability

`envkit`, `providerkit`, the plugins, `dotenvcmd.Execute` and its exported constants, and the helper packages are the public Go surface. Their exported signatures are API, and breaking one is a major-version event. The CLI's *flag* names and `--json` envelope are covered by the same promise. Unexported identifiers in `dotenvcmd` are not.

## Related

- [Commands](commands.md) — every CLI verb and flag.
- [Multi-environment](multi-env.md) — `envs`, `matrix` and `diff` from the terminal.
- [Plugins](plugins.md) — the plugin contract and exec plugins; [github](plugins/github.md) and [vercel](plugins/vercel.md) plugin pages.
- [Feature index](reference/features.md) — every feature with its guide and Example.
- The library itself: [README](../README.md).
