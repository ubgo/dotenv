package providerkit

import (
	"context"
	"testing"
)

// hintingStore is a lister that supplies its own empty-list hint.
type hintingStore struct{ fakeStore }

func (hintingStore) EmptyListHint() string { return "no secrets here — try --scope" }

// TestList_EmptyHintComesFromThePlugin pins bug 14: the kit's empty-store
// hint named GitHub's --environment and --org for every plugin. The default
// is now neutral, and a lister that implements EmptyListHinter supplies its
// own backend-specific nudge.
func TestList_EmptyHintComesFromThePlugin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		store SecretLister
		want  string
	}{
		{"neutral default", &fakeStore{}, DefaultEmptyListHint + "\n"},
		{"plugin hint", &hintingStore{}, "no secrets here — try --scope\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			deps, out := testDeps(selection(nil), true, true)
			c := NewListCmd(deps, tt.store)
			c.SetArgs([]string{})
			if err := c.ExecuteContext(context.Background()); err != nil {
				t.Fatal(err)
			}
			if out.String() != tt.want {
				t.Errorf("out = %q, want %q", out.String(), tt.want)
			}
		})
	}
}

// TestList_EmptyHintIsHumanOnly pins that the hint never pollutes --json: an
// empty store is an empty names array, nothing else.
func TestList_EmptyHintIsHumanOnly(t *testing.T) {
	t.Parallel()
	deps, out := testDeps(selection(nil), true, true)
	deps.Printer.JSON = true
	c := NewListCmd(deps, &hintingStore{})
	c.SetArgs([]string{})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if want := `{"ok":true,"data":{"names":[]}}` + "\n"; out.String() != want {
		t.Errorf("out = %q, want %q", out.String(), want)
	}
}
