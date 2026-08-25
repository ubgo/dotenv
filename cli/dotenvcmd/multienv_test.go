package dotenvcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/dotenv/cli/envkit"
)

// multiEnvDir builds a directory with a family of env files and returns it.
func multiEnvDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// theFamily is the canonical fixture: drift across dev/stag/prod plus a
// contract, exercising every cell state.
var theFamily = map[string]string{
	".env.dev":     "DB_HOST=localhost\nDB_PASS=devpass\nFEATURE_X=on\n",
	".env.staging": "DB_HOST=stag.db\nDB_PASS=__YOU__\n# FEATURE_X=off\nEXTRA=only-stag\n",
	".env.prod":    "DB_HOST=prod.db\nDB_PASS=prodpass\nFEATURE_X=\nHOME\n",
	".env.example": "DB_HOST=example\nDB_PASS=example\nAPI_KEY=example\n",
	".env.bak":     "JUNK=1\n",
}

func TestEnvs(t *testing.T) {
	t.Parallel()
	dir := multiEnvDir(t, theFamily)

	code, out, _ := runCLI(t, "envs", "--dir", dir, "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d (%s)", code, out)
	}
	var env struct {
		Data envsPayload `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("bad json %q: %v", out, err)
	}

	// .bak excluded; order dev, stag, prod, contract-last.
	if len(env.Data.Files) != 4 {
		t.Fatalf("files = %+v, want 4", env.Data.Files)
	}
	wantEnvs := []string{"dev", "stag", "prod", "example"}
	for i, w := range wantEnvs {
		if env.Data.Files[i].Env != w {
			t.Errorf("file %d env = %q, want %q", i, env.Data.Files[i].Env, w)
		}
	}
	stag := env.Data.Files[1]
	if stag.Keys != 3 || stag.Disabled != 1 || stag.Placeholders != 1 {
		t.Errorf("stag counts = %+v, want keys 3 disabled 1 placeholders 1", stag)
	}
	prod := env.Data.Files[2]
	if prod.Inherited != 1 {
		t.Errorf("prod inherited = %d, want 1", prod.Inherited)
	}
	if !env.Data.Files[3].Contract {
		t.Error("example not marked contract")
	}

	// Empty dir: exit 0, empty list, human message.
	code, out, _ = runCLI(t, "envs", "--dir", t.TempDir())
	if code != ExitOK || !strings.Contains(out, "no env files found") {
		t.Errorf("empty dir: exit %d out %q", code, out)
	}
}

func TestMatrix_StatesAndOrder(t *testing.T) {
	t.Parallel()
	dir := multiEnvDir(t, theFamily)

	code, out, _ := runCLI(t, "matrix", "--dir", dir, "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d (%s)", code, out)
	}
	var env struct {
		Data envkit.Matrix `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("bad json: %v", err)
	}

	// Contract excluded from columns; env order dev→stag→prod.
	wantEnvs := []string{"dev", "stag", "prod"}
	if len(env.Data.Envs) != 3 {
		t.Fatalf("envs = %v", env.Data.Envs)
	}
	for i, w := range wantEnvs {
		if env.Data.Envs[i] != w {
			t.Errorf("env %d = %q, want %q", i, env.Data.Envs[i], w)
		}
	}

	states := map[string]map[string]envkit.CellState{}
	for _, row := range env.Data.Rows {
		states[row.Key] = map[string]envkit.CellState{}
		for _, c := range row.Cells {
			states[row.Key][c.Env] = c.State
			if c.Value != "" {
				t.Errorf("default json leaked a value: %s/%s = %q", row.Key, c.Env, c.Value)
			}
		}
	}

	wantStates := map[string]map[string]envkit.CellState{
		"DB_HOST":   {"dev": StatePresent, "stag": StatePresent, "prod": StatePresent},
		"DB_PASS":   {"dev": StatePresent, "stag": StatePlaceholder, "prod": StatePresent},
		"FEATURE_X": {"dev": StatePresent, "stag": StateDisabled, "prod": StateEmpty},
		"EXTRA":     {"dev": StateMissing, "stag": StatePresent, "prod": StateMissing},
		"HOME":      {"dev": StateMissing, "stag": StateMissing, "prod": StateInherited},
	}
	for key, envs := range wantStates {
		for e, want := range envs {
			if got := states[key][e]; got != want {
				t.Errorf("%s/%s = %s, want %s", key, e, got, want)
			}
		}
	}
}

