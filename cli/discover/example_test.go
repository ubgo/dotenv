package discover_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ubgo/dotenv/cli/discover"
)

// apiDiscoverFileMode is 0600 because env files hold credentials.
const apiDiscoverFileMode = 0o600

// snippet:start api_discover_Scan

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

// snippet:end api_discover_Scan

// snippet:start api_discover_Classify

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

// snippet:end api_discover_Classify
