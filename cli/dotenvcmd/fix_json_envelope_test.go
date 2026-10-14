package dotenvcmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// assertOneEnvelope fails unless stdout is exactly one newline-terminated
// JSON object with an "ok" field — the --json contract scripts rely on when
// they pipe dotenvctl into jq.
func assertOneEnvelope(t *testing.T, stdout string) {
	t.Helper()
	if !strings.HasSuffix(stdout, "\n") || strings.Count(stdout, "\n") != 1 {
		t.Fatalf("stdout is not exactly one line:\n%s", stdout)
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout is not one JSON object (%v):\n%s", err, stdout)
	}
	if _, ok := env["ok"]; !ok {
		t.Fatalf("envelope has no ok field: %s", stdout)
	}
}

// TestJSON_EveryVerbPrintsExactlyOneEnvelope sweeps the built-in verbs, on
// success and on failure, including usage errors: under --json stdout must
// carry exactly one envelope and nothing else. `run` is excluded by design —
// its stdout is the child's — and so are exec plugins, whose output is their
// own.
func TestJSON_EveryVerbPrintsExactlyOneEnvelope(t *testing.T) {
	t.Parallel()
	other := writeFixture(t, "DB_HOST=elsewhere\n")
	third := writeFixture(t, "DB_HOST=third\n")
	tests := []struct {
		name string
		args []string
	}{
		{"get ok", []string{"get", "DB_HOST"}},
		{"get missing", []string{"get", "NOPE"}},
		{"get usage: no key", []string{"get"}},
		{"get usage: unknown flag", []string{"get", "DB_HOST", "--nope"}},
		{"set ok", []string{"set", "NEW=1"}},
		{"set usage", []string{"set", "NOEQUALS"}},
		{"set missing anchor", []string{"set", "X=1", "--after", "NOPE"}},
		{"unset missing", []string{"unset", "NOPE"}},
		{"restore missing", []string{"restore", "NOPE"}},
		{"list", []string{"list"}},
		{"list --disabled --inherited", []string{"list", "--disabled", "--inherited"}},
		{"keys", []string{"keys"}},
		{"keys bad include", []string{"keys", "--include", "NOPE"}},
		{"envs", []string{"envs"}},
		{"matrix", []string{"matrix"}},
		{"matrix contract", []string{"matrix", "--contract", ".env"}},
		{"diff differs", []string{"diff", other, third}},
		{"diff same", []string{"diff", other, other}},
		{"diff usage", []string{"diff", other}},
		{"unknown verb", []string{"nosuchverb"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := writeFixture(t, fixture)
			args := append([]string{"-f", path, "--json"}, tt.args...)
			_, out, _ := runCLI(t, args...)
			assertOneEnvelope(t, out)
		})
	}
}

// TestJSON_UsageErrorKeepsExitCode pins that the usage envelope does not
// change the exit-code contract: a malformed invocation is still exit 2.
func TestJSON_UsageErrorKeepsExitCode(t *testing.T) {
	t.Parallel()
	path := writeFixture(t, fixture)
	code, out, errOut := runCLI(t, "-f", path, "--json", "get")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(out, `"code":"usage"`) {
		t.Errorf("stdout = %q, want a usage envelope", out)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want empty under --json", errOut)
	}
}

// TestPluginsVerb_JSONIsOneEnvelope pins bug 12: with a plugin installed,
// `plugins --json` printed the human table before the envelope.
func TestPluginsVerb_JSONIsOneEnvelope(t *testing.T) {
	dir := t.TempDir()
	writePluginScript(t, dir, "acme", `exit 0`)
	if err := os.WriteFile(filepath.Join(dir, "unrelated"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	withPluginPath(t, dir)

	var out, errBuf bytes.Buffer
	if code := Execute([]string{"plugins", "--json"}, &out, &errBuf); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	assertOneEnvelope(t, out.String())
	if !strings.Contains(out.String(), `"name":"acme"`) {
		t.Errorf("plugin missing from envelope: %s", out.String())
	}
}

// TestJSONRequested walks the forms cobra accepts for --json, so a usage
// error that happens before cobra binds the flag still gets an envelope.
func TestJSONRequested(t *testing.T) {
	t.Parallel()
	tests := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"get", "K"}, false},
		{[]string{"--json", "nosuchverb"}, true},
		{[]string{"nosuchverb", "--json"}, true},
		{[]string{"get", "--json=true"}, true},
		{[]string{"get", "--json=1"}, true},
		{[]string{"get", "--json=false"}, false},
		{[]string{"get", "--json=maybe"}, false},
		{[]string{"get", "--json", "--json=false"}, false},
		{[]string{"run", "--", "tool", "--json"}, false},
		{[]string{"get", "--jsonx"}, false},
	}
	for _, tt := range tests {
		if got := jsonRequested(tt.args); got != tt.want {
			t.Errorf("jsonRequested(%q) = %t, want %t", tt.args, got, tt.want)
		}
	}
}
