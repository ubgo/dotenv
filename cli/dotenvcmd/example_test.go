package dotenvcmd_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ubgo/dotenv/cli/dotenvcmd"
)

// The examples below drive the whole CLI in-process through
// dotenvcmd.Execute, exactly as the dotenvctl binary does, over files in a
// fresh temp directory. Each one changes into that directory first so the
// printed output shows relative names (.env, .env.prod) and never a temp
// path. Examples in one package run one after another, so the chdir is safe
// as long as every example restores the old directory before returning.

// cliFilePerm is the mode the fixtures are written with: env files hold
// credentials, and dotenvctl itself creates new files 0600.
const cliFilePerm = 0o600

// cliDefaultEnv is the family's default file: what every verb reads when
// -f/--file is not given.
const cliDefaultEnv = "# local development\nDB_HOST=localhost\nDB_PORT=5432\nAPI_URL=https://api.${DOMAIN}\nDOMAIN=acme.dev\n"

// cliStagingEnv and cliProdEnv are two environments that drift on purpose:
// prod carries a placeholder (DB_PASS), a commented-out setting (FEATURE_X)
// and a key staging lacks (SENTRY_DSN).
const (
	cliStagingEnv = "DB_HOST=stag.db.internal\nDB_PASS=stag-pass\nDOMAIN=staging.acme.io\nFEATURE_X=on\n"
	cliProdEnv    = "DB_HOST=prod.db.internal\nDB_PASS=__YOU__\nDOMAIN=acme.io\n# FEATURE_X=on\nSENTRY_DSN=https://key@sentry.example/1\n"
)

// cliContractEnv is the template every environment must satisfy.
const cliContractEnv = "DB_HOST=\nDB_PASS=\nDOMAIN=\nSENTRY_DSN=\n"

// cliFamily is the fixture set, in a fixed order so setup is deterministic.
var cliFamily = []struct{ name, content string }{
	{".env", cliDefaultEnv},
	{".env.staging", cliStagingEnv},
	{".env.prod", cliProdEnv},
	{".env.example", cliContractEnv},
}

// cliEnterFamily writes the fixture family into a new temp directory, changes
// into it, and returns a cleanup that restores the old directory and removes
// the temp one. Any setup failure is printed, so it shows up as an Output
// mismatch instead of passing silently.
func cliEnterFamily() (cleanup func()) {
	noop := func() {}
	dir, err := os.MkdirTemp("", "dotenvctl-example-")
	if err != nil {
		fmt.Println("setup:", err)
		return noop
	}
	for _, f := range cliFamily {
		if err := os.WriteFile(filepath.Join(dir, f.name), []byte(f.content), cliFilePerm); err != nil {
			fmt.Println("setup:", err)
			return noop
		}
	}
	prev, err := os.Getwd()
	if err != nil {
		fmt.Println("setup:", err)
		return noop
	}
	if err := os.Chdir(dir); err != nil {
		fmt.Println("setup:", err)
		return noop
	}
	return func() {
		if err := os.Chdir(prev); err != nil {
			fmt.Println("cleanup:", err)
		}
		if err := os.RemoveAll(dir); err != nil {
			fmt.Println("cleanup:", err)
		}
	}
}

// snippet:start cli_Run

// cliRun runs one dotenvctl invocation with both streams on stdout, so the
// Output block shows errors too, then prints the exit code.
func cliRun(args ...string) {
	code := dotenvcmd.Execute(args, os.Stdout, os.Stdout)
	fmt.Printf("exit=%d\n", code)
}

// snippet:end cli_Run

// cliCat prints a file from the current directory, to show what a mutating
// verb left on disk.
func cliCat(name string) {
	b, err := os.ReadFile(name)
	if err != nil {
		fmt.Println("read:", err)
		return
	}
	fmt.Print(string(b))
}

// snippet:start cli_Execute

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

// snippet:end cli_Execute

// snippet:start cli_ExecuteProgramName

// ExampleExecute_programName mounts the CLI inside a host program under the
// words its users type, "myapp env": WithProgramName puts that name in the
// lines the CLI prints about itself, so a failure never names a dotenvctl
// binary the user does not have.
func ExampleExecute_programName() {
	// No such verb: a usage failure, exit code 2, with the host's name in the
	// message and in the hint. Stderr goes to stdout here so it is checked.
	code := dotenvcmd.Execute([]string{"nosuchverb"}, os.Stdout, os.Stdout,
		dotenvcmd.WithProgramName("myapp env"))
	fmt.Println("exit code:", code)
	// Output:
	// myapp env: unknown command "nosuchverb" for "myapp env"
	// Run 'myapp env --help' for usage.
	// exit code: 2
}

// snippet:end cli_ExecuteProgramName

// snippet:start cli_Get

