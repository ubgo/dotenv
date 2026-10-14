package githubplugin

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ubgo/dotenv/cli/outfmt"
	"github.com/ubgo/dotenv/cli/providerkit"
)

// countCalls counts recorded runner calls whose argv starts with prefix.
func countCalls(r *fakeRunner, prefix string) int {
	n := 0
	for _, c := range r.calls {
		if strings.HasPrefix(c.args, prefix) {
			n++
		}
	}
	return n
}

// TestPush_ResolvesAccountAndTargetOnce pins bug 17: a push ran `gh api user`
// (and `gh repo view`) twice, once for the banner and again for the --json
// metadata, even in human mode where the metadata is never printed. Each
// lookup must run exactly once per invocation, in both modes, for push and
// prune, and again on a second run of the same command tree.
func TestPush_ResolvesAccountAndTargetOnce(t *testing.T) {
	t.Parallel()
	for _, verb := range []string{"push", "prune"} {
		for _, jsonOut := range []bool{false, true} {
			name := verb
			if jsonOut {
				name += " --json"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				runner := &fakeRunner{responses: ghResponses()}
				var out bytes.Buffer
				deps := providerkit.Deps{
					Printer:     &outfmt.Printer{Out: &out, Err: &out, JSON: jsonOut},
					File:        ".env.test",
					Source:      func(providerkit.SelectOpts) (providerkit.Source, error) { return oneKeySrc(), nil },
					Runner:      runner,
					Interactive: func() bool { return false },
					Ask:         func(string) bool { return false },
				}
				c := New().Command(deps)
				for run := 1; run <= 2; run++ {
					c.SetArgs([]string{verb, "--yes"})
					if err := c.Execute(); err != nil {
						t.Fatalf("run %d: %v\n%s", run, err, out.String())
					}
					if got := countCalls(runner, "api user"); got != run {
						t.Errorf("run %d: gh api user ran %d times in total, want %d", run, got, run)
					}
					if got := countCalls(runner, "repo view"); got != run {
						t.Errorf("run %d: gh repo view ran %d times in total, want %d", run, got, run)
					}
				}
				if jsonOut && !strings.Contains(out.String(), `"account":"khanakia (stored gh auth)"`) {
					t.Errorf("--json lost the account meta: %s", out.String())
				}
			})
		}
	}
}
