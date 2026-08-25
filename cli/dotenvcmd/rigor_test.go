// Branch-level tests for paths the behavioral suites don't reach: error
// arms, human renderers, and the small helpers whose failure modes are
// invisible until they fire in production.
package dotenvcmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/ubgo/dotenv/cli/outfmt"
	"github.com/ubgo/dotenv/cli/providerkit"
)

func TestListExpand(t *testing.T) {
	t.Parallel()

	t.Run("resolves references", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, "HOST=db\nURL=${HOST}:5432\n")
		code, out, _ := runCLI(t, "-f", path, "list", "--expand")
		if code != ExitOK || !strings.Contains(out, "db:5432") {
			t.Errorf("exit %d out %q", code, out)
		}
	})

	t.Run("required-var error exits 1", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, "A=${NOPE:?set me}\n")
		code, out, _ := runCLI(t, "-f", path, "list", "--expand")
		if code != ExitFailure || !strings.Contains(out, "set me") {
			t.Errorf("exit %d out %q", code, out)
		}
	})
}

func TestGetExpandRequiredError(t *testing.T) {
	t.Parallel()
	path := writeFixture(t, "A=${NOPE:?fill in NOPE}\n")
	code, out, _ := runCLI(t, "-f", path, "get", "A", "--expand")
	if code != ExitFailure || !strings.Contains(out, "fill in NOPE") {
		t.Errorf("exit %d out %q", code, out)
	}
}

func TestDiffExpand(t *testing.T) {
	t.Parallel()

	t.Run("expanded values compare equal despite different raw text", func(t *testing.T) {
		t.Parallel()
		a := writeFixture(t, "HOST=db\nURL=${HOST}\n")
		b := writeFixture(t, "HOST=db\nURL=db\n")
		if code, _, _ := runCLI(t, "diff", a, b, "--expand"); code != ExitOK {
			t.Error("expanded-equal files must diff clean")
		}
		// Raw comparison sees the reference text and differs.
		if code, _, _ := runCLI(t, "diff", a, b); code != ExitFailure {
			t.Error("raw comparison must see the difference")
		}
	})

	t.Run("required-var error is trouble, not a difference", func(t *testing.T) {
		t.Parallel()
		a := writeFixture(t, "A=${NOPE:?msg}\n")
		b := writeFixture(t, "A=1\n")
		code, out, _ := runCLI(t, "diff", a, b, "--expand")
		if code != ExitFailure || !strings.Contains(out, "msg") {
			t.Errorf("exit %d out %q", code, out)
		}
	})
}

func TestDiffHTMLReveal(t *testing.T) {
	t.Parallel()
	a := writeFixture(t, "TOKEN=old-secret\n")
	b := writeFixture(t, "TOKEN=new-secret\n")
	out := filepath.Join(t.TempDir(), "diff.html")

	code, _, _ := runCLI(t, "diff", a, b, "--format", "html", "-o", out, "--reveal")
	if code != ExitFailure { // differences still signal via exit code
		t.Fatalf("exit = %d", code)
	}
	page := mustReadFile(t, out)
	if !strings.Contains(page, "old-secret") || !strings.Contains(page, "CONTAINS SECRETS") {
		t.Error("revealed diff page missing values or banner")
	}
}

func TestEnvsHumanTable(t *testing.T) {
	t.Parallel()
	dir := multiEnvDir(t, map[string]string{
		".env.prod":    "A=1\n# B=2\nHOME\nP=__YOU__\n",
		".env.example": "A=x\n",
	})
	code, out, _ := runCLI(t, "envs", "--dir", dir)
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"ENV", "PLACEHOLDERS", ".env.prod", "example (contract)"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}
	// prod row: 2 active keys, 1 disabled, 1 inherited, 1 placeholder.
	for _, row := range strings.Split(out, "\n") {
		if strings.Contains(row, ".env.prod") {
			fields := strings.Fields(row)
			if len(fields) != 6 || fields[2] != "2" || fields[3] != "1" || fields[4] != "1" || fields[5] != "1" {
				t.Errorf("prod counts row = %q", row)
			}
		}
	}
}