// ExampleExecute_get reads one value: raw by default, resolved with
// --expand, and exit code 1 when the key has no active entry.
func ExampleExecute_get() {
	defer cliEnterFamily()()

	cliRun("get", "API_URL")
	cliRun("get", "API_URL", "--expand")
	cliRun("-f", ".env.prod", "get", "FEATURE_X") // commented out, so not active
	// Output:
	// https://api.${DOMAIN}
	// exit=0
	// https://api.acme.dev
	// exit=0
	// dotenvctl: key "FEATURE_X" not found in .env.prod
	// exit=1
}

// snippet:end cli_Get

// snippet:start cli_Set

// ExampleExecute_set upserts values. An existing key changes in place, a new
// key lands after its --after anchor, and --dry-run prints the line diff
// without writing.
func ExampleExecute_set() {
	defer cliEnterFamily()()

	cliRun("set", "DB_PORT=6543", "--dry-run")
	cliRun("set", "DB_PORT=6543", "DB_USER=admin")
	cliRun("set", "DB_PASSWORD=s3cret", "--after", "DB_USER")
	cliRun("set", "X=1", "--after", "NOPE") // a missing anchor writes nothing
	cliCat(".env")
	// Output:
	// -DB_PORT=5432
	// +DB_PORT=6543
	// exit=0
	// exit=0
	// exit=0
	// dotenvctl: envkit: anchor key not found: "NOPE"
	// exit=1
	// # local development
	// DB_HOST=localhost
	// DB_PORT=6543
	// API_URL=https://api.${DOMAIN}
	// DOMAIN=acme.dev
	// DB_USER=admin
	// DB_PASSWORD=s3cret
}

// snippet:end cli_Set

// snippet:start cli_UnsetRestore

// ExampleExecute_unset comments a key out by default (reversible), and
// restore brings it back byte for byte. --delete removes the line instead.
func ExampleExecute_unset() {
	defer cliEnterFamily()()

	cliRun("unset", "DB_PORT")
	cliCat(".env")
	cliRun("restore", "DB_PORT")
	cliRun("unset", "DB_HOST", "--delete", "--dry-run")
	cliRun("restore", "NOPE")
	cliCat(".env")
	// Output:
	// exit=0
	// # local development
	// DB_HOST=localhost
	// # DB_PORT=5432
	// API_URL=https://api.${DOMAIN}
	// DOMAIN=acme.dev
	// exit=0
	// -DB_HOST=localhost
	// exit=0
	// dotenvctl: envkit: key not found: no disabled entry for "NOPE"
	// exit=1
	// # local development
	// DB_HOST=localhost
	// DB_PORT=5432
	// API_URL=https://api.${DOMAIN}
	// DOMAIN=acme.dev
}

// snippet:end cli_UnsetRestore

// snippet:start cli_List

// ExampleExecute_list shows the file's active pairs as an aligned table, and
// a selected, renamed view with the shared selection flags.
func ExampleExecute_list() {
	defer cliEnterFamily()()

	cliRun("list")
	cliRun("-f", ".env.prod", "list", "--disabled")
	cliRun("list", "--prefix", "DB_", "--strip-prefix", "--json")
	// Output:
	// DB_HOST  localhost
	// DB_PORT  5432
	// API_URL  https://api.${DOMAIN}
	// DOMAIN   acme.dev
	// exit=0
	// DB_HOST     prod.db.internal
	// DB_PASS     __YOU__
	// DOMAIN      acme.io
	// SENTRY_DSN  https://key@sentry.example/1
	//
	// # disabled
	// # FEATURE_X=on
	// exit=0
	// {"ok":true,"data":{"pairs":[{"key":"HOST","value":"localhost"},{"key":"PORT","value":"5432"}],"disabled":[],"inherited":[],"expanded":false}}
	// exit=0
}

// snippet:end cli_List

// snippet:start cli_Keys

// ExampleExecute_keys prints active key names one per line, for pipes. The
// shared selection flags filter and rename them, and a named key that does
// not exist fails loudly.
func ExampleExecute_keys() {
	defer cliEnterFamily()()

	cliRun("keys")
	cliRun("keys", "--prefix", "DB_", "--strip-prefix")
	cliRun("keys", "--exclude-prefix", "DB_", "--json")
	cliRun("keys", "--include", "NOPE")
	// Output:
	// DB_HOST
	// DB_PORT
	// API_URL
	// DOMAIN
	// exit=0
	// HOST
	// PORT
	// exit=0
	// {"ok":true,"data":{"keys":["API_URL","DOMAIN"]}}
	// exit=0
	// dotenvctl: envkit: --include key "NOPE" not found
	// exit=1
}

// snippet:end cli_Keys

// snippet:start cli_Envs

// ExampleExecute_envs discovers the directory's .env family: long names
// collapse to short ones, .env.example is a contract, and the PLACEHOLDERS
// column counts __LIKE_THIS__ values.
func ExampleExecute_envs() {
	defer cliEnterFamily()()

	cliRun("envs")
	// Output:
	// ENV                 FILE          KEYS  DISABLED  INHERITED  PLACEHOLDERS
	// default             .env          4     0         0          0
	// stag                .env.staging  4     0         0          0
	// prod                .env.prod     4     1         0          1
	// example (contract)  .env.example  4     0         0          0
	// exit=0
}

