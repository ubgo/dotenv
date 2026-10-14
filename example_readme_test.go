package dotenv_test

// The examples in this file back the code blocks in the repository README.
// Each one is short on purpose: the README is a tour, and every topic links
// on to its full guide under docs/library/, where the longer examples live.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/dotenv"
)

// snippet:start readme_Quickstart

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

// snippet:end readme_Quickstart

// snippet:start readme_Reading

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

// snippet:end readme_Reading

// snippet:start readme_Editing

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

// snippet:end readme_Editing

// snippet:start readme_Generating

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

// snippet:end readme_Generating

// snippet:start readme_Disabled

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

// snippet:end readme_Disabled

// snippet:start readme_Inherited

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

// snippet:end readme_Inherited

// snippet:start readme_Cloning

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

// snippet:end readme_Cloning

// snippet:start readme_Plugins

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

// snippet:end readme_Plugins

// snippet:start readme_Expansion

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

// snippet:end readme_Expansion

// snippet:start readme_LineEndings

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

// snippet:end readme_LineEndings
