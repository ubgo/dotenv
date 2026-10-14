package envkit_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/discover"
	"github.com/ubgo/dotenv/cli/envkit"
)

// apiEnvkitFileMode is the permission the examples write env files with —
// 0600 because env files hold credentials, the same mode envkit.Set uses.
const apiEnvkitFileMode = 0o600

// apiEnvkitDir creates a scratch directory holding the given files
// (name → content). Examples print only file NAMES, never this temp path, so
// their output stays deterministic.
func apiEnvkitDir(files map[string]string) (string, error) {
	dir, err := os.MkdirTemp("", "envkit-example-")
	if err != nil {
		return "", err
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), apiEnvkitFileMode); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// snippet:start api_envkit_SelectFile

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

// snippet:end api_envkit_SelectFile

// snippet:start api_envkit_Select

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

// snippet:end api_envkit_Select

// snippet:start api_envkit_Discover

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

// snippet:end api_envkit_Discover

// snippet:start api_envkit_Inventories

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

// snippet:end api_envkit_Inventories

// snippet:start api_envkit_BuildMatrix

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

// snippet:end api_envkit_BuildMatrix

// snippet:start api_envkit_SparseColumn

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

// snippet:end api_envkit_SparseColumn

// snippet:start api_envkit_DriftRows

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

// snippet:end api_envkit_DriftRows

// snippet:start api_envkit_DisambiguateColumns

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

// snippet:end api_envkit_DisambiguateColumns

// snippet:start api_envkit_EnvNameForPath

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

// snippet:end api_envkit_EnvNameForPath

// snippet:start api_envkit_Diff

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

// snippet:end api_envkit_Diff

// snippet:start api_envkit_DiffMaps

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

// snippet:end api_envkit_DiffMaps

// snippet:start api_envkit_Set

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

// snippet:end api_envkit_Set

// snippet:start api_envkit_Unset

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

// snippet:end api_envkit_Unset

// snippet:start api_envkit_Restore

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

// snippet:end api_envkit_Restore

// snippet:start api_envkit_Run

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

// snippet:end api_envkit_Run

// snippet:start api_envkit_ChildEnv

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

// snippet:end api_envkit_ChildEnv

// snippet:start api_envkit_MergeEnv

// ExampleMergeEnv is the merge alone, for values you already hold.
func ExampleMergeEnv() {
	env := envkit.MergeEnv([]string{"A=base", "B=keep"}, map[string]string{"A": "file"})
	fmt.Println(env)
	// Output:
	// [B=keep A=file]
}

// snippet:end api_envkit_MergeEnv
