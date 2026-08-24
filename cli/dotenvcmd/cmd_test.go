package dotenvcmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFixture creates a .env fixture in a fresh temp dir and returns its
// path. Each call gets its own dir so parallel tests cannot collide.
func writeFixture(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// runCLI drives the full CLI in-process — flag parsing to output — exactly as
// main would, capturing both streams.
func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code = Execute(args, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

// mustReadFile returns the file's current bytes for byte-preservation checks.
func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// fixture exercises every syntax feature the CLI must preserve: comments,
// blanks, export, quoting, a disabled pair, an inherited declaration, and a
// reference.
const fixture = "# database section\nDB_HOST=localhost\nexport DB_PORT=5432 # tcp\n\n# DB_USER=admin\nSECRET='literal ${X}'\nURL=${DB_HOST}:${DB_PORT}\nHOME\n"

func TestGet(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
	}{
		{"raw value", []string{"get", "DB_HOST"}, ExitOK, "localhost\n"},
		{"reference stays raw without --expand", []string{"get", "URL"}, ExitOK, "${DB_HOST}:${DB_PORT}\n"},
		{"expanded", []string{"get", "URL", "--expand"}, ExitOK, "localhost:5432\n"},
		{"single quotes suppress expansion", []string{"get", "SECRET", "--expand"}, ExitOK, "literal ${X}\n"},
		{"missing key fails", []string{"get", "NOPE"}, ExitFailure, "dotenvctl: key \"NOPE\" not found"},
		{"disabled key is not active", []string{"get", "DB_USER"}, ExitFailure, "not found"},
		{"inherited name has no value", []string{"get", "HOME"}, ExitFailure, "not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := writeFixture(t, fixture)
			code, out, _ := runCLI(t, append([]string{"-f", path}, tt.args[0:]...)...)
			if code != tt.wantCode {
				t.Fatalf("exit = %d, want %d (out %q)", code, tt.wantCode, out)
			}
			if !strings.Contains(out, tt.wantOut) {
				t.Errorf("out = %q, want containing %q", out, tt.wantOut)
			}
		})
	}
}

func TestGet_JSONEnvelope(t *testing.T) {
	t.Parallel()
	path := writeFixture(t, fixture)

	code, out, _ := runCLI(t, "-f", path, "get", "DB_HOST", "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if want := `{"ok":true,"data":{"key":"DB_HOST","value":"localhost","expanded":false}}` + "\n"; out != want {
		t.Errorf("json = %q, want %q", out, want)
	}

	code, out, _ = runCLI(t, "-f", path, "get", "NOPE", "--json")
	if code != ExitFailure {
		t.Fatalf("exit = %d, want %d", code, ExitFailure)
	}
	var env struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid envelope %q: %v", out, err)
	}
	if env.OK || env.Error.Code != "not_found" {
		t.Errorf("envelope = %q, want ok:false code:not_found", out)
	}
}

