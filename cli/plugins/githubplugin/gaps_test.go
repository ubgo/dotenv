// Coverage for the gh-failure arms: every place the plugin talks to gh can
// fail (not installed, not authenticated, API refusal), and each arm must
// fail loudly with its own message rather than proceed on a half-resolved
// target. One test per arm, driven by fakeRunner.failOn.
package githubplugin

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/envkit"
	"github.com/ubgo/dotenv/cli/outfmt"
	"github.com/ubgo/dotenv/cli/providerkit"
)

func oneKeySrc() providerkit.Source {
	return &envkit.Selection{Pairs: []dotenv.Pair{{Key: "DEPLOY_PATH", Value: "/srv/app"}}}
}

// runGithubQuiet is runGithub with cobra's own error/usage output captured —
// these tests exercise failure arms, and cobra would otherwise print usage to
// the process stderr and pollute `go test` output.
func runGithubQuiet(t *testing.T, runner *fakeRunner, src providerkit.Source, jsonOut bool, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	deps := providerkit.Deps{
		Printer:     &outfmt.Printer{Out: &out, JSON: jsonOut},
		File:        ".env.test",
		Source:      func(providerkit.SelectOpts) (providerkit.Source, error) { return src, nil },
		Runner:      runner,
		Interactive: func() bool { return false },
		Ask:         func(string) bool { return false },
	}
	c := New().Command(deps)
	c.SetArgs(args)
	c.SetOut(&out)
	c.SetErr(&out)
	err := c.Execute()
	return out.String(), err
}

// The gate resolves target then account before any write; each resolution
// failing must abort push with a pointer to the fix (install gh / auth).
func TestPush_TargetAndAccountFailures(t *testing.T) {
	t.Parallel()

	t.Run("target resolution fails", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{responses: ghResponses(), failOn: "repo view"}
		if _, err := runGithubQuiet(t, runner, oneKeySrc(), false, "push", "--yes"); err == nil {
			t.Error("push proceeded without a resolvable target repo")
		}
	})

	t.Run("account resolution fails", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{responses: ghResponses(), failOn: "api user"}
		if _, err := runGithubQuiet(t, runner, oneKeySrc(), false, "push", "--yes"); err == nil {
			t.Error("push proceeded without knowing the acting account")
		}
	})
}

// list's meta block (target, account) is informational: a failure there
// DOWNGRADES TO OMISSION by design (providerkit verbs.go — the listing
// already happened; hiding it over a label would destroy information). A
// failing name fetch, by contrast, must fail the verb.
func TestList_MetaFailures(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, failOn string }{
		{"target", "repo view"},
		{"account", "api user"},
	} {
		t.Run(tc.name+" failure omits meta, list still succeeds", func(t *testing.T) {
			t.Parallel()
			runner := &fakeRunner{responses: ghResponses(), failOn: tc.failOn}
			out, err := runGithubQuiet(t, runner, oneKeySrc(), true, "list")
			if err != nil {
				t.Fatalf("list must survive a %s-resolution failure: %v", tc.name, err)
			}
			if strings.Contains(out, `"meta"`) {
				t.Errorf("meta block present despite %s resolution failing:\n%s", tc.name, out)
			}
		})
	}

	t.Run("names fetch failing fails the verb", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{responses: ghResponses(), failOn: "secret list"}
		if _, err := runGithubQuiet(t, runner, oneKeySrc(), false, "list"); err == nil {
			t.Error("list succeeded without being able to fetch remote names")
		}
	})
}

// meta's own error returns: whichever half of (target, account) fails first
// propagates, so the verbs' downgrade-to-omission rule has a real error to
// downgrade. Driven directly — the verb path swallows the error by design.
func TestMeta_ResolutionFailures(t *testing.T) {
	t.Parallel()

	t.Run("target fails first", func(t *testing.T) {
		t.Parallel()
		s := &ghStore{deps: providerkit.Deps{Runner: &fakeRunner{responses: ghResponses(), failOn: "repo view"}}}
		if _, err := s.meta(t.Context()); err == nil {
			t.Error("nil error with target resolution failing")
		}
	})

	t.Run("account fails second", func(t *testing.T) {
		t.Parallel()
		s := &ghStore{deps: providerkit.Deps{Runner: &fakeRunner{responses: ghResponses(), failOn: "api user"}}}
		if _, err := s.meta(t.Context()); err == nil {
			t.Error("nil error with account resolution failing")
		}
	})
}

// env-create has two gh calls — repo resolution and the PUT — and each
// failure arm carries its own message so the operator knows which half broke.
func TestEnvCreate_Failures(t *testing.T) {
	t.Parallel()

	t.Run("repo resolution fails", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{responses: ghResponses(), failOn: "repo view"}
		out, err := runGithubQuiet(t, runner, oneKeySrc(), false, "env-create", "prod", "--yes")
		if err == nil || !strings.Contains(out+err.Error(), "resolve repository") {
			t.Errorf("err %v out %q — want a resolve-repository failure", err, out)
		}
	})

	t.Run("PUT fails", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{responses: ghResponses(), failOn: "--method PUT"}
		out, err := runGithubQuiet(t, runner, oneKeySrc(), false, "env-create", "prod", "--yes")
		if err == nil || !strings.Contains(out+err.Error(), "create environment") {
			t.Errorf("err %v out %q — want a create-environment failure", err, out)
		}
	})
}
