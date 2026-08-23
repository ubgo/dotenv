package dotenvcmd

import (
	"slices"
	"testing"
)

func TestMergeEnv(t *testing.T) {
	t.Parallel()

	base := []string{"KEEP=parent", "OVERRIDE=parent", "NOEQ_MALFORMED"}
	got := mergeEnv(base, map[string]string{"OVERRIDE": "file", "NEW": "file"})
	slices.Sort(got)

	want := []string{"KEEP=parent", "NEW=file", "NOEQ_MALFORMED", "OVERRIDE=file"}
	if !slices.Equal(got, want) {
		t.Errorf("mergeEnv = %v, want %v", got, want)
	}

	// No duplicate names may survive — tools reading the raw environ block
	// (env | sort) must see each name once.
	seen := map[string]bool{}
	for _, e := range got {
		if seen[e] {
			t.Errorf("duplicate entry %q", e)
		}
		seen[e] = true
	}
}

func TestBuildDiff(t *testing.T) {
	t.Parallel()

	p := buildDiff(
		map[string]string{"A": "1", "B": "2", "C": "3"},
		map[string]string{"B": "2", "C": "9", "D": "4"},
		false,
	)

	if len(p.Added) != 1 || p.Added[0].Key != "D" {
		t.Errorf("Added = %v", p.Added)
	}
	if len(p.Removed) != 1 || p.Removed[0].Key != "A" {
		t.Errorf("Removed = %v", p.Removed)
	}
	if len(p.Changed) != 1 || p.Changed[0].Key != "C" || p.Changed[0].From != "3" || p.Changed[0].To != "9" {
		t.Errorf("Changed = %v", p.Changed)
	}

	empty := buildDiff(map[string]string{}, map[string]string{}, true)
	if empty.Added == nil || empty.Removed == nil || empty.Changed == nil {
		t.Error("sections must be empty arrays, never nil — the JSON schema promises arrays")
	}
	if !empty.Expanded {
		t.Error("Expanded flag not carried through")
	}
}