func TestSet(t *testing.T) {
	t.Parallel()

	t.Run("updates only the target line", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, fixture)
		code, _, _ := runCLI(t, "-f", path, "set", "DB_HOST=db.prod")
		if code != ExitOK {
			t.Fatalf("exit = %d", code)
		}
		got := mustReadFile(t, path)
		want := strings.Replace(fixture, "DB_HOST=localhost", "DB_HOST=db.prod", 1)
		if got != want {
			t.Errorf("file drifted beyond the one line:\n got: %q\nwant: %q", got, want)
		}
	})

	t.Run("same value is a byte and mtime no-op", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, fixture)
		beforeInfo, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		code, _, _ := runCLI(t, "-f", path, "set", "DB_HOST=localhost")
		if code != ExitOK {
			t.Fatalf("exit = %d", code)
		}
		if got := mustReadFile(t, path); got != fixture {
			t.Errorf("no-op set changed bytes: %q", got)
		}
		afterInfo, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
			t.Error("no-op set rewrote the file (mtime changed)")
		}
	})

	t.Run("anchored placement", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, "A=1\nB=2\n")
		if code, _, _ := runCLI(t, "-f", path, "set", "MID=x", "--after", "A"); code != ExitOK {
			t.Fatal("set --after failed")
		}
		if got := mustReadFile(t, path); got != "A=1\nMID=x\nB=2\n" {
			t.Errorf("placement wrong: %q", got)
		}
	})

	t.Run("missing anchor fails without writing", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, "A=1\n")
		code, _, _ := runCLI(t, "-f", path, "set", "X=1", "--before", "NOPE")
		if code != ExitFailure {
			t.Fatalf("exit = %d, want %d", code, ExitFailure)
		}
		if got := mustReadFile(t, path); got != "A=1\n" {
			t.Errorf("failed set wrote the file: %q", got)
		}
	})

	t.Run("after and before together is usage", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, "A=1\n")
		if code, _, _ := runCLI(t, "-f", path, "set", "X=1", "--after", "A", "--before", "A"); code != ExitFailure {
			t.Errorf("exit = %d, want failure", code)
		}
	})

	t.Run("bad assignment is usage", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, "")
		if code, _, _ := runCLI(t, "-f", path, "set", "NOEQUALS"); code != ExitFailure {
			t.Errorf("exit = %d, want failure", code)
		}
	})

	t.Run("value may contain equals", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, "")
		if code, _, _ := runCLI(t, "-f", path, "set", "CONN=a=b=c"); code != ExitOK {
			t.Fatal("set failed")
		}
		code, out, _ := runCLI(t, "-f", path, "get", "CONN")
		if code != ExitOK || out != "a=b=c\n" {
			t.Errorf("get = %q (exit %d)", out, code)
		}
	})

	t.Run("dry-run previews and writes nothing", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, "A=1\n")
		code, out, _ := runCLI(t, "-f", path, "set", "A=2", "--dry-run")
		if code != ExitOK {
			t.Fatalf("exit = %d", code)
		}
		if !strings.Contains(out, "-A=1") || !strings.Contains(out, "+A=2") {
			t.Errorf("dry-run diff missing: %q", out)
		}
		if got := mustReadFile(t, path); got != "A=1\n" {
			t.Errorf("dry-run wrote the file: %q", got)
		}
	})

	t.Run("bootstraps a missing file at 0600", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), ".env")
		if code, _, _ := runCLI(t, "-f", path, "set", "A=1"); code != ExitOK {
			t.Fatal("set failed")
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

func TestUnsetRestore(t *testing.T) {
	t.Parallel()

	t.Run("unset comments out, restore reverses to original bytes", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, fixture)
		if code, _, _ := runCLI(t, "-f", path, "unset", "DB_HOST"); code != ExitOK {
			t.Fatal("unset failed")
		}
		if got := mustReadFile(t, path); !strings.Contains(got, "# DB_HOST=localhost") {
			t.Fatalf("not commented out: %q", got)
		}
		if code, _, _ := runCLI(t, "-f", path, "restore", "DB_HOST"); code != ExitOK {
			t.Fatal("restore failed")
		}
		if got := mustReadFile(t, path); got != fixture {
			t.Errorf("unset+restore was lossy:\n got: %q\nwant: %q", got, fixture)
		}
	})

	t.Run("unset --delete removes the line", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, "A=1\nB=2\n")
		if code, _, _ := runCLI(t, "-f", path, "unset", "A", "--delete"); code != ExitOK {
			t.Fatal("unset --delete failed")
		}
		if got := mustReadFile(t, path); got != "B=2\n" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("unset --dry-run writes nothing", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, "A=1\n")
		code, out, _ := runCLI(t, "-f", path, "unset", "A", "--dry-run")
		if code != ExitOK || !strings.Contains(out, "+# A=1") {
			t.Fatalf("exit %d out %q", code, out)
		}
		if got := mustReadFile(t, path); got != "A=1\n" {
			t.Errorf("dry-run wrote: %q", got)
		}
	})

	t.Run("unset missing key fails", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, "A=1\n")
		if code, _, _ := runCLI(t, "-f", path, "unset", "NOPE"); code != ExitFailure {
			t.Error("want failure")
		}
	})

	t.Run("restore with nothing disabled fails", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, "A=1\n")
		if code, _, _ := runCLI(t, "-f", path, "restore", "A"); code != ExitFailure {
			t.Error("want failure")
		}
	})
}

