package vercelplugin

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/envkit"
	"github.com/ubgo/dotenv/cli/outfmt"
	"github.com/ubgo/dotenv/cli/providerkit"
)

// TestPush_ResolvesAccountOnce pins bug 17 for Vercel: `vercel whoami` must
// run once per invocation (banner and --json metadata share it), in both
// modes, for push and prune, and again on a second run of the same tree.
func TestPush_ResolvesAccountOnce(t *testing.T) {
	t.Parallel()
	src := &envkit.Selection{Pairs: []dotenv.Pair{{Key: "DB_URL", Value: "postgres://x"}}}
	for _, verb := range []string{"push", "prune"} {
		for _, jsonOut := range []bool{false, true} {
			name := verb
			if jsonOut {
				name += " --json"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				runner := &fakeRunner{responses: vercelResponses()}
				var out bytes.Buffer
				deps := providerkit.Deps{
					Printer:     &outfmt.Printer{Out: &out, Err: &out, JSON: jsonOut},
					File:        ".env.test",
					Source:      func(providerkit.SelectOpts) (providerkit.Source, error) { return src, nil },
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
					n := 0
					for _, call := range runner.calls {
						if strings.HasPrefix(call.args, "whoami") {
							n++
						}
					}
					if n != run {
						t.Errorf("run %d: vercel whoami ran %d times in total, want %d", run, n, run)
					}
				}
				if jsonOut && !strings.Contains(out.String(), `"account":"khanakia (stored vercel auth)"`) {
					t.Errorf("--json lost the account meta: %s", out.String())
				}
			})
		}
	}
}
