package vercelplugin

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/internal/outfmt"
	"github.com/ubgo/dotenv/cli/providerkit"
)

// call records one Runner invocation.
type call struct {
	args  string
	stdin string
}

// fakeRunner scripts vercel responses by argv prefix and records every call.
type fakeRunner struct {
	calls     []call
	responses map[string]string
	failOn    string
}

func (r *fakeRunner) Run(_ context.Context, stdin string, _ string, args ...string) ([]byte, error) {
	joined := strings.Join(args, " ")
	r.calls = append(r.calls, call{args: joined, stdin: stdin})
	if r.failOn != "" && strings.Contains(joined, r.failOn) {
		return nil, fmt.Errorf("vercel: boom")
	}
	for prefix, out := range r.responses {
		if strings.HasPrefix(joined, prefix) {
			return []byte(out), nil
		}
	}
	return []byte(""), nil
}

// envLsTable mimics the vercel CLI's human table, chatter included.
const envLsTable = "Vercel CLI 33.0.0\n> Environment Variables found for acme/api\n\n name         value               environments        created\n DB_URL       Encrypted           Production          2d ago\n API_KEY      Encrypted           Production          5d ago\n\n"

func vercelResponses() map[string]string {
	return map[string]string{
		"whoami": "khanakia\n",
		"env ls": envLsTable,
	}
}

// staticSource is a canned selection.
type staticSource struct {
	pairs []dotenv.Pair
	skips []providerkit.Skip
}

func (s *staticSource) Pairs() []dotenv.Pair        { return s.pairs }
func (s *staticSource) Skipped() []providerkit.Skip { return s.skips }

// runVercel drives the plugin's command tree with scripted deps.
func runVercel(t *testing.T, runner *fakeRunner, src providerkit.Source, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	deps := providerkit.Deps{
		Printer:     &outfmt.Printer{Out: &out},
		File:        ".env.test",
		Source:      func(providerkit.SelectOpts) (providerkit.Source, error) { return src, nil },
		Runner:      runner,
		Interactive: func() bool { return false },
		Ask:         func(string) bool { return false },
	}
	c := New().Command(deps)
	c.SetArgs(args)
	err := c.Execute()
	return out.String(), err
}

func TestPush_ArgvStdinAndDefaultTarget(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{responses: vercelResponses()}
	src := &staticSource{pairs: []dotenv.Pair{{Key: "DB_URL", Value: "postgres://x"}}}

	out, err := runVercel(t, runner, src, "push", "--yes")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}

	var add *call
	for i := range runner.calls {
		if strings.HasPrefix(runner.calls[i].args, "env add") {
			add = &runner.calls[i]
		}
	}
	if add == nil {
		t.Fatalf("no env add call: %+v", runner.calls)
	}
	// Default target, --force upsert, value on STDIN never argv.
	if add.args != "env add DB_URL development --force" {
		t.Errorf("argv = %q", add.args)
	}
	if add.stdin != "postgres://x" || strings.Contains(add.args, "postgres") {
		t.Errorf("value transport wrong: stdin=%q argv=%q", add.stdin, add.args)
	}
	for _, want := range []string{"account: khanakia (stored vercel auth)", "targets: development", "DB_URL: pushed"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPush_MultiTargetAndSensitive(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{responses: vercelResponses()}
	src := &staticSource{pairs: []dotenv.Pair{{Key: "K", Value: "v"}}}

	if _, err := runVercel(t, runner, src, "push", "--yes", "--target", "production", "--target", "preview", "--sensitive"); err != nil {
		t.Fatal(err)
	}
	var adds []string
	for _, c := range runner.calls {
		if strings.HasPrefix(c.args, "env add") {
			adds = append(adds, c.args)
		}
	}
	want := []string{
		"env add K production --force --sensitive",
		"env add K preview --force --sensitive",
	}
	if len(adds) != 2 || adds[0] != want[0] || adds[1] != want[1] {
		t.Errorf("adds = %v, want %v", adds, want)
	}
}

func TestPush_TargetFailureNamesTheTarget(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{responses: vercelResponses(), failOn: "env add"}
	src := &staticSource{pairs: []dotenv.Pair{{Key: "K", Value: "v"}}}

	out, err := runVercel(t, runner, src, "push", "--yes")
	if err == nil {
		t.Fatal("want failure")
	}
	if !strings.Contains(out, "target development") {
		t.Errorf("failure must name the target:\n%s", out)
	}
}

func TestList_ParsesTableChatterImmune(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{responses: vercelResponses()}

	out, err := runVercel(t, runner, &staticSource{}, "list")
	if err != nil {
		t.Fatal(err)
	}
	// Data rows survive; banner/header rows ("Vercel CLI", ">", "name") don't.
	if !strings.Contains(out, "DB_URL") || !strings.Contains(out, "API_KEY") {
		t.Errorf("names missing: %q", out)
	}
	for _, junk := range []string{"Vercel", "name", ">"} {
		for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
			if strings.HasPrefix(line, junk) {
				t.Errorf("chatter leaked into names: %q", line)
			}
		}
	}
}

