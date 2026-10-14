# Saving a .env file

Write an edited file back to disk atomically, with its permissions, line endings and trailing newline kept, or write a copy somewhere else.

On this page:

- [Before you start](#before-you-start)
- [How a save works](#how-a-save-works)
- [Step 1 — Save to the file you opened](#step-1--save-to-the-file-you-opened)
- [Step 2 — Know which mode the file gets](#step-2--know-which-mode-the-file-gets)
- [Step 3 — Keep line endings and the trailing newline](#step-3--keep-line-endings-and-the-trailing-newline)
- [Step 4 — Write a copy with SaveAs](#step-4--write-a-copy-with-saveas)
- [Try it](#try-it)
- [Pitfalls](#pitfalls)
- [Related](#related)

`.env` files usually hold credentials, so saving is built to never expose a half-written file, never widen its permissions, and never produce a diff you did not ask for.

## Before you start

```sh
go get github.com/ubgo/dotenv
```

You need a `*dotenv.File` with the edits you want, from `dotenv.Open` (which sets `Path`) or `dotenv.Parse` (which does not). Editing is covered in [editing.md](editing.md).

## How a save works

`Save()` and `SaveAs(path)` share one write path:

1. Every save guard plugin runs first. If one refuses, nothing is written and the destination is untouched.
2. The rendered content is written to a sibling temp file, [`<path>.dotenv-tmp`](ds:block?id=tempsuffix-vba443ms), created with mode `0600`. The file is never readable by others, even for a moment.
3. The temp file is `chmod`ed to the target mode. This explicit step is needed because the mode passed to `os.WriteFile` is filtered by the process umask, so it cannot reproduce a mode on its own.
4. The temp file is [renamed over the destination](ds:block?id=writeto-eemankjv). A rename within one directory is atomic, so a reader sees either the old file or the new one, never half of one, and a crash before the rename leaves the original intact.
5. If any step fails, the temp file is removed, so no stray copy of your secrets is left beside the real file.

What is written is exactly `Render()`: every untouched line keeps its original bytes, and `${references}` are written as references. Nothing in the write path expands a value.

## Step 1 — Save to the file you opened

`Open` a path (a missing file opens empty), edit, then `Save`. `Save` writes to `f.Path`. The directory listing afterwards shows that no temp file was left behind.

<!-- snippet: example_edit_test.go#edit_Save -->
```go
// Save writes the file back to f.Path through a temp file and a rename, so a
// reader never sees half a file and no temp file is left behind.
func ExampleFile_Save() {
	dir, cleanup := editTempDir()
	defer cleanup()
	path := filepath.Join(dir, editEnvName)

	f, err := dotenv.Open(path) // a missing file opens empty
	if err != nil {
		fmt.Println("open:", err)
		return
	}
	fmt.Println("existed:", f.Existed())
	f.Set("API_KEY", "secret")
	if err := f.Save(); err != nil {
		fmt.Println("save:", err)
		return
	}
	editReadFile(path)
	editListDir(dir)
	// Output:
	// existed: false
	// "API_KEY=secret\n"
	// [.env]
}
```

A `File` made with `Parse` has no path, so `Save` returns an error instead of guessing. Set `f.Path` first, or use [SaveAs](#step-4--write-a-copy-with-saveas):

<!-- snippet: example_edit_test.go#edit_Save_noPath -->
```go
// A File from Parse has no Path, so Save refuses. Set Path, or use SaveAs.
func ExampleFile_Save_noPath() {
	f := dotenv.Parse("A=1\n")
	fmt.Println(f.Save())
	// Output:
	// dotenv: no path set; use Open, or set Path before Save
}
```

## Step 2 — Know which mode the file gets

| Case | Mode written |
|---|---|
| `Save` on a file that did not exist when opened (or a `Parse`d file with `Path` set) | [`0600`](ds:block?id=defaultfilemode-jwzcsmwz) |
| `Save` on a file that existed when opened | the mode it had when `Open` read it |
| `SaveAs` to a path that already exists | the destination's own current mode |
| `SaveAs` to a new path | this file's mode (`Mode()`) |

`f.Mode()` returns the mode `Save` will apply. The rules exist so that a save never widens a file someone locked down and never quietly loosens a file it is about to fill with secrets.

<!-- snippet: example_edit_test.go#edit_Save_mode -->
```go
// A new file is created 0600. An existing file keeps its own mode across a
// save, whatever the process umask is.
func ExampleFile_Save_mode() {
	dir, cleanup := editTempDir()
	defer cleanup()
	path := filepath.Join(dir, editEnvName)

	f, err := dotenv.Open(path)
	if err != nil {
		fmt.Println("open:", err)
		return
	}
	f.Set("A", "1")
	if err := f.Save(); err != nil {
		fmt.Println("save:", err)
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		fmt.Println("stat:", err)
		return
	}
	fmt.Printf("new file: %o\n", info.Mode().Perm())

	// The owner widens it on purpose; the next save must keep that choice.
	const sharedMode = 0o640
	if err := os.Chmod(path, sharedMode); err != nil {
		fmt.Println("chmod:", err)
		return
	}
	g, err := dotenv.Open(path)
	if err != nil {
		fmt.Println("open:", err)
		return
	}
	g.Set("A", "2")
	if err := g.Save(); err != nil {
		fmt.Println("save:", err)
		return
	}
	info, err = os.Stat(path)
	if err != nil {
		fmt.Println("stat:", err)
		return
	}
	fmt.Printf("after save: %o\n", info.Mode().Perm())
	// Output:
	// new file: 600
	// after save: 640
}
```

## Step 3 — Keep line endings and the trailing newline

Line terminators are kept per line, not per file. A file with mixed endings stays mixed rather than being normalised, which would be a whole-file diff nobody asked for.

Lines the package writes (new keys, changed values, appended entries) [end in `\r\n` if the source contains any `\r\n` line](ds:block?id=eol-eefm88t3), and in `\n` otherwise. So appending to a Windows file does not introduce a stray `\n`:

<!-- snippet: example_edit_test.go#edit_Save_lineEndings -->
```go
// Every existing line keeps its own terminator. Lines the package writes use
// CRLF when the file has any CRLF line, so a Windows file stays CRLF.
func ExampleFile_Save_lineEndings() {
	dir, cleanup := editTempDir()
	defer cleanup()
	path := filepath.Join(dir, editEnvName)
	if err := os.WriteFile(path, []byte("A=1\r\nB=2\r\n"), 0o600); err != nil {
		fmt.Println("write:", err)
		return
	}

	f, err := dotenv.Open(path)
	if err != nil {
		fmt.Println("open:", err)
		return
	}
	f.Set("B", "two")
	f.Set("C", "3")
	if err := f.Save(); err != nil {
		fmt.Println("save:", err)
		return
	}
	editReadFile(path)

	mixed := dotenv.Parse("A=1\nB=2\r\n") // mixed stays mixed
	mixed.Set("C", "3")
	fmt.Printf("%q\n", mixed.Render())
	// Output:
	// "A=1\r\nB=two\r\nC=3\r\n"
	// "A=1\nB=2\r\nC=3\r\n"
}
```

A file that ended without a trailing newline still ends without one after edits, and a file that had one keeps it. Adding or removing it would be a spurious diff on every save. An empty `Parse("")` counts as having a trailing newline, so a file you build from scratch ends with one.

<!-- snippet: example_edit_test.go#edit_Save_trailingNewline -->
```go
// A file that ended without a newline still ends without one after edits,
// and a file that had one keeps it.
func ExampleFile_Save_trailingNewline() {
	without := dotenv.Parse("A=1")
	without.Set("B", "2")

	with := dotenv.Parse("A=1\n")
	with.Set("B", "2")

	fmt.Printf("%q\n", without.Render())
	fmt.Printf("%q\n", with.Render())
	// Output:
	// "A=1\nB=2"
	// "A=1\nB=2\n"
}
```

## Step 4 — Write a copy with SaveAs

`SaveAs(path)` writes the file to another path and leaves `f.Path` unchanged. Combined with [Clone](editing.md#step-9--derive-another-environment-with-clone), one loaded file can emit as many variants as you need, and the source file is never touched:

<!-- snippet: example_edit_test.go#edit_SaveAs -->
```go
// SaveAs writes to another path and leaves f.Path alone, so one loaded file
// can emit several variants. A new destination gets this file's mode; an
// existing one keeps its own.
func ExampleFile_SaveAs() {
	dir, cleanup := editTempDir()
	defer cleanup()
	stagingPath := filepath.Join(dir, ".env.staging")
	const src = "DOMAIN=staging.acme.io\nAPI_URL=https://api.${DOMAIN}\n"
	if err := os.WriteFile(stagingPath, []byte(src), 0o600); err != nil {
		fmt.Println("write:", err)
		return
	}

	staging, err := dotenv.Open(stagingPath)
	if err != nil {
		fmt.Println("open:", err)
		return
	}
	variants := []struct{ name, domain string }{ // a slice keeps the order fixed
		{"prod", "acme.io"},
		{"qa", "qa.acme.io"},
	}
	for _, v := range variants {
		c := staging.Clone()
		c.Set("DOMAIN", v.domain)
		if err := c.SaveAs(filepath.Join(dir, ".env."+v.name)); err != nil {
			fmt.Println("save:", err)
			return
		}
	}

	fmt.Println("Path unchanged:", staging.Path == stagingPath)
	editListDir(dir)
	editReadFile(filepath.Join(dir, ".env.prod"))
	editReadFile(stagingPath)
	// Output:
	// Path unchanged: true
	// [.env.prod .env.qa .env.staging]
	// "DOMAIN=acme.io\nAPI_URL=https://api.${DOMAIN}\n"
	// "DOMAIN=staging.acme.io\nAPI_URL=https://api.${DOMAIN}\n"
}
```

[`SaveAs`](ds:block?id=saveas-tpakytbt) uses the same atomic write path as [`Save`](ds:block?id=save-fcetxc66), and an empty path is an error.

## Try it

Every snippet on this page is a Go `Example` in [`example_edit_test.go`](../../example_edit_test.go). Each one writes only inside its own `os.MkdirTemp` directory and removes it afterwards. Run them from the repo root:

```sh
go test -run 'ExampleFile_Save' -v .
```

Real output:

```text
=== RUN   ExampleFile_Save
--- PASS: ExampleFile_Save (0.00s)
=== RUN   ExampleFile_Save_mode
--- PASS: ExampleFile_Save_mode (0.00s)
=== RUN   ExampleFile_Save_noPath
--- PASS: ExampleFile_Save_noPath (0.00s)
=== RUN   ExampleFile_Save_lineEndings
--- PASS: ExampleFile_Save_lineEndings (0.00s)
=== RUN   ExampleFile_Save_trailingNewline
--- PASS: ExampleFile_Save_trailingNewline (0.00s)
=== RUN   ExampleFile_SaveAs
--- PASS: ExampleFile_SaveAs (0.00s)
PASS
ok  	github.com/ubgo/dotenv	0.007s
```

The text each example prints is the `// Output:` block at the end of its snippet above.

## Pitfalls

- **A clone keeps the source's `Path`.** `Clone()` copies `Path`, so `clone.Save()` overwrites the file you cloned from. Use `SaveAs` for a clone.
- **`SaveAs` onto an existing file keeps that file's mode, not yours.** Overwriting a `0644` file with `SaveAs` leaves it `0644` even if the source is `0600`. Tighten the destination yourself if it holds secrets.
- **The mode comes from `Open` time.** `Save` reapplies the mode read when the file was opened. If someone `chmod`s the file between your `Open` and your `Save`, the save puts the old mode back.
- **The temp file needs a writable directory.** The temp file is created next to the destination, so `Save` fails in a directory you can read but not write, even if the file itself is writable. Saving into a directory that does not exist fails too; it is not created for you.
- **The temp name is fixed.** Two processes saving the same path at the same moment use the same `<path>.dotenv-tmp` name. Serialise saves to one file yourself.
- **A symlink is replaced, not followed.** The rename puts a regular file where the link was; the link's target is not updated.
- **An unterminated quote swallows what you append.** If the source's last value opens a quote that never closes, appended content is read back as part of that value. See [editing.md › Pitfalls](editing.md#pitfalls).
- **Modes are Unix modes.** Windows does not have Unix permission bits, so the mode rules above only hold on Unix-like systems, and the expected output of `ExampleFile_Save_mode` assumes one.

## Related

- [editing.md](editing.md) — `Set`, `Unset`, `Restore`, `Append`, `Insert*`, `Clone`.
- Reading guide: [reading.md](reading.md) — `Open`, `Existed`, `Mode`, `Path`.
- Save guard plugins, which can refuse a save: [plugins.md](plugins.md) and [README › Plugins](../../README.md#plugins).
- [Feature index](../reference/features.md).
- Examples: [`example_edit_test.go`](../../example_edit_test.go) — `ExampleFile_Save`, `ExampleFile_Save_noPath`, `ExampleFile_Save_mode`, `ExampleFile_Save_lineEndings`, `ExampleFile_Save_trailingNewline`, `ExampleFile_SaveAs`, `ExampleFile_Clone`.