// snippet:end cli_Envs

// snippet:start cli_Matrix

// ExampleExecute_matrix lays keys against environments. --contract turns it
// into a CI gate: exit 1 and the gaps named when an environment misses a
// contract key.
func ExampleExecute_matrix() {
	defer cliEnterFamily()()

	cliRun("matrix", ".env.staging", ".env.prod", "--only-drift")
	cliRun("matrix", ".env.staging", ".env.prod", "--contract", ".env.example")
	// Output:
	// KEY         stag  prod
	// DB_PASS     ✓     !
	// FEATURE_X   ✓     #
	// SENTRY_DSN  —     ✓
	//
	// ✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
	// exit=0
	// KEY         stag  prod
	// DB_HOST     ✓     ✓
	// DB_PASS     ✓     !
	// DOMAIN      ✓     ✓
	// FEATURE_X   ✓     #
	// SENTRY_DSN  —     ✓
	//
	// ✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
	// contract: stag is missing 1 key(s): [SENTRY_DSN]
	// exit=1
}

// snippet:end cli_Matrix

// snippet:start cli_MatrixPlaceholder

// ExampleExecute_matrix_placeholder swaps the placeholder pattern. The flag
// REPLACES the default __LIKE_THIS__ pattern, so keep it in an alternation
// when you want both.
func ExampleExecute_matrix_placeholder() {
	defer cliEnterFamily()()

	// The DSN counts as a placeholder; __YOU__ no longer does.
	cliRun("matrix", ".env.staging", ".env.prod", "--placeholder", "^https://key@")
	// Both patterns at once.
	cliRun("matrix", ".env.staging", ".env.prod", "--placeholder", "^(__[A-Z0-9_]+__|https://key@.*)$", "--only-drift")
	// Output:
	// KEY         stag  prod
	// DB_HOST     ✓     ✓
	// DB_PASS     ✓     ✓
	// DOMAIN      ✓     ✓
	// FEATURE_X   ✓     #
	// SENTRY_DSN  —     !
	//
	// ✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
	// exit=0
	// KEY         stag  prod
	// DB_PASS     ✓     !
	// FEATURE_X   ✓     #
	// SENTRY_DSN  —     !
	//
	// ✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
	// exit=0
}

// snippet:end cli_MatrixPlaceholder

// snippet:start cli_Diff

// ExampleExecute_diff compares two files' effective configuration. Like
// diff(1), it exits 1 when they differ.
func ExampleExecute_diff() {
	defer cliEnterFamily()()

	cliRun("diff", ".env.staging", ".env.prod")
	cliRun("diff", ".env.staging", ".env.staging")
	// Output:
	// + SENTRY_DSN=https://key@sentry.example/1
	// - FEATURE_X=on
	// ~ DB_HOST: stag.db.internal -> prod.db.internal
	// ~ DB_PASS: stag-pass -> __YOU__
	// ~ DOMAIN: staging.acme.io -> acme.io
	// exit=1
	// exit=0
}

// snippet:end cli_Diff

// snippet:start cli_JSON

// ExampleExecute_json shows the --json envelope: exactly one object on
// stdout, {"ok":true,"data":…} on success and {"ok":false,"error":…} on
// failure, with a stable error code.
func ExampleExecute_json() {
	defer cliEnterFamily()()

	cliRun("get", "DB_HOST", "--json")
	cliRun("get", "NOPE", "--json")
	cliRun("diff", ".env.staging", ".env.prod", "--json")
	cliRun("envs", "--json")
	// Output:
	// {"ok":true,"data":{"key":"DB_HOST","value":"localhost","expanded":false}}
	// exit=0
	// {"ok":false,"error":{"code":"not_found","message":"key \"NOPE\" not found in .env"}}
	// exit=1
	// {"ok":true,"data":{"added":[{"key":"SENTRY_DSN","value":"https://key@sentry.example/1"}],"removed":[{"key":"FEATURE_X","value":"on"}],"changed":[{"key":"DB_HOST","from":"stag.db.internal","to":"prod.db.internal"},{"key":"DB_PASS","from":"stag-pass","to":"__YOU__"},{"key":"DOMAIN","from":"staging.acme.io","to":"acme.io"}],"expanded":false,"file_a":".env.staging","file_b":".env.prod"}}
	// exit=1
	// {"ok":true,"data":{"files":[{"env":"default","file":".env","path":"./.env","contract":false,"keys":4,"disabled":0,"inherited":0,"placeholders":0},{"env":"stag","file":".env.staging","path":"./.env.staging","contract":false,"keys":4,"disabled":0,"inherited":0,"placeholders":0},{"env":"prod","file":".env.prod","path":"./.env.prod","contract":false,"keys":4,"disabled":1,"inherited":0,"placeholders":1},{"env":"example","file":".env.example","path":"./.env.example","contract":true,"keys":4,"disabled":0,"inherited":0,"placeholders":0}]}}
	// exit=0
}

// snippet:end cli_JSON