func TestMatrix_ValuesMaskingAndReveal(t *testing.T) {
	t.Parallel()
	dir := multiEnvDir(t, theFamily)

	_, masked, _ := runCLI(t, "matrix", "--dir", dir, "--values")
	if strings.Contains(masked, "devpass") || strings.Contains(masked, "prod.db") {
		t.Errorf("masked --values leaked a value:\n%s", masked)
	}
	if !strings.Contains(masked, "••••••") {
		t.Errorf("mask glyph missing:\n%s", masked)
	}

	_, revealed, _ := runCLI(t, "matrix", "--dir", dir, "--values", "--reveal")
	if !strings.Contains(revealed, "devpass") {
		t.Errorf("--reveal did not show values:\n%s", revealed)
	}
	if !strings.Contains(revealed, "treat this output as a secret") {
		t.Error("--reveal missing its warning")
	}

	// JSON carries values only under --reveal.
	_, out, _ := runCLI(t, "matrix", "--dir", dir, "--json", "--reveal")
	if !strings.Contains(out, "devpass") {
		t.Error("revealed json missing values")
	}
}

func TestMatrix_OnlyDrift(t *testing.T) {
	t.Parallel()
	dir := multiEnvDir(t, theFamily)

	_, out, _ := runCLI(t, "matrix", "--dir", dir, "--only-drift")
	if strings.Contains(out, "DB_HOST") {
		t.Errorf("uniformly-present row survived --only-drift:\n%s", out)
	}
	if !strings.Contains(out, "DB_PASS") {
		t.Errorf("drifting row missing:\n%s", out)
	}
}

