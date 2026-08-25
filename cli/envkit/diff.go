package envkit

import "sort"

// DiffPair is one added or removed key.
type DiffPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// DiffChange is one key whose value differs between the two files.
type DiffChange struct {
	Key  string `json:"key"`
	From string `json:"from"`
	To   string `json:"to"`
}

// DiffResult is the effective-configuration comparison of two files.
//
// All three slices are non-nil (empty rather than null) so JSON consumers
// never need a null guard, and all three are key-sorted so output is
// deterministic and itself diffable.
type DiffResult struct {
	// Added: keys present in B but not A.
	Added []DiffPair `json:"added"`
	// Removed: keys present in A but not B.
	Removed []DiffPair `json:"removed"`
	// Changed: keys in both with different values.
	Changed []DiffChange `json:"changed"`
	// Expanded records whether values were compared after ${reference}
	// resolution.
	Expanded bool `json:"expanded"`
	// FileA and FileB are the compared paths, for report headers.
	FileA string `json:"file_a"`
	FileB string `json:"file_b"`
}

// Empty reports whether the two files are equivalent — the question callers
// actually ask (a CI gate, an exit code), so nobody re-derives it by summing
// three lengths.
func (d *DiffResult) Empty() bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Changed) == 0
}

// DiffOptions configures Diff.
type DiffOptions struct {
	// Expand compares resolved values instead of raw text, so two files that
	// spell the same value differently (`${HOST}` vs the host) compare equal.
	Expand bool
}

// Diff compares the EFFECTIVE view of two env files: last-wins maps, the same
// thing a consumer of each file would see.
//
// Formatting, comments, and shadowed duplicates are invisible on purpose —
// byte-level diffing is what diff(1) is for; this answers "does the
// configuration differ". Both files must exist: an absent file would read as
// "everything was removed", which is a lie about the filesystem rather than a
// comparison.
func Diff(pathA, pathB string, opts DiffOptions) (*DiffResult, error) {
	mapA, err := effectiveMapOf(pathA, opts.Expand)
	if err != nil {
		return nil, err
	}
	mapB, err := effectiveMapOf(pathB, opts.Expand)
	if err != nil {
		return nil, err
	}
	d := DiffMaps(mapA, mapB, opts.Expand)
	d.FileA, d.FileB = pathA, pathB
	return d, nil
}

// DiffMaps compares two already-resolved maps — the pure core, exported for
// callers who obtained their values some other way (a remote store, a
// template) and want the same comparison semantics.
func DiffMaps(mapA, mapB map[string]string, expanded bool) *DiffResult {
	d := &DiffResult{
		Added:    []DiffPair{},
		Removed:  []DiffPair{},
		Changed:  []DiffChange{},
		Expanded: expanded,
	}

	for k, vb := range mapB {
		va, inA := mapA[k]
		switch {
		case !inA:
			d.Added = append(d.Added, DiffPair{Key: k, Value: vb})
		case va != vb:
			d.Changed = append(d.Changed, DiffChange{Key: k, From: va, To: vb})
		}
	}
	for k, va := range mapA {
		if _, inB := mapB[k]; !inB {
			d.Removed = append(d.Removed, DiffPair{Key: k, Value: va})
		}
	}

	sort.Slice(d.Added, func(i, j int) bool { return d.Added[i].Key < d.Added[j].Key })
	sort.Slice(d.Removed, func(i, j int) bool { return d.Removed[i].Key < d.Removed[j].Key })
	sort.Slice(d.Changed, func(i, j int) bool { return d.Changed[i].Key < d.Changed[j].Key })
	return d
}

// effectiveMapOf loads one file and returns its last-wins view.
func effectiveMapOf(path string, expand bool) (map[string]string, error) {
	f, err := openExisting(path)
	if err != nil {
		return nil, err
	}
	return effectiveValues(f, expand)
}