func TestMatrixContractHumanMessage(t *testing.T) {
	t.Parallel()
	dir := multiEnvDir(t, map[string]string{
		".env.dev":     "A=1\n",
		".env.prod":    "A=1\n",
		".env.example": "A=x\nMISSING_EVERYWHERE=x\n",
	})
	code, out, _ := runCLI(t, "matrix", "--dir", dir, "--contract", filepath.Join(dir, ".env.example"))
	if code != ExitFailure {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(out, "contract: dev is missing 1 key(s): [MISSING_EVERYWHERE]") {
		t.Errorf("human contract message missing:\n%s", out)
	}
}

func TestSetBeforeSuccess(t *testing.T) {
	t.Parallel()
	path := writeFixture(t, "A=1\nB=2\n")
	if code, _, _ := runCLI(t, "-f", path, "set", "NEW=x", "--before", "B"); code != ExitOK {
		t.Fatal("set --before failed")
	}
	if got := mustReadFile(t, path); got != "A=1\nNEW=x\nB=2\n" {
		t.Errorf("placement = %q", got)
	}
}

func TestKeysEmptyFile(t *testing.T) {
	t.Parallel()
	path := writeFixture(t, "# only a comment\n")
	code, out, _ := runCLI(t, "-f", path, "keys", "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	// The empty selection must be [], never null — the jq contract.
	if !strings.Contains(out, `"keys":[]`) {
		t.Errorf("json = %q, want empty array", out)
	}
}

func TestEmitFormattedOutputFile(t *testing.T) {
	t.Parallel()

	t.Run("-o into a missing directory fails cleanly", func(t *testing.T) {
		t.Parallel()
		dir := multiEnvDir(t, map[string]string{".env.dev": "A=1\n", ".env.prod": "A=1\n"})
		code, _, _ := runCLI(t, "matrix", "--dir", dir, "--format", "html", "-o", filepath.Join(dir, "absent", "x.html"))
		if code != ExitFailure {
			t.Errorf("exit = %d, want failure", code)
		}
	})

	t.Run("-o works for json too, envelope lands in the file", func(t *testing.T) {
		t.Parallel()
		dir := multiEnvDir(t, map[string]string{".env.dev": "A=1\n", ".env.prod": "A=1\n"})
		out := filepath.Join(t.TempDir(), "matrix.json")
		if code, _, _ := runCLI(t, "matrix", "--dir", dir, "--format", "json", "-o", out); code != ExitOK {
			t.Fatal("json -o failed")
		}
		if !strings.Contains(mustReadFile(t, out), `"ok":true`) {
			t.Error("envelope not written to the file")
		}
	})
}

func TestOpenUnreadableFile(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores modes")
	}
	path := writeFixture(t, "A=1\n")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	code, out, _ := runCLI(t, "-f", path, "get", "A")
	if code != ExitFailure || !strings.Contains(out, "dotenvctl:") {
		t.Errorf("exit %d out %q — unreadable file must fail loudly", code, out)
	}
}

func TestSaveIntoReadonlyDirFails(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores modes")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Save writes a temp sibling then renames; a read-only directory blocks it.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	code, _, _ := runCLI(t, "-f", path, "set", "A=2")
	if code != ExitFailure {
		t.Errorf("exit = %d, want failure", code)
	}
	_ = os.Chmod(dir, 0o700)
	if got := mustReadFile(t, path); got != "A=1\n" {
		t.Errorf("failed save mutated the file: %q", got)
	}
}

// errWriter fails every write — exercises the failure-while-reporting arms.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("pipe closed") }

func TestFailurePathsSurviveBrokenStdout(t *testing.T) {
	t.Parallel()
	path := writeFixture(t, "A=1\n")
	var stderr bytes.Buffer
	// stdout broken: the envelope can't be written, but the exit code must
	// still tell the truth.
	code := Execute([]string{"-f", path, "get", "NOPE"}, errWriter{}, &stderr)
	if code != ExitFailure {
		t.Errorf("exit = %d, want %d even with a broken stdout", code, ExitFailure)
	}
}

func TestVersionString(t *testing.T) {
	t.Parallel()
	if got := versionString(debug.ReadBuildInfo); got == "" {
		t.Error("versionString must never be empty — cobra renders it")
	}
	// The source-build fallback: no build info at all.
	if got := versionString(func() (*debug.BuildInfo, bool) { return nil, false }); got != "devel" {
		t.Errorf("fallback = %q, want devel", got)
	}
	// Build info present but unversioned (bare `go build`).
	if got := versionString(func() (*debug.BuildInfo, bool) { return &debug.BuildInfo{}, true }); got != "devel" {
		t.Errorf("unversioned = %q, want devel", got)
	}
}

func TestErrorStringers(t *testing.T) {
	t.Parallel()
	if got := (&exitError{code: 3}).Error(); got != "exit 3" {
		t.Errorf("bare exitError = %q", got)
	}
	if got := (&exitError{code: 1, err: errors.New("why")}).Error(); got != "why" {
		t.Errorf("wrapped exitError = %q", got)
	}
	if got := (&providerkit.CmdError{Code: 2}).Error(); got != "exit 2" {
		t.Errorf("bare CmdError = %q", got)
	}
	if got := (&providerkit.CmdError{Code: 1, Err: errors.New("boom")}).Error(); got != "boom" {
		t.Errorf("wrapped CmdError = %q", got)
	}
}

// TestAskOnTerminal covers the prompt writer's failure arm and the read-EOF
// deny default; the approve path is scripted in the confirm-matrix tests.
func TestAskOnTerminal(t *testing.T) {
	t.Parallel()
	// Broken prompt writer → deny without reading.
	a := &app{printer: &outfmt.Printer{Out: &bytes.Buffer{}}, errOut: errWriter{}}
	if a.askOnTerminal("proceed? ") {
		t.Error("broken prompt writer must deny")
	}
	// Healthy writer, but test stdin is not a terminal and yields EOF → deny.
	var errBuf bytes.Buffer
	b := &app{printer: &outfmt.Printer{Out: &bytes.Buffer{}}, errOut: &errBuf}
	if b.askOnTerminal("proceed? ") {
		t.Error("EOF stdin must deny")
	}
	if errBuf.String() != "proceed? " {
		t.Errorf("prompt = %q", errBuf.String())
	}
}

// TestStdinIsTerminal pins the isatty semantics: under `go test`, stdin is
// /dev/null — a CHARACTER DEVICE but not a terminal. The naive char-device
// check said true here (the CI misclassification bug); x/term must say false.
func TestStdinIsTerminal(t *testing.T) {
	t.Parallel()
	if stdinIsTerminal() {
		t.Error("test stdin (/dev/null) must NOT classify as interactive")
	}
}

// TestReadVerbSelection pins the core selection flags on list/keys: same
// grammar as store verbs, but guards LIFTED — reads must show placeholder
// values, not hide them.
func TestReadVerbSelection(t *testing.T) {
	t.Parallel()
	const content = "GITHUB_SECRET_PAT=tok\nGITHUB_SECRET_WHO=__YOU__\nAPP_X=1\nAPP_X=2\n"

	t.Run("list --prefix --strip-prefix shows placeholders", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, content)
		code, out, _ := runCLI(t, "-f", path, "list", "--prefix", "GITHUB_SECRET_", "--strip-prefix", "--json")
		if code != ExitOK {
			t.Fatalf("exit = %d (%s)", code, out)
		}
		for _, want := range []string{`{"key":"PAT","value":"tok"}`, `{"key":"WHO","value":"__YOU__"}`} {
			if !strings.Contains(out, want) {
				t.Errorf("json missing %s:\n%s", want, out)
			}
		}
		if strings.Contains(out, "APP_X") {
			t.Errorf("unselected key leaked:\n%s", out)
		}
	})

	t.Run("keys --exclude-prefix", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, content)
		code, out, _ := runCLI(t, "-f", path, "keys", "--exclude-prefix", "GITHUB_SECRET_")
		if code != ExitOK || out != "APP_X\n" {
			t.Errorf("exit %d out %q, want only APP_X", code, out)
		}
	})

	t.Run("bare list keeps the legacy duplicate-preserving view", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, content)
		code, out, _ := runCLI(t, "-f", path, "list", "--json")
		if code != ExitOK {
			t.Fatalf("exit = %d", code)
		}
		// Both APP_X occurrences visible without selection flags.
		if strings.Count(out, `"key":"APP_X"`) != 2 {
			t.Errorf("legacy view lost duplicates:\n%s", out)
		}
	})

	t.Run("list selection composes with --expand", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, "HOST=db\nGITHUB_SECRET_URL=${HOST}:5432\n")
		code, out, _ := runCLI(t, "-f", path, "list", "--prefix", "GITHUB_SECRET_", "--strip-prefix", "--expand", "--json")
		if code != ExitOK || !strings.Contains(out, `{"key":"URL","value":"db:5432"}`) {
			t.Errorf("exit %d out %q", code, out)
		}
	})

	t.Run("keys --include forces a key past the prefix filter", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, content)
		code, out, _ := runCLI(t, "-f", path, "keys", "--prefix", "GITHUB_SECRET_", "--include", "APP_X")
		if code != ExitOK || !strings.Contains(out, "APP_X") {
			t.Errorf("exit %d out %q", code, out)
		}
	})
}

