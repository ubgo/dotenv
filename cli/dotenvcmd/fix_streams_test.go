package dotenvcmd

import (
	"strings"
	"testing"
)

// TestExecute_HumanFailureGoesToStderr pins bug 11: a human-mode failure must
// land on stderr, so `v=$(dotenvctl get NOPE 2>/dev/null)` captures nothing
// and a script never mistakes the error text for a value. Every verb that
// fails through the Printer is covered by the same seam; these rows walk the
// verbs that report not-found, usage and IO failures.
func TestExecute_HumanFailureGoesToStderr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"get missing key", []string{"get", "NOPE"}, `dotenvctl: key "NOPE" not found`},
		{"unset missing key", []string{"unset", "NOPE"}, "dotenvctl: "},
		{"restore missing key", []string{"restore", "NOPE"}, "dotenvctl: "},
		{"set bad pair", []string{"set", "NOEQUALS"}, "dotenvctl: "},
		{"diff missing file", []string{"diff", "/nonexistent/a.env"}, "dotenvctl: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := writeFixture(t, fixture)
			code, out, errOut := runCLI(t, append([]string{"-f", path}, tt.args...)...)
			if code == ExitOK {
				t.Fatalf("exit = %d, want a failure", code)
			}
			if out != "" {
				t.Errorf("stdout = %q, want empty: failures belong on stderr", out)
			}
			if !strings.Contains(errOut, tt.wantErr) {
				t.Errorf("stderr = %q, want containing %q", errOut, tt.wantErr)
			}
		})
	}
}

// TestExecute_JSONFailureStaysOnStdout pins the other half of bug 11's fix:
// under --json the failure envelope IS the output, so it stays on stdout and
// stderr stays empty.
func TestExecute_JSONFailureStaysOnStdout(t *testing.T) {
	t.Parallel()
	path := writeFixture(t, fixture)
	code, out, errOut := runCLI(t, "-f", path, "--json", "get", "NOPE")
	if code != ExitFailure {
		t.Fatalf("exit = %d, want %d", code, ExitFailure)
	}
	if !strings.HasPrefix(out, `{"ok":false`) {
		t.Errorf("stdout = %q, want the failure envelope", out)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want empty under --json", errOut)
	}
}
