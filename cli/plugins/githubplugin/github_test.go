package githubplugin

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/internal/outfmt"
	"github.com/ubgo/dotenv/cli/providerkit"
)

// call records one Runner invocation for argv/stdin assertions.
type call struct {
	name  string
	args  string // space-joined for readable assertions
	stdin string
}

// fakeRunner scripts gh responses by argv prefix and records every call —
// the no-network, no-gh test seam the spec mandates.
type fakeRunner struct {
	calls     []call
	responses map[string]string // argv-prefix → stdout
	failOn    string            // argv containing this substring errors
}

func (r *fakeRunner) Run(_ context.Context, stdin string, name string, args ...string) ([]byte, error) {
	joined := strings.Join(args, " ")
	r.calls = append(r.calls, call{name: name, args: joined, stdin: stdin})
	if r.failOn != "" && strings.Contains(joined, r.failOn) {
		return nil, fmt.Errorf("gh: boom")
	}
	for prefix, out := range r.responses {
		if strings.HasPrefix(joined, prefix) {
			return []byte(out), nil
		}
	}
	return []byte(""), nil
}

// ghResponses is the standard happy-path script.
func ghResponses() map[string]string {
	return map[string]string{
		"repo view":   "solverhood/sync_go\n",
		"api user":    "khanakia\n",
		"secret list": `[{"name":"OLD_STALE"},{"name":"DEPLOY_PATH"}]`,
	}
}

