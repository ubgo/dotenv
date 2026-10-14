package dotenv_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ubgo/dotenv"
)

// snippet:start read_Read

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

// snippet:end read_Read

// snippet:start read_Open

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

// snippet:end read_Open

// snippet:start read_OpenMissing

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

// snippet:end read_OpenMissing

// snippet:start read_Parse

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

// snippet:end read_Parse

// snippet:start read_ParseReader

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

// snippet:end read_ParseReader

// snippet:start read_Get

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

// snippet:end read_Get

// snippet:start read_Has

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

// snippet:end read_Has

// snippet:start read_Count

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

// snippet:end read_Count

// snippet:start read_Keys

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

// snippet:end read_Keys

// snippet:start read_Pairs

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

// snippet:end read_Pairs

// snippet:start read_Map

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

// snippet:end read_Map

// snippet:start read_Existed

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

// snippet:end read_Existed

// snippet:start read_Mode

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

// snippet:end read_Mode

// snippet:start read_Path

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

// snippet:end read_Path

// snippet:start read_Entries

// ExampleFile_Entries walks every entry in file order, comments and blanks
// included. This is the view for tools that inspect structure, such as a
// linter that flags disabled settings or unrecognised lines.
func ExampleFile_Entries() {
	const content = `# database
DB_HOST=localhost
# DB_USER=admin

HOME
not a setting
`
	f := dotenv.Parse(content)

	for _, e := range f.Entries() {
		switch e.Kind {
		case dotenv.KindPair:
			fmt.Printf("%-13s %s=%s\n", e.Kind, e.Key, e.Value)
		case dotenv.KindDisabledPair:
			fmt.Printf("%-13s %s (off, would be %s)\n", e.Kind, e.Key, e.Value)
		case dotenv.KindInherited:
			fmt.Printf("%-13s %s (from the environment)\n", e.Kind, e.Key)
		default:
			fmt.Printf("%-13s %q\n", e.Kind, e.Raw[0])
		}
	}
	// Output:
	// comment       "# database"
	// pair          DB_HOST=localhost
	// disabled-pair DB_USER (off, would be admin)
	// blank         ""
	// inherited     HOME (from the environment)
	// other         "not a setting"
}

// snippet:end read_Entries

// snippet:start read_Kind

// ExampleKind shows the exact line that produces each of the six kinds.
func ExampleKind() {
	lines := []string{
		"PORT=8080",         // a pair
		"# a note",          // a comment
		"",                  // a blank line
		"not a setting!",    // unrecognised, kept verbatim
		"# DEBUG=true",      // a commented-out setting
		"export AWS_REGION", // a name with no value: inherited
	}

	f := dotenv.Parse(strings.Join(lines, "\n") + "\n")
	for _, e := range f.Entries() {
		fmt.Printf("%-13s %q\n", e.Kind, e.Raw[0])
	}
	// Output:
	// pair          "PORT=8080"
	// comment       "# a note"
	// blank         ""
	// other         "not a setting!"
	// disabled-pair "# DEBUG=true"
	// inherited     "export AWS_REGION"
}

// snippet:end read_Kind

// snippet:start read_KindPitfalls

// ExampleKind_pitfalls shows lines that parse as something other than what a
// reader might expect. Each is the documented behaviour, and each line still
// renders back byte for byte.
func ExampleKind_pitfalls() {
	const content = `# TODO=fix this
# Note: rotate monthly
# Note - rotate monthly
http://example.com
MY KEY=value
`
	f := dotenv.Parse(content)

	for _, e := range f.Entries() {
		fmt.Printf("%-13s key=%q value=%q\n", e.Kind, e.Key, e.Value)
	}
	fmt.Println("round trip:", f.Render() == content)
	// Output:
	// disabled-pair key="TODO" value="fix this"
	// disabled-pair key="Note" value="rotate monthly"
	// comment       key="" value=""
	// pair          key="http" value="//example.com"
	// other         key="" value=""
	// round trip: true
}

// snippet:end read_KindPitfalls

// snippet:start read_KindString

// ExampleKind_String prints a Kind as a name, which is what %s, %v and a log
// line show. A value outside the six kinds prints as Kind(n).
func ExampleKind_String() {
	kinds := []dotenv.Kind{
		dotenv.KindPair,
		dotenv.KindComment,
		dotenv.KindBlank,
		dotenv.KindOther,
		dotenv.KindDisabledPair,
		dotenv.KindInherited,
		dotenv.Kind(42),
	}
	for _, k := range kinds {
		fmt.Println(k)
	}
	// Output:
	// pair
	// comment
	// blank
	// other
	// disabled-pair
	// inherited
	// Kind(42)
}

// snippet:end read_KindString

// snippet:start read_Entry

// ExampleEntry shows the four exported fields. Raw keeps the exact physical
// lines (a multi-line value spans several), Value is the decoded value with
// quotes stripped and lines joined by \n.
func ExampleEntry() {
	const content = "export CERT=\"line one\nline two\" # pem\n# DEBUG='true'\n"

	f := dotenv.Parse(content)
	for _, e := range f.Entries() {
		fmt.Println("Kind: ", e.Kind)
		fmt.Printf("Raw:   %q\n", e.Raw)
		fmt.Printf("Key:   %q\n", e.Key)
		fmt.Printf("Value: %q\n", e.Value)
	}
	// Output:
	// Kind:  pair
	// Raw:   ["export CERT=\"line one" "line two\" # pem"]
	// Key:   "CERT"
	// Value: "line one\nline two"
	// Kind:  disabled-pair
	// Raw:   ["# DEBUG='true'"]
	// Key:   "DEBUG"
	// Value: "true"
}

// snippet:end read_Entry
