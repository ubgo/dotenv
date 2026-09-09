// Coverage for the remaining error arms the behavioral and rigor suites do
// not reach: report-write failures, unreadable files, exec-plugin discovery
// edge cases, and the providerkit error seam in the root dispatcher. Each
// test documents WHICH arm it pins so a future refactor knows what breaks.
package dotenvcmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/dotenv/cli/envkit"
	"github.com/ubgo/dotenv/cli/outfmt"
)

// TestGetJSONWriteFailure pins get's success-path OK() error arm: the value
// was found, but the envelope cannot be written (closed pipe). The command
// must fail rather than exit 0 having reported nothing.
func TestGetJSONWriteFailure(t *testing.T) {
	t.Parallel()
	path := writeFixture(t, "A=1\n")
	var stderr bytes.Buffer
	if code := Execute([]string{"-f", path, "get", "A", "--json"}, errWriter{}, &stderr); code == ExitOK {
		t.Error("exit 0 despite the JSON envelope never reaching stdout")
	}
}

// TestOpenErrorOnDirectory pins the a.open() error arm shared by keys and
// list: -f pointing at a directory is an I/O failure, not an empty file.
func TestOpenErrorOnDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, verb := range []string{"keys", "list"} {
		if code, stdout, stderr := runCLI(t, "-f", dir, verb); code != ExitFailure || stdout+stderr == "" {
			t.Errorf("%s on a directory: exit %d output %q — want loud I/O failure", verb, code, stdout+stderr)
		}
	}
}

// TestKeysNamedIncludeMissing pins keys' selection error arm: a key you NAME
// must exist (silently skipping a named key would report success for work
// that never happened — the core selection-grammar rule).
func TestKeysNamedIncludeMissing(t *testing.T) {
	t.Parallel()
	path := writeFixture(t, "A=1\n")
	if code, _, _ := runCLI(t, "-f", path, "keys", "--include", "NOPE"); code != ExitFailure {
		t.Errorf("exit %d for --include of an absent key — want %d", code, ExitFailure)
	}
}

// TestEnvsInventoryError pins envs' Inventories error arm: discovery finds an
// env file, but opening it fails (permissions). The command must fail, not
// render a partial table.
func TestEnvsInventoryError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	locked := filepath.Join(dir, ".env")
	if err := os.WriteFile(locked, []byte("K=1\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(locked); err == nil {
		t.Skip("mode 0000 is readable here (privileged run) — the arm cannot fire")
	}
	if code, _, _ := runCLI(t, "envs", "--dir", dir); code != ExitFailure {
		t.Errorf("exit %d for an unreadable .env — want %d", code, ExitFailure)
	}
}

// TestDiffHTMLWriteFailure pins two arms at once: diff's emitFormatted error
// return, and emitFormatted's page-write failure (the report rendered but
// stdout is gone).
func TestDiffHTMLWriteFailure(t *testing.T) {
	t.Parallel()
	a := writeFixture(t, "A=1\n")
	b := writeFixture(t, "A=2\n")
	var stderr bytes.Buffer
	if code := Execute([]string{"diff", a, b, "--format", "html"}, errWriter{}, &stderr); code != ExitFailure {
		t.Errorf("exit %d writing an HTML report to a dead pipe — want %d", code, ExitFailure)
	}
}

// TestEmitFormattedHTMLGenerationError pins the htmlFn error arm directly —
// no CLI path can currently make report generation fail, but the arm is the
// contract for any future report that can.
func TestEmitFormattedHTMLGenerationError(t *testing.T) {
	t.Parallel()
	a := &app{printer: &outfmt.Printer{Out: &bytes.Buffer{}}}
	err := a.emitFormatted(formatHTML, "", nil, func() {}, func() ([]byte, error) {
		return nil, errors.New("template exploded")
	})
	if err == nil {
		t.Error("nil error from a failing HTML generator")
	}
}

// TestRenderMatrixHumanJSONGuard pins the defensive JSON guard: if the human
// renderer is ever invoked while the printer is in JSON mode, it must emit
// nothing — mixed output would corrupt the machine envelope.
func TestRenderMatrixHumanJSONGuard(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	a := &app{printer: &outfmt.Printer{Out: &out, JSON: true}}
	a.renderMatrixHuman(&envkit.Matrix{}, false, false)
	if out.Len() != 0 {
		t.Errorf("human table leaked into JSON output: %q", out.String())
	}
}

// TestScanForSubcommandEqualsForms pins the `-f=PATH` / `--file=PATH` spelling
// in the pre-cobra scan — the combined form is parsed by hand there, and a
// regression would silently hand exec plugins the default file.
func TestScanForSubcommandEqualsForms(t *testing.T) {
	t.Parallel()
	name, rest, file, jsonOut := scanForSubcommand([]string{"-f=custom.env", "acme", "push"})
	if name != "acme" || file != "custom.env" || jsonOut {
		t.Errorf("got name=%q file=%q json=%v — want acme/custom.env/false", name, file, jsonOut)
	}
	if len(rest) != 1 || rest[0] != "push" {
		t.Errorf("rest = %v — want [push]", rest)
	}

	// --json before the plugin name must reach the child as DOTENVCTL_JSON=1.
	if _, _, _, jsonOut := scanForSubcommand([]string{"--json", "acme"}); !jsonOut {
		t.Error("--json before the subcommand was not picked up by the scan")
	}

	// An unknown flag is skipped by the scan (cobra owns rejecting it later);
	// the subcommand after it must still be found.
	if name, _, _, _ := scanForSubcommand([]string{"--verbose", "acme"}); name != "acme" {
		t.Errorf("name %q — an unknown flag must not hide the subcommand", name)
	}
}

