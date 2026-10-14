# Reading a .env file

How to load a `.env` with the `dotenv` library, and what every read accessor returns, including for duplicate keys and commented-out settings.

**On this page**

- [Which call to use](#which-call-to-use)
- [Before you start](#before-you-start)
- [Step 1 — Read the values in one call](#step-1--read-the-values-in-one-call)
- [Step 2 — Open a file you may edit](#step-2--open-a-file-you-may-edit)
- [Step 3 — Parse a string or a reader](#step-3--parse-a-string-or-a-reader)
- [Step 4 — Query the file](#step-4--query-the-file)
- [Step 5 — Check where the file came from](#step-5--check-where-the-file-came-from)
- [Try it](#try-it)
- [Cost at scale](#cost-at-scale)
- [Pitfalls](#pitfalls)
- [Related](#related)

## Which call to use

There are four ways in. Pick by where the content lives and whether you will write it back.

| You have | You want | Call | Missing file |
|---|---|---|---|
| a path | the final values, `${refs}` resolved, read-only | `dotenv.Read(path)` → `map[string]string` | error |
| a path | to inspect, edit and save the file | `dotenv.Open(path)` → `*File` | not an error: an empty `File` |
| a string | to inspect or edit content already in memory | `dotenv.Parse(content)` → `*File` | n/a |
| an `io.Reader` | the same, from a pipe, an HTTP body or an open file | `dotenv.ParseReader(r)` → `*File` | n/a |

`Read` is `Open` followed by `ExpandedMap`. The other three give you a `*File`, whose accessors return **raw** values: a `${DOMAIN}` reference stays as written, so saving the file never flattens it. When you want resolved values from a `*File`, use `GetExpanded` or `ExpandedMap` (see [expansion.md](expansion.md)).

None of these touch `os.Environ`. Reading a file and exporting it into the process are separate decisions; the library only does the first.

## Before you start

```sh
go get github.com/ubgo/dotenv
```

The library needs Go 1.22 or later and has no third-party dependencies.

## Step 1 — Read the values in one call

Use `Read` when a program only consumes configuration. It skips comments, blanks, disabled settings and unrecognised lines, collapses duplicates to the last one, and resolves `${VAR}` references against the same file. Unlike `Open`, a missing file is an error, because `Read` is for consuming a file that should exist.

<!-- snippet: example_read_test.go#read_Read -->
```go
// ExampleRead loads a file straight into a map with every ${reference}
// resolved: the one-call path for a program that only consumes configuration
// and never writes the file back.
func ExampleRead() {
	// envFile is the name the example writes inside its own temp directory.
	const envFile = ".env"

	dir, err := os.MkdirTemp("", "dotenv-read")
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

	path := filepath.Join(dir, envFile)
	content := "DOMAIN=example.com\nAPI_URL=https://api.${DOMAIN}\n# DEBUG=true\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		fmt.Println(err)
		return
	}

	m, err := dotenv.Read(path)
	if err != nil {
		fmt.Println(err)
		return
	}
	// A map has no order; sort the keys so the output is stable.
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%s=%s\n", k, m[k])
	}

	// Unlike Open, Read treats a missing file as an error.
	_, err = dotenv.Read(filepath.Join(dir, "missing.env"))
	fmt.Println("missing file is an error:", err != nil)
	// Output:
	// API_URL=https://api.example.com
	// DOMAIN=example.com
	// missing file is an error: true
}
```

Example: [`ExampleRead`](../../example_read_test.go).

## Step 2 — Open a file you may edit

Use `Open` when you will inspect structure, edit or save. It records the path, the file's permissions and whether it existed. Values come back raw.

<!-- snippet: example_read_test.go#read_Open -->
```go
// ExampleOpen opens a file from disk for reading and editing. Values come back
// raw: a ${reference} stays as written, so the file can be saved without
// flattening it.
func ExampleOpen() {
	// envFile is the name the example writes inside its own temp directory.
	const envFile = ".env"

	dir, err := os.MkdirTemp("", "dotenv-open")
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

	path := filepath.Join(dir, envFile)
	content := "DOMAIN=example.com\nAPI_URL=https://api.${DOMAIN}\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		fmt.Println(err)
		return
	}

	f, err := dotenv.Open(path)
	if err != nil {
		fmt.Println(err)
		return
	}
	v, ok := f.Get("API_URL")
	fmt.Println(v, ok)
	fmt.Println("existed:", f.Existed())
	// Output:
	// https://api.${DOMAIN} true
	// existed: true
}
```

A missing file is **not** an error. You get an empty `File`; `Existed` reports `false`, and `Save` creates the file with mode `0600`. That is how a tool bootstraps a fresh `.env`:

<!-- snippet: example_read_test.go#read_OpenMissing -->
```go
// ExampleOpen_missing shows that opening a file that does not exist is not an
// error: you get an empty File you can fill and Save, which creates it.
func ExampleOpen_missing() {
	// envFile is the name the example creates inside its own temp directory.
	const envFile = ".env"

	dir, err := os.MkdirTemp("", "dotenv-open-missing")
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

	f, err := dotenv.Open(filepath.Join(dir, envFile))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("existed:", f.Existed(), "keys:", len(f.Keys()))

	f.Set("PORT", "8080")
	if err := f.Save(); err != nil {
		fmt.Println(err)
		return
	}
	data, err := os.ReadFile(filepath.Join(dir, envFile))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%q\n", data)
	// Output:
	// existed: false keys: 0
	// "PORT=8080\n"
}
```

`Open` returns an error for every other failure: a file it may not read, or a path that is a directory. The message names the path and the failed step (`dotenv: open <path>: …` or `dotenv: read <path>: …`).

Examples: [`ExampleOpen`](../../example_read_test.go), [`ExampleOpen_missing`](../../example_read_test.go).

## Step 3 — Parse a string or a reader

`Parse` takes a string and never fails: any line it does not understand is kept as an "other" entry rather than rejected (see [entries.md](entries.md)). Rendering an untouched `File` gives back the exact input bytes.

<!-- snippet: example_read_test.go#read_Parse -->
```go
// ExampleParse parses a string with no filesystem involved: an embedded fixture,
// a value from a secret store, or a test input. Rendering it back gives the
// exact bytes it was parsed from.
func ExampleParse() {
	const content = "# app\nPORT=8080\nexport NAME='my app'\n"

	f := dotenv.Parse(content)
	port, _ := f.Get("PORT")
	name, _ := f.Get("NAME")
	fmt.Println(port)
	fmt.Println(name)
	fmt.Println("round trip:", f.Render() == content)
	// Output:
	// 8080
	// my app
	// round trip: true
}
```

`ParseReader` takes any `io.Reader`. Prefer it over `io.ReadAll` followed by `Parse` for large input: it copies the content once into a `strings.Builder`, while `io.ReadAll` grows its buffer by doubling and the `[]byte` to `string` conversion copies everything again. Its only error is the reader's own error, wrapped as `dotenv: read: …`.

<!-- snippet: example_read_test.go#read_ParseReader -->
```go
// ExampleParseReader parses from any io.Reader: a pipe, an HTTP body, a file
// you already opened. It copies the input once, so it is the cheaper choice
// for large content compared with io.ReadAll followed by Parse.
func ExampleParseReader() {
	r := strings.NewReader("PORT=8080\nHOST=localhost\n")

	f, err := dotenv.ParseReader(r)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(f.Keys())
	// Output:
	// [PORT HOST]
}
```

Both accept the same options as `Open` (for example `dotenv.WithEscapes`, or plugins). A `File` from `Parse` or `ParseReader` has an empty `Path` and `Existed() == false`; set `Path` before calling `Save` (see [Step 5](#step-5--check-where-the-file-came-from)).

Examples: [`ExampleParse`](../../example_read_test.go), [`ExampleParseReader`](../../example_read_test.go).

## Step 4 — Query the file

Six accessors read a `File`. They differ only in how they treat duplicates; all six ignore everything that is not an **active** pair, which means comments, blanks, unrecognised lines, commented-out settings (`# DB_USER=admin`) and name-only inherited lines (`HOME`).

Every example below parses the same input:

```env
# database
DB_HOST=localhost
DB_PORT=5432
# DB_USER=admin
DB_HOST=db.internal
HOME
```

`DB_HOST` is set twice, `DB_USER` is commented out, and `HOME` is a name with no value. Here is what each accessor returns for it. Every cell is checked by an Example's `// Output:` block.

| Call | Returns | Duplicates | Disabled `DB_USER` | Inherited `HOME` |
|---|---|---|---|---|
| `f.Get("DB_HOST")` | `"db.internal", true` | last line wins | `Get("DB_USER")` → `"", false` | `Get("HOME")` → `"", false` |
| `f.Has("DB_HOST")` | `true` | any active line counts | `false` | `false` |
| `f.Count("DB_HOST")` | `2` | counts every active line | `0` | `0` |
| `f.Keys()` | `[DB_HOST DB_PORT]` | each key once, first-seen order | left out | left out |
| `f.Pairs()` | `DB_HOST=localhost`, `DB_PORT=5432`, `DB_HOST=db.internal` | every line, file order | left out | left out |
| `f.Map()` | 2 keys: `DB_HOST` → `db.internal`, `DB_PORT` → `5432` | last line wins | left out | left out |

`Get` returns a value and a found flag. Use the flag: an empty value (`KEY=`) and a missing key both give `""`.

<!-- snippet: example_read_test.go#read_Get -->
```go
// ExampleFile_Get reads one raw value. With a duplicated key the LAST active
// line wins, a commented-out setting is not found, and neither is a name-only
// (inherited) line.
func ExampleFile_Get() {
	const content = `# database
DB_HOST=localhost
DB_PORT=5432
# DB_USER=admin
DB_HOST=db.internal
HOME
`
	f := dotenv.Parse(content)

	for _, key := range []string{"DB_HOST", "DB_PORT", "DB_USER", "HOME", "NOPE"} {
		v, ok := f.Get(key)
		fmt.Printf("%s: %q %v\n", key, v, ok)
	}
	// Output:
	// DB_HOST: "db.internal" true
	// DB_PORT: "5432" true
	// DB_USER: "" false
	// HOME: "" false
	// NOPE: "" false
}
```

`Has` is `Get` without the value.

<!-- snippet: example_read_test.go#read_Has -->
```go
// ExampleFile_Has checks for an ACTIVE pair. A disabled setting and an
// inherited name both answer false, because neither is in effect.
func ExampleFile_Has() {
	const content = `# database
DB_HOST=localhost
DB_PORT=5432
# DB_USER=admin
DB_HOST=db.internal
HOME
`
	f := dotenv.Parse(content)

	fmt.Println("DB_HOST:", f.Has("DB_HOST"))
	fmt.Println("DB_USER:", f.Has("DB_USER"))
	fmt.Println("HOME:", f.Has("HOME"))
	// Output:
	// DB_HOST: true
	// DB_USER: false
	// HOME: false
}
```

`Count` is how you find duplicates. Only the last line of a duplicated key is in effect; the earlier ones are dead weight a reviewer should hear about.

<!-- snippet: example_read_test.go#read_Count -->
```go
// ExampleFile_Count reports how many active lines share a key. Anything above
// one is a duplicate: only the last line is in effect, the others are dead
// weight a reviewer should know about.
func ExampleFile_Count() {
	const content = `# database
DB_HOST=localhost
DB_PORT=5432
# DB_USER=admin
DB_HOST=db.internal
HOME
`
	f := dotenv.Parse(content)

	for _, key := range f.Keys() {
		if n := f.Count(key); n > 1 {
			fmt.Printf("%s is set %d times\n", key, n)
		}
	}
	fmt.Println("DB_USER:", f.Count("DB_USER"))
	// Output:
	// DB_HOST is set 2 times
	// DB_USER: 0
}
```

`Keys` lists each active key once, in the order it first appears.

<!-- snippet: example_read_test.go#read_Keys -->
```go
// ExampleFile_Keys lists the active keys in file order, each once. Disabled
// settings and inherited names are left out.
func ExampleFile_Keys() {
	const content = `# database
DB_HOST=localhost
DB_PORT=5432
# DB_USER=admin
DB_HOST=db.internal
HOME
`
	f := dotenv.Parse(content)

	fmt.Println(f.Keys())
	// Output:
	// [DB_HOST DB_PORT]
}
```

`Pairs` returns every active line as a `Pair{Key, Value}` in file order, duplicates included: the view a linear read of the file gives.

<!-- snippet: example_read_test.go#read_Pairs -->
```go
// ExampleFile_Pairs lists every active pair in file order, duplicates
// included: the view a linear read of the file gives.
func ExampleFile_Pairs() {
	const content = `# database
DB_HOST=localhost
DB_PORT=5432
# DB_USER=admin
DB_HOST=db.internal
HOME
`
	f := dotenv.Parse(content)

	for _, p := range f.Pairs() {
		fmt.Printf("%s=%s\n", p.Key, p.Value)
	}
	// Output:
	// DB_HOST=localhost
	// DB_PORT=5432
	// DB_HOST=db.internal
}
```

`Map` is the effective view, one value per key with the last line winning, the same answer `Get` gives. Hand it to anything that wants a `map[string]string`; use `Pairs` when order or duplicates matter.

<!-- snippet: example_read_test.go#read_Map -->
```go
// ExampleFile_Map returns the effective view: one value per key, last line
// wins, values raw (references not expanded).
func ExampleFile_Map() {
	const content = `# database
DB_HOST=localhost
DB_PORT=5432
# DB_USER=admin
DB_HOST=db.internal
HOME
`
	f := dotenv.Parse(content)

	m := f.Map()
	fmt.Println(len(m), m["DB_HOST"], m["DB_PORT"])
	// Output:
	// 2 db.internal 5432
}
```

To see disabled settings and inherited names rather than skip them, use `f.Disabled()`, `f.Inherited()` or walk `f.Entries()` (see [entries.md](entries.md) and [editing.md](editing.md)).

Examples: [`ExampleFile_Get`](../../example_read_test.go), [`ExampleFile_Has`](../../example_read_test.go), [`ExampleFile_Count`](../../example_read_test.go), [`ExampleFile_Keys`](../../example_read_test.go), [`ExampleFile_Pairs`](../../example_read_test.go), [`ExampleFile_Map`](../../example_read_test.go).

## Step 5 — Check where the file came from

Three facts describe a file's place on disk.

| Fact | `Open` of an existing file | `Open` of a missing file | `Parse` / `ParseReader` |
|---|---|---|---|
| `f.Existed()` | `true` | `false` | `false` |
| `f.Mode()` | the file's own permission bits | `0600` (`-rw-------`) | `0600` |
| `f.Path` | the path passed to `Open` | the path passed to `Open` | `""` |

`Existed` tells a fresh file apart from one already on disk, for example to print "created .env" rather than "updated .env".

<!-- snippet: example_read_test.go#read_Existed -->
```go
// ExampleFile_Existed tells a freshly created file apart from one that was
// already on disk. A File from Parse never existed on disk.
func ExampleFile_Existed() {
	// envFile is the name the example writes inside its own temp directory.
	const envFile = ".env"

	dir, err := os.MkdirTemp("", "dotenv-existed")
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
	path := filepath.Join(dir, envFile)

	before, err := dotenv.Open(path)
	if err != nil {
		fmt.Println(err)
		return
	}
	if err := os.WriteFile(path, []byte("A=1\n"), 0o600); err != nil {
		fmt.Println(err)
		return
	}
	after, err := dotenv.Open(path)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("before writing:", before.Existed())
	fmt.Println("after writing:", after.Existed())
	fmt.Println("from Parse:", dotenv.Parse("A=1\n").Existed())
	// Output:
	// before writing: false
	// after writing: true
	// from Parse: false
}
```

`Mode` is the permission set `Save` will apply. An existing file keeps its mode; a new one gets `0600`, because a `.env` holds credentials and a group-readable secret is a leak.

<!-- snippet: example_read_test.go#read_Mode -->
```go
// ExampleFile_Mode reports the permissions Save will apply: the file's own
// mode when it existed, 0600 for a file Save will create, because a .env holds
// credentials.
func ExampleFile_Mode() {
	// sharedMode is a deliberately wider mode, to show Open keeps it.
	const sharedMode fs.FileMode = 0o640
	// envFile is the name the example writes inside its own temp directory.
	const envFile = ".env"

	dir, err := os.MkdirTemp("", "dotenv-mode")
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
	path := filepath.Join(dir, envFile)

	fresh, err := dotenv.Open(path)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("missing file:", fresh.Mode())

	if err := os.WriteFile(path, []byte("A=1\n"), sharedMode); err != nil {
		fmt.Println(err)
		return
	}
	// WriteFile's mode is filtered by the umask; set it exactly.
	if err := os.Chmod(path, sharedMode); err != nil {
		fmt.Println(err)
		return
	}
	existing, err := dotenv.Open(path)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("existing file:", existing.Mode())
	// Output:
	// missing file: -rw-------
	// existing file: -rw-r-----
}
```

`Path` is an exported field, not a method. `Save` writes to it, so you can retarget a `File` by assigning it. `SaveAs` writes elsewhere without changing it (see [saving.md](saving.md)).

<!-- snippet: example_read_test.go#read_Path -->
```go
// ExampleFile_Path shows where Save writes: the path given to Open. Parse has
// no path, so Path starts empty and must be set before Save.
func ExampleFile_Path() {
	// envFile is the name the example writes inside its own temp directory.
	const envFile = ".env.local"

	dir, err := os.MkdirTemp("", "dotenv-path")
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

	opened, err := dotenv.Open(filepath.Join(dir, envFile))
	if err != nil {
		fmt.Println(err)
		return
	}
	// Print only the base name: the temp directory differs on every run.
	fmt.Println("Open:", filepath.Base(opened.Path))

	parsed := dotenv.Parse("A=1\n")
	fmt.Printf("Parse: %q\n", parsed.Path)

	parsed.Path = filepath.Join(dir, envFile)
	if err := parsed.Save(); err != nil {
		fmt.Println(err)
		return
	}
	data, err := os.ReadFile(parsed.Path)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("saved: %q\n", data)
	// Output:
	// Open: .env.local
	// Parse: ""
	// saved: "A=1\n"
}
```

Examples: [`ExampleFile_Existed`](../../example_read_test.go), [`ExampleFile_Mode`](../../example_read_test.go), [`ExampleFile_Path`](../../example_read_test.go).

## Try it

Every snippet on this page is an `Example` function in [`example_read_test.go`](../../example_read_test.go). `go test` runs each one and compares what it prints with its `// Output:` block, so the outputs shown above are checked on every run. From the repository root:

```sh
go test -run '^Example(Read|Open|Open_missing|Parse|ParseReader|File_(Get|Has|Count|Keys|Pairs|Map|Existed|Mode|Path))$' -v .
```

```text
=== RUN   ExampleRead
--- PASS: ExampleRead (0.00s)
=== RUN   ExampleOpen
--- PASS: ExampleOpen (0.00s)
=== RUN   ExampleOpen_missing
--- PASS: ExampleOpen_missing (0.00s)
=== RUN   ExampleParse
--- PASS: ExampleParse (0.00s)
=== RUN   ExampleParseReader
--- PASS: ExampleParseReader (0.00s)
=== RUN   ExampleFile_Get
--- PASS: ExampleFile_Get (0.00s)
=== RUN   ExampleFile_Has
--- PASS: ExampleFile_Has (0.00s)
=== RUN   ExampleFile_Count
--- PASS: ExampleFile_Count (0.00s)
=== RUN   ExampleFile_Keys
--- PASS: ExampleFile_Keys (0.00s)
=== RUN   ExampleFile_Pairs
--- PASS: ExampleFile_Pairs (0.00s)
=== RUN   ExampleFile_Map
--- PASS: ExampleFile_Map (0.00s)
=== RUN   ExampleFile_Existed
--- PASS: ExampleFile_Existed (0.00s)
=== RUN   ExampleFile_Mode
--- PASS: ExampleFile_Mode (0.00s)
=== RUN   ExampleFile_Path
--- PASS: ExampleFile_Path (0.00s)
PASS
ok  	github.com/ubgo/dotenv	0.010s
```

A passing Example prints nothing beyond `PASS`: the printed text matched its `// Output:` block exactly. Change an expected line in the file and run it again to see the diff `go test` reports.

## Cost at scale

The parser holds the whole file in memory. That is not an oversight: byte-exact rendering needs every original line, forward `${REF}` references need the entries further down, and insertion needs an index into a complete entry list. `ParseReader` lowers the cost of getting the bytes in; nothing here streams.

What the test suite and benchmarks back:

- **Parsing is linear in the file size.** `TestParse_ScalesLinearly` parses a 1.25 MB and a 5 MB file of short `KEY_n=…` lines and fails if quadrupling the input costs more than 9 times as long (linear is about 4 times, quadratic about 16). It also checks the 5 MB file yields over 100,000 keys and renders back byte for byte. It guards a real regression: pre-trimming the lookahead for every pair once made a 5 MB file take 23 seconds. The test is skipped under `go test -short`.
- **`ParseReader` allocates less than `io.ReadAll` then `Parse`.** `BenchmarkParseLarge` parses a file of about 1.4 MB holding one huge quoted value. The measurement recorded in the `ParseReader` doc comment is 9.8 MB and 67 allocations, against 13.0 MB and 93 for `ReadAll` then `Parse`.
- **Per-entry cost** is measured by `BenchmarkParseManyEntries` (1 MB of short entries), the shape where the quadratic bug lived.

Benchmark numbers depend on the machine. Re-run them rather than trusting the figures above:

```sh
task bench
# or, without Task:
go test -run '^$' -bench . -benchmem .
```

## Pitfalls

- **`Read` returns expanded values; `Open` and `Parse` return raw ones.** Writing a value from `Read` or `ExpandedMap` back with `Set` replaces the `${REF}` with a copy of its target, and the link is lost. Read raw values with `Get` or `Map` when the file will be saved.
- **`Open` of a missing file succeeds.** A typo in the path gives an empty `File`, not an error. Check `f.Existed()` when the file must already exist, or use `Read`, which fails on a missing file.
- **Duplicates are silent.** `Get` and `Map` quietly return the last line. Run `Count` over `Keys()` (as in `ExampleFile_Count`) if a duplicate should be reported.
- **`Get` on a commented-out setting reports not found.** `# DB_USER=admin` is inactive. Use `f.Disabled()` to tell "disabled" apart from "missing".
- **A name-only line is not a value.** `HOME` on its own line declares a variable inherited from the environment. The library never reads the environment, so `Get("HOME")` is `"", false`; use `f.Inherited()` and apply `os.LookupEnv` yourself.
- **A malformed pair disappears from every accessor.** `MY KEY=value` (a space in the key) is an unrecognised line, not an error; walk `Entries()` and flag `KindOther` if you want strictness (see [entries.md](entries.md#pitfalls)).
- **`Parse` gives an empty `Path`.** Calling `Save` on it before assigning `Path` fails; set `f.Path` or use `SaveAs`.
- **`Read` expansion can fail.** A `${VAR:?message}` whose variable is unset makes `Read` and `ExpandedMap` return an error and no map. See [expansion.md](expansion.md).

## Related

- [entries.md](entries.md) — the `Entry` type, the six kinds, and walking `Entries()`.
- [editing.md](editing.md) — `Set`, `Unset`, `Restore`, `Disabled`, `Inherited`.
- [saving.md](saving.md) — `Save`, `SaveAs`, modes and line endings.
- [syntax.md](syntax.md) — quoting, comments and escapes, line by line.
- [expansion.md](expansion.md) — `GetExpanded`, `ExpandedMap` and every `${…}` form.
- [Feature index](../reference/features.md#library-reading) — every library feature with its guide and example.
- Examples: [`example_read_test.go`](../../example_read_test.go).
