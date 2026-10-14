package dotenv_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ubgo/dotenv"
)

// editEnvName is the file name every saving example writes inside its own
// temporary directory. Only this base name is ever printed, never the
// directory, so the output is the same on every machine.
const editEnvName = ".env"

// editTempDir makes a private scratch directory for one example and returns a
// cleanup func. Examples cannot take a *testing.T, so this stands in for
// t.TempDir.
func editTempDir() (string, func()) {
	dir, err := os.MkdirTemp("", "dotenv-example-")
	if err != nil {
		panic(err) // no scratch directory means the example cannot run at all
	}
	return dir, func() {
		if rmErr := os.RemoveAll(dir); rmErr != nil {
			fmt.Println("cleanup:", rmErr)
		}
	}
}

// editListDir prints the base names in dir, sorted, so an example can show
// that Save left no temp file behind.
func editListDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Println("read dir:", err)
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	fmt.Println(names)
}

// editReadFile prints a file's exact bytes, quoted so line endings show.
func editReadFile(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("read:", err)
		return
	}
	fmt.Printf("%q\n", b)
}

// snippet:start edit_NewPair

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

// snippet:end edit_NewPair

// snippet:start edit_NewComment

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

// snippet:end edit_NewComment

// snippet:start edit_Set

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
	// export DB_HOST =db.internal # dev only
	// DB_PORT=5432
}

// snippet:end edit_Set

// snippet:start edit_Set_noop

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

// snippet:end edit_Set_noop

// snippet:start edit_Set_quoting

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

// snippet:end edit_Set_quoting

// snippet:start edit_Set_duplicates

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

// snippet:end edit_Set_duplicates

// snippet:start edit_Set_dollar

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

// snippet:end edit_Set_dollar

// snippet:start edit_SetAfter

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

// snippet:end edit_SetAfter

// snippet:start edit_SetAfter_anchorNotFound

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

// snippet:end edit_SetAfter_anchorNotFound

// snippet:start edit_SetBefore

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

// snippet:end edit_SetBefore

// snippet:start edit_Unset

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

// snippet:end edit_Unset

// snippet:start edit_Restore

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

// snippet:end edit_Restore

// snippet:start edit_Disabled

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

// snippet:end edit_Disabled

// snippet:start edit_Inherited

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

// snippet:end edit_Inherited

// snippet:start edit_Append

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

// snippet:end edit_Append

// snippet:start edit_Append_vsSet

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

// snippet:end edit_Append_vsSet

// snippet:start edit_Append_verbatim

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

// snippet:end edit_Append_verbatim

// snippet:start edit_Append_unterminatedQuote

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

// snippet:end edit_Append_unterminatedQuote

// snippet:start edit_InsertAfter

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

// snippet:end edit_InsertAfter

// snippet:start edit_InsertBefore

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

// snippet:end edit_InsertBefore

// snippet:start edit_Clone

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

// snippet:end edit_Clone

// snippet:start edit_Clone_baked

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

// snippet:end edit_Clone_baked

// snippet:start edit_Save

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

// snippet:end edit_Save

// snippet:start edit_Save_mode

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

// snippet:end edit_Save_mode

// snippet:start edit_Save_noPath

// A File from Parse has no Path, so Save refuses. Set Path, or use SaveAs.
func ExampleFile_Save_noPath() {
	f := dotenv.Parse("A=1\n")
	fmt.Println(f.Save())
	// Output:
	// dotenv: no path set; use Open, or set Path before Save
}

// snippet:end edit_Save_noPath

// snippet:start edit_Save_lineEndings

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

// snippet:end edit_Save_lineEndings

// snippet:start edit_Save_trailingNewline

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

// snippet:end edit_Save_trailingNewline

// snippet:start edit_SaveAs

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

// snippet:end edit_SaveAs
