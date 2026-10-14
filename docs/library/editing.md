# Editing a .env file

Change values, place new keys beside related ones, comment settings out and back in, generate whole files, and derive one environment's file from another, while every line you did not touch keeps its exact bytes.

On this page:

- [Before you start](#before-you-start)
- [Two families: upsert and literal](#two-families-upsert-and-literal)
- [Step 1 — Change or add a value with Set](#step-1--change-or-add-a-value-with-set)
- [Step 2 — Place a new key with SetAfter and SetBefore](#step-2--place-a-new-key-with-setafter-and-setbefore)
- [Step 3 — Turn a setting off with Unset](#step-3--turn-a-setting-off-with-unset)
- [Step 4 — Turn it back on with Restore](#step-4--turn-it-back-on-with-restore)
- [Step 5 — List disabled settings](#step-5--list-disabled-settings)
- [Step 6 — Handle inherited declarations](#step-6--handle-inherited-declarations)
- [Step 7 — Generate content with the constructors](#step-7--generate-content-with-the-constructors)
- [Step 8 — Place blocks with Append, InsertAfter and InsertBefore](#step-8--place-blocks-with-append-insertafter-and-insertbefore)
- [Step 9 — Derive another environment with Clone](#step-9--derive-another-environment-with-clone)
- [Try it](#try-it)
- [Pitfalls](#pitfalls)
- [Related](#related)

Every edit works on a `*dotenv.File` in memory. Nothing reaches disk until you call `Save` or `SaveAs`, which [saving.md](saving.md) covers.

## Before you start

```sh
go get github.com/ubgo/dotenv
```

You need a `*dotenv.File`. Get one with `dotenv.Open(path)` for a file on disk (a missing file opens empty, ready to create) or `dotenv.Parse(content)` for a string. Both are explained in the reading guide. The examples below use `Parse` so they run without touching disk.

## Two families: upsert and literal

The package has two groups of write methods. Pick by what you are doing.

| Family | Methods | What it does | Use it for |
|---|---|---|---|
| Upsert | `Set`, `SetAfter`, `SetBefore` | Keeps one active entry per key. Updates in place if the key exists, adds it if not. | Editing a file a person wrote. |
| Literal | `Append`, `InsertAfter`, `InsertBefore` | Places exactly the entries you pass, duplicates included. | Generating a file: banners, blank lines, sections. |

Mixing them up is how a key silently ends up twice in a file. Here is the difference on one key:

<!-- snippet: example_edit_test.go#edit_Append_vsSet -->
```go
// Set upserts: one active entry. Append is literal: it adds a second entry,
// and the last one wins when the file is read.
func ExampleFile_Append_vsSet() {
	upsert := dotenv.Parse("A=1\n")
	upsert.Set("A", "2")

	literal := dotenv.Parse("A=1\n")
	literal.Append(dotenv.NewPair("A", "2"))

	fmt.Printf("Set:    %q count=%d\n", upsert.Render(), upsert.Count("A"))
	fmt.Printf("Append: %q count=%d\n", literal.Render(), literal.Count("A"))
	// Output:
	// Set:    "A=2\n" count=1
	// Append: "A=1\nA=2\n" count=2
}
```

`Count` shows two entries after `Append`. Only the last one is in effect when the file is read.

## Step 1 — Change or add a value with Set

[`Set(key, value)`](ds:block?id=set-jusdwzq7) updates the key where it is, or appends `KEY=value` at the end when the key is missing. It returns `created`, which is `true` only when it appended.

<!-- snippet: example_edit_test.go#edit_Set -->
```go
// Set updates a key in place, or appends it when it is missing. Everything
// else in the file keeps its exact bytes.
func ExampleFile_Set() {
	f := dotenv.Parse("# database\nexport DB_HOST = localhost # dev only\n")

	updated := f.Set("DB_HOST", "db.internal") // existing: changed in place
	created := f.Set("DB_PORT", "5432")        // missing: appended at the end

	fmt.Println("DB_HOST created:", updated)
	fmt.Println("DB_PORT created:", created)
	fmt.Print(f.Render())
	// Output:
	// DB_HOST created: false
	// DB_PORT created: true
	// # database
	// export DB_HOST = db.internal # dev only
	// DB_PORT=5432
}
```

What `Set` keeps when it changes a value:

- the `export` prefix, the spacing on both sides of the delimiter, and the delimiter itself (`=` or the Compose-style `:`; a colon pair is never rewritten to `=`)
- the inline comment after the value (`# dev only` above)
- every other line of the file, byte for byte

Setting the value a key already has is a true no-op, even when the source used quotes or odd spacing. This is why a tool can call `Set` on every run without producing a diff:

<!-- snippet: example_edit_test.go#edit_Set_noop -->
```go
// Setting the value a key already has changes nothing, byte for byte, even
// when the value was written with quotes or odd spacing.
func ExampleFile_Set_noop() {
	const src = "KEY =   'same'   # note\n"
	f := dotenv.Parse(src)

	f.Set("KEY", "same")

	fmt.Println(f.Render() == src)
	// Output:
	// true
}
```

`Set` chooses the quoting of a changed value for you. The value is bare when it reads back the same; it is double-quoted when it contains a space, a tab, `#`, any of the three quote characters, or a backslash; and it becomes a double-quoted block across several lines when it contains a newline. Inside double quotes only `\"` and `\\` are escaped.

<!-- snippet: example_edit_test.go#edit_Set_quoting -->
```go
// Set picks the quoting for you: bare when that reads back the same, double
// quotes when the value has a space, '#', a quote or a backslash, and a
// double-quoted block when it has newlines.
func ExampleFile_Set_quoting() {
	f := dotenv.Parse("")
	f.Set("PLAIN", "simple")
	f.Set("SPACED", "two words")
	f.Set("HASH", "a #b")
	f.Set("QUOTED", `say "hi"`)
	f.Set("EMPTY", "")
	f.Set("PEM", "line one\nline two")
	fmt.Print(f.Render())
	// Output:
	// PLAIN=simple
	// SPACED="two words"
	// HASH="a #b"
	// QUOTED="say \"hi\""
	// EMPTY=
	// PEM="line one
	// line two"
}
```

A value containing `$` is [stored literally](ds:block?id=quotingtriggers-xsur5zum): in single quotes, or in backticks when it contains a single quote. Written bare or in double quotes, `a$b` would read back as `a` followed by a reference to `b`, so a password with a `$` could not be stored. The one value that cannot be stored literally holds `$`, a single quote and a backtick together; it is double-quoted and its `$` stays a reference. Whichever quoting `Set` picks, `GetExpanded` gives the same answer before and after the file is saved and read again.

<!-- snippet: example_edit_test.go#edit_Set_dollar -->
```go
// Set stores a value containing '$' literally, in single quotes, so a
// password such as "a$b" comes back unchanged from GetExpanded, in this
// session and after the file is read again.
func ExampleFile_Set_dollar() {
	f := dotenv.Parse("")
	f.Set("PASSWORD", "a$b")

	raw, _ := f.Get("PASSWORD")
	expanded, _, err := f.GetExpanded("PASSWORD")
	if err != nil {
		fmt.Println("expand:", err)
		return
	}
	fmt.Print(f.Render())
	fmt.Printf("Get=%q GetExpanded=%q\n", raw, expanded)
	// Output:
	// PASSWORD='a$b'
	// Get="a$b" GetExpanded="a$b"
}
```

When a key appears more than once, `Set` changes the last one, because that is the one a reader of the file gets. The same rule is used by `Get`, `Unset`, `SetAfter` and `InsertAfter`.

<!-- snippet: example_edit_test.go#edit_Set_duplicates -->
```go
// With a duplicated key, Set changes the LAST occurrence, because that is the
// one a reader of the file actually gets.
func ExampleFile_Set_duplicates() {
	f := dotenv.Parse("MODE=a\nMODE=b\n")
	f.Set("MODE", "c")
	fmt.Print(f.Render())
	// Output:
	// MODE=a
	// MODE=c
}
```

A commented-out key is not active, so `Set` on it appends a new live entry and leaves the comment alone. Use [Restore](#step-4--turn-it-back-on-with-restore) to bring back the commented line itself.

## Step 2 — Place a new key with SetAfter and SetBefore

`Set` always appends new keys at the end. To keep related settings together, name an anchor key that is already in the file. A new key lands right after (or before) it. A key that already exists is updated where it is and is not moved, because moving it would rewrite two places in the file for a one-value change.

<!-- snippet: example_edit_test.go#edit_SetAfter -->
```go
// SetAfter puts a NEW key right after an anchor key, so related settings stay
// together. A key that already exists is updated where it is, not moved.
func ExampleFile_SetAfter() {
	f := dotenv.Parse("DB_HOST=localhost\nDB_PORT=5432\n\nAPP_NAME=shop\n")

	created, err := f.SetAfter("DB_PORT", "DB_PASSWORD", "secret")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("created:", created)

	// DB_HOST exists, so it is updated in place; the anchor is not used.
	created, err = f.SetAfter("APP_NAME", "DB_HOST", "db.internal")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("created:", created)
	fmt.Print(f.Render())
	// Output:
	// created: true
	// created: false
	// DB_HOST=db.internal
	// DB_PORT=5432
	// DB_PASSWORD=secret
	//
	// APP_NAME=shop
}
```

`SetBefore` works the same way on the other side of the anchor:

<!-- snippet: example_edit_test.go#edit_SetBefore -->
```go
// SetBefore is SetAfter on the other side of the anchor.
func ExampleFile_SetBefore() {
	f := dotenv.Parse("DB_HOST=localhost\nDB_PORT=5432\n")

	created, err := f.SetBefore("DB_HOST", "DB_DRIVER", "postgres")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("created:", created)
	fmt.Print(f.Render())
	// Output:
	// created: true
	// DB_DRIVER=postgres
	// DB_HOST=localhost
	// DB_PORT=5432
}
```

If the anchor has no active entry, both return [`dotenv.ErrAnchorNotFound`](ds:block?id=erranchornotfound-7696gb9k) and change nothing. They do not fall back to appending at the end, because then the call would report success while ignoring the one thing you asked for. Check with `errors.Is` and decide yourself whether to fall back to `Set`:

<!-- snippet: example_edit_test.go#edit_SetAfter_anchorNotFound -->
```go
// A missing anchor is an error, and the file is left alone. A commented-out
// key does not count as an anchor, because it is not active.
func ExampleFile_SetAfter_anchorNotFound() {
	const src = "# DB_PORT=5432\n"
	f := dotenv.Parse(src)

	_, err := f.SetAfter("DB_PORT", "DB_PASSWORD", "secret")

	fmt.Println(errors.Is(err, dotenv.ErrAnchorNotFound))
	fmt.Println("unchanged:", f.Render() == src)
	// Output:
	// true
	// unchanged: true
}
```

When the anchor is a multi-line value, `SetAfter` places the new key after the whole block, never inside it.

## Step 3 — Turn a setting off with Unset

[`Unset(key, del)`](ds:block?id=unset-ahj88kj2) deactivates the last active entry for `key` and reports whether it found one.

- `del = false` comments out every physical line of the entry [with `# `](ds:block?id=commentmarker-6gh5ejhx). The setting stays in the file, readable and reversible, and any comment above it still has something to describe.
- `del = true` removes the entry outright.

<!-- snippet: example_edit_test.go#edit_Unset -->
```go
// Unset(key, false) comments the setting out, which you can undo later.
// Unset(key, true) deletes the line. Both report whether the key was active.
func ExampleFile_Unset() {
	f := dotenv.Parse("OLD_FLAG=1\nLEGACY_URL=http://old\nKEEP=yes\n")

	fmt.Println(f.Unset("OLD_FLAG", false))  // comment out
	fmt.Println(f.Unset("LEGACY_URL", true)) // delete
	fmt.Println(f.Unset("MISSING", false))   // not there
	fmt.Print(f.Render())
	// Output:
	// true
	// true
	// false
	// # OLD_FLAG=1
	// KEEP=yes
}
```

Calling `Unset` again on a key that is already commented out returns `false`: there is no active entry left to deactivate.

## Step 4 — Turn it back on with Restore

[`Restore(key)`](ds:block?id=restore-jyvxum4p) re-activates the last commented-out entry for `key` and reports whether one was found. It removes the exact comment marker that was recorded for that line, so a line written by hand as `#DEBUG=true` comes back as `DEBUG=true` without inventing or removing a space, and an indented `   # KEY=v` keeps its indentation. `Unset(key, false)` followed by `Restore(key)` gives back the original bytes.

<!-- snippet: example_edit_test.go#edit_Restore -->
```go
// Restore turns a commented-out setting back on. It removes exactly the
// marker that was there, so "#K=v" (no space) comes back as "K=v", and
// Unset followed by Restore gives back the original bytes.
func ExampleFile_Restore() {
	const src = "DB_USER=admin\n#DEBUG=true\n"
	f := dotenv.Parse(src)

	f.Unset("DB_USER", false)
	fmt.Println(f.Restore("DB_USER"))
	fmt.Println(f.Restore("DEBUG"))   // written by hand, no space after '#'
	fmt.Println(f.Restore("MISSING")) // nothing to restore
	fmt.Print(f.Render())
	// Output:
	// true
	// true
	// false
	// DB_USER=admin
	// DEBUG=true
}
```

## Step 5 — List disabled settings

A commented-out line shaped like a setting (`# DB_USER=admin`) is its own entry kind, `KindDisabledPair`, whether a person commented it out or `Unset` did. `Disabled()` returns those settings as `[]dotenv.Pair` in file order. A plain comment such as `# just a note` is not listed.

<!-- snippet: example_edit_test.go#edit_Disabled -->
```go
// Disabled lists the commented-out settings in file order. They are not
// active: Get and Has ignore them, just as an app reading the file would.
func ExampleFile_Disabled() {
	f := dotenv.Parse("# DB_USER=admin\n# just a note\nDB_HOST=localhost\n#DEBUG=true\n")

	for _, p := range f.Disabled() {
		fmt.Printf("%s=%s\n", p.Key, p.Value)
	}
	fmt.Println("Has(DB_USER):", f.Has("DB_USER"))
	// Output:
	// DB_USER=admin
	// DEBUG=true
	// Has(DB_USER): false
}
```

Disabled settings are inactive: `Get`, `Has`, `Count`, `Keys`, `Map` and `Pairs` ignore them, the same way an app reading the file would. The point of `Disabled` is that a tool can say "DB_USER is disabled" instead of "DB_USER is missing", which are different problems with different fixes.

## Step 6 — Handle inherited declarations

Docker Compose's env-file format allows a line that is only a name, such as `HOME`. To Compose it means "take this variable from the environment". This package recognises those lines as `KindInherited` but never resolves them, because it never reads the process environment. `Inherited()` returns the declared names in file order, without duplicates, and leaves the decision to you:

<!-- snippet: example_edit_test.go#edit_Inherited -->
```go
// A line that is just a name means "take this from the environment" in
// Docker Compose. The package lists those names but never reads the
// environment; resolving them is your call. A fixed map stands in for
// os.LookupEnv here so the output is stable.
func ExampleFile_Inherited() {
	f := dotenv.Parse("DATABASE_URL=postgres://localhost/dev\nHOME\nAWS_REGION\n")

	fmt.Println(f.Inherited())
	fmt.Println(f.Keys()) // inherited names are not active keys

	hostEnv := map[string]string{"HOME": "/srv/app"} // stand-in for os.LookupEnv
	env := f.Map()
	for _, name := range f.Inherited() {
		if v, ok := hostEnv[name]; ok {
			env[name] = v
		}
	}
	fmt.Println(len(env), env["HOME"])
	// Output:
	// [HOME AWS_REGION]
	// [DATABASE_URL]
	// 2 /srv/app
}
```

In a real program, replace the fixed map with `os.LookupEnv`, a secret store, or whatever source your app trusts, with your own precedence.

Inherited names are not active keys. A `${HOME}` reference to one expands to the empty string like any other unset variable, unless a lookup plugin supplies it.

## Step 7 — Generate content with the constructors

Three constructors build entries that are not in any file yet. Pass them to `Append`, `InsertAfter` or `InsertBefore`.

| Constructor | Builds |
|---|---|
| `NewPair(key, value)` | A `KEY=value` entry. Always uses `=`. Quoting follows the same rules as `Set`. |
| `NewComment(lines...)` | One comment line per argument. Plain text gets `# `; a line that already starts with `#` (after optional spaces) is kept as is. No arguments gives a bare `#`. |
| `NewBlank()` | One empty line, for separating sections. |

A constructed entry has no rendered text until it is placed, because its line ending comes from the destination file. The same `NewPair` renders with `\n` in one file and `\r\n` in another:

<!-- snippet: example_edit_test.go#edit_NewPair -->
```go
// NewPair builds a KEY=value entry that is not in any file yet. Quoting is
// chosen from the value; the line ending comes from the file it is placed in.
func ExampleNewPair() {
	lf := dotenv.Parse("A=1\n")
	crlf := dotenv.Parse("A=1\r\n")

	lf.Append(dotenv.NewPair("GREETING", "hello world"))
	crlf.Append(dotenv.NewPair("GREETING", "hello world"))

	fmt.Printf("%q\n", lf.Render())
	fmt.Printf("%q\n", crlf.Render())
	// Output:
	// "A=1\nGREETING=\"hello world\"\n"
	// "A=1\r\nGREETING=\"hello world\"\r\n"
}
```

<!-- snippet: example_edit_test.go#edit_NewComment -->
```go
// NewComment builds one comment line per argument. Plain text gets "# ";
// a line that already starts with "#" is kept as given, so you never get "##".
func ExampleNewComment() {
	f := dotenv.Parse("")
	f.Append(
		dotenv.NewComment("----------", "DATABASE", "----------"),
		dotenv.NewComment("# already decorated", "  # indented"),
		dotenv.NewComment(), // no lines still gives a bare marker
	)
	fmt.Print(f.Render())
	// Output:
	// # ----------
	// # DATABASE
	// # ----------
	// # already decorated
	//   # indented
	// #
}
```

## Step 8 — Place blocks with Append, InsertAfter and InsertBefore

`Append(entries...)` adds entries at the end, exactly as given. This is how you generate a file such as `.env.example`:

<!-- snippet: example_edit_test.go#edit_Append -->
```go
// Append places entries at the end exactly as given. Use it to generate a
// file with banners, blank lines and sections.
func ExampleFile_Append() {
	f := dotenv.Parse("")
	f.Append(
		dotenv.NewComment("------------------", "DATABASE", "------------------"),
		dotenv.NewPair("DB_HOST", "localhost"),
		dotenv.NewPair("DB_PORT", "5432"),
		dotenv.NewBlank(),
		dotenv.NewComment("------------------", "AUTH", "------------------"),
		dotenv.NewPair("AUTH_SECRET", "change me"),
	)
	fmt.Print(f.Render())
	// Output:
	// # ------------------
	// # DATABASE
	// # ------------------
	// DB_HOST=localhost
	// DB_PORT=5432
	//
	// # ------------------
	// # AUTH
	// # ------------------
	// AUTH_SECRET="change me"
}
```

`InsertAfter(anchor, entries...)` and `InsertBefore(anchor, entries...)` place a block next to an anchor key, as one unit. A missing anchor returns `ErrAnchorNotFound` and changes nothing.

<!-- snippet: example_edit_test.go#edit_InsertAfter -->
```go
// InsertAfter places a block right after an anchor key, as one unit and
// exactly as given. It is the literal twin of SetAfter.
func ExampleFile_InsertAfter() {
	f := dotenv.Parse("DB_HOST=localhost\nDB_PORT=5432\nAPP=shop\n")

	err := f.InsertAfter("DB_PORT",
		dotenv.NewComment("added by the installer"),
		dotenv.NewPair("DB_PASSWORD", "s3cret"),
	)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Print(f.Render())
	// Output:
	// DB_HOST=localhost
	// DB_PORT=5432
	// # added by the installer
	// DB_PASSWORD=s3cret
	// APP=shop
}
```

Because they are literal, they add a duplicate if the key already exists. Use `SetAfter` or `SetBefore` when you want one entry per key.

<!-- snippet: example_edit_test.go#edit_InsertBefore -->
```go
// InsertBefore places entries right before the anchor. Being literal, it adds
// a duplicate if the key already exists; a missing anchor is an error.
func ExampleFile_InsertBefore() {
	f := dotenv.Parse("DB_HOST=localhost\n")

	if err := f.InsertBefore("DB_HOST", dotenv.NewPair("DB_DRIVER", "postgres")); err != nil {
		fmt.Println("error:", err)
		return
	}
	err := f.InsertBefore("NOPE", dotenv.NewPair("X", "1"))
	fmt.Println(errors.Is(err, dotenv.ErrAnchorNotFound))
	fmt.Print(f.Render())
	// Output:
	// true
	// DB_DRIVER=postgres
	// DB_HOST=localhost
}
```

An entry taken from another file's `Entries()` is already attached to a file, so it is copied verbatim: quotes, spacing, inline comment and its own line ending all come along. This is also the way to write a reference such as `${HOST}` into a file, since `Set` stores a value containing `$` literally (see [Step 1](#step-1--change-or-add-a-value-with-set)):

<!-- snippet: example_edit_test.go#edit_Append_verbatim -->
```go
// An entry taken from another file's Entries is copied verbatim: its quotes,
// spacing, comment and even its own line ending come along unchanged. This is
// also the way to write a reference such as ${HOST} into a file, since Set
// stores a value containing '$' literally.
func ExampleFile_Append_verbatim() {
	src := dotenv.Parse("API_URL = https://${HOST}/v1   # follows HOST\n")
	dst := dotenv.Parse("HOST=api.example.com\n")

	dst.Append(src.Entries()[0])

	v, _, err := dst.GetExpanded("API_URL")
	if err != nil {
		fmt.Println("expand:", err)
		return
	}
	fmt.Print(dst.Render())
	fmt.Printf("GetExpanded=%q\n", v)
	// Output:
	// HOST=api.example.com
	// API_URL = https://${HOST}/v1   # follows HOST
	// GetExpanded="https://api.example.com/v1"
}
```

## Step 9 — Derive another environment with Clone

`Clone()` returns an independent copy of a file. Editing either side leaves the other alone. The copy keeps the source's line-ending style, trailing-newline state, file mode, `Path`, `Existed()` state and parse options (plugins included).

Use it to derive `.env.prod` from `.env.staging`: clone, change only what differs, and write the result with `SaveAs`. Every line you did not change stays byte-identical, including `${reference}` lines, because writing never expands anything.

<!-- snippet: example_edit_test.go#edit_Clone -->
```go
// Clone makes an independent copy. Change only what differs and every other
// line, references included, stays byte-identical.
func ExampleFile_Clone() {
	staging := dotenv.Parse("# shared\nDOMAIN=staging.acme.io\nAPI_URL=https://api.${DOMAIN}\nSECRET=${VAULT_TOKEN:?set me}\n")

	prod := staging.Clone()
	prod.Set("DOMAIN", "acme.io")

	fmt.Print(prod.Render())
	api, _, err := prod.GetExpanded("API_URL")
	if err != nil {
		fmt.Println("expand:", err)
		return
	}
	fmt.Println("prod API_URL:", api)
	orig, _ := staging.Get("DOMAIN")
	fmt.Println("staging DOMAIN:", orig)
	// Output:
	// # shared
	// DOMAIN=acme.io
	// API_URL=https://api.${DOMAIN}
	// SECRET=${VAULT_TOKEN:?set me}
	// prod API_URL: https://api.acme.io
	// staging DOMAIN: staging.acme.io
}
```

A reference is a formula, like `=A1*2` in a spreadsheet. Keep the formula and it follows its source. Bake its result and it stops following. If you build the new file from `ExpandedMap()` instead of cloning, every reference is baked, and the result looks right on the day you write it but silently stops tracking:

<!-- snippet: example_edit_test.go#edit_Clone_baked -->
```go
// Building a new file from ExpandedMap instead of cloning bakes every
// reference into a fixed value. Change DOMAIN later and API_URL no longer
// follows it.
func ExampleFile_Clone_baked() {
	staging := dotenv.Parse("DOMAIN=staging.acme.io\nAPI_URL=https://api.${DOMAIN}\n")

	values, err := staging.ExpandedMap()
	if err != nil {
		fmt.Println("expand:", err)
		return
	}
	baked := dotenv.Parse("")
	for _, k := range staging.Keys() { // Keys keeps file order; a map would not
		baked.Set(k, values[k])
	}
	baked.Set("DOMAIN", "acme.io")

	api, _, err := baked.GetExpanded("API_URL")
	if err != nil {
		fmt.Println("expand:", err)
		return
	}
	fmt.Println(api)
	// Output:
	// https://api.staging.acme.io
}
```

To freeze one value on purpose, for example a secret resolved once at deploy time, read it with `GetExpanded` and write it back with `Set`. That is a deliberate choice for that one key, not a side effect.

`Clone` is cheaper and more faithful than `Parse(f.Render())`: it does not re-parse, so no entry can be classified differently on a second pass.

Writing the clone to several files is shown in [saving.md › Step 4](saving.md#step-4--write-a-copy-with-saveas).

## Try it

Every snippet on this page is a Go `Example` in [`example_edit_test.go`](../../example_edit_test.go), and its `// Output:` block is checked by `go test`. Run them all from the repo root:

```sh
go test -run 'Example(NewPair|NewComment|File_(Set|Unset|Restore|Disabled|Inherited|Append|Insert|Clone))' -v . | grep -v '=== RUN'
```

Real output:

```text
--- PASS: ExampleNewPair (0.00s)
--- PASS: ExampleNewComment (0.00s)
--- PASS: ExampleFile_Set (0.00s)
--- PASS: ExampleFile_Set_noop (0.00s)
--- PASS: ExampleFile_Set_quoting (0.00s)
--- PASS: ExampleFile_Set_duplicates (0.00s)
--- PASS: ExampleFile_Set_dollar (0.00s)
--- PASS: ExampleFile_SetAfter (0.00s)
--- PASS: ExampleFile_SetAfter_anchorNotFound (0.00s)
--- PASS: ExampleFile_SetBefore (0.00s)
--- PASS: ExampleFile_Unset (0.00s)
--- PASS: ExampleFile_Restore (0.00s)
--- PASS: ExampleFile_Disabled (0.00s)
--- PASS: ExampleFile_Inherited (0.00s)
--- PASS: ExampleFile_Append (0.00s)
--- PASS: ExampleFile_Append_vsSet (0.00s)
--- PASS: ExampleFile_Append_verbatim (0.00s)
--- PASS: ExampleFile_Append_unterminatedQuote (0.00s)
--- PASS: ExampleFile_InsertAfter (0.00s)
--- PASS: ExampleFile_InsertBefore (0.00s)
--- PASS: ExampleFile_Clone (0.00s)
--- PASS: ExampleFile_Clone_baked (0.00s)
PASS
ok  	github.com/ubgo/dotenv	0.007s
```

The text each example prints is the `// Output:` block at the end of its snippet above.

## Pitfalls

- **`Set` cannot write a reference.** A changed value containing `$` is stored literally, so `Set("URL", "${HOST}/v1")` writes `URL='${HOST}/v1'`, and `GetExpanded` returns that text. To write a reference, place a parsed line with `Append` or `InsertAfter`, as in [Step 8](#step-8--place-blocks-with-append-insertafter-and-insertbefore). Setting the value an entry already has is still a no-op, so an existing reference stays a reference.
- **In library v0.1.1 and earlier, `Set` wrote `$` bare and dropped literal quotes.** `Set("P", "a$b")` wrote `P=a$b`, which reads back as `a`, and `S='${A}'` set to `${A}y` was written `S=${A}y`, so `GetExpanded` gave one answer in memory and another after a reload.
- **In library v0.1.1 and earlier, the spacing after the delimiter was lost on edit.** `KEY = old` became `KEY =new`; it is now `KEY = new`.
- **Appending after an unterminated quote.** If the last value in a file opens a quote and never closes it, everything you append is read back as part of that value, because the open quote keeps consuming lines. The source was already broken, and guessing where the quote should have closed would corrupt real multi-line values. Check files you did not write before appending to them. Pinned by `TestAppend_UnterminatedQuoteSwallowsWhatFollows`:

<!-- snippet: example_edit_test.go#edit_Append_unterminatedQuote -->
```go
// If the file's last value opens a quote and never closes it, anything you
// append ends up INSIDE that value when the file is read back. The source was
// already broken; check files you did not write before appending to them.
func ExampleFile_Append_unterminatedQuote() {
	f := dotenv.Parse(`A="open`)
	f.Append(dotenv.NewPair("APPENDED", "1"))

	back := dotenv.Parse(f.Render())
	_, found := back.Get("APPENDED")
	a, _ := back.Get("A")
	fmt.Println("APPENDED found:", found)
	fmt.Printf("A=%q\n", a)
	// Output:
	// APPENDED found: false
	// A="open\nAPPENDED=1"
}
```

- **`Restore` targets the last disabled entry.** With more than one commented-out entry for a key, `Restore` re-enables the last one, which is not always the one `Unset` just created. Pinned by `TestRestore_TargetsTheLastDisabledEntry`.
- **A commented-out multi-line value cannot be restored after a reload.** In one session, `Unset` keeps the block together and `Restore` reverses it fully. Once saved and re-read, each commented line is a separate comment, so only the first line is recognised as a disabled setting.
- **Prose shaped like a setting counts as disabled.** `# TODO=fix this` is listed by `Disabled()` and can be re-enabled by `Restore("TODO")`. A commented value whose quote does not close on the same line stays a plain comment.
- **`Set` on an inherited name adds a new pair.** For a file with a bare `HOME` line, `Set("HOME", "/x")` appends `HOME=/x` and keeps the `HOME` declaration, so `Inherited()` still lists `HOME`.
- **A clone keeps the source's `Path`.** `clone.Save()` overwrites the file you cloned from. Write a clone with `SaveAs(newPath)`, or set `clone.Path` first.
- **Edits are not safe for concurrent use.** A `*File` has no locking; do not edit one from several goroutines at once.

## Related

- [saving.md](saving.md) — `Save`, `SaveAs`, file modes, line endings, trailing newline.
- Reading guide: [reading.md](reading.md) — `Open`, `Parse`, `Get`, `Map` and friends; [entries.md](entries.md) — `Entry`, the six `Kind`s, walking `Entries()`.
- [Feature index](../reference/features.md) — every feature with its guide and example.
- Examples: [`example_edit_test.go`](../../example_edit_test.go) — `ExampleFile_Set`, `ExampleFile_Set_noop`, `ExampleFile_Set_quoting`, `ExampleFile_Set_duplicates`, `ExampleFile_Set_dollar`, `ExampleFile_SetAfter`, `ExampleFile_SetAfter_anchorNotFound`, `ExampleFile_SetBefore`, `ExampleFile_Unset`, `ExampleFile_Restore`, `ExampleFile_Disabled`, `ExampleFile_Inherited`, `ExampleNewPair`, `ExampleNewComment`, `ExampleFile_Append`, `ExampleFile_Append_vsSet`, `ExampleFile_Append_verbatim`, `ExampleFile_Append_unterminatedQuote`, `ExampleFile_InsertAfter`, `ExampleFile_InsertBefore`, `ExampleFile_Clone`, `ExampleFile_Clone_baked`.
- The CLI wraps these methods as `dotenvctl set`, `unset` and `restore`: [commands.md](../commands.md).