// runGithub drives the plugin's command tree directly with scripted deps.
func runGithub(t *testing.T, runner *fakeRunner, src providerkit.Source, args ...string) (string, error) {
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

// staticSource is a canned selection.
type staticSource struct {
	pairs []dotenv.Pair
	skips []providerkit.Skip
}

func (s *staticSource) Pairs() []dotenv.Pair        { return s.pairs }
func (s *staticSource) Skipped() []providerkit.Skip { return s.skips }

func TestPush_ArgvAndStdin(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{responses: ghResponses()}
	src := &staticSource{pairs: []dotenv.Pair{{Key: "DEPLOY_PATH", Value: "/srv/app"}}}

	out, err := runGithub(t, runner, src, "push", "--yes")
	if err != nil {
		t.Fatalf("push: %v\n%s", err, out)
	}

	var set *call
	for i := range runner.calls {
		if strings.HasPrefix(runner.calls[i].args, "secret set") {
			set = &runner.calls[i]
		}
	}
	if set == nil {
		t.Fatalf("no secret set call: %+v", runner.calls)
	}
	// The value must travel by STDIN and must never appear in argv.
	if set.args != "secret set DEPLOY_PATH --body -" {
		t.Errorf("argv = %q", set.args)
	}
	if set.stdin != "/srv/app" {
		t.Errorf("stdin = %q, want the value", set.stdin)
	}
	if strings.Contains(set.args, "/srv/app") {
		t.Error("value leaked into argv")
	}
	// Banner resolved from gh, auth source labeled.
	for _, want := range []string{"target : solverhood/sync_go (from cwd git remote)", "account: khanakia", "DEPLOY_PATH: pushed"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPush_ScopeFlagsPassThrough(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{responses: ghResponses()}
	src := &staticSource{pairs: []dotenv.Pair{{Key: "K", Value: "v"}}}

	if _, err := runGithub(t, runner, src, "push", "--yes", "--repo", "o/r", "--environment", "production"); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range runner.calls {
		if c.args == "secret set K --body - --repo o/r --env production" {
			found = true
		}
	}
	if !found {
		t.Errorf("scope flags not passed through: %+v", runner.calls)
	}

	// --org excludes --repo.
	if _, err := runGithub(t, runner, src, "push", "--yes", "--org", "acme", "--repo", "o/r"); err == nil {
		t.Error("org+repo must be a usage error")
	}
}

func TestPush_InvalidSecretNamesAbortBeforeAnyWrite(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{responses: ghResponses()}
	// Legal env keys under the Compose charset, illegal as GH secret names.
	src := &staticSource{pairs: []dotenv.Pair{
		{Key: "GOOD_NAME", Value: "x"},
		{Key: "spring.datasource.url", Value: "x"},
		{Key: "MY-TOKEN", Value: "x"},
	}}

	out, err := runGithub(t, runner, src, "push", "--yes")
	if err == nil {
		t.Fatal("want failure")
	}
	for _, c := range runner.calls {
		if strings.HasPrefix(c.args, "secret set") {
			t.Errorf("write happened despite invalid names: %q", c.args)
		}
	}
	if !strings.Contains(out, "spring.datasource.url") || !strings.Contains(out, "MY-TOKEN") {
		t.Errorf("offending keys not listed:\n%s", out)
	}
}

func TestPush_NonInteractiveRequiresYes(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{responses: ghResponses()}
	src := &staticSource{pairs: []dotenv.Pair{{Key: "K", Value: "v"}}}

	if _, err := runGithub(t, runner, src, "push"); err == nil {
		t.Error("non-interactive push without --yes must refuse")
	}
	for _, c := range runner.calls {
		if strings.HasPrefix(c.args, "secret set") {
			t.Error("refused push still wrote")
		}
	}
}

func TestPrune_DeletesOnlyStale(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{responses: ghResponses()} // remote: OLD_STALE, DEPLOY_PATH
	src := &staticSource{pairs: []dotenv.Pair{{Key: "DEPLOY_PATH", Value: "x"}}}

	out, err := runGithub(t, runner, src, "prune", "--yes")
	if err != nil {
		t.Fatalf("prune: %v\n%s", err, out)
	}
	deletes := []string{}
	for _, c := range runner.calls {
		if strings.HasPrefix(c.args, "secret delete") {
			deletes = append(deletes, c.args)
		}
	}
	if len(deletes) != 1 || deletes[0] != "secret delete OLD_STALE" {
		t.Errorf("deletes = %v, want only the stale name", deletes)
	}
}

func TestList_ParsesGhJSON(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{responses: ghResponses()}

	out, err := runGithub(t, runner, &staticSource{}, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "DEPLOY_PATH") || !strings.Contains(out, "OLD_STALE") {
		t.Errorf("list output = %q", out)
	}
}

func TestGate_GhFailureIsActionableAndAborts(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{failOn: "repo view"} // gh missing/unauthenticated
	src := &staticSource{pairs: []dotenv.Pair{{Key: "K", Value: "v"}}}

	out, err := runGithub(t, runner, src, "push", "--yes")
	if err == nil {
		t.Fatal("want failure")
	}
	if !strings.Contains(out, "cli.github.com") {
		t.Errorf("failure not actionable:\n%s", out)
	}
	for _, c := range runner.calls {
		if strings.HasPrefix(c.args, "secret set") {
			t.Error("wrote despite failed resolution")
		}
	}
}

func TestPush_TokenOverrideLabeledInBanner(t *testing.T) {
	// t.Setenv forbids t.Parallel — the label depends on process env.
	t.Setenv("GH_TOKEN", "ghp_test_override")
	runner := &fakeRunner{responses: ghResponses()}
	src := &staticSource{pairs: []dotenv.Pair{{Key: "K", Value: "v"}}}

	out, err := runGithub(t, runner, src, "push", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "khanakia (via GH_TOKEN)") {
		t.Errorf("auth source not labeled:\n%s", out)
	}
}

func TestPush_JSONCarriesMetaAndResults(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{responses: ghResponses()}
	src := &staticSource{
		pairs: []dotenv.Pair{{Key: "PAT", Value: "v"}},
		skips: []providerkit.Skip{{Name: "WHO", Action: providerkit.ActionSkippedPlaceholder}},
	}

	var out bytes.Buffer
	deps := providerkit.Deps{
		Printer:     &outfmt.Printer{Out: &out, JSON: true},
		Source:      func(providerkit.SelectOpts) (providerkit.Source, error) { return src, nil },
		Runner:      runner,
		Interactive: func() bool { return false },
		Ask:         func(string) bool { return false },
	}
	c := New().Command(deps)
	c.SetArgs([]string{"push", "--yes"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"target":"solverhood/sync_go (from cwd git remote)"`,
		`"account":"khanakia (stored gh auth)"`,
		`{"name":"WHO","action":"skipped-placeholder"}`,
		`{"name":"PAT","action":"pushed"}`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("json missing %s:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), `"v"`) {
		t.Error("a value leaked into json output")
	}
}

func TestPush_OrgScope(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{responses: ghResponses()}
	src := &staticSource{pairs: []dotenv.Pair{{Key: "K", Value: "v"}}}

	out, err := runGithub(t, runner, src, "push", "--yes", "--org", "acme")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	found := false
	for _, c := range runner.calls {
		if c.args == "secret set K --body - --org acme" {
			found = true
		}
	}
	if !found {
		t.Errorf("org scope not passed: %+v", runner.calls)
	}
	if !strings.Contains(out, "target : org:acme") {
		t.Errorf("banner should name the org target:\n%s", out)
	}
}

func TestStore_DirectEdges(t *testing.T) {
	t.Parallel()

	t.Run("Set rejects invalid names even without the gate", func(t *testing.T) {
		t.Parallel()
		s := &ghStore{deps: providerkit.Deps{Runner: &fakeRunner{}}}
		if err := s.Set(context.Background(), "bad-name", "v"); err == nil {
			t.Error("defense-in-depth name check missing")
		}
	})

	t.Run("Names surfaces unparseable gh output", func(t *testing.T) {
		t.Parallel()
		s := &ghStore{deps: providerkit.Deps{Runner: &fakeRunner{responses: map[string]string{"secret list": "not json"}}}}
		if _, err := s.Names(context.Background()); err == nil || !strings.Contains(err.Error(), "parse gh secret list") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("resolveTarget covers every scope shape", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{responses: ghResponses()}
		tests := []struct {
			name, repo, env, org, want string
		}{
			{"org", "", "", "acme", "org:acme"},
			{"repo only", "o/r", "", "", "o/r"},
			{"repo with environment", "o/r", "prod", "", "o/r (environment prod)"},
			{"cwd with environment", "", "prod", "", "solverhood/sync_go (from cwd git remote) (environment prod)"},
		}
		for _, tt := range tests {
			s := &ghStore{deps: providerkit.Deps{Runner: runner}, repo: tt.repo, environment: tt.env, org: tt.org}
			got, err := s.resolveTarget(context.Background())
			if err != nil || got != tt.want {
				t.Errorf("%s: target = %q err %v, want %q", tt.name, got, err, tt.want)
			}
		}
	})

	t.Run("meta propagates resolution failure so the kit omits it", func(t *testing.T) {
		t.Parallel()
		s := &ghStore{deps: providerkit.Deps{Runner: &fakeRunner{failOn: "repo view"}}}
		if _, err := s.meta(context.Background()); err == nil {
			t.Error("meta must not fabricate context on failure")
		}
	})
}

func TestEnvAliasFlag(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{responses: ghResponses()}
	src := &staticSource{pairs: []dotenv.Pair{{Key: "K", Value: "v"}}}

	// --env must behave exactly like --environment (gh's own spelling).
	if _, err := runGithub(t, runner, src, "push", "--yes", "--env", "production"); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range runner.calls {
		if c.args == "secret set K --body - --env production" {
			found = true
		}
	}
	if !found {
		t.Errorf("--env alias not honored: %+v", runner.calls)
	}
}

func TestEnvCreate(t *testing.T) {
	t.Parallel()

	t.Run("resolves repo, gates, PUTs the environments API", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{responses: ghResponses()}
		out, err := runGithub(t, runner, &staticSource{}, "env-create", "prod", "--yes")
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		found := false
		for _, c := range runner.calls {
			if c.args == "api --method PUT repos/solverhood/sync_go/environments/prod" {
				found = true
			}
		}
		if !found {
			t.Errorf("PUT call missing: %+v", runner.calls)
		}
		if !strings.Contains(out, "prod: environment ready") {
			t.Errorf("output:\n%s", out)
		}
	})

	t.Run("explicit --repo skips resolution", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{responses: ghResponses()}
		if _, err := runGithub(t, runner, &staticSource{}, "env-create", "stag", "--repo", "o/r", "--yes"); err != nil {
			t.Fatal(err)
		}
		for _, c := range runner.calls {
			if strings.HasPrefix(c.args, "repo view") {
				t.Error("resolved cwd repo despite explicit --repo")
			}
		}
	})

	t.Run("non-interactive without --yes refuses before any API call", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{responses: ghResponses()}
		if _, err := runGithub(t, runner, &staticSource{}, "env-create", "prod"); err == nil {
			t.Error("want refusal")
		}
		for _, c := range runner.calls {
			if strings.Contains(c.args, "--method PUT") {
				t.Error("wrote despite refusal")
			}
		}
	})
}

func TestPush_SelectionFlagsEndToEnd(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{responses: ghResponses()}
	src := &staticSource{pairs: []dotenv.Pair{{Key: "KEEP", Value: "v"}}}

	// The kit registers the new flags on plugin verbs; they parse and flow
	// into SelectOpts (semantics unit-tested at the Source; this pins wiring).
	if _, err := runGithub(t, runner, src, "push", "--yes", "--exclude-prefix", "X_", "--include", "A", "--exclude", "B"); err != nil {
		t.Fatalf("new selection flags did not parse: %v", err)
	}
}
