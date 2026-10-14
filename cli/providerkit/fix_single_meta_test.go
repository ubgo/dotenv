package providerkit

import (
	"context"
	"testing"

	"github.com/ubgo/dotenv"
)

// TestEmitResults_MetaOnlyUnderJSON pins the kit half of bug 17: Meta feeds
// only the --json payload, so in human mode it must not run at all (each
// call can be a backend lookup), and under --json it runs exactly once.
func TestEmitResults_MetaOnlyUnderJSON(t *testing.T) {
	t.Parallel()
	for _, jsonOut := range []bool{false, true} {
		calls := 0
		cfg := VerbConfig{Meta: func(context.Context) (map[string]string, error) {
			calls++
			return map[string]string{"target": "t"}, nil
		}}
		deps, _ := testDeps(selection([]dotenv.Pair{{Key: "A", Value: "1"}}), false, false)
		deps.Printer.JSON = jsonOut
		c := NewPushCmd(deps, &fakeStore{}, cfg)
		c.SetArgs([]string{"--yes"})
		if err := c.Execute(); err != nil {
			t.Fatal(err)
		}
		want := 0
		if jsonOut {
			want = 1
		}
		if calls != want {
			t.Errorf("json=%t: Meta ran %d times, want %d", jsonOut, calls, want)
		}
	}
}
