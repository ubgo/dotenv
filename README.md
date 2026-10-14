<h1 align="center">dotenv</h1>
<p align="center"><strong>The comment-preserving .env toolkit for Go.</strong></p>
<p align="center">Parse, edit, compare, and sync .env files — every byte you don't touch survives verbatim.</p>

<p align="center">
  <a href="https://pkg.go.dev/github.com/ubgo/dotenv"><img src="https://pkg.go.dev/badge/github.com/ubgo/dotenv.svg" alt="Go Reference on pkg.go.dev"></a>
  <a href="https://goreportcard.com/report/github.com/ubgo/dotenv"><img src="https://goreportcard.com/badge/github.com/ubgo/dotenv" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-2ea44f" alt="License: Apache-2.0"></a>
  <img src="https://img.shields.io/badge/dependencies-zero-2ea44f" alt="Zero dependencies — stdlib only">
  <img src="https://img.shields.io/badge/coverage-100%25%20lib%20·%2099.5%25%20cli-2ea44f" alt="Statement coverage: 100% library, 99.5% CLI">
</p>

A comment-preserving `.env` parser, editor, and CLI for Go. The library (`github.com/ubgo/dotenv`) is a stdlib-only, zero-dependency package for reading, editing, and writing dotenv files with byte-exact round-tripping and full Docker Compose interpolation; the CLI (`dotenvctl`) adds environment-variable management from the shell — get/set/unset, an environment drift matrix, effective-config diff, `run`, and secrets sync to GitHub Actions and Vercel. Open source, fuzz-tested, and built for the twelve-factor configuration workflow.

```sh
go get github.com/ubgo/dotenv                                # the library
go install github.com/ubgo/dotenv/cli/cmd/dotenvctl@main     # the CLI
```

**Quickstart** — open a file, read a value raw and expanded, change another, save. The comment and the reference survive byte for byte:

<!-- snippet: example_readme_test.go#readme_Quickstart -->
```go
// Example_quickstart is the five-minute tour: open a .env, read one value raw
// and expanded, change another, and save. Every byte you did not touch,
// including the comment and the reference, is written back as it was.
func Example_quickstart() {
	dir, err := os.MkdirTemp("", "dotenv-quickstart")
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

	path := filepath.Join(dir, ".env")
	src := "# where the API lives\nDOMAIN=staging.acme.io\nAPI_URL=https://api.${DOMAIN}\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		fmt.Println(err)
		return
	}

	f, err := dotenv.Open(path) // a missing file is not an error
	if err != nil {
		fmt.Println(err)
		return
	}

	raw, _ := f.Get("API_URL") // the bool reports presence; API_URL is present
	url, _, err := f.GetExpanded("API_URL")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(raw)
	fmt.Println(url)

	f.Set("DOMAIN", "acme.io")
	if err := f.Save(); err != nil { // atomic: temp file at 0600, then rename
		fmt.Println(err)
		return
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Print(string(saved))
	// Output:
	// https://api.${DOMAIN}
	// https://api.staging.acme.io
	// # where the API lives
	// DOMAIN=acme.io
	// API_URL=https://api.${DOMAIN}
}
```

![dotenv architecture](docs/assets/architecture.svg)