func TestPrune_UnionAcrossTargetsDeletesStale(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{responses: vercelResponses()} // remote: DB_URL, API_KEY
	src := &staticSource{pairs: []dotenv.Pair{{Key: "DB_URL", Value: "x"}}}

	if _, err := runVercel(t, runner, src, "prune", "--yes", "--target", "production"); err != nil {
		t.Fatal(err)
	}
	var rms []string
	for _, c := range runner.calls {
		if strings.HasPrefix(c.args, "env rm") {
			rms = append(rms, c.args)
		}
	}
	if len(rms) != 1 || rms[0] != "env rm API_KEY production --yes" {
		t.Errorf("rms = %v, want only the stale name on the selected target", rms)
	}
}

func TestGate_WhoamiFailureIsActionable(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{failOn: "whoami"}
	src := &staticSource{pairs: []dotenv.Pair{{Key: "K", Value: "v"}}}

	out, err := runVercel(t, runner, src, "push", "--yes")
	if err == nil {
		t.Fatal("want failure")
	}
	if !strings.Contains(out, "vercel.com/docs/cli") {
		t.Errorf("failure not actionable:\n%s", out)
	}
	for _, c := range runner.calls {
		if strings.HasPrefix(c.args, "env add") {
			t.Error("wrote despite failed resolution")
		}
	}
}

func TestResolveProject_UnlinkedDirectory(t *testing.T) {
	t.Parallel()
	// Tests run outside any .vercel-linked directory, so the fallback is the
	// honest answer — and the one the banner must show rather than guessing.
	s := &vercelStore{}
	if got := s.resolveProject(); !strings.Contains(got, "unlinked directory") {
		t.Errorf("resolveProject = %q", got)
	}
}

func TestTokenOverrideLabeledInBanner(t *testing.T) {
	// t.Setenv forbids t.Parallel.
	t.Setenv(tokenEnvVar, "vercel_test_token")
	runner := &fakeRunner{responses: vercelResponses()}
	src := &staticSource{pairs: []dotenv.Pair{{Key: "K", Value: "v"}}}

	out, err := runVercel(t, runner, src, "push", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "khanakia (via VERCEL_TOKEN)") {
		t.Errorf("auth source not labeled:\n%s", out)
	}
}

func TestParseEnvLs_EdgeShapes(t *testing.T) {
	t.Parallel()
	got := parseEnvLs("")
	if len(got) != 0 {
		t.Errorf("empty output → %v", got)
	}
	got = parseEnvLs("WARN outdated\nERROR nope\n MY_VAR Encrypted Production\n")
	if len(got) != 1 || got[0] != "MY_VAR" {
		t.Errorf("got %v, want only MY_VAR", got)
	}
}

func TestResolveProject_LinkedAndCorrupt(t *testing.T) {
	// t.Chdir forbids t.Parallel.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".vercel"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	s := &vercelStore{}

	// Corrupt link file → honest unlinked answer, never a guess.
	if err := os.WriteFile(filepath.Join(dir, projectLinkFile), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := s.resolveProject(); !strings.Contains(got, "unlinked") {
		t.Errorf("corrupt link = %q", got)
	}

	// Valid link → project id with provenance.
	if err := os.WriteFile(filepath.Join(dir, projectLinkFile), []byte(`{"projectId":"prj_abc","orgId":"team_x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := s.resolveProject(); got != "prj_abc (from "+projectLinkFile+")" {
		t.Errorf("linked = %q", got)
	}
}

func TestDeleteAndMeta_ErrorArms(t *testing.T) {
	t.Parallel()
	s := &vercelStore{deps: providerkit.Deps{Runner: &fakeRunner{failOn: "env rm"}}, targets: []string{"production"}}
	if err := s.Delete(context.Background(), "K"); err == nil || !strings.Contains(err.Error(), "target production") {
		t.Errorf("Delete err = %v, want target-named failure", err)
	}

	s2 := &vercelStore{deps: providerkit.Deps{Runner: &fakeRunner{failOn: "whoami"}}}
	if _, err := s2.meta(context.Background()); err == nil {
		t.Error("meta must propagate account-resolution failure")
	}
}