func TestListKeys(t *testing.T) {
	t.Parallel()

	t.Run("keys newline output", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, fixture)
		code, out, _ := runCLI(t, "-f", path, "keys")
		if code != ExitOK {
			t.Fatalf("exit = %d", code)
		}
		if out != "DB_HOST\nDB_PORT\nSECRET\nURL\n" {
			t.Errorf("keys = %q", out)
		}
	})

	t.Run("list json always carries all sections", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, fixture)
		code, out, _ := runCLI(t, "-f", path, "list", "--json")
		if code != ExitOK {
			t.Fatalf("exit = %d", code)
		}
		var env struct {
			OK   bool `json:"ok"`
			Data struct {
				Pairs     []map[string]string `json:"pairs"`
				Disabled  []map[string]string `json:"disabled"`
				Inherited []string            `json:"inherited"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(out), &env); err != nil {
			t.Fatalf("invalid json %q: %v", out, err)
		}
		if len(env.Data.Pairs) != 4 || len(env.Data.Disabled) != 1 || len(env.Data.Inherited) != 1 {
			t.Errorf("sections = %d/%d/%d, want 4/1/1 (%q)", len(env.Data.Pairs), len(env.Data.Disabled), len(env.Data.Inherited), out)
		}
	})

	t.Run("list human shows sections only when asked", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, fixture)
		_, plain, _ := runCLI(t, "-f", path, "list")
		if strings.Contains(plain, "inherited") {
			t.Errorf("unrequested section leaked: %q", plain)
		}
		_, full, _ := runCLI(t, "-f", path, "list", "--disabled", "--inherited")
		if !strings.Contains(full, "# DB_USER=admin") || !strings.Contains(full, "HOME") {
			t.Errorf("sections missing: %q", full)
		}
	})
}

func TestDiff(t *testing.T) {
	t.Parallel()

	t.Run("identical files exit zero", func(t *testing.T) {
		t.Parallel()
		a := writeFixture(t, "A=1\n")
		b := writeFixture(t, "# different formatting, same config\nA=1\n")
		code, out, _ := runCLI(t, "diff", a, b)
		if code != ExitOK {
			t.Fatalf("exit = %d out %q — diff must compare config, not bytes", code, out)
		}
	})

	t.Run("differences exit one with sorted sections", func(t *testing.T) {
		t.Parallel()
		a := writeFixture(t, "A=1\nB=2\nC=3\n")
		b := writeFixture(t, "B=2\nC=changed\nD=4\n")
		code, out, _ := runCLI(t, "diff", a, b)
		if code != ExitFailure {
			t.Fatalf("exit = %d, want %d", code, ExitFailure)
		}
		want := "+ D=4\n- A=1\n~ C: 3 -> changed\n"
		if out != want {
			t.Errorf("diff = %q, want %q", out, want)
		}
	})

	t.Run("missing file is an error not an empty side", func(t *testing.T) {
		t.Parallel()
		a := writeFixture(t, "A=1\n")
		code, _, _ := runCLI(t, "diff", a, filepath.Join(t.TempDir(), "absent.env"))
		if code != ExitFailure {
			t.Errorf("exit = %d, want failure", code)
		}
	})
}

// TestRun is deliberately NOT parallel: the first subtest uses t.Setenv, which
// panics under a parallel ancestor.
func TestRun(t *testing.T) {
	t.Run("file values reach the child and win over inherited env", func(t *testing.T) {
		path := writeFixture(t, "RUN_PROBE=from-file\n")
		t.Setenv("RUN_PROBE", "from-parent")
		var out bytes.Buffer
		code := Execute([]string{"-f", path, "run", "--", "sh", "-c", "printf %s \"$RUN_PROBE\""}, &out, &out)
		if code != ExitOK {
			t.Fatalf("exit = %d (%s)", code, out.String())
		}
		if got := out.String(); got != "from-file" {
			t.Errorf("child saw %q, want the file value to win", got)
		}
	})

	t.Run("child exit code passes through verbatim", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, "")
		code, _, _ := runCLI(t, "-f", path, "run", "--", "sh", "-c", "exit 42")
		if code != 42 {
			t.Errorf("exit = %d, want 42", code)
		}
	})

	t.Run("unstartable command fails with our code", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, "")
		code, _, _ := runCLI(t, "-f", path, "run", "--", "definitely-not-a-command-xyz")
		if code != ExitFailure {
			t.Errorf("exit = %d, want %d", code, ExitFailure)
		}
	})

	t.Run("required-var error stops before exec", func(t *testing.T) {
		t.Parallel()
		path := writeFixture(t, "A=${MISSING:?set me}\n")
		code, _, _ := runCLI(t, "-f", path, "run", "--", "sh", "-c", "true")
		if code != ExitFailure {
			t.Errorf("exit = %d, want %d", code, ExitFailure)
		}
	})
}

func TestUsageErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
	}{
		{"unknown flag", []string{"get", "K", "--bogus"}},
		{"unknown verb", []string{"frobnicate"}},
		{"get without key", []string{"get"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			code, _, stderr := runCLI(t, tt.args...)
			if code != ExitUsage {
				t.Errorf("exit = %d, want %d", code, ExitUsage)
			}
			// A usage error must never be silent — a bare exit 2 reads like
			// "the command ran and found nothing" (the --env regression).
			if !strings.Contains(stderr, "dotenvctl:") {
				t.Errorf("usage error printed nothing to stderr: %q", stderr)
			}
		})
	}
}
