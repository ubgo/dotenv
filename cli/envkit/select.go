package envkit

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ubgo/dotenv"
)

// SelectOptions is the shared selection grammar — which keys, under which
// names, with which values.
//
// Resolution order, deterministic and pinned by tests:
//
//	explicit Keys (as given; prefix filters do not second-guess them)
//	→ else: all active keys → Prefix keeps → ExcludePrefix drops
//	→ IncludeKeys force-add (bypassing the prefix filters)
//	→ ExcludeKeys drop
//	→ StripPrefix renames
//	→ the placeholder/empty guards classify what is left
//
// Invariant every caller depends on: a key the user NAMED (Keys or
// IncludeKeys) that does not exist is an error, never a silent skip —
// reporting success for work that never happened is the failure mode this
// grammar exists to prevent. Excluding an absent key is fine: "make sure X
// never ships" is valid even when X is already gone.
type SelectOptions struct {
	// Keys are explicit key names; empty means every active key.
	Keys []string
	// Prefix keeps only keys with this prefix ("" keeps all).
	Prefix string
	// ExcludePrefix drops keys with this prefix — the inverse selector, for
	// "everything EXCEPT the deploy-routing keys".
	ExcludePrefix string
	// IncludeKeys force-include specific keys even when the prefix filters
	// would drop them.
	IncludeKeys []string
	// ExcludeKeys drop specific keys. Matched against the key's ORIGINAL
	// name, before StripPrefix renaming.
	ExcludeKeys []string
	// StripPrefix removes Prefix from the emitted names. Keys that do not
	// carry the prefix (explicit or force-included) keep their names.
	StripPrefix bool
	// Expand resolves ${references} against the file. Callers pushing values
	// outward normally want this ON — a literal ${DB_HOST} stored in a remote
	// secret is nearly always wrong. Readers normally want it OFF.
	Expand bool
	// IncludePlaceholders lifts the guard that classifies __LIKE_THIS__
	// values as skipped.
	IncludePlaceholders bool
	// IncludeEmpty lifts the guard that classifies empty values as skipped.
	IncludeEmpty bool
	// PlaceholderPattern overrides DefaultPlaceholderPattern. Empty means the
	// default.
	PlaceholderPattern string
}

// SkipReason says why a selected key was held back by a guard. Closed set;
// callers switch on it to report or to override.
type SkipReason string

const (
	// SkipPlaceholder: the value matches the placeholder pattern — configured
	// but still a stand-in.
	SkipPlaceholder SkipReason = "placeholder"
	// SkipEmpty: the value is the empty string.
	SkipEmpty SkipReason = "empty"
)

// Skip is one guarded-out key, reported under its final (post-rename) name so
// callers name it the way the destination would.
type Skip struct {
	Name   string     `json:"name"`
	Reason SkipReason `json:"reason"`
}

// Selection is the resolved result: what to act on, and what was held back.
//
// Skipped is deliberately part of the result rather than swallowed — a caller
// that only sees Pairs cannot tell "nothing matched" from "everything was a
// placeholder", and those need different responses from a human.
type Selection struct {
	Pairs   []dotenv.Pair
	Skipped []Skip
}

// Names returns the selected pair names in order — the common projection for
// banners, confirmations, and set arithmetic.
func (s *Selection) Names() []string {
	out := make([]string, 0, len(s.Pairs))
	for _, p := range s.Pairs {
		out = append(out, p.Key)
	}
	return out
}

// SkippedNames returns the held-back names in order.
func (s *Selection) SkippedNames() []string {
	out := make([]string, 0, len(s.Skipped))
	for _, sk := range s.Skipped {
		out = append(out, sk.Name)
	}
	return out
}

// Map returns the selection as a plain map, last-wins — for callers handing
// values to something that wants a map (template data, an env builder).
func (s *Selection) Map() map[string]string {
	out := make(map[string]string, len(s.Pairs))
	for _, p := range s.Pairs {
		out[p.Key] = p.Value
	}
	return out
}

// SelectFile opens path and applies opts. The file must exist.
func SelectFile(path string, opts SelectOptions) (*Selection, error) {
	f, err := openExisting(path)
	if err != nil {
		return nil, err
	}
	return Select(f, opts)
}

// Select applies the selection grammar to an already-parsed file — the
// overload for callers holding a *dotenv.File who must not pay to re-parse.
func Select(f *dotenv.File, opts SelectOptions) (*Selection, error) {
	placeholderRe, err := placeholderMatcher(opts.PlaceholderPattern)
	if err != nil {
		return nil, err
	}
	values, err := effectiveValues(f, opts.Expand)
	if err != nil {
		return nil, err
	}
	keys, err := selectKeys(f, opts)
	if err != nil {
		return nil, err
	}

	sel := &Selection{}
	for _, key := range keys {
		value := values[key]
		name := key
		if opts.StripPrefix && opts.Prefix != "" {
			name = strings.TrimPrefix(key, opts.Prefix)
		}
		switch {
		case !opts.IncludePlaceholders && placeholderRe.MatchString(value):
			sel.Skipped = append(sel.Skipped, Skip{Name: name, Reason: SkipPlaceholder})
		case !opts.IncludeEmpty && value == "":
			sel.Skipped = append(sel.Skipped, Skip{Name: name, Reason: SkipEmpty})
		default:
			sel.Pairs = append(sel.Pairs, dotenv.Pair{Key: name, Value: value})
		}
	}
	return sel, nil
}

// selectKeys resolves which ORIGINAL key names the selection covers, in file
// order, applying SelectOptions' documented precedence.
func selectKeys(f *dotenv.File, opts SelectOptions) ([]string, error) {
	var keys []string
	switch {
	case len(opts.Keys) > 0:
		for _, k := range opts.Keys {
			if !f.Has(k) {
				return nil, fmt.Errorf("envkit: key %q not found", k)
			}
		}
		keys = slices.Clone(opts.Keys)
	default:
		for _, k := range f.Keys() {
			if opts.Prefix != "" && !strings.HasPrefix(k, opts.Prefix) {
				continue
			}
			if opts.ExcludePrefix != "" && strings.HasPrefix(k, opts.ExcludePrefix) {
				continue
			}
			keys = append(keys, k)
		}
	}

	for _, k := range opts.IncludeKeys {
		if !f.Has(k) {
			return nil, fmt.Errorf("envkit: --include key %q not found", k)
		}
		if !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}

	if len(opts.ExcludeKeys) > 0 {
		kept := keys[:0]
		for _, k := range keys {
			if !slices.Contains(opts.ExcludeKeys, k) {
				kept = append(kept, k)
			}
		}
		keys = kept
	}
	return keys, nil
}
