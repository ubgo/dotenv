package dotenvcmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writePluginScript drops an executable dotenvctl-<name> fixture into dir.
func writePluginScript(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, execPluginPrefix+name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// withPluginPath prepends dir to PATH for the test. t.Setenv forbids
// t.Parallel, so every dispatch test is serial by construction.
func withPluginPath(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestExecDispatch_ArgvEnvAndExitPassthrough(t *testing.T) {
	dir := t.TempDir()
	// The fixture proves the whole contract in one run: argv passthrough,
	// every context env var, and a chosen exit code.
	writePluginScript(t, dir, "probe", `echo "args:$@"
echo "file:$DOTENVCTL_FILE"
echo "json:$DOTENVCTL_JSON"
echo "ph:$DOTENVCTL_PLACEHOLDER"
[ -n "$DOTENVCTL_BIN" ] && echo "bin:set"
[ -n "$DOTENVCTL_VERSION" ] && echo "version:set"
exit 7`)
	withPluginPath(t, dir)

	envFile := writeFixture(t, "A=1\n")
	var out, errBuf bytes.Buffer
	code := Execute([]string{"-f", envFile, "probe", "push", "--flag"}, &out, &errBuf)

	if code != 7 {
		t.Fatalf("exit = %d, want the plugin's 7 (out %q err %q)", code, out.String(), errBuf.String())
	}
	for _, want := range []string{
		"args:push --flag",
		"file:" + envFile, // writeFixture returns an absolute path already
		"json:0",
		"ph:" + defaultPlaceholderPattern,
		"bin:set",
		"version:set",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("plugin output missing %q:\n%s", want, out.String())
		}
	}
}

func TestExecDispatch_JSONFlagReachesChild(t *testing.T) {
	dir := t.TempDir()
	writePluginScript(t, dir, "jsonprobe", `echo "json:$DOTENVCTL_JSON"`)
	withPluginPath(t, dir)

	var out, errBuf bytes.Buffer
	if code := Execute([]string{"--json", "jsonprobe"}, &out, &errBuf); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "json:1") {
		t.Errorf("child did not see --json: %q", out.String())
	}
}

func TestExecDispatch_BuiltinsAlwaysWin(t *testing.T) {
	dir := t.TempDir()
	// A malicious/shadowing dotenvctl-github must NEVER be dispatched to.
	writePluginScript(t, dir, "github", `echo HIJACKED; exit 0`)
	withPluginPath(t, dir)

	var out, errBuf bytes.Buffer
	// `github` without a subcommand prints the builtin's help.
	code := Execute([]string{"github"}, &out, &errBuf)
	joined := out.String() + errBuf.String()
	if strings.Contains(joined, "HIJACKED") {
		t.Fatal("PATH binary shadowed a built-in")
	}
	if code == 7 {
		t.Fatal("exit code came from the shadowing plugin")
	}
}

func TestExecDispatch_UnknownWithoutPluginIsUsage(t *testing.T) {
	// PATH untouched: no dotenvctl-nosuchthing exists.
	var out, errBuf bytes.Buffer
	if code := Execute([]string{"nosuchthing"}, &out, &errBuf); code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestScanForSubcommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		args     []string
		wantName string
		wantFile string
		wantJSON bool
		wantRest []string
	}{
		{"plain", []string{"acme", "push"}, "acme", defaultEnvFile, false, []string{"push"}},
		{"split file flag", []string{"-f", "x.env", "acme"}, "acme", "x.env", false, []string{}},
		{"joined file flag", []string{"--file=y.env", "acme", "a"}, "acme", "y.env", false, []string{"a"}},
		{"json flag", []string{"--json", "acme"}, "acme", defaultEnvFile, true, []string{}},
		{"flags only, no subcommand", []string{"--json", "-f", "z"}, "", "z", true, nil},
		{"empty", nil, "", defaultEnvFile, false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			name, rest, file, jsonOut := scanForSubcommand(tt.args)
			if name != tt.wantName || file != tt.wantFile || jsonOut != tt.wantJSON {
				t.Errorf("got (%q,%q,%v), want (%q,%q,%v)", name, file, jsonOut, tt.wantName, tt.wantFile, tt.wantJSON)
			}
			if len(rest) != len(tt.wantRest) {
				t.Errorf("rest = %v, want %v", rest, tt.wantRest)
			}
		})
	}
}

func TestPluginsVerb(t *testing.T) {
	dir := t.TempDir()
	writePluginScript(t, dir, "acme", `exit 0`)
	writePluginScript(t, dir, "github", `exit 0`) // shadowed by the built-in
	// A non-executable candidate must not list (POSIX).
	if err := os.WriteFile(filepath.Join(dir, execPluginPrefix+"noexec"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withPluginPath(t, dir)

	var out, errBuf bytes.Buffer
	if code := Execute([]string{"plugins"}, &out, &errBuf); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	got := out.String()
	if !strings.Contains(got, "acme") || !strings.Contains(got, "active") {
		t.Errorf("missing active plugin:\n%s", got)
	}
	if !strings.Contains(got, "shadowed by built-in") {
		t.Errorf("shadowing not reported:\n%s", got)
	}
	if strings.Contains(got, "noexec") {
		t.Errorf("non-executable listed:\n%s", got)
	}
}

func TestPluginsVerb_EmptyPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	var out, errBuf bytes.Buffer
	if code := Execute([]string{"plugins", "--json"}, &out, &errBuf); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), `"plugins":[]`) {
		t.Errorf("empty discovery must be an empty array: %q", out.String())
	}
}

// TestExecDispatch_CallbackE2E is the full protocol: a plugin script calls
// the REAL dotenvctl binary back via $DOTENVCTL_BIN to fetch a value. Builds
// the binary once (skipped under -short).
func TestExecDispatch_CallbackE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the real binary")
	}
	binDir := t.TempDir()
	bin := filepath.Join(binDir, "dotenvctl")
	build := exec.Command("go", "build", "-o", bin, "./cmd/dotenvctl")
	build.Dir = ".." // package dir is cli/dotenvcmd; the module root is cli/
	if outp, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, outp)
	}

	pluginDir := t.TempDir()
	writePluginScript(t, pluginDir, "cb", `"$DOTENVCTL_BIN" -f "$DOTENVCTL_FILE" get GREETING --expand`)
	envFile := writeFixture(t, "WHO=world\nGREETING=hello ${WHO}\n")

	child := exec.Command(bin, "-f", envFile, "cb")
	child.Env = append(os.Environ(), "PATH="+pluginDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	outp, err := child.CombinedOutput()
	if err != nil {
		t.Fatalf("dispatch: %v\n%s", err, outp)
	}
	if got := strings.TrimSpace(string(outp)); got != "hello world" {
		t.Errorf("callback = %q, want the expanded value through the host", got)
	}
}

func TestIsExecutable_Edges(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if isExecutable(dir) {
		t.Error("a directory is not executable-as-plugin")
	}
	if isExecutable(filepath.Join(dir, "absent")) {
		t.Error("a missing path is not executable")
	}
}
