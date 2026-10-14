package dotenvcmd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/dotenv/cli/dotenvcmd"
)

// TestExecute_ProgramName pins bug 16 for embedders: a host that mounts the
// CLI under its own name must see that name, not "dotenvctl", in every line
// the CLI prints about itself — operation failures, usage failures and their
// "--help" hint, the help Usage line and --version. Without the option the
// default stays "dotenvctl", which the standalone binary relies on.
func TestExecute_ProgramName(t *testing.T) {
	t.Parallel()
	envFile := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envFile, []byte("HOST=localhost\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		args     []string
		opts     []dotenvcmd.ExecuteOption
		wantCode int
		// stream is where the name must show: "out" or "err".
		stream string
		want   []string
		// renamed: no "dotenvctl" may appear at all in the stream checked.
		renamed bool
	}{
		{"failure prefix", []string{"-f", envFile, "get", "MISSING"}, []dotenvcmd.ExecuteOption{dotenvcmd.WithProgramName("myapp env")}, dotenvcmd.ExitFailure, "err", []string{`myapp env: key "MISSING" not found`}, true},
		{"usage failure and hint", []string{"nosuchverb"}, []dotenvcmd.ExecuteOption{dotenvcmd.WithProgramName("myapp env")}, dotenvcmd.ExitUsage, "err", []string{"myapp env: unknown command", "Run 'myapp env --help' for usage."}, true},
		{"help usage line", []string{"get", "--help"}, []dotenvcmd.ExecuteOption{dotenvcmd.WithProgramName("myapp env")}, dotenvcmd.ExitOK, "out", []string{"myapp env get KEY"}, false},
		{"version line", []string{"--version"}, []dotenvcmd.ExecuteOption{dotenvcmd.WithProgramName("myapp env")}, dotenvcmd.ExitOK, "out", []string{"myapp env version "}, true},
		{"default stays dotenvctl", []string{"-f", envFile, "get", "MISSING"}, nil, dotenvcmd.ExitFailure, "err", []string{`dotenvctl: key "MISSING" not found`}, false},
		{"empty name means default", []string{"nosuchverb"}, []dotenvcmd.ExecuteOption{dotenvcmd.WithProgramName("")}, dotenvcmd.ExitUsage, "err", []string{"dotenvctl: unknown command", "Run 'dotenvctl --help'"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			code := dotenvcmd.Execute(tt.args, &stdout, &stderr, tt.opts...)
			if code != tt.wantCode {
				t.Fatalf("exit = %d, want %d; stderr=%q", code, tt.wantCode, stderr.String())
			}
			got := stderr.String()
			if tt.stream == "out" {
				got = stdout.String()
			}
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("%s missing %q:\n%s", tt.stream, w, got)
				}
			}
			if tt.renamed && strings.Contains(got, "dotenvctl") {
				t.Errorf("%s still carries the fixed dotenvctl: prefix:\n%s", tt.stream, got)
			}
		})
	}
}
