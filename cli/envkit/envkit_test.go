// Package envkit_test exercises the API the way an EXTERNAL Go program does
// — external test package, public identifiers only. If something here needs
// an unexported helper, the API has a hole.
package envkit_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/envkit"
)

// writeFile drops content at dir/name and returns the path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// fixture is the canonical multi-audience env file: app config, prefixed
// deploy secrets, a placeholder, an empty, a reference, a disabled entry and
// an inherited declaration.
const fixture = "# app\nDB_HOST=db.internal\nURL=${DB_HOST}:5432\n" +
	"GITHUB_SECRET_PAT=ghtoken\nGITHUB_SECRET_WHO=__YOU__\nGITHUB_SECRET_BLANK=\n" +
	"# OLD_KEY=disabled\nHOME\n"

func TestSelect(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeFile(t, dir, ".env", fixture)

	t.Run("prefix, strip, and guards", func(t *testing.T) {
		t.Parallel()
		sel, err := envkit.SelectFile(path, envkit.SelectOptions{
			Prefix:      "GITHUB_SECRET_",
			StripPrefix: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := sel.Names(); !slices.Equal(got, []string{"PAT"}) {
			t.Errorf("Names() = %v, want [PAT]", got)
		}
		reasons := map[string]envkit.SkipReason{}
		for _, s := range sel.Skipped {
			reasons[s.Name] = s.Reason
		}
		if reasons["WHO"] != envkit.SkipPlaceholder || reasons["BLANK"] != envkit.SkipEmpty {
			t.Errorf("skips = %v, want placeholder+empty under STRIPPED names", reasons)
		}
		if m := sel.Map(); m["PAT"] != "ghtoken" || len(m) != 1 {
			t.Errorf("Map() = %v", m)
		}
	})

	t.Run("expansion is opt-in", func(t *testing.T) {
		t.Parallel()
		raw, err := envkit.SelectFile(path, envkit.SelectOptions{Keys: []string{"URL"}})
		if err != nil {
			t.Fatal(err)
		}
		if raw.Pairs[0].Value != "${DB_HOST}:5432" {
			t.Errorf("raw = %q, want the literal reference", raw.Pairs[0].Value)
		}
		expanded, err := envkit.SelectFile(path, envkit.SelectOptions{Keys: []string{"URL"}, Expand: true})
		if err != nil {
			t.Fatal(err)
		}
		if expanded.Pairs[0].Value != "db.internal:5432" {
			t.Errorf("expanded = %q", expanded.Pairs[0].Value)
		}
	})

	t.Run("exclude-prefix inverts, include bypasses, exclude drops", func(t *testing.T) {
		t.Parallel()
		sel, err := envkit.SelectFile(path, envkit.SelectOptions{
			ExcludePrefix: "GITHUB_SECRET_",
			ExcludeKeys:   []string{"URL"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := sel.Names(); !slices.Equal(got, []string{"DB_HOST"}) {
			t.Errorf("Names() = %v, want [DB_HOST]", got)
		}

		forced, err := envkit.SelectFile(path, envkit.SelectOptions{
			Prefix:      "GITHUB_SECRET_",
			IncludeKeys: []string{"DB_HOST"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(forced.Names(), "DB_HOST") {
			t.Errorf("--include did not bypass the prefix filter: %v", forced.Names())
		}
	})

	t.Run("named-but-absent keys fail loudly", func(t *testing.T) {
		t.Parallel()
		if _, err := envkit.SelectFile(path, envkit.SelectOptions{Keys: []string{"NOPE"}}); err == nil {
			t.Error("want an error for a named missing key")
		}
		if _, err := envkit.SelectFile(path, envkit.SelectOptions{IncludeKeys: []string{"NOPE"}}); err == nil {
			t.Error("want an error for a named missing --include key")
		}
		// Excluding an absent key is legitimate.
		if _, err := envkit.SelectFile(path, envkit.SelectOptions{ExcludeKeys: []string{"NOPE"}}); err != nil {
			t.Errorf("excluding an absent key must be a no-op, got %v", err)
		}
	})

	t.Run("custom placeholder pattern", func(t *testing.T) {
		t.Parallel()
		p := writeFile(t, t.TempDir(), ".env", "A=TODO\nB=real\n")
		sel, err := envkit.SelectFile(p, envkit.SelectOptions{PlaceholderPattern: `^TODO$`})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(sel.Names(), []string{"B"}) || len(sel.Skipped) != 1 {
			t.Errorf("pairs=%v skips=%v", sel.Names(), sel.Skipped)
		}
		if _, err := envkit.SelectFile(p, envkit.SelectOptions{PlaceholderPattern: "([broken"}); err == nil {
			t.Error("a bad pattern must error, not panic")
		}
	})

	t.Run("Select over an already-parsed file", func(t *testing.T) {
		t.Parallel()
		f, err := dotenv.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		sel, err := envkit.Select(f, envkit.SelectOptions{Prefix: "GITHUB_SECRET_"})
		if err != nil {
			t.Fatal(err)
		}
		if len(sel.Pairs) != 1 {
			t.Errorf("pairs = %v", sel.Pairs)
		}
	})

	t.Run("required-var errors surface before any use", func(t *testing.T) {
		t.Parallel()
		p := writeFile(t, t.TempDir(), ".env", "A=${MISSING:?fill me in}\n")
		_, err := envkit.SelectFile(p, envkit.SelectOptions{Expand: true})
		if err == nil || !strings.Contains(err.Error(), "fill me in") {
			t.Errorf("err = %v, want the required-var message", err)
		}
	})
}

func TestDiscoverAndInventories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, ".env.prod", "A=1\n# B=2\nHOME\nP=__YOU__\n")
	writeFile(t, dir, ".env.staging", "A=1\n")
	writeFile(t, dir, ".env.example", "A=x\n")
	writeFile(t, dir, ".env.bak", "JUNK=1\n")

	found, err := envkit.Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	var envs []string
	for _, ef := range found {
		envs = append(envs, ef.Env)
	}
	// Junk excluded, long form canonicalized, contract last.
	if !slices.Equal(envs, []string{"stag", "prod", "example"}) {
		t.Errorf("Discover = %v", envs)
	}

	inv, err := envkit.Inventories(envkit.InventoryOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var prod envkit.Inventory
	for _, i := range inv {
		if i.Env == "prod" {
			prod = i
		}
	}
	if prod.Keys != 2 || prod.Disabled != 1 || prod.Inherited != 1 || prod.Placeholders != 1 {
		t.Errorf("prod inventory = %+v", prod)
	}
}

func TestBuildMatrix(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, ".env.dev", "SHARED=1\nDEV_ONLY=x\nGUARD=real\n")
	writeFile(t, dir, ".env.prod", "SHARED=1\nGUARD=__YOU__\n# DISABLED_KEY=v\n")
	contract := writeFile(t, dir, ".env.example", "SHARED=x\nCONTRACT_ONLY=x\n")

	m, err := envkit.BuildMatrix(envkit.MatrixOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.Envs, []string{"dev", "prod"}) {
		t.Fatalf("envs = %v — contracts must not be columns", m.Envs)
	}

	state := func(key, env string) envkit.CellState {
		for _, row := range m.Rows {
			if row.Key != key {
				continue
			}
			for _, c := range row.Cells {
				if c.Env == env {
					return c.State
				}
			}
		}
		return "absent-row"
	}
	for _, tc := range []struct {
		key, env string
		want     envkit.CellState
	}{
		{"SHARED", "dev", envkit.StatePresent},
		{"DEV_ONLY", "prod", envkit.StateMissing},
		{"GUARD", "prod", envkit.StatePlaceholder},
		{"DISABLED_KEY", "prod", envkit.StateDisabled},
	} {
		if got := state(tc.key, tc.env); got != tc.want {
			t.Errorf("%s/%s = %s, want %s", tc.key, tc.env, got, tc.want)
		}
	}

	// Values are absent unless revealed — the leak-proof default.
	for _, row := range m.Rows {
		for _, c := range row.Cells {
			if c.Value != "" {
				t.Errorf("unrevealed matrix carried a value: %s = %q", row.Key, c.Value)
			}
		}
	}
	revealed, err := envkit.BuildMatrix(envkit.MatrixOptions{Dir: dir, Reveal: true})
	if err != nil {
		t.Fatal(err)
	}
	revealedValue := func(key, env string) string {
		for _, row := range revealed.Rows {
			if row.Key != key {
				continue
			}
			for _, c := range row.Cells {
				if c.Env == env {
					return c.Value
				}
			}
		}
		return ""
	}
	if got := revealedValue("SHARED", "dev"); got != "1" {
		t.Errorf("revealed SHARED/dev = %q, want the real value", got)
	}
	// Raw() exposes the value regardless of Reveal, for masking renderers.
	for _, row := range m.Rows {
		for _, c := range row.Cells {
			if c.State == envkit.StatePresent && c.Raw() == "" {
				t.Errorf("Raw() empty for a present cell: %s/%s", row.Key, c.Env)
			}
		}
	}

	// Contract gate.
	gated, err := envkit.BuildMatrix(envkit.MatrixOptions{Dir: dir, ContractPath: contract})
	if err != nil {
		t.Fatal(err)
	}
	if !gated.HasContractGaps() {
		t.Error("CONTRACT_ONLY is missing everywhere — want a gap")
	}
	if !slices.Contains(gated.ContractMissing["dev"], "CONTRACT_ONLY") {
		t.Errorf("contract_missing = %v", gated.ContractMissing)
	}

	// Drift filter + sparse diagnosis.
	before := len(m.Rows)
	drifted := envkit.DriftRows(slices.Clone(m.Rows))
	if len(drifted) >= before {
		t.Error("drift filter hid nothing on a fixture with a uniform row")
	}

	// Fewer than two columns is an error, not a degenerate table.
	single := t.TempDir()
	writeFile(t, single, ".env", "A=1\n")
	if _, err := envkit.BuildMatrix(envkit.MatrixOptions{Dir: single}); err == nil {
		t.Error("one column must error")
	}
}

func TestMatrixDisambiguatesCollidingColumns(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, ".env.stag", "A=from-stag\n")
	writeFile(t, dir, ".env.staging", "A=from-staging\nB=extra\n")

	m, err := envkit.BuildMatrix(envkit.MatrixOptions{Dir: dir, Reveal: true})
	if err != nil {
		t.Fatal(err)
	}
	// Both spellings canonicalize to "stag"; labels must stay unique or one
	// file's data silently renders under both columns.
	if !slices.Equal(m.Envs, []string{".env.stag", ".env.staging"}) {
		t.Fatalf("envs = %v, want filename fallback", m.Envs)
	}
	values := map[string]string{}
	for _, c := range m.Rows[0].Cells {
		values[c.Env] = c.Value
	}
	if values[".env.stag"] == values[".env.staging"] {
		t.Errorf("collided columns share data: %v", values)
	}
}

func TestDiff(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := writeFile(t, dir, "a.env", "SAME=1\nGONE=2\nCHANGED=old\nHOST=h\nREF=${HOST}\n")
	b := writeFile(t, dir, "b.env", "SAME=1\nCHANGED=new\nADDED=3\nHOST=h\nREF=h\n")

	d, err := envkit.Diff(a, b, envkit.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if d.Empty() {
		t.Fatal("want differences")
	}
	if len(d.Added) != 1 || d.Added[0].Key != "ADDED" {
		t.Errorf("added = %v", d.Added)
	}
	if len(d.Removed) != 1 || d.Removed[0].Key != "GONE" {
		t.Errorf("removed = %v", d.Removed)
	}
	// Raw comparison sees REF's literal text as a change…
	if !slices.ContainsFunc(d.Changed, func(c envkit.DiffChange) bool { return c.Key == "REF" }) {
		t.Errorf("raw diff should flag REF: %v", d.Changed)
	}
	// …expanded comparison resolves it away.
	expanded, err := envkit.Diff(a, b, envkit.DiffOptions{Expand: true})
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(expanded.Changed, func(c envkit.DiffChange) bool { return c.Key == "REF" }) {
		t.Errorf("expanded diff should resolve REF: %v", expanded.Changed)
	}

	// Identical files are Empty, and a missing file is an error rather than
	// "everything was removed".
	same, err := envkit.Diff(a, a, envkit.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !same.Empty() {
		t.Error("a file compared with itself must be empty")
	}
	if _, err := envkit.Diff(a, filepath.Join(dir, "absent.env"), envkit.DiffOptions{}); err == nil {
		t.Error("missing file must error")
	}
}

func TestEditOperations(t *testing.T) {
	t.Parallel()

	t.Run("set preserves every untouched byte", func(t *testing.T) {
		t.Parallel()
		const original = "# a comment\nexport A = 1 # inline\n\nB: two\n"
		path := writeFile(t, t.TempDir(), ".env", original)

		res, err := envkit.Set(path, []dotenv.Pair{{Key: "A", Value: "9"}}, envkit.EditOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !res.Changed || !res.Written {
			t.Fatalf("res = %+v", res)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		want := strings.Replace(original, "export A = 1 # inline", "export A =9 # inline", 1)
		if string(got) != want {
			t.Errorf("file drifted:\n got: %q\nwant: %q", got, want)
		}
	})

	t.Run("setting the current value is a true no-op", func(t *testing.T) {
		t.Parallel()
		path := writeFile(t, t.TempDir(), ".env", "A=1\n")
		before, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		res, err := envkit.Set(path, []dotenv.Pair{{Key: "A", Value: "1"}}, envkit.EditOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if res.Changed || res.Written {
			t.Errorf("no-op reported as a change: %+v", res)
		}
		after, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !after.ModTime().Equal(before.ModTime()) {
			t.Error("no-op rewrote the file")
		}
	})

	t.Run("dry-run returns the diff and writes nothing", func(t *testing.T) {
		t.Parallel()
		path := writeFile(t, t.TempDir(), ".env", "A=1\n")
		res, err := envkit.Set(path, []dotenv.Pair{{Key: "A", Value: "2"}}, envkit.EditOptions{DryRun: true})
		if err != nil {
			t.Fatal(err)
		}
		if !res.Changed || res.Written {
			t.Errorf("res = %+v", res)
		}
		if !slices.Contains(res.Diff, "-A=1") || !slices.Contains(res.Diff, "+A=2") {
			t.Errorf("diff = %v", res.Diff)
		}
		if b, _ := os.ReadFile(path); string(b) != "A=1\n" {
			t.Errorf("dry-run wrote: %q", b)
		}
	})

	t.Run("anchored placement and the anchor sentinel", func(t *testing.T) {
		t.Parallel()
		path := writeFile(t, t.TempDir(), ".env", "A=1\nB=2\n")
		if _, err := envkit.Set(path, []dotenv.Pair{{Key: "MID", Value: "x"}}, envkit.EditOptions{Anchor: "A"}); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(path); string(b) != "A=1\nMID=x\nB=2\n" {
			t.Errorf("placement = %q", b)
		}
		_, err := envkit.Set(path, []dotenv.Pair{{Key: "X", Value: "1"}}, envkit.EditOptions{Anchor: "NOPE"})
		if !errorsIs(err, envkit.ErrAnchorNotFound) {
			t.Errorf("err = %v, want ErrAnchorNotFound", err)
		}
	})

	t.Run("unset then restore is byte-exact", func(t *testing.T) {
		t.Parallel()
		const original = "# doc\nA=1\nB=2\n"
		path := writeFile(t, t.TempDir(), ".env", original)

		if _, err := envkit.Unset(path, []string{"A"}, envkit.EditOptions{}); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(path); !strings.Contains(string(b), "# A=1") {
			t.Fatalf("not commented out: %q", b)
		}
		if _, err := envkit.Restore(path, []string{"A"}, envkit.EditOptions{}); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(path); string(b) != original {
			t.Errorf("round trip lossy: %q", b)
		}

		if _, err := envkit.Unset(path, []string{"NOPE"}, envkit.EditOptions{}); !errorsIs(err, envkit.ErrKeyNotFound) {
			t.Errorf("err = %v, want ErrKeyNotFound", err)
		}
	})

	t.Run("delete removes the line", func(t *testing.T) {
		t.Parallel()
		path := writeFile(t, t.TempDir(), ".env", "A=1\nB=2\n")
		if _, err := envkit.Unset(path, []string{"A"}, envkit.EditOptions{Delete: true}); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(path); string(b) != "B=2\n" {
			t.Errorf("got %q", b)
		}
	})

	t.Run("set creates a missing file at 0600", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), ".env")
		if _, err := envkit.Set(path, []dotenv.Pair{{Key: "A", Value: "1"}}, envkit.EditOptions{}); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v, want 0600", info.Mode().Perm())
		}
	})
}

func TestChildEnvAndRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeFile(t, dir, ".env", "HOST=h\nURL=${HOST}/api\nOVERRIDE=from-file\n")

	t.Run("ChildEnv overlays file-wins and expands", func(t *testing.T) {
		t.Parallel()
		env, err := envkit.ChildEnv(path, []string{"OVERRIDE=from-base", "KEEP=base"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(env)
		want := []string{"HOST=h", "KEEP=base", "OVERRIDE=from-file", "URL=h/api"}
		if !slices.Equal(env, want) {
			t.Errorf("env = %v, want %v", env, want)
		}
	})

	t.Run("nil base yields ONLY the file's values", func(t *testing.T) {
		t.Parallel()
		env, err := envkit.ChildEnv(path, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(env) != 3 {
			t.Errorf("env = %v, want exactly the file's three keys", env)
		}
	})

	t.Run("selection narrows what the child sees", func(t *testing.T) {
		t.Parallel()
		env, err := envkit.ChildEnv(path, nil, &envkit.SelectOptions{Keys: []string{"HOST"}})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(env, []string{"HOST=h"}) {
			t.Errorf("env = %v", env)
		}
	})

	t.Run("Run passes the child's exit code through", func(t *testing.T) {
		t.Parallel()
		var out strings.Builder
		code, err := envkit.Run(context.Background(), path, []string{"sh", "-c", "printf %s \"$URL\"; exit 3"}, envkit.RunOptions{
			Stdout: &out,
			Stderr: &out,
		})
		if err != nil {
			t.Fatalf("our error should be nil for a failing child: %v", err)
		}
		if code != 3 {
			t.Errorf("code = %d, want the child's 3", code)
		}
		if out.String() != "h/api" {
			t.Errorf("child saw %q, want the expanded value", out.String())
		}
	})

	t.Run("an unstartable command is OUR error", func(t *testing.T) {
		t.Parallel()
		if _, err := envkit.Run(context.Background(), path, []string{"definitely-not-a-binary-xyz"}, envkit.RunOptions{}); err == nil {
			t.Error("want an error")
		}
		if _, err := envkit.Run(context.Background(), path, nil, envkit.RunOptions{}); err == nil {
			t.Error("empty argv must error")
		}
	})
}

// errorsIs keeps the assertions readable without importing errors in every
// subtest.
func errorsIs(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func TestMatrixExplicitFilesAndSparseDiagnosis(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A realistic shape: two full files and one nearly-empty .env. The sizes
	// matter — the sparse diagnosis only fires when the filter hides ≤10% of
	// rows, which needs more than a handful of keys.
	var body strings.Builder
	for i := range 20 {
		fmt.Fprintf(&body, "KEY_%02d=v\n", i)
	}
	a := writeFile(t, dir, ".env.staging", body.String())
	b := writeFile(t, dir, ".env.production", body.String())
	sparse := writeFile(t, dir, ".env", "KEY_00=v\n")

	// Explicit files are labeled by the same classifier discovery uses, so
	// `.env.production` reads as "prod" like everywhere else.
	m, err := envkit.BuildMatrix(envkit.MatrixOptions{Files: []string{a, b}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.Envs, []string{"stag", "prod"}) {
		t.Errorf("envs = %v", m.Envs)
	}
	if !slices.Equal(m.Sources, []string{a, b}) {
		t.Errorf("sources = %v", m.Sources)
	}
	if envkit.EnvNameForPath("/x/.env.production") != "prod" {
		t.Error("EnvNameForPath should canonicalize")
	}
	if got := envkit.EnvNameForPath("/x/weird.conf"); got != "weird.conf" {
		t.Errorf("EnvNameForPath fallback = %q", got)
	}

	// A sparse column drags nearly every row into "drift", so the filter hides
	// (almost) nothing — SparseColumn names the culprit so a caller can say
	// why instead of showing an unchanged table.
	full, err := envkit.BuildMatrix(envkit.MatrixOptions{Files: []string{sparse, a, b}})
	if err != nil {
		t.Fatal(err)
	}
	rowsBefore := len(full.Rows)
	full.Rows = envkit.DriftRows(full.Rows)
	env, missing, ok := envkit.SparseColumn(full, rowsBefore)
	if !ok || env != "default" || missing == 0 {
		t.Errorf("SparseColumn = (%q, %d, %v), want the bare .env named", env, missing, ok)
	}

	// When the filter genuinely works (here: two identical files, so every
	// row is uniform and all of them are hidden), no column is blamed.
	m2, err := envkit.BuildMatrix(envkit.MatrixOptions{Files: []string{a, b}})
	if err != nil {
		t.Fatal(err)
	}
	rowsBefore2 := len(m2.Rows)
	m2.Rows = envkit.DriftRows(m2.Rows)
	if _, _, ok := envkit.SparseColumn(m2, rowsBefore2); ok {
		t.Error("no column should be blamed when the filter worked")
	}

	// Missing/unreadable inputs are errors, never silent empty columns.
	if _, err := envkit.BuildMatrix(envkit.MatrixOptions{Files: []string{a, filepath.Join(dir, "absent.env")}}); err == nil {
		t.Error("missing explicit file must error")
	}
	if _, err := envkit.BuildMatrix(envkit.MatrixOptions{Dir: filepath.Join(dir, "no-such-dir")}); err == nil {
		t.Error("missing dir must error")
	}
	if _, err := envkit.BuildMatrix(envkit.MatrixOptions{Files: []string{a, b}, ContractPath: filepath.Join(dir, "absent.env")}); err == nil {
		t.Error("missing contract must error")
	}
	if _, err := envkit.BuildMatrix(envkit.MatrixOptions{Files: []string{a, b}, PlaceholderPattern: "([bad"}); err == nil {
		t.Error("bad placeholder pattern must error")
	}
}

func TestSelectionProjections(t *testing.T) {
	t.Parallel()
	path := writeFile(t, t.TempDir(), ".env", "REAL=v\nPH=__YOU__\nEMPTY=\n")
	sel, err := envkit.SelectFile(path, envkit.SelectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sel.Names(), []string{"REAL"}) {
		t.Errorf("Names() = %v", sel.Names())
	}
	got := sel.SkippedNames()
	slices.Sort(got)
	if !slices.Equal(got, []string{"EMPTY", "PH"}) {
		t.Errorf("SkippedNames() = %v", got)
	}
}

func TestErrorArms(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Every read entry point rejects a missing file rather than treating it
	// as empty — an absent file is a wrong path, not a configuration answer.
	absent := filepath.Join(dir, "absent.env")
	if _, err := envkit.SelectFile(absent, envkit.SelectOptions{}); err == nil {
		t.Error("SelectFile must error on a missing file")
	}
	if _, err := envkit.Unset(absent, []string{"A"}, envkit.EditOptions{}); err == nil {
		t.Error("Unset must error on a missing file")
	}
	if _, err := envkit.Restore(absent, []string{"A"}, envkit.EditOptions{}); err == nil {
		t.Error("Restore must error on a missing file")
	}
	if _, err := envkit.Inventories(envkit.InventoryOptions{Dir: filepath.Join(dir, "nope")}); err == nil {
		t.Error("Inventories must error on a missing dir")
	}
	if _, err := envkit.Inventories(envkit.InventoryOptions{Dir: dir, PlaceholderPattern: "([bad"}); err == nil {
		t.Error("bad placeholder pattern must error")
	}
	if _, err := envkit.Discover(filepath.Join(dir, "nope")); err == nil {
		t.Error("Discover must error on a missing dir")
	}

	// Restore with nothing disabled, and anchored Set with no pairs.
	path := writeFile(t, dir, ".env", "A=1\n")
	if _, err := envkit.Restore(path, []string{"A"}, envkit.EditOptions{}); !errorsIs(err, envkit.ErrKeyNotFound) {
		t.Errorf("err = %v, want ErrKeyNotFound", err)
	}
	if _, err := envkit.Set(path, nil, envkit.EditOptions{Anchor: "A"}); err == nil {
		t.Error("anchored set with no pairs must error")
	}
	// AnchorBefore's sentinel path.
	if _, err := envkit.Set(path, []dotenv.Pair{{Key: "X", Value: "1"}}, envkit.EditOptions{Anchor: "NOPE", AnchorBefore: true}); !errorsIs(err, envkit.ErrAnchorNotFound) {
		t.Errorf("err = %v, want ErrAnchorNotFound", err)
	}
	// AnchorBefore success.
	if _, err := envkit.Set(path, []dotenv.Pair{{Key: "FIRST", Value: "1"}}, envkit.EditOptions{Anchor: "A", AnchorBefore: true}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "FIRST=1\nA=1\n" {
		t.Errorf("before-anchor placement = %q", b)
	}
}

func TestSmallSurfaces(t *testing.T) {
	t.Parallel()

	t.Run("HasContractGaps is false without gaps and without a contract", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, dir, ".env.dev", "A=1\n")
		writeFile(t, dir, ".env.prod", "A=1\n")
		contract := writeFile(t, dir, ".env.example", "A=x\n")

		noContract, err := envkit.BuildMatrix(envkit.MatrixOptions{Dir: dir})
		if err != nil {
			t.Fatal(err)
		}
		if noContract.HasContractGaps() {
			t.Error("no contract checked → no gaps")
		}
		satisfied, err := envkit.BuildMatrix(envkit.MatrixOptions{Dir: dir, ContractPath: contract})
		if err != nil {
			t.Fatal(err)
		}
		if satisfied.HasContractGaps() {
			t.Errorf("every env has A: %v", satisfied.ContractMissing)
		}
	})

	t.Run("cellFor reports inherited declarations", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, dir, ".env.dev", "HOME\nA=1\n")
		writeFile(t, dir, ".env.prod", "A=1\n")
		m, err := envkit.BuildMatrix(envkit.MatrixOptions{Dir: dir})
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range m.Rows {
			if row.Key != "HOME" {
				continue
			}
			for _, c := range row.Cells {
				want := envkit.StateInherited
				if c.Env == "prod" {
					want = envkit.StateMissing
				}
				if c.State != want {
					t.Errorf("HOME/%s = %s, want %s", c.Env, c.State, want)
				}
			}
		}
	})

	t.Run("DiffMaps handles empty inputs and identical maps", func(t *testing.T) {
		t.Parallel()
		d := envkit.DiffMaps(map[string]string{}, map[string]string{}, false)
		if !d.Empty() || d.Added == nil || d.Removed == nil || d.Changed == nil {
			t.Errorf("empty diff = %+v — slices must be empty, not nil", d)
		}
		same := envkit.DiffMaps(map[string]string{"A": "1"}, map[string]string{"A": "1"}, true)
		if !same.Empty() || !same.Expanded {
			t.Errorf("identical maps = %+v", same)
		}
	})

	t.Run("Run honors Dir and an explicit Stdin", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := writeFile(t, dir, ".env", "A=1\n")
		var out strings.Builder
		code, err := envkit.Run(context.Background(), path, []string{"sh", "-c", "pwd; cat"}, envkit.RunOptions{
			Dir:    dir,
			Stdin:  strings.NewReader("piped"),
			Stdout: &out,
			Stderr: &out,
		})
		if err != nil || code != 0 {
			t.Fatalf("code=%d err=%v", code, err)
		}
		if !strings.Contains(out.String(), "piped") {
			t.Errorf("explicit stdin not delivered: %q", out.String())
		}
	})

	t.Run("MergeEnv with an empty base is just the values", func(t *testing.T) {
		t.Parallel()
		got := envkit.MergeEnv(nil, map[string]string{"A": "1"})
		if !slices.Equal(got, []string{"A=1"}) {
			t.Errorf("MergeEnv = %v", got)
		}
		if got := envkit.MergeEnv([]string{"KEEP=1"}, nil); !slices.Equal(got, []string{"KEEP=1"}) {
			t.Errorf("MergeEnv with no values = %v", got)
		}
	})
}

func TestEditFailurePaths(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
	t.Parallel()

	t.Run("unreadable file", func(t *testing.T) {
		t.Parallel()
		path := writeFile(t, t.TempDir(), ".env", "A=1\n")
		if err := os.Chmod(path, 0o000); err != nil {
			t.Fatal(err)
		}
		if _, err := envkit.SelectFile(path, envkit.SelectOptions{}); err == nil {
			t.Error("unreadable file must error")
		}
		if _, err := envkit.Diff(path, path, envkit.DiffOptions{}); err == nil {
			t.Error("unreadable diff input must error")
		}
	})

	t.Run("save into a read-only directory leaves the file intact", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := writeFile(t, dir, ".env", "A=1\n")
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

		_, err := envkit.Set(path, []dotenv.Pair{{Key: "A", Value: "2"}}, envkit.EditOptions{})
		if err == nil {
			t.Error("save into a read-only dir must error")
		}
		_ = os.Chmod(dir, 0o700)
		if b, _ := os.ReadFile(path); string(b) != "A=1\n" {
			t.Errorf("failed save mutated the file: %q", b)
		}
	})
}

// TestDiscoverDefaultsToWorkingDirectory is separate and NOT parallel:
// t.Chdir cannot be used under a parallel parent.
func TestDiscoverDefaultsToWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".env", "A=1\n")
	t.Chdir(dir)
	found, err := envkit.Discover("")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Env != "default" {
		t.Errorf("Discover(\"\") = %+v", found)
	}
	inv, err := envkit.Inventories(envkit.InventoryOptions{})
	if err != nil || len(inv) != 1 {
		t.Errorf("Inventories default dir = %+v, %v", inv, err)
	}
	// MatrixOptions{} with no Dir and no Files also means "." — here that
	// is a single file, so it must be the two-column error, not a panic.
	if _, err := envkit.BuildMatrix(envkit.MatrixOptions{}); err == nil {
		t.Error("one discovered file must error")
	}

}

// TestRemainingArms covers the last reachable branches: sort comparators
// (which need 2+ entries per category), and error propagation from the
// helpers into each public entry point.
func TestRemainingArms(t *testing.T) {
	t.Parallel()

	t.Run("diff sorts every category", func(t *testing.T) {
		t.Parallel()
		d := envkit.DiffMaps(
			map[string]string{"Z_GONE": "1", "A_GONE": "1", "Z_CH": "old", "A_CH": "old"},
			map[string]string{"Z_NEW": "1", "A_NEW": "1", "Z_CH": "new", "A_CH": "new"},
			false,
		)
		if d.Added[0].Key != "A_NEW" || d.Added[1].Key != "Z_NEW" {
			t.Errorf("added not sorted: %+v", d.Added)
		}
		if d.Removed[0].Key != "A_GONE" || d.Removed[1].Key != "Z_GONE" {
			t.Errorf("removed not sorted: %+v", d.Removed)
		}
		if d.Changed[0].Key != "A_CH" || d.Changed[1].Key != "Z_CH" {
			t.Errorf("changed not sorted: %+v", d.Changed)
		}
	})

	t.Run("ChildEnv propagates selection failures", func(t *testing.T) {
		t.Parallel()
		if _, err := envkit.ChildEnv(filepath.Join(t.TempDir(), "absent.env"), nil, nil); err == nil {
			t.Error("missing file must error")
		}
		path := writeFile(t, t.TempDir(), ".env", "A=1\n")
		if _, err := envkit.ChildEnv(path, nil, &envkit.SelectOptions{Keys: []string{"NOPE"}}); err == nil {
			t.Error("named missing key must error")
		}
		// Run surfaces the same failure before spawning anything.
		if _, err := envkit.Run(context.Background(), path, []string{"true"}, envkit.RunOptions{
			Select: &envkit.SelectOptions{Keys: []string{"NOPE"}},
		}); err == nil {
			t.Error("Run must fail before exec when selection fails")
		}
	})

	t.Run("Set errors when the path is unopenable", func(t *testing.T) {
		t.Parallel()
		// A path whose parent is a FILE, not a directory: open fails.
		dir := t.TempDir()
		file := writeFile(t, dir, "notadir", "x")
		_, err := envkit.Set(filepath.Join(file, ".env"), []dotenv.Pair{{Key: "A", Value: "1"}}, envkit.EditOptions{})
		if err == nil {
			t.Error("unopenable path must error")
		}
	})

	t.Run("Inventories surfaces an unreadable member", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root ignores file modes")
		}
		t.Parallel()
		dir := t.TempDir()
		bad := writeFile(t, dir, ".env.prod", "A=1\n")
		if err := os.Chmod(bad, 0o000); err != nil {
			t.Fatal(err)
		}
		if _, err := envkit.Inventories(envkit.InventoryOptions{Dir: dir}); err == nil {
			t.Error("an unreadable env file must surface, not be skipped")
		}
	})
}
