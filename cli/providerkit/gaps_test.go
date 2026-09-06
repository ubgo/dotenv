// Coverage for prune's Source error arm: when the selection itself cannot be
// resolved (a named key missing, an unreadable file), prune must fail before
// computing a keep-set — a delete verb running on a wrong selection is the
// one failure with no undo.
package providerkit

import (
	"bytes"
	"errors"
	"testing"

	"github.com/ubgo/dotenv/cli/outfmt"
)

func TestPrune_SourceError(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	deps := Deps{
		Printer:     &outfmt.Printer{Out: &out},
		File:        ".env.test",
		Source:      func(SelectOpts) (Source, error) { return nil, errors.New("selection exploded") },
		Runner:      ExecRunner{},
		Interactive: func() bool { return false },
		Ask:         func(string) bool { return false },
	}
	store := &fakeStore{}
	c := NewPruneCmd(deps, store, store, VerbConfig{})
	c.SetArgs([]string{"--yes"})
	c.SetOut(&out)
	c.SetErr(&out)
	if err := c.Execute(); err == nil {
		t.Error("nil error when the selection cannot be resolved — prune must not proceed")
	}
	if len(store.deletes) != 0 {
		t.Errorf("prune deleted %v despite a failed selection", store.deletes)
	}
}
