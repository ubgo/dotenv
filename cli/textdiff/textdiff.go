// Package textdiff renders the minimal line diff a --dry-run preview needs.
//
// Why not a real diff library: dotenv edits are line-local by construction —
// the library changes exactly one entry (or appends) — so trimming the common
// prefix and suffix of the two renderings isolates the changed region without
// an LCS. A dependency-free ~40-line helper beats pulling a diff module into
// the CLI for output that is only ever a preview, never applied.
package textdiff

import "strings"

// Lines returns unified-style "-old"/"+new" lines for the region where before
// and after differ, or nil when they are byte-identical.
//
// Invariant: nil means "no change" — callers use that to report a true no-op
// (e.g. `set KEY=<current value>`) rather than printing an empty diff.
func Lines(before, after string) []string {
	if before == after {
		return nil
	}

	b := strings.Split(before, "\n")
	a := strings.Split(after, "\n")

	// Trim the shared prefix.
	p := 0
	for p < len(b) && p < len(a) && b[p] == a[p] {
		p++
	}

	// Trim the shared suffix, never crossing the prefix boundary — without the
	// bound, a line that belongs to both regions would be claimed twice and
	// the middle would go negative on inputs like "x" → "x\nx".
	s := 0
	for s < len(b)-p && s < len(a)-p && b[len(b)-1-s] == a[len(a)-1-s] {
		s++
	}

	out := make([]string, 0, (len(b)-p-s)+(len(a)-p-s))
	for _, l := range b[p : len(b)-s] {
		out = append(out, "-"+l)
	}
	for _, l := range a[p : len(a)-s] {
		out = append(out, "+"+l)
	}
	return out
}
