// Coverage for Names' runner-failure arm: `vercel env ls` failing must fail
// the listing — prune computes deletions from Names, so a silently empty
// answer would mark every remote name stale.
package vercelplugin

import (
	"context"
	"testing"

	"github.com/ubgo/dotenv/cli/providerkit"
)

func TestNames_RunnerFailure(t *testing.T) {
	t.Parallel()
	s := &vercelStore{deps: providerkit.Deps{Runner: &fakeRunner{failOn: "env ls"}}, targets: []string{"production"}}
	if _, err := s.Names(context.Background()); err == nil {
		t.Error("nil error when `vercel env ls` fails — Names must not report an empty store")
	}
}