**Learn it:** [the learning path](docs/README.md#learning-path) takes you in 12 steps from first use to a full production setup · [Choosing guides](docs/choosing/README.md) answer "which call, option or backend do I need?" · [the feature index](docs/reference/features.md) lists all 122 features with their guide and runnable example.

**Two ways in:**

| | You want | Start here |
|---|---|---|
| 📦 **Go library** | parse, edit, expand, and write `.env` files from Go | this page, next section down |
| 🖥️ **`dotenvctl` CLI** | `get`/`set`/`unset` from the shell, env-drift matrix, `diff`, `run`, GitHub/Vercel secret sync | [the CLI section below](#cli--dotenvctl) → [cli/](cli/README.md) → [full docs](docs/README.md) |

**Contents:** [Why not a dotenv library](#why-not-a-dotenv-library) · [Scope](#scope) · [CLI — dotenvctl](#cli--dotenvctl) · [Reading](#reading) · [Writing](#writing) · [Generating a file](#generating-a-file) · [Disabled settings](#disabled-settings) · [Inherited declarations](#inherited-declarations) · [Cloning between environments](#cloning-between-environments) · [Plugins](#plugins) · [Typed configuration](#typed-configuration) · [Feature support](#feature-support) · [Variable expansion](#variable-expansion) · [Saving safely](#saving-safely) · [Line endings](#line-endings-and-trailing-newlines) · [API](#api) · [Testing](#testing) · [FAQ](#faq)

## Why not a dotenv library

Parse-to-map libraries keep the keys and throw away everything else. Write the file back and the comments, blank lines, ordering, and quoting style are gone.

That matters because a `.env` is documentation as much as configuration. A tool that strips the comment explaining *why* a value exists has damaged the file even though every key survived.

This package edits **line-wise**: every byte outside the entry you touch survives verbatim. Two invariants, both pinned by tests:

```
parse → render with no changes   ⇒  byte-identical output
Set(key, <current value>)        ⇒  byte-identical output (a true no-op)
```

How that places it against the other ways people handle `.env` files:

| | **dotenv + dotenvctl** | Loaders (godotenv, Node/Python dotenv) | Encryption tools (dotenvx, sops) | sed / hand-editing |
|---|---|---|---|---|
| Edit without destroying comments & formatting | ✅ byte-exact, fuzz-pinned | ❌ parse-to-map, write loses everything | ⚠️ varies | ❌ breaks on quoting & multiline |
| Reversible disable (`unset` → `restore`) | ✅ byte-identical round trip | ❌ | ❌ | ❌ |
| Distinguish disabled / missing / placeholder | ✅ | ❌ all collapse to "absent" | ❌ | ❌ |
| Drift matrix + contract CI gate across envs | ✅ `matrix`, exit-code gates | ❌ | ❌ | ❌ |
| Selection-based secret sync (GitHub / Vercel) | ✅ with placeholder guards | ❌ | ⚠️ own model | ❌ hand-rolled loops |
| Leaves `os.Environ` alone | ✅ by design, test-pinned | ❌ loading is the point | ⚠️ | — |

If you want a loader that injects variables into your process, godotenv already does that well; if you want the **file itself** managed — edited safely, compared across environments, synced outward — that is this toolkit.

## Scope

A parser and editor. Three things it deliberately does **not** do:

| Not this | Why | Do instead |
|---|---|---|
| Set `os.Environ` | Reading a `.env` and exporting it into the process are separate decisions. Conflating them is why `godotenv.Load` means something this package must not | Apply your own precedence over `f.Map()` |
| Merge multiple files | Merging is a precedence policy — which file wins, per key. The guarantee here is that one file maps to one set of bytes, and merged content has no single file to render back to | Layer it above |
| Coerce values to types | A `.env` has no types — `PORT=8443` is four characters. Which Go type that becomes depends on the field it is bound to, which is the application's decision. Typed getters also fail quietly: `PORT=eighty` returning a default is a production incident | [Compose with a binder](#typed-configuration) |
| Stream a file in chunks | Forward references, insertion, and byte-exact rendering each need the whole file. A stream could resolve *backward* references only, which would make the same bytes expand differently here and there — worse than not offering it. The full reasoning and measurements live in the package doc | `ParseReader` closes the cost that was real |

The first is enforced by a test that snapshots the environment across a full parse → expand → edit → save → reopen cycle.

> **Why `Open`, not `Load`.** `godotenv.Load` sets `os.Environ`. This package deliberately never touches the environment — it hands back a file you read, edit, and save — so a `Load` here would mean the opposite of every other `Load` in the ecosystem. The package is named for the format, the way `encoding/json` and `yaml` are.

## CLI — dotenvctl

Everything below is also available from the shell. `cli/` ships `dotenvctl`, a terminal front-end over this library — every edit keeps the byte-preservation guarantees, and the tool never touches its own process environment.

```sh
go install github.com/ubgo/dotenv/cli/cmd/dotenvctl@main
```

```sh
dotenvctl get DATABASE_URL --expand        # print one value, references resolved
dotenvctl set DB_HOST=db.prod --after DB_PORT
dotenvctl unset OLD_KEY                    # comments it out — reversible
dotenvctl restore OLD_KEY                  # …and back, byte-identical
dotenvctl run -- npm start                 # child gets the file's values; your env untouched
dotenvctl list --prefix GITHUB_SECRET_ --strip-prefix --json   # read one audience of a shared file
```

And the multi-environment layer — the part `sed` and parse-to-map tools cannot offer at all:

```sh
$ dotenvctl matrix .env.staging .env.prod --only-drift
KEY                        stag  prod
DB_PASS                    ✓     !
EXTRA                      —     ✓
FEATURE_X                  ✓     #
GITHUB_SECRET_DEPLOY_PATH  ✓     —

✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
```

```sh
dotenvctl envs                             # discover the directory's .env family
dotenvctl diff .env.staging .env.prod      # effective-config diff; exit 1 on difference, like diff(1)
dotenvctl matrix --contract .env.example   # CI gate: exit 1 when an env misses a contract key
dotenvctl matrix --format html -o envs.html   # shareable report — secrets masked by default
dotenvctl github push --prefix GITHUB_SECRET_ --strip-prefix   # sync to GitHub Actions secrets via gh
```

Every verb takes `-f <file>` (default `./.env`) and `--json` (stable `{ok,data|error}` envelope). Mutating verbs support `--dry-run`. Exit codes: `0` ok, `1` operation failed, `2` usage. The CLI is a separate Go module, so this library stays dependency-free — and every CLI operation is also an exported Go function ([`envkit`](docs/go-api.md)).

**Full CLI documentation:** [cli/README.md](cli/README.md) for the pitch and tour · [docs/](docs/README.md) for [getting started](docs/getting-started.md), the [command reference](docs/commands.md), [multi-env tools](docs/multi-env.md), [recipes](docs/recipes.md), and [plugins](docs/plugins.md).

The rest of this page is the **library** documentation.

## Reading

Just want the values? One call: `m, err := dotenv.Read(".env")` returns a `map[string]string` with references resolved ([`ExampleRead`](example_read_test.go)).

That is `Open` + `ExpandedMap`, for the common case where you only need the configuration and will never write the file back. Comments, blanks, disabled settings, and unrecognised lines are all skipped; duplicates collapse to last-wins; `${VAR}` references arrive resolved.

Use the full API when you need to edit, inspect structure, or keep references intact. `dotenv.Open(path)` reads from disk (a missing file is not an error); `dotenv.Parse(content)` and `dotenv.ParseReader(r)` take a string or any `io.Reader`, so a pipe, an embedded fixture, or a secret store needs no filesystem. Every accessor then works the same way:

<!-- snippet: example_readme_test.go#readme_Reading -->
```go
// Example_reading shows what each read accessor returns for a file with a
// duplicated key and a commented-out setting: duplicates resolve to the last
// occurrence, and disabled lines are invisible to every accessor.
func Example_reading() {
	f := dotenv.Parse("A=1\nB=2\nA=3\n# C=off\n")

	v, ok := f.Get("A")
	fmt.Println(v, ok)             // last occurrence wins
	fmt.Println(f.Has("C"))        // disabled lines are not active keys
	fmt.Println(f.Count("A"))      // how many active A lines exist
	fmt.Println(f.Keys())          // active keys, file order, deduplicated
	fmt.Println(len(f.Pairs()))    // active pairs, duplicates included
	fmt.Println(f.Map()["A"])      // effective view
	fmt.Println(len(f.Disabled())) // the commented-out setting is still known
	// Output:
	// 3 true
	// false
	// 2
	// [A B]
	// 3
	// 3
	// 1
}
```

**Prefer `ParseReader` for anything large.** `io.ReadAll` doubles its buffer as it grows, and converting the resulting `[]byte` to a string copies everything again. `ParseReader` copies into a `strings.Builder`, whose buffer becomes the string directly, so both costs disappear:

```
BenchmarkParseLarge/ReadAll_then_Parse    13,040,856 B/op    93 allocs/op
BenchmarkParseLarge/ParseReader            9,845,502 B/op    67 allocs/op
```

That is a ~1.4 MB file — realistic once a value holds a PEM block or a base64 certificate. Re-run it with `task bench` rather than trusting the numbers.

Neither call streams: preserving a file byte for byte means holding every line. `ParseReader` lowers the cost, it does not remove it.

### Cost at scale

Measured on this machine, a file of many small entries:

| File | Entries | Parse time | Heap |
|---|---|---|---|
| 1 MB | 22,547 | 26 ms | +5 MB |
| 5 MB | 111,542 | 144 ms | +19 MB |

Roughly 4× the file size in memory and linear in time. `TestParse_ScalesLinearly` guards the complexity class — it caught a quadratic regression that made the 5 MB case take **23 seconds**, which no unit test noticed because they all run on a handful of lines.

`Open` does not guess a location; you pass the path. Resolving *where* a file lives (a config directory, the repository root) is a path-resolution library's job, not this package's: resolve the path first, then pass it to `Open`.

Full guide: [Reading a .env file](docs/library/reading.md) · [Entries and kinds](docs/library/entries.md). Not sure which entry point? See [Choosing how to read](docs/choosing/reading.md).

## Writing

`Set` updates a key in place or appends it, [`Unset(key, false)`](ds:block?id=unset-ahj88kj2) comments it out reversibly (`true` removes the line), and [`Save` writes atomically](ds:block?id=writeto-eemankjv).

### Placing a new key

`Set` appends at the end of the file. To put a new key beside related ones, anchor it on a key already there with `SetAfter(anchor, key, value)` or `SetBefore(anchor, key, value)`:

<!-- snippet: example_readme_test.go#readme_Editing -->
```go
// Example_editing covers the upsert family: Set updates in place or appends,
// SetAfter places a new key next to an anchor, a missing anchor is an error
// rather than a silent append, and Unset comments a key out reversibly.
func Example_editing() {
	f := dotenv.Parse("DB_HOST=localhost\nDB_PORT=5432\nOLD_KEY=x\n")

	created := f.Set("API_KEY", "secret") // appends: the key was new
	fmt.Println("created:", created)

	if _, err := f.SetAfter("DB_PORT", "DB_PASSWORD", "s3cret"); err != nil {
		fmt.Println(err)
		return
	}

	_, err := f.SetAfter("NO_SUCH_KEY", "X", "1")
	fmt.Println("missing anchor:", errors.Is(err, dotenv.ErrAnchorNotFound))

	f.Unset("OLD_KEY", false) // false: comment it out; true would delete the line
	fmt.Print(f.Render())
	// Output:
	// created: true
	// missing anchor: true
	// DB_HOST=localhost
	// DB_PORT=5432
	// DB_PASSWORD=s3cret
	// # OLD_KEY=x
	// API_KEY=secret
}
```

An **existing** key is updated in place and never moved — relocating it would rewrite two regions of the file for a one-value change. A missing anchor returns `ErrAnchorNotFound` and leaves the file untouched, rather than silently appending at the end while reporting success.

Inserting by *section name* is designed but deferred: section boundaries are a convention rather than syntax, so it has to guess, while `SetAfter` cannot be wrong.

`Set` on a duplicated key updates the **last** occurrence, because that is the one a consumer actually sees.

What `Set` keeps on the line it edits: the `export` prefix, any indentation, the spacing *before* the delimiter, the `=` or `:` delimiter itself, and an inline comment. Spacing *after* the delimiter is not kept (`A = 1` becomes `A =2`), and the value's quoting is chosen afresh for the new value ([`ExampleFile_Set_quoting`](example_edit_test.go)).

Step-by-step guides: [Editing](docs/library/editing.md) · [Saving](docs/library/saving.md). Not sure which method? See [Choosing how to edit](docs/choosing/editing.md).

## Generating a file

`Set` upserts. To emit content — comments, blank lines, whole sections — build entries and place them literally. `Append` adds them at the end; `InsertAfter` and `InsertBefore` anchor a block on an existing key, and it lands as a unit. Set `f.Path` and call `Save` (or call `SaveAs`) to write the result:

<!-- snippet: example_readme_test.go#readme_Generating -->
```go
// Example_generating builds a file from nothing with the literal family:
// constructors make entries, Append places them at the end, and InsertAfter
// places a block next to an anchor as one unit.
func Example_generating() {
	// banner is the rule line around each section heading.
	const banner = "------------------"

	f := dotenv.Parse("")
	f.Append(
		dotenv.NewComment(banner, "DATABASE", banner),
		dotenv.NewPair("DB_HOST", "localhost"),
		dotenv.NewPair("DB_PORT", "5432"),
		dotenv.NewBlank(),
		dotenv.NewComment(banner, "AUTH", banner),
		dotenv.NewPair("AUTH_SECRET", "change me"),
	)
	err := f.InsertAfter("DB_PORT", dotenv.NewPair("DB_PASSWORD", "s"))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Print(f.Render())
	// Output:
	// # ------------------
	// # DATABASE
	// # ------------------
	// DB_HOST=localhost
	// DB_PORT=5432
	// DB_PASSWORD=s
	//
	// # ------------------
	// # AUTH
	// # ------------------
	// AUTH_SECRET="change me"
}
```

### Two families, and the difference matters

| Family | Semantics | Use for |
|---|---|---|
| `Set`, `SetAfter`, `SetBefore` | **upsert** — guarantees one active entry; updates in place if the key exists | editing |
| `Append`, `InsertAfter`, `InsertBefore` | **literal** — places exactly what you pass, duplicates included | generating |

On `A=1`, `Set("A", "2")` leaves one entry, `A=2`; `Append(NewPair("A", "2"))` leaves two, `A=1` and `A=2`, and the last wins ([`ExampleFile_Append_vsSet`](example_edit_test.go)). Blurring them is how a key silently moves.

### Constructors

| | |
|---|---|
| `NewPair(key, value)` | a `KEY=value` entry; quoting is chosen when it is placed |
| `NewComment(lines...)` | one line per argument, each prefixed `# ` unless it already starts with `#` |
| `NewBlank()` | an empty line, for separating sections |

Constructed entries carry no rendered text until `Append` or `Insert` attaches them — the line ending belongs to the destination file, which the entry does not know yet. An entry taken from `Entries()` is already attached and is copied **verbatim**, so re-placing one preserves its exact original bytes.

> **Hazard.** Appending to a file whose last value has an *unterminated quote* puts the new content inside that value, because the open quote keeps consuming lines. The source is already malformed and nothing here can fix it — a parser that guessed where the quote should have closed would corrupt legitimate multi-line values. Check the result when writing to files you did not author.

## Disabled settings

A commented-out setting is its own kind, not prose:

```env
# DB_USER=admin      ← KindDisabledPair: Key and Value populated, INACTIVE
# just a note        ← KindComment
```

That holds regardless of who commented it out — by hand or via `Unset` — because the two mean the same thing. `f.Entries()` returns every entry with its `Kind` (`KindPair`, `KindDisabledPair`, `KindComment`, `KindBlank`, `KindInherited`, `KindOther`), and `f.Disabled()` lists every commented-out setting as a `Pair`, in file order:

<!-- snippet: example_readme_test.go#readme_Disabled -->
```go
// Example_disabledSettings shows that a commented-out setting is its own kind
// with a readable key and value, that it stays inactive, and that Restore turns
// it back on without inventing any bytes.
func Example_disabledSettings() {
	f := dotenv.Parse("# DB_USER=admin\n# just a note\nDB_HOST=db\n")

	for _, e := range f.Entries() {
		fmt.Printf("%-13s %q\n", e.Kind, e.Key)
	}
	fmt.Println(f.Has("DB_USER"), f.Disabled()[0].Key)

	f.Restore("DB_USER")
	fmt.Print(f.Render())
	// Output:
	// disabled-pair "DB_USER"
	// comment       ""
	// pair          "DB_HOST"
	// false DB_USER
	// DB_USER=admin
	// # just a note
	// DB_HOST=db
}
```

Disabled entries stay **inactive** — `Get`, `Has`, `Count`, `Map`, and `Keys` all ignore them, exactly as a consumer of the file would. The point is that a tool can now say *"DB_USER is disabled"* rather than *"DB_USER is missing"*, which are different problems with different fixes.

**The trade:** prose shaped like an assignment is misfiled. `# TODO=fix this` reads as a disabled setting — which is also what a human skimming the file would assume. Because `:` is a delimiter too, so do `# Note: rotate monthly` and `# http://example.com` ([`ExampleKind_pitfalls`](example_read_test.go)). A commented value whose quote does not close on its own line stays prose, since a multi-line block cannot be reassembled from separate comment lines.

### Turning one back on

`f.Unset("DB_USER", false)` comments a setting out; `f.Restore("DB_USER")` turns it back on ([`ExampleFile_Restore`](example_edit_test.go)). `Restore` removes the exact marker recorded for that entry, so `#DB_USER=admin` comes back without inventing a space, and an indented `   # DB_USER=admin` keeps its indentation. Disable followed by restore returns the file to its **original bytes** — a fuzz property, not just a test case.

Two limits, both pinned by tests:

- With more than one disabled entry for a key, `Restore` targets the **last** — consistent with `Get`, `Set`, and `Unset`, but not necessarily the one `Unset` just created.
- A commented-out **multi-line** value cannot be restored from a re-read file: each of its lines parses as a separate comment. Within one session, where `Unset` kept the block together, it reverses completely.

See [Entries and kinds](docs/library/entries.md) for all six kinds and the parsing pitfalls, and [Editing, steps 3-5](docs/library/editing.md#step-3--turn-a-setting-off-with-unset) for `Unset`, `Restore` and `Disabled`.

## Inherited declarations

Compose's env-file grammar allows a line that is **just a name** — no delimiter, no value:

```env
DATABASE_URL=postgres://localhost/dev
HOME
AWS_SECRET_ACCESS_KEY
```

To Compose that means *inherit this variable from the process environment* — the idiom for whitelisting host variables into a container without ever writing their values into the file. This package recognises the declaration as its own kind, `KindInherited`, but **never resolves it**: reading `os.Environ` is exactly what this package promises not to do. So the entry is inactive — `Get`, `Keys`, and `Map` skip it, and a `${reference}` to it expands like any other unset variable.

`Inherited` lists the declared names so an application that wants Compose's behaviour applies its own source, with its own precedence:

In a real program the source is usually `os.LookupEnv`; here a map stands in for it:

<!-- snippet: example_readme_test.go#readme_Inherited -->
```go
// Example_inheritedDeclarations reads a Compose-style name-only line. The
// package lists the name but never resolves it; the application layers its
// own source on top (a map stands in for os.LookupEnv here).
func Example_inheritedDeclarations() {
	f := dotenv.Parse("DATABASE_URL=postgres://localhost/dev\nAWS_REGION\n")
	host := map[string]string{"AWS_REGION": "eu-west-1"} // stands in for the process environment

	env := f.Map()
	fmt.Println(f.Inherited(), len(env))
	for _, name := range f.Inherited() {
		if v, ok := host[name]; ok { // the caller's decision, not the parser's
			env[name] = v
		}
	}
	fmt.Println(env["AWS_REGION"])
	// Output:
	// [AWS_REGION] 1
	// eu-west-1
}
```

The same layering the package prescribes for loading in general: the file describes, the application decides. Full guide: [Editing, step 6](docs/library/editing.md#step-6--handle-inherited-declarations).

## Cloning between environments

Deriving `.env.prod` from `.env.staging` is the case this package's read/write split was designed for.

`Clone` makes an independent copy, `Set` changes only what differs, and `SaveAs(".env.prod")` writes it elsewhere without touching the source's `Path`:

<!-- snippet: example_readme_test.go#readme_Cloning -->
```go
// Example_cloning derives production from staging: Clone, change one value,
// and every reference stays a reference, so the derived values follow the
// edit. The source file is left untouched.
func Example_cloning() {
	staging := dotenv.Parse("DOMAIN=staging.acme.io\nAPI_URL=https://api.${DOMAIN}\nSECRET=${VAULT_TOKEN:?set me}\n")

	prod := staging.Clone() // independent deep copy
	prod.Set("DOMAIN", "acme.io")
	fmt.Print(prod.Render())

	url, _, err := prod.GetExpanded("API_URL")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(url)
	domain, _ := staging.Get("DOMAIN") // present in the source
	fmt.Println(domain)
	// Output:
	// DOMAIN=acme.io
	// API_URL=https://api.${DOMAIN}
	// SECRET=${VAULT_TOKEN:?set me}
	// https://api.acme.io
	// staging.acme.io
}
```

Given this source:

```env
# ------------------------------
# SHARED
# ------------------------------
DOMAIN=staging.acme.io

# ------------------------------
# DERIVED — these must stay as references
# ------------------------------
API_URL=https://api.${DOMAIN}
CALLBACK=${API_URL}/oauth/callback
SECRET=${VAULT_TOKEN:?set me}
```

the clone comes out with **one line changed** and everything else byte-identical — banners, blank lines, references, and the required-variable guard all intact:

```env
DOMAIN=acme.io
API_URL=https://api.${DOMAIN}          ← still a reference
CALLBACK=${API_URL}/oauth/callback     ← still chained
SECRET=${VAULT_TOKEN:?set me}          ← still armed
```

### Why references survive: `Render` never expands

`f.Get("API_URL")` returns `https://api.${DOMAIN}`, what is on disk; `f.GetExpanded("API_URL")` returns `https://api.staging.acme.io`, the derived value consumers see.

`Render` and `Save` write **only** `Raw`. Expansion happens in exactly one method, and nothing in the write path calls it — so a clone keeps references literal by default. You would have to go out of your way to bake them in.

### The cascade, and why it matters

A reference is a **formula**, not a value — like `=A1*2` in a spreadsheet rather than a typed number. Store the formula and it re-evaluates; store the result and it is frozen.

Changing `DOMAIN` once, in a file that stored the formula:

| | on disk after the edit | `API_URL` resolves to |
|---|---|---|
| stored as a reference | `https://api.${DOMAIN}` | `https://api.acme.io` ✅ |
| stored baked | `https://api.staging.acme.io` | `https://api.staging.acme.io` ❌ |

The baked file is now **wrong and silent**: it still points at staging, nothing errors, and you find out when production traffic hits the staging API.

`CALLBACK=${API_URL}/oauth/callback` follows too, because it references `API_URL` which references `DOMAIN`. One edit, the whole chain re-resolves.

> **The trap.** Building the new file from `ExpandedMap()` instead of cloning bakes *every* reference at once. The result looks correct on the day you write it and quietly stops tracking from then on.

### Freezing a value on purpose

Sometimes you *want* a value pinned so it stops following its source — a secret resolved once at deploy time, say. That is a caller decision, so it is a recipe rather than an API: read the value with `GetExpanded`, then `Set` it back, and it is literal from then on. [`ExampleFile_Clone_baked`](example_edit_test.go) shows the difference.

### Several variants from one source

`SaveAs` does not mutate `f.Path`, so a single loaded file can emit as many as you need: loop over your environments, and for each one `Clone` the source, `Set` what differs and `SaveAs(".env." + name)` ([`ExampleFile_SaveAs`](example_edit_test.go)).

`Clone` is an independent deep copy — editing either side leaves the other alone — and carries the file's line-ending style, mode, and parse options across. It is cheaper and more faithful than `Parse(f.Render())`, which re-parses and can reclassify entries on the second pass.

**Permissions on `SaveAs`:** an existing destination keeps its own mode; a new one gets the source file's. Overwriting must never widen a file somebody deliberately locked down, nor loosen one about to hold secrets.

Full guide: [Editing, step 9 — Clone](docs/library/editing.md#step-9--derive-another-environment-with-clone) · [Saving, step 4 — SaveAs](docs/library/saving.md#step-4--write-a-copy-with-saveas).

## Plugins

Decryption, secret stores, auditing, and save-time validation live outside this package while running inside its pipeline. A plugin is an object implementing whichever capability interfaces it needs — one plugin may hold several, sharing a client and a cache between them.

You install one with `dotenv.Open(path, dotenv.WithPlugin(p))` (or the same option on `Parse`), and `f.Plugins()` reports which capabilities each installed plugin was detected as having ([`ExamplePluginInfo`](example_plugin_test.go)).

### The six hooks

| Capability | When | Ordering | May fail | Affects bytes |
|---|---|---|---|---|
| `EntryObserver` | once per entry during parse | all, install order | no | no |
| `Lookuper` | a `${VAR}` the file does not define | **first non-miss wins** | yes | no |
| `ValueTransformer` | after a value is expanded | **chained**, each sees the last output | yes | no |
| `CommandRunner` | a `$(cmd)` | **last installed wins** — a replacement, not a chain | no — a runner error expands to `""`, as a shell does | no |
| `ExpandObserver` | every reference resolution | all, install order | no | no |
| `SaveGuard` | before `Save` / `SaveAs` writes | all; **first error aborts** | yes | veto only |

### The firewall

> **A hook may change what a value READS as. It may never change what gets WRITTEN.**

Every value-changing hook lives in the expansion path, which `Render` and `Save` never call. So round-trip stays byte-exact regardless of which plugins are installed, and a decrypted secret can never be written back in plaintext. `SaveGuard` sits nearest the write path and may only return an error — it cannot rewrite.

That is structural rather than a convention, and it is asserted two ways: `TestPlugins_NeverAffectBytes`, a plain test that renders one file with every hook and with none, and `FuzzPluginsNeverAffectBytes`, which does the same for arbitrary input. Two caveats apply to plugin code you write: a `SaveGuard` receives the live `*File`, so a guard that calls `f.Set` changes what is saved, and an `EntryObserver` receives the live `*Entry`, so mutating it changes the rendered line. Treat both as read-only ([plugins guide, pitfalls](docs/library/plugins.md#pitfalls)).

### Worked examples

**Environment fallback and decryption** — `WithLookup(os.LookupEnv)` gives Compose's behaviour, opted into rather than imposed; `WithValueTransform` gives `encrypted:` parity as a plugin rather than a feature this package owns. Here a map stands in for each source:

<!-- snippet: example_readme_test.go#readme_Plugins -->
```go
// Example_plugins installs two single-function hooks: a lookup that fills a
// reference the file does not define, and a value transform that decrypts.
// Get still returns what is on disk; only GetExpanded sees the plaintext, and
// the rendered bytes never change.
func Example_plugins() {
	// cipherPrefix marks a value the transform must decrypt.
	const cipherPrefix = "encrypted:"
	vault := map[string]string{"c2VjcmV0": "hunter2"} // a fake secret store
	fallback := map[string]string{"REGION": "eu-west-1"}

	src := "DB_PASS=encrypted:c2VjcmV0\nZONE=${REGION}-a\n"
	f := dotenv.Parse(src,
		dotenv.WithLookup(func(name string) (string, bool) {
			v, ok := fallback[name]
			return v, ok
		}),
		dotenv.WithValueTransform(func(key, v string) (string, error) {
			s, ok := strings.CutPrefix(v, cipherPrefix)
			if !ok {
				return v, nil
			}
			plain, found := vault[s]
			if !found {
				return "", fmt.Errorf("%s: no secret for %q", key, s)
			}
			return plain, nil
		}),
	)

	raw, _ := f.Get("DB_PASS") // present in the file
	pass, _, err := f.GetExpanded("DB_PASS")
	if err != nil {
		fmt.Println(err)
		return
	}
	zone, _, err := f.GetExpanded("ZONE")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(raw)
	fmt.Println(pass)
	fmt.Println(zone)
	fmt.Println(f.Render() == src)
	// Output:
	// encrypted:c2VjcmV0
	// hunter2
	// eu-west-1-a
	// true
}
```

The file always wins; a lookup only fills references the file does not define. A lookup that returns an **error** (the `Lookuper` interface, or `WithPlugin`) aborts rather than counting as a miss, because an unreachable secret store must not look like an unset variable — otherwise a deploy proceeds with an empty password ([`ExampleWithPlugin_lookupError`](example_plugin_test.go)). `Get` still returns the ciphertext, so that is what gets written back. Only `GetExpanded` sees plaintext.

**Refusing to save a plaintext secret** — `WithSaveGuard(func(f *dotenv.File) error)` runs before `Save` and `SaveAs` write; returning an error vetoes the write. A refusal leaves the destination exactly as it was — nothing is written, and no temp file is left beside it ([`ExampleWithSaveGuard`](example_plugin_test.go)).

**Dead-key detection** — an `ExpandObserver` sees every reference resolution, so it can record which declared variables nothing references ([`ExampleExpandObserver`](example_plugin_test.go)).

Observers see **every** entry, including `KindDisabledPair` and unrecognised lines: a commented-out setting is a finding for an auditor, not noise.

### Writing a plugin

A plugin is a type with a `Name() string` method plus whichever capability methods it needs, for example `Lookup(name string) (string, bool, error)` and `TransformValue(key, val string) (string, error)`. Pair it with compile-time assertions such as `var _ dotenv.Lookuper = (*Vault)(nil)`: an optional interface with a wrong signature compiles, installs, and never runs, and the assertion turns that silence into a build failure. [`ExampleWithPlugin`](example_plugin_test.go) is a complete plugin with several capabilities.

That assertion is not optional style. `func (v *Vault) LookUp(...)` or a missing `error` return compiles fine and leaves the plugin inert. `f.Plugins()` reports what was actually detected, so a missing capability is visible — but a compile-time assertion catches it before it ships.

Capabilities are resolved **once at construction** into pre-filtered slices, so a plugin without a capability costs nothing on the expansion path.

Full guide: [Plugins](docs/library/plugins.md). Not sure which hook fits? See [Choosing a plugin hook](docs/choosing/plugins.md).

## Typed configuration

Every value here is a `string`, deliberately. To get `int`, `bool`, `time.Duration`, or a slice, hand the file to a binder — this package supplies the values, the binder supplies the types.

The standard library alone is enough for a handful of fields: [`Example_typedConfig`](example_plugin_test.go) reads the file with `dotenv.Read` and converts each value with `strconv` and `time`, reporting every bad value.

For struct tags, defaults and required fields, use a binder. A `*File` satisfies [`sethvargo/go-envconfig`](https://pkg.go.dev/github.com/sethvargo/go-envconfig)'s `Lookuper` interface with a three-line adapter (`Lookup(key string) (string, bool)` calling `f.GetExpanded`), or you pass it the map from `Read` through `envconfig.MapLookuper(m)`, so the two compose with no dependency in either direction. go-envconfig v1 takes both through its config struct: `envconfig.ProcessWith(ctx, &envconfig.Config{Target: &cfg, Lookuper: fileLookuper{f}})`. The older `ProcessWith(ctx, &cfg, lookuper)` form is pre-v1 and does not compile against v1.

`dotenv` handles comments, references, and round-tripping; the binder handles types, defaults, required fields, slices, and nested prefixes. go-envconfig stops at the first field it cannot decode; [`caarlos0/env`](https://pkg.go.dev/github.com/caarlos0/env/v11) and cfgkit report every binding error at once. caarlos0/env reads from a map: `env.ParseWithOptions(&cfg, env.Options{Environment: m})` with `m` from `dotenv.Read`.

Complete, compiled programs for each binder are in [Typed configuration](docs/library/typed-config.md): [the standard library](docs/library/typed-config.md#step-2--bind-them-with-the-standard-library), [go-envconfig](docs/library/typed-config.md#step-3--or-bind-them-with-go-envconfig), [caarlos0/env](docs/library/typed-config.md#step-4--or-bind-them-with-caarlos0env), [cfgkit](docs/library/typed-config.md#step-5--or-use-cfgkit-for-layered-configuration).

### A binder built on this package

`github.com/ubgo/cfgkit` is the same idea taken further, and it reads through this package rather than through a plain map — so quoting, multiline values and `${VAR}` expansion behave in the binder exactly as they do here, including `${VAR:-default}` and `${VAR:?message}`. It uses the `Lookuper` seam to let a later file in a `.env` / `.env.local` chain reference a value an earlier one defined, and loads a whole configuration with one generic call, `cfgkit.Load`, over its default sources ([typed-config guide, step 5](docs/library/typed-config.md#step-5--or-use-cfgkit-for-layered-configuration)).

It adds what a binder alone cannot: it reports **which source set each field** (`.env`, `.env.local`, the environment, a secret store), masks values marked secret so they cannot reach a log, validates a configuration **without booting the application** so a stale file fails CI instead of a container, and generates the `.env.example` contract from the struct so the file and the code cannot drift.

The trade is scope. `go-envconfig` and `caarlos0/env` are small and do one thing; `cfgkit` is a layered configuration system. If environment variables are your only source and you want the smallest surface, the two above remain the better fit — this package composes with all three and depends on none of them.

### Why there is no `f.Int("PORT", 8080)`

A `.env` has no types. `PORT=8443` is four characters, and which Go type it becomes depends on the field receiving it — the application's decision, not the format's. `encoding/json` draws the line in the same place: you unmarshal into a struct, there is no `json.GetInt`.

Typed getters also fail in the worst way available. `PORT=eighty` returns the default, silently, and the service comes up on the wrong port with nothing in the logs. A binder reports it as an error alongside every other problem in the file.

Inferring types from a value's *shape* — `8443` becoming a number because it looks numeric — was evaluated and rejected: `ZIP=01234` loses its leading zero, `DESCRIPTION=Hello, world` becomes an array, and the usual escape hatches collide with backtick and glob syntax this package already supports.

> Note `${VAR:?message}` already covers *required* at the file level, so a missing value can fail during expansion rather than at bind time — useful when the variable is referenced by another value rather than bound directly.

Full guide: [Typed configuration](docs/library/typed-config.md).

## Feature support

Every `.env` construct in common use, including the full [Docker Compose interpolation spec](https://docs.docker.com/reference/compose-file/interpolation/). Each row is asserted by a test in `conformance_test.go` — this table reflects behaviour, not intent. Full guides: [.env syntax](docs/library/syntax.md) · [Variable expansion](docs/library/expansion.md).

### Quoting

| Feature | Example | Supported |
|---|---|---|
| Unquoted | `SIMPLE=xyz123` | ✅ |
| Double-quoted | `VAR="VAL"` | ✅ |
| Single-quoted, literal | `VAR='VAL'` | ✅ |
| Backtick, literal | ``VAR=`VAL` `` | ✅ |
| Backtick multiline | ``KEY=`line one``<br>``line two` `` | ✅ |
| Double-quoted multiline | `KEY="line one`<br>`line two"` | ✅ |
| Spaces around `=` | `KEY = value` | ✅ |
| `export` prefix | `export KEY=value` | ✅ |
| Empty value | `KEY=` | ✅ |
| `KEY: value` — colon delimiter | `FOO: bar` | ✅ — in the Compose grammar and Node dotenv's parser; the author's delimiter is preserved on edit (spacing after it is not) |
| Key charset: letters, digits, `_` `.` `-` `[` `]` | `spring.datasource.url=x`, `2FA_SECRET=x` | ✅ — Compose's charset, a superset of Node dotenv's and godotenv's `[\w.-]` |

### Comments

| Feature | Example | Supported |
|---|---|---|
| Whole-line comment | `# a note` | ✅ |
| After a quoted value | `KEY="v" # note` | ✅ |
| `#` with no leading space is **not** a comment | `KEY=VAL# literal` | ✅ |
| `#` inside quotes is preserved | `KEY="a-#-b"` | ✅ |
| Blank lines | | ✅ |
| Unrecognised lines kept verbatim | `K!=v`, `HAS KEY=v` | ✅ — deliberate divergence from compose-go, which errors and refuses the whole file; an editor must stay able to open, edit, and save around content it will never touch |

### Escape sequences

Only inside **double** quotes. Single-quoted and backtick values are fully literal.

| Escape | `EscapeExtended` (default) | `EscapeCompose` |
|---|---|---|
| `\"` | ✅ `"` | ✅ `"` |
| `\\` | ✅ `\` | ✅ `\` |
| `\n` `\r` `\t` | ✅ decoded | ❌ kept literal |
| anything else (`\d`, `C:\Users`) | kept literal | kept literal |

The two modes are genuinely incompatible — under Compose, `\n` stays two characters; under Extended it becomes a newline — so it is a choice rather than a default to work around. Pass `dotenv.WithEscapes(dotenv.EscapeCompose)` to `Parse` or `Open`:

<!-- snippet: example_syntax_test.go#syn_EscapeCompose -->
```go
// ExampleEscapeCompose shows the trap EscapeCompose exists for: a Windows
// path whose folder starts with n or t. Under the default EscapeExtended the
// \n and \t become a newline and a tab; under EscapeCompose the path is kept.
func ExampleEscapeCompose() {
	const src = `DIR="C:\new\tools"` + "\n"

	extended, _ := dotenv.Parse(src).Get("DIR")
	compose, _ := dotenv.Parse(src, dotenv.WithEscapes(dotenv.EscapeCompose)).Get("DIR")
	fmt.Printf("extended: %q\n", extended)
	fmt.Printf("compose:  %q\n", compose)
	// Output:
	// extended: "C:\new\tools"
	// compose:  "C:\\new\\tools"
}
```

Use `EscapeCompose` for files Docker Compose reads, and for values holding Windows paths or regexes where a backslash means itself.

### Interpolation

Applied to unquoted and double-quoted values only. Covers the full [Docker Compose set](https://docs.docker.com/reference/compose-file/interpolation/) — the broadest in common use, so a `.env` written for any other tool also reads correctly here.

| Form | Meaning | Supported |
|---|---|---|
| `${VAR}` / `$VAR` | direct substitution | ✅ |
| `${VAR:-default}` | default when unset **or empty** | ✅ |
| `${VAR-default}` | default only when unset | ✅ |
| `${VAR:+alt}` | alt when set **and non-empty** | ✅ |
| `${VAR+alt}` | alt when set, even if empty | ✅ |
| `${VAR:?error}` | **error** when unset or empty | ✅ |
| `${VAR?error}` | **error** when unset | ✅ |
| `$$` | literal `$` — `$${VAR}` yields `${VAR}` | ✅ |
| Nesting | `${VAR:-${FOO:-default}}` | ✅ |
| Chained / recursive | `C=${B}` where `B=${A}` | ✅ |
| Cycles terminate: the re-entering reference resolves to `""` | `A=a${B}`, `B=b${A}` → `A` is `aba` | ✅ |
| Unresolved → `""`, not an error | `${NOPE}` | ✅ |
| **Single quotes suppress it** when you read that key | `B='${A}'` → `${A}` | ✅ — but see the note below the table |
| **Backticks suppress it** | ``B=`${A}` `` → `${A}` | ✅ |
| `${VAR/foo/bar}` shell-style edits | — | ❌ not supported by Compose either |

A single-quoted or backtick value is literal when you read *that* key. When another value references it (`C=${B}` with `B='${A}'`), the quoted text is expanded during that resolution today, so `C` resolves `${A}`. Keep literal `${…}` text out of values that other values reference ([expansion guide, step 6](docs/library/expansion.md#step-6--quotes-that-turn-expansion-off)).

The two `?` forms exist to fail loudly, so they are the only ones that produce an error, a `*RequiredError` you match with `errors.As`:

<!-- snippet: example_expand_test.go#exp_RequiredError -->
```go
// ExampleRequiredError shows how to catch a required-variable failure with
// errors.As and read which variable is missing. Empty is true for the ":?"
// form (unset or empty) and false for "?" (unset only).
func ExampleRequiredError() {
	src := "DB_PASSWORD=\n" +
		"URL=postgres://app:${DB_PASSWORD:?set it in .env}@db\n" +
		"TOKEN=${API_TOKEN?}\n"

	f := dotenv.Parse(src)
	for _, key := range []string{"URL", "TOKEN"} {
		_, _, err := f.GetExpanded(key)

		var re *dotenv.RequiredError
		if errors.As(err, &re) {
			fmt.Printf("key=%s empty=%v message=%q\n", re.Key, re.Empty, re.Message)
			fmt.Println(" ", err)
		}
	}
	// Output:
	// key=DB_PASSWORD empty=true message="set it in .env"
	//   dotenv: required variable DB_PASSWORD is not set or is empty: set it in .env
	// key=API_TOKEN empty=false message=""
	//   dotenv: required variable API_TOKEN is not set
}
```

`ExpandedMap` stops at the first such error rather than returning a half-expanded map, because a partial map gives the caller no way to tell which values are trustworthy.

Expansion is hand-written rather than delegating to `os.Expand`, which cannot express `$$` or nesting — it stops at the first `}`, so the inner default of `${A:-${B:-c}}` swallows the outer one.

A reference that re-enters a variable already being expanded resolves to `""`, the same answer an unset variable gives and the only one that terminates. The rest of each value is kept, so a cycle can still produce text: with `A=a${B}` and `B=b${A}`, `A` expands to `aba` ([`ExampleFile_GetExpanded_nesting`](example_expand_test.go)). A key also cannot extend itself: `A=1` then `A=${A}2` gives `22`, and `ZONE=${ZONE:-x}` gives `""`, not `x`. Two *separate* references to one variable are not a cycle: `K=${A}-${A}` expands both.

### How `$` is dispatched

One character decides, in a single left-to-right pass. The five cases are mutually exclusive:

| After `$` | Meaning | Example → result |
|---|---|---|
| `$` | literal dollar | `cost 5$$` → `cost 5$` |
| `(` | command | `$(whoami)` → command output |
| `{` | braced variable | `${NAME}` → `world` |
| letter or `_` | bare variable | `$NAME/x` → `world/x` |
| anything else | literal dollar | `100$ or so` → unchanged |

`$${NAME}` is row 1 feeding row 3: the scanner consumes `$$`, emits one `$`, and skips **both** bytes — so the `{NAME}` that follows is ordinary text. Result: `${NAME}`, exactly as Compose does it.

All three forms mix freely in one value, and a command's text is expanded **before** it runs:

```
$USER-$(id)-${NAME}   →   ada-501-world
$(greet ${NAME})      →   runs: greet world
```

That ordering is the shell's own, and it means `${VAR:?err}` errors and the cycle guard both work *through* a command.

### Command substitution

| Feature | Example | Supported |
|---|---|---|
| `$(command)` | `URL="db://$(whoami)@host"` | ✅ **opt-in** |

**Off by default, deliberately.** With it on, merely *reading* a config file executes arbitrary shell — so a `.env` from a repository, a container image, or a teammate becomes remote code execution. That is a decision the calling application makes about files it trusts, not something a parser should do silently.

Turn it on with `dotenv.WithCommandSubstitution(true)`, which runs `sh -c`, or pass `dotenv.WithCommandRunner(runner)` to supply a sandboxed or allow-listed runner instead. Installing a runner turns substitution on; `WithCommandRunner(nil)` turns on the real `sh -c`. Off, the `$(…)` text stays literal:

<!-- snippet: example_expand_test.go#exp_SubstitutionOff -->
```go
// ExampleWithCommandSubstitution_off shows the default: $(command) is never
// run, and the text comes back exactly as written. Reading a file must not
// execute it.
func ExampleWithCommandSubstitution_off() {
	f := dotenv.Parse("WHO=\"user-$(whoami)\"\n")
	got, _, err := f.GetExpanded("WHO")
	fmt.Printf("%q %v\n", got, err)
	// Output:
	// "user-$(whoami)" <nil>
}
```

A failing command expands to `""`, as a shell would inside a value, rather than failing the whole read. [`ExampleWithCommandRunner_allowList`](example_expand_test.go) shows an allow-listed runner; the [expansion guide, step 9](docs/library/expansion.md#step-9--command-substitution-opt-in) covers the risk in full.

### Other

| Feature | Notes |
|---|---|
| `encrypted:` values | Stored verbatim — this package does not decrypt. Pair it with your own key handling |
| CRLF files | Round-trip as CRLF |
| Missing trailing newline | Preserved |
| Duplicate keys | Last wins, matching every dotenv implementation. `Count` surfaces them |
| Name-only lines (`HOME`) | Recognised as a Compose *inherited* declaration — see below. Never resolved: this package does not read the environment |

## Variable expansion

`Get` returns the raw value. `GetExpanded` resolves references against the other entries in the same file — the interpolation Compose performs when *it* reads the file:

`f.ExpandedMap()` expands every active key at once:

<!-- snippet: example_readme_test.go#readme_Expansion -->
```go
// Example_expansion contrasts Get with GetExpanded and shows the common
// operators: a default, a literal dollar, and a single-quoted value read
// directly, which stays literal.
func Example_expansion() {
	f := dotenv.Parse(strings.Join([]string{
		"SECRET=hunter2",
		"ADMIN_PASS=${SECRET}",
		"PORT=${HTTP_PORT:-8080}",
		"PRICE=5$$",
		"LITERAL='${SECRET}'",
	}, "\n") + "\n")

	raw, _ := f.Get("ADMIN_PASS") // present in the file
	fmt.Println(raw)
	for _, key := range []string{"ADMIN_PASS", "PORT", "PRICE", "LITERAL"} {
		v, _, err := f.GetExpanded(key)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("%s=%s\n", key, v)
	}
	// Output:
	// ${SECRET}
	// ADMIN_PASS=hunter2
	// PORT=8080
	// PRICE=5$
	// LITERAL=${SECRET}
}
```

Read directly, single-quoted and backtick values are not interpolated — `SECRET='${A}'` means the literal text `${A}`. Expanding it would silently replace a value the author explicitly marked as literal. (A reference *to* such a value is the exception noted under [Interpolation](#interpolation).)

Expansion is depth-bounded, so a reference cycle terminates instead of hanging the caller.

> **Use `Get`, not `GetExpanded`, for anything you write back.** Expansion must feed what a consumer *reads*, never what gets persisted — otherwise a reference is flattened into a copy of its target and the link is lost.

Full guide: [Variable expansion](docs/library/expansion.md). Not sure which setup? See [Choosing an expansion setup](docs/choosing/expansion.md).

## Saving safely

`.env` files hold credentials, so `Save`:

- writes a sibling temp file at **0600 first** — never a wider window, even briefly
- `chmod`s it to the target mode, then **renames** over the destination

A rename within a directory is atomic, so a reader never observes a half-written credentials file, and a crash mid-save leaves the original intact. A failed save removes the temp file rather than leaving it beside the real one.

Existing file modes are preserved; new files are created `0600`. The explicit `chmod` is necessary because `os.WriteFile`'s mode argument is filtered by the process umask, so it cannot be relied on to reproduce the original permissions.

Full guide: [Saving a .env file](docs/library/saving.md).

## Line endings and trailing newlines

Terminators are preserved **per line**, not per file. A file with mixed endings round-trips as mixed rather than being normalised into one style, which would be a whole-file diff nobody asked for.

Lines this package *writes* [use CRLF as soon as the file contains a single CRLF anywhere](ds:block?id=eol-eefm88t3), otherwise LF — so appending to a CRLF file does not introduce a stray LF. It is not a majority vote: one CRLF line in an otherwise LF file is enough.

<!-- snippet: example_readme_test.go#readme_LineEndings -->
```go
// Example_lineEndings shows that each existing line keeps its own terminator,
// and that a line the package writes uses CRLF as soon as the file contains a
// single CRLF anywhere, not the style most of its lines use.
func Example_lineEndings() {
	f := dotenv.Parse("A=1\nB=2\r\nC=3\n") // two LF lines, one CRLF line
	f.Set("D", "4")
	fmt.Printf("%q\n", f.Render())
	// Output:
	// "A=1\nB=2\r\nC=3\nD=4\r\n"
}
```

A file that ended without a trailing newline still does not. Adding or removing one would be a spurious diff on every save. Full guide: [Saving, step 3](docs/library/saving.md#step-3--keep-line-endings-and-the-trailing-newline).

## API

Every symbol below, with the guide section and runnable example for each, is in [the feature index](docs/reference/features.md).

| Symbol | Purpose |
|---|---|
| `Read(path)` | the shortcut — straight to `map[string]string`, expanded |
| `Open(path)` / `Parse(content)` / `ParseReader(r)` | read from disk / a string / any `io.Reader` |
| `f.Get` / `Has` / `Count` | read one key |
| `f.GetExpanded` | read one key, interpolated — returns `(value, found, error)` |
| `f.Keys` / `Pairs` / `Map` / `ExpandedMap` | read all |
| `f.Set` / `SetAfter` / `SetBefore` | edit — upsert |
| `f.Append` / `InsertAfter` / `InsertBefore` | generate — literal placement |
| `NewPair` / `NewComment` / `NewBlank` | build entries to place |
| `f.Unset` / `Restore` / `Disabled` | turn settings off and on |
| `f.Inherited` | names declared as inherited from the environment (never resolved here) |
| `WithEscapes` / `WithCommandSubstitution` | dialect options |
| `WithPlugin` | install a plugin |
| `WithLookup` / `WithValueTransform` / `WithSaveGuard` / `WithCommandRunner` | single-function hooks |
| `f.Plugins` | what each installed plugin was detected as |
| `Plugin`, `Lookuper`, `ValueTransformer`, `CommandRunner`, `SaveGuard`, `EntryObserver`, `ExpandObserver` | capability interfaces |
| `f.Render` / `Save` / `SaveAs` | serialize / write atomically / write elsewhere |
| `f.Clone` | independent deep copy |
| `f.Entries` | every entry, including comments and blanks |
| `f.Existed` / `Mode` / `Path` | file facts |
| `Kind` (6 kinds), `Entry`, `Pair`, `RequiredError`, `ErrAnchorNotFound` | types and errors |

## Testing

| | |
|---|---|
| Statement coverage — library | 100.00% (611/611 statements) |
| Statement coverage — CLI | 99.52% (1237/1243 statements) |
| Test functions | 255 |
| Test cases including subtests | 845 |
| Fuzz properties | 8 |
| Dependencies (library) | 0 |

Re-measure any of it with `task cover` (both modules) or `task test:uncovered` (what's left, per function). The numbers above are the measured ones, not rounded claims.

The CLI's remaining fraction is **six statements**, enumerable rather than mystery: the `os.Exit` wrapper in `main`, the Windows arm of `isExecutable` on a non-Windows host, two fallbacks for a host that cannot locate its own working directory or executable, and two expansion-error arms the CLI's own wiring cannot produce — it never attaches a library plugin, and a plugin is the only thing that makes expansion fail with something other than a `RequiredError`. Every one is named, with its reason, in [CONTRIBUTING](CONTRIBUTING.md#coverage-and-the-six-statements-that-are-not-covered).

Conformance is asserted by tests, not claimed — `conformance_test.go` has one assertion per syntax rule in the tables above. Every code block in the docs is a runnable `Example` (compiled and output-checked by `go test`), and `task docs:check` fails if a block drifts from its example.

**Property-based fuzzing** covers what example tests structurally cannot:

| Property | Guarantee |
|---|---|
| `FuzzParseRender` | parse → render returns the input byte for byte, for *any* input |
| `FuzzParseIsIdempotent` | re-processing a file never drifts |
| `FuzzSetRoundTrip` | whatever `Set` writes, the parser reads back identically |
| `FuzzSetIsIdempotent` | repeating an edit produces no churn |
| `FuzzExpandTerminates` | expansion always halts and never panics |
| `FuzzAppendRoundTrip` | whatever `Append` places, the parser reads back identically |
| `FuzzDisableEnableIsReversible` | `Unset` then `Restore` returns the file to its original bytes |
| `FuzzPluginsNeverAffectBytes` | no plugin can change the rendered bytes, for any input |

```sh
task fuzz -- FuzzParseRender 30s   # one property
task fuzz:all                      # every property, 30s each
task fuzz:list                     # what is available
```

### Bugs the property tests found

Worth stating because all three passed 100% line coverage — coverage proves every line *ran*, not that it was *right*.

1. **Backtick values were corrupted.** Adding backtick quoting to the parser did not teach the renderer about it, so `Set(k, "`x`")` wrote a bare value that parsed back as `x`. Caught by `FuzzSetRoundTrip`.
2. **Mixed line endings were normalised.** A single whole-file CRLF flag turned `"\r\n\n"` into `"\r\n\r\n"`. Terminators are now preserved per line. Caught by `FuzzParseRender`.
3. **Parsing was quadratic.** The lookahead for multi-line values pre-trimmed carriage returns by copying the entire remaining line slice — once per pair. A 5 MB file took 23 seconds; trimming lazily brought it to 0.14. Found by measuring, not by a test, which is why `TestParse_ScalesLinearly` now exists.
4. **`A=$A$A` hung the process.** A depth bound alone cannot stop a value that branches at every level — bounded at 32, that is still 2³² expansions. Cycles are now cut by variable name. Caught by `FuzzExpandTerminates`.

Two further properties surfaced **hazards rather than bugs**, both now pinned by their own tests: appending after an unterminated quote, and `Restore` picking the last of several disabled entries. Neither is fixable — the first is a malformed source, the second is the "last wins" rule this package applies everywhere — so they are documented instead of papered over.

Both failing inputs are checked in under `testdata/fuzz/`, so they run on every `go test` forever.

## FAQ

**Does it load variables into my process like godotenv?** No, deliberately — it never touches `os.Environ`. It parses, edits, and writes the file; you apply the values with your own precedence via `f.Map()`, or run a child process with them via `dotenvctl run`. That is why the constructor is `Open`, not `Load`.

**Will editing a file destroy my comments and formatting?** No — that is the whole point. Parse → render with no changes is byte-identical, and `Set` touches only the entry you name. Both invariants are pinned by fuzz tests, not just claimed.

**Is it compatible with Docker Compose `.env` files?** Yes — the full Compose interpolation spec (`${VAR:-default}`, `${VAR:?err}`, nesting, `$$`), Compose's key charset, the `:` delimiter, and name-only inherited declarations. Every rule is asserted in `conformance_test.go`; use `WithEscapes(EscapeCompose)` for exact escape parity.

**Does the library have dependencies?** Zero — stdlib only. The CLI is a separate Go module, so cobra never enters the library's dependency graph.

**How do I get typed values like `int` or `time.Duration`?** Compose with a binder — `sethvargo/go-envconfig` or `caarlos0/env` for the smallest surface, or `ubgo/cfgkit`, which is built on this package and adds layered sources, provenance, secret masking and `.env.example` generation. go-envconfig takes the file through a three-line adapter, and caarlos0/env takes the map from `Read`; [Typed configuration](docs/library/typed-config.md) has compiled programs for each. Typed getters were rejected on purpose: `PORT=eighty` silently returning a default is a production incident.

**Can it manage secrets in GitHub Actions or Vercel?** Yes — `dotenvctl github push` / `vercel push` sync a *selection* of your file (prefix-based, renamed, placeholder-guarded) via the `gh`/`vercel` CLIs. Values travel by stdin, never argv, and output never prints them.

**How is `dotenvctl` different from dotenv-cli or dotenvx?** Those focus on loading a file into a process (and, for dotenvx, encrypting it). `dotenvctl` manages the file itself: byte-preserving edits, reversible disable, an environment drift matrix, contract gates for CI, and outward secret sync. See the [comparison](#why-not-a-dotenv-library).

**Is it safe to write files that hold credentials?** Saves are atomic (temp file at `0600`, then rename), new files are created `0600`, existing permissions are preserved, and shareable CLI output masks values unless you pass `--reveal`.

<sub>dotenv is an open-source, comment-preserving .env file parser, editor, and CLI for Go — byte-exact round-tripping, Docker Compose interpolation, environment drift detection, and GitHub Actions / Vercel secrets sync, with zero dependencies. Apache-2.0 licensed.</sub>