// TestAskOnTerminal_Answers covers the prompt's decision table now that the
// answer source is injectable — this is the branch that decides whether a
// secret gets written, so every spelling is pinned.
func TestAskOnTerminal_Answers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  bool
	}{
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", true},
		{"YES\n", true},
		{"  y  \n", true},
		{"n\n", false},
		{"no\n", false},
		{"\n", false},
		{"yeah\n", false},
		{"", false}, // EOF without a newline: deny
	}
	for _, tt := range tests {
		t.Run(strings.TrimSpace(tt.input)+"/"+fmt.Sprint(tt.want), func(t *testing.T) {
			t.Parallel()
			var errBuf bytes.Buffer
			a := &app{
				printer: &outfmt.Printer{Out: &bytes.Buffer{}},
				errOut:  &errBuf,
				stdin:   strings.NewReader(tt.input),
			}
			if got := a.askOnTerminal("proceed? "); got != tt.want {
				t.Errorf("answer %q → %v, want %v", tt.input, got, tt.want)
			}
			if errBuf.String() != "proceed? " {
				t.Errorf("prompt = %q", errBuf.String())
			}
		})
	}
}

// TestSelectionErrorMapping pins how envkit failures become CLI exit codes —
// the three arms a user experiences as different messages.
func TestSelectionErrorMapping(t *testing.T) {
	t.Parallel()

	// Named-but-missing key → not_found.
	path := writeFixture(t, "A=1\n")
	code, out, _ := runCLI(t, "-f", path, "list", "--include", "NOPE", "--json")
	if code != ExitFailure || !strings.Contains(out, `"code":"not_found"`) {
		t.Errorf("exit %d out %q", code, out)
	}

	// Unsatisfied required reference → required.
	path = writeFixture(t, "A=${MISSING:?fill me}\n")
	code, out, _ = runCLI(t, "-f", path, "list", "--prefix", "A", "--expand", "--json")
	if code != ExitFailure || !strings.Contains(out, `"code":"required"`) {
		t.Errorf("exit %d out %q", code, out)
	}
}

// TestSelectionErrorMapping_IOArm covers the default (IO) arm of the
// envkit-error mapping: a missing file is neither not_found nor required.
func TestSelectionErrorMapping_IOArm(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "absent.env")
	code, out, _ := runCLI(t, "-f", missing, "list", "--prefix", "A", "--json")
	if code != ExitFailure || !strings.Contains(out, `"code":"io"`) {
		t.Errorf("exit %d out %q, want an io-coded failure", code, out)
	}
}