// TestDiscoverExecPluginsEdgeCases pins discovery's skip rules: an empty name
// after the prefix, a duplicate name in a later PATH dir (first wins), a
// directory that matches the prefix, and a file without the execute bit.
func TestDiscoverExecPluginsEdgeCases(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()
	writeExec := func(dir, base string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, base), []byte("#!/bin/sh\nexit 0\n"), mode); err != nil {
			t.Fatal(err)
		}
	}
	writeExec(dirA, "dotenvctl-", 0o755)                                           // empty name → skipped
	writeExec(dirA, "dotenvctl-dup", 0o755)                                        // first occurrence → kept
	writeExec(dirB, "dotenvctl-dup", 0o755)                                        // duplicate in a later dir → skipped
	writeExec(dirB, "dotenvctl-plain", 0o644)                                      // no execute bit → skipped
	if err := os.Mkdir(filepath.Join(dirB, "dotenvctl-adir"), 0o755); err != nil { // a directory → skipped
		t.Fatal(err)
	}
	t.Setenv("PATH", dirA+string(os.PathListSeparator)+dirB)

	root := newRootCmd(&app{printer: &outfmt.Printer{Out: &bytes.Buffer{}}})
	found := discoverExecPlugins(root)
	if len(found) != 1 || found[0].Name != "dup" || !strings.HasPrefix(found[0].Path, dirA) {
		t.Errorf("found %+v — want exactly one 'dup' from the first PATH dir", found)
	}
}

// TestIsExecutableMissing pins the stat-failure arm: a path that does not
// exist is not executable, never a panic.
func TestIsExecutableMissing(t *testing.T) {
	t.Parallel()
	if isExecutable(filepath.Join(t.TempDir(), "ghost")) {
		t.Error("a missing file reported as executable")
	}
}

// TestExecPluginStartFailure pins the non-ExitError arm of dispatch: the
// binary exists and is executable but cannot be started (garbage format, so
// the kernel rejects it). That is our failure to report, not the plugin's.
func TestExecPluginStartFailure(t *testing.T) {
	dir := t.TempDir()
	// No shebang and a NUL byte: exec(2) refuses with ENOEXEC and Go's
	// os/exec does not fall back to a shell.
	if err := os.WriteFile(filepath.Join(dir, "dotenvctl-borked"), []byte("\x00not a binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	code, _, stderr := runCLI(t, "borked")
	if code != ExitFailure || !strings.Contains(stderr, "exec plugin") {
		t.Errorf("exit %d stderr %q — want %d with an 'exec plugin' report", code, stderr, ExitFailure)
	}
}

// boomRunner fails every backend invocation — the seam for driving
// providerkit.CmdError through the real verb tree.
type boomRunner struct{}

func (boomRunner) Run(context.Context, string, string, ...string) ([]byte, error) {
	return nil, errors.New("gh: boom")
}

// TestRootMapsCmdError pins root's providerkit.CmdError arm: a plugin verb
// failing on the other side of the seam must surface its own 0/1/2 code, not
// be re-wrapped as a generic failure.
func TestRootMapsCmdError(t *testing.T) {
	t.Parallel()
	var out, errBuf bytes.Buffer
	a := &app{
		printer:       &outfmt.Printer{Out: &out},
		errOut:        &errBuf,
		runner:        boomRunner{},
		interactiveFn: func() bool { return false },
	}
	if code := executeApp(a, []string{"github", "list"}, &out, &errBuf); code != ExitFailure {
		t.Errorf("exit %d for a backend failure — want %d via CmdError", code, ExitFailure)
	}
}

// TestRootPrintsNonQuietCmdError pins the OTHER half of root's CmdError arm —
// the `!pe.Quiet` branch, which TestRootMapsCmdError cannot reach.
//
// providerkit.Fail normally returns a QUIET CmdError, because it has already
// written the failure envelope itself. The one time it returns a loud one is
// when that write FAILED, and then root is the last thing standing between a
// broken pipe and a process that exits non-zero having said nothing at all.
//
// Driving it needs a printer whose own output is broken, which is why the
// backend must also fail: a successful command never calls Fail.
func TestRootPrintsNonQuietCmdError(t *testing.T) {
	t.Parallel()
	var errBuf bytes.Buffer
	a := &app{
		// Out is broken, so the envelope write inside Fail fails and the
		// resulting CmdError carries Err with Quiet unset.
		printer:       &outfmt.Printer{Out: errWriter{}},
		errOut:        &errBuf,
		runner:        boomRunner{},
		interactiveFn: func() bool { return false },
	}
	code := executeApp(a, []string{"github", "list"}, errWriter{}, &errBuf)
	if code != ExitFailure {
		t.Errorf("exit %d for a backend failure with a broken stdout — want %d", code, ExitFailure)
	}
	if !strings.Contains(errBuf.String(), "dotenvctl:") {
		t.Errorf("stderr = %q — want the dotenvctl: line, since the envelope "+
			"never reached stdout and the exit code alone would be silent", errBuf.String())
	}
}