func TestMatrix_Contract(t *testing.T) {
	t.Parallel()
	dir := multiEnvDir(t, theFamily)

	code, out, _ := runCLI(t, "matrix", "--dir", dir, "--contract", filepath.Join(dir, ".env.example"), "--json")
	if code != ExitFailure {
		t.Fatalf("exit = %d, want %d — API_KEY is missing everywhere", code, ExitFailure)
	}
	var env struct {
		Data envkit.Matrix `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	for _, e := range []string{"dev", "stag", "prod"} {
		found := false
		for _, k := range env.Data.ContractMissing[e] {
			if k == "API_KEY" {
				found = true
			}
		}
		if !found {
			t.Errorf("contract_missing[%s] = %v, want API_KEY", e, env.Data.ContractMissing[e])
		}
	}
	// The synthesized all-missing row must be visible in the table too.
	found := false
	for _, row := range env.Data.Rows {
		if row.Key == "API_KEY" {
			found = true
		}
	}
	if !found {
		t.Error("API_KEY row not synthesized into the matrix")
	}
}

func TestMatrix_ExplicitFilesAndUsage(t *testing.T) {
	t.Parallel()
	dir := multiEnvDir(t, theFamily)

	code, out, _ := runCLI(t, "matrix", filepath.Join(dir, ".env.staging"), filepath.Join(dir, ".env.prod"), "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d (%s)", code, out)
	}
	if !strings.Contains(out, `"envs":["stag","prod"]`) {
		t.Errorf("explicit files misclassified: %s", out)
	}

	// One column is a usage failure, not a degenerate table.
	if code, _, _ := runCLI(t, "matrix", filepath.Join(dir, ".env.prod")); code != ExitFailure {
		t.Errorf("single-file matrix: exit %d, want failure", code)
	}
	// A missing explicit file is an error, not an all-missing column.
	if code, _, _ := runCLI(t, "matrix", filepath.Join(dir, ".env.prod"), filepath.Join(dir, "absent.env")); code != ExitFailure {
		t.Errorf("missing file: exit %d, want failure", code)
	}
}

func TestHTMLReports(t *testing.T) {
	t.Parallel()
	dir := multiEnvDir(t, theFamily)

	t.Run("matrix html masked has no secret bytes", func(t *testing.T) {
		t.Parallel()
		out := filepath.Join(t.TempDir(), "matrix.html")
		code, _, _ := runCLI(t, "matrix", "--dir", dir, "--format", "html", "-o", out)
		if code != ExitOK {
			t.Fatalf("exit = %d", code)
		}
		page := mustReadFile(t, out)
		for _, secret := range []string{"devpass", "prodpass", "stag.db", "only-stag"} {
			if strings.Contains(page, secret) {
				t.Errorf("masked html contains secret %q", secret)
			}
		}
		for _, want := range []string{"DB_PASS", "values masked", "<!doctype html>"} {
			if !strings.Contains(page, want) {
				t.Errorf("html missing %q", want)
			}
		}
		info, err := os.Stat(out)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("report mode = %v, want 0600", info.Mode().Perm())
		}
	})

	t.Run("matrix html revealed carries values and banner", func(t *testing.T) {
		t.Parallel()
		out := filepath.Join(t.TempDir(), "matrix.html")
		if code, _, _ := runCLI(t, "matrix", "--dir", dir, "--format", "html", "-o", out, "--reveal"); code != ExitOK {
			t.Fatal("reveal html failed")
		}
		page := mustReadFile(t, out)
		if !strings.Contains(page, "devpass") || !strings.Contains(page, "CONTAINS SECRETS") {
			t.Error("revealed html missing values or banner")
		}
	})

	t.Run("diff html masked, exit still signals difference", func(t *testing.T) {
		t.Parallel()
		out := filepath.Join(t.TempDir(), "diff.html")
		code, _, _ := runCLI(t, "diff", filepath.Join(dir, ".env.staging"), filepath.Join(dir, ".env.prod"), "--format", "html", "-o", out)
		if code != ExitFailure {
			t.Fatalf("exit = %d, want 1 (differences)", code)
		}
		page := mustReadFile(t, out)
		if strings.Contains(page, "prodpass") || strings.Contains(page, "stag.db") {
			t.Error("masked diff html leaked values")
		}
		if !strings.Contains(page, "DB_HOST") || !strings.Contains(page, "••••••") {
			t.Error("diff html missing keys or mask glyphs")
		}
	})

	t.Run("bad format is usage", func(t *testing.T) {
		t.Parallel()
		if code, _, _ := runCLI(t, "matrix", "--dir", dir, "--format", "yaml"); code != ExitFailure {
			t.Error("want failure on unknown format")
		}
	})
}

func TestMatrix_OnlyDriftSparseColumnHint(t *testing.T) {
	t.Parallel()
	// A tiny default .env beside two full files: --only-drift can hide
	// nothing, and the hint must name the sparse column.
	dir := multiEnvDir(t, map[string]string{
		".env":         "ONLY_HERE=1\n",
		".env.staging": "A=1\nB=2\nC=3\nD=4\n",
		".env.prod":    "A=1\nB=2\nC=3\nD=4\n",
	})

	_, out, _ := runCLI(t, "matrix", "--dir", dir, "--only-drift")
	if !strings.Contains(out, "hint:") || !strings.Contains(out, `"default"`) {
		t.Errorf("sparse-column hint missing or not naming the culprit:\n%s", out)
	}

	// When the filter genuinely works (no sparse column), no hint appears.
	dirOK := multiEnvDir(t, map[string]string{
		".env.staging": "A=1\nB=2\nC=only-stag\n",
		".env.prod":    "A=1\nB=2\nC=different\n",
	})
	_, out, _ = runCLI(t, "matrix", "--dir", dirOK, "--only-drift")
	if strings.Contains(out, "hint:") {
		t.Errorf("hint fired on an effective filter:\n%s", out)
	}
}

func TestMatrix_DuplicateEnvClassification(t *testing.T) {
	t.Parallel()
	// .env.stag and .env.staging both canonicalize to "stag". Before the
	// disambiguation fix, the env-keyed file map dropped one file and rendered
	// the other's data under BOTH columns — silent misdata, the worst kind.
	dir := multiEnvDir(t, map[string]string{
		".env.stag":    "A=from-stag\n",
		".env.staging": "A=from-staging\nB=extra\n",
	})

	code, out, _ := runCLI(t, "matrix", "--dir", dir, "--values", "--reveal")
	if code != ExitOK {
		t.Fatalf("exit = %d (%s)", code, out)
	}
	if !strings.Contains(out, ".env.stag") || !strings.Contains(out, ".env.staging") {
		t.Errorf("collided columns not relabeled by filename:\n%s", out)
	}
	if !strings.Contains(out, "from-stag") || !strings.Contains(out, "from-staging") {
		t.Errorf("one file's data vanished:\n%s", out)
	}

	// Explicit args whose BASENAMES also collide (two dirs' .env) fall back
	// to full paths as labels.
	dirA := multiEnvDir(t, map[string]string{".env": "X=a\n"})
	dirB := multiEnvDir(t, map[string]string{".env": "X=b\n"})
	code, out, _ = runCLI(t, "matrix", filepath.Join(dirA, ".env"), filepath.Join(dirB, ".env"), "--values", "--reveal")
	if code != ExitOK {
		t.Fatalf("exit = %d (%s)", code, out)
	}
	if !strings.Contains(out, dirA) || !strings.Contains(out, dirB) {
		t.Errorf("basename collision not relabeled by path:\n%s", out)
	}
}
