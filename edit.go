package dotenv

import (
	"errors"
	"os/exec"
	"strings"
)

// ErrAnchorNotFound is returned by SetAfter and SetBefore when the anchor key
// has no active entry.
//
// A dedicated error rather than a silent append: the whole point of positional
// insertion is WHERE the key lands, so quietly putting it at the end would
// defeat the call while reporting success.
var ErrAnchorNotFound = errors.New("dotenv: anchor key not found")

// shellRun executes a $(command) substitution through sh -c.
//
// Trailing newlines are trimmed because a command's output almost always ends
// with one and almost never wants it: $(whoami) in a connection string must not
// embed a line break. A failing command yields "", matching how a shell treats a
// failed substitution inside a value rather than aborting the whole read.
func shellRun(cmd string) (string, error) {
	out, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

// Pair is a key/value snapshot.
type Pair struct {
	Key   string
	Value string
}

// Get returns the raw value for key.
//
// With duplicate keys the LAST one wins, matching dotenv and Compose semantics:
// what Get reports is what a consumer of the file would actually see.
func (f *File) Get(key string) (string, bool) {
	for i := len(f.entries) - 1; i >= 0; i-- {
		if e := f.entries[i]; e.Kind == KindPair && e.Key == key {
			return e.Value, true
		}
	}
	return "", false
}

// Has reports whether an active pair exists for key. A commented-out entry does
// not count — it is not in effect.
func (f *File) Has(key string) bool {
	_, ok := f.Get(key)
	return ok
}

// Count returns how many ACTIVE pair entries exist for key.
//
// Duplicates are worth surfacing: only the last is effective, so a caller that
// silently ignores the others hides a real authoring mistake.
func (f *File) Count(key string) int {
	n := 0
	for _, e := range f.entries {
		if e.Kind == KindPair && e.Key == key {
			n++
		}
	}
	return n
}

// Keys returns the active keys in file order, without duplicates.
func (f *File) Keys() []string {
	seen := make(map[string]bool, len(f.entries))
	var out []string
	for _, e := range f.entries {
		if e.Kind == KindPair && !seen[e.Key] {
			seen[e.Key] = true
			out = append(out, e.Key)
		}
	}
	return out
}

// Pairs returns the active pairs in file order.
//
// Duplicates appear as-is, so the caller sees the same view a linear read would
// give rather than a silently de-duplicated one.
func (f *File) Pairs() []Pair {
	var out []Pair
	for _, e := range f.entries {
		if e.Kind == KindPair {
			out = append(out, Pair{Key: e.Key, Value: e.Value})
		}
	}
	return out
}

// Map returns the effective key/value view: last occurrence wins, matching Get.
//
// Use this to hand the file to something that wants a map. Prefer Pairs when
// order or duplicates matter.
func (f *File) Map() map[string]string {
	out := make(map[string]string, len(f.entries))
	for _, e := range f.entries {
		if e.Kind == KindPair {
			out[e.Key] = e.Value
		}
	}
	return out
}

// GetExpanded is Get with references resolved against the other entries in the
// same file — the interpolation Docker Compose performs when IT reads the .env.
//
// It exists for consumers that read the file THEMSELVES rather than through
// compose. Without it, a value authored as a reference — ADMIN_PASS=${SECRET} —
// reaches such a consumer as the literal string "${SECRET}" instead of the
// shared secret.
//
// The error is non-nil only for a ${VAR:?message} or ${VAR?message} reference
// whose variable is unsatisfied; see RequiredError. Every other unresolved
// reference expands to "" rather than failing, matching Compose.
//
// Get stays the right call for anything written back to disk: expansion must
// feed what a consumer READS, never what is persisted, or a reference would be
// flattened into a copy of its target and the link lost.
func (f *File) GetExpanded(key string) (string, bool, error) {
	for i := len(f.entries) - 1; i >= 0; i-- {
		e := f.entries[i]
		if e.Kind != KindPair || e.Key != key {
			continue
		}
		v, err := f.expandEntry(e)
		return v, true, err
	}
	return "", false, nil
}

// expandEntry expands a value only when the delimiter it was written with takes
// part in interpolation.
//
// This is why quoteStyle is retained: SECRET='${A}' means the literal text
// ${A}, and expanding it would silently replace a value the author explicitly
// marked as literal — a data-corrupting bug, not a formatting one.
func (f *File) expandEntry(e *Entry) (string, error) {
	v := e.Value
	if e.quote.interpolates() {
		var err error
		v, err = f.expandFor(e.Key, e.Value, 0, visiting{})
		if err != nil {
			return "", err
		}
	}
	return f.transform(e.Key, v)
}

// transform runs the ValueTransformer chain over a value that has already been
// expanded.
//
// Chained: each transformer sees the previous one's output, so a value can be
// dereferenced by one plugin and decoded by another. The result is NOT expanded
// again — a decrypted secret containing ${...} stays literal, which is what a
// secret full of dollar signs wants.
//
// Applied to literal values too. Quoting controls interpolation, not
// transformation: an "encrypted:" value is very often single-quoted precisely to
// keep the parser out of it.
func (f *File) transform(key, value string) (string, error) {
	for i, t := range f.opts.transformers {
		out, err := t.TransformValue(key, value)
		if err != nil {
			name := f.opts.pluginNameFor(CapValueTransformer, i)
			return "", pluginError(name, "transform "+key, err)
		}
		value = out
	}
	return value, nil
}

// ExpandedMap is Map with every value expanded. It stops at the first
// RequiredError rather than returning a half-expanded map.
func (f *File) ExpandedMap() (map[string]string, error) {
	out := make(map[string]string, len(f.entries))
	for _, e := range f.entries {
		if e.Kind != KindPair {
			continue
		}
		v, err := f.expandEntry(e)
		if err != nil {
			return nil, err
		}
		out[e.Key] = v
	}
	return out, nil
}

// Set updates key to value, appending `KEY=value` when the key is absent.
//
// Returns created=true when appended. The editing rules exist to keep diffs
// honest:
//
//   - only the LAST occurrence is updated, because it is the effective one
//   - setting the identical value is a byte-level no-op
//   - everything left of the value (export prefix, spacing, and the `=` or
//     `:` delimiter the author chose) and the inline comment after it are
//     preserved exactly — a colon-delimited pair is never rewritten to `=`
//   - a changed value is re-rendered with canonical quoting, and a value
//     containing newlines becomes a double-quoted multi-line block
func (f *File) Set(key, value string) (created bool) {
	for i := len(f.entries) - 1; i >= 0; i-- {
		e := f.entries[i]
		if e.Kind != KindPair || e.Key != key {
			continue
		}
		if e.Value == value {
			return false // no-op: byte-identical output guaranteed
		}
		e.Value = value
		e.Raw = renderPair(e.prefix, value, e.inlineComment, f.eol())
		return false
	}

	f.Append(NewPair(key, value))
	return true
}

// SetAfter sets key, placing a NEW entry immediately after the anchor key's
// entry.
//
// An existing key is updated in place and NOT moved: relocating it would rewrite
// two regions of the file for a one-value change, and the author put it where it
// is for a reason. created reports whether an entry was added.
//
// Returns ErrAnchorNotFound if the anchor has no active entry, leaving the file
// untouched, so a caller can decide between falling back to Set and treating it
// as a template error.
func (f *File) SetAfter(anchor, key, value string) (created bool, err error) {
	return f.setPositional(anchor, key, value, true)
}

// SetBefore is SetAfter, inserting immediately before the anchor's entry.
func (f *File) SetBefore(anchor, key, value string) (created bool, err error) {
	return f.setPositional(anchor, key, value, false)
}

// setPositional implements SetAfter and SetBefore.
func (f *File) setPositional(anchor, key, value string, after bool) (bool, error) {
	// An existing key is a plain update; position is irrelevant because the
	// entry already has one.
	if f.Has(key) {
		return f.Set(key, value), nil
	}

	if err := f.insert(anchor, []*Entry{NewPair(key, value)}, after); err != nil {
		return false, err
	}
	return true, nil
}

// commentMarker is what Unset prepends. Recorded on the entry so Restore
// removes exactly this and not, say, a "#" the author had already written.
const commentMarker = "# "

// Append adds entries to the end of the file, exactly as given.
//
// Append is LITERAL: it places what you hand it, duplicates included. That is
// the opposite of Set, which upserts to guarantee a single active entry. A
// generator wants literal placement; an editor wants upsert. Blurring the two is
// how a key silently moves.
//
//	f.Append(
//	    dotenv.NewBlank(),
//	    dotenv.NewComment("----------", "DATABASE", "----------"),
//	    dotenv.NewPair("DB_HOST", "localhost"),
//	)
func (f *File) Append(entries ...*Entry) {
	for _, e := range entries {
		f.entries = append(f.entries, f.attach(e))
	}
}

// InsertAfter places entries immediately after the anchor key's entry.
//
// Returns ErrAnchorNotFound and changes nothing when the anchor has no active
// entry. Like Append, this is literal placement — use SetAfter for upsert
// semantics.
func (f *File) InsertAfter(anchorKey string, entries ...*Entry) error {
	return f.insert(anchorKey, entries, true)
}

// InsertBefore is InsertAfter, placing entries immediately before the anchor.
func (f *File) InsertBefore(anchorKey string, entries ...*Entry) error {
	return f.insert(anchorKey, entries, false)
}

// insert implements InsertAfter and InsertBefore.
func (f *File) insert(anchorKey string, entries []*Entry, after bool) error {
	idx := f.indexOfActive(anchorKey)
	if idx < 0 {
		return ErrAnchorNotFound
	}

	at := idx
	if after {
		// Past every physical line of the anchor, so inserting after a
		// multi-line value does not land inside its block.
		at = idx + 1
	}

	attached := make([]*Entry, 0, len(entries))
	for _, e := range entries {
		attached = append(attached, f.attach(e))
	}

	f.entries = append(f.entries, make([]*Entry, len(attached))...)
	copy(f.entries[at+len(attached):], f.entries[at:])
	copy(f.entries[at:], attached)
	return nil
}

// attach renders an entry into this file's line-ending style.
//
// An entry that is already attached — one obtained from Entries — is copied
// verbatim, because its Raw is the source's exact bytes and re-rendering it
// would discard the formatting this package exists to preserve.
func (f *File) attach(e *Entry) *Entry {
	if e.attached {
		clone := *e
		clone.Raw = append([]string(nil), e.Raw...)
		return &clone
	}

	out := *e
	out.attached = true
	switch e.Kind {
	case KindPair:
		out.Raw = renderPair(e.prefix, e.Value, e.inlineComment, f.eol())
	default:
		out.Raw = make([]string, len(e.Raw))
		for i, l := range e.Raw {
			out.Raw[i] = l + f.eol()
		}
	}
	return &out
}

// indexOfActive returns the index of the last ACTIVE entry for key, or -1.
//
// The last, consistent with Get and Set: that is the entry in effect, so it is
// the one a caller means when naming an anchor.
func (f *File) indexOfActive(key string) int {
	for i := len(f.entries) - 1; i >= 0; i-- {
		if e := f.entries[i]; e.Kind == KindPair && e.Key == key {
			return i
		}
	}
	return -1
}

// Disabled returns the commented-out settings in file order.
//
// These are inactive — Get and Map ignore them — but visible, so a tool can
// report "DB_USER is disabled" rather than "DB_USER is missing", which are
// different problems with different fixes.
func (f *File) Disabled() []Pair {
	var out []Pair
	for _, e := range f.entries {
		if e.Kind == KindDisabledPair {
			out = append(out, Pair{Key: e.Key, Value: e.Value})
		}
	}
	return out
}

// Inherited returns the names declared as inherited-from-the-environment — the
// Compose name-only lines (`HOME`) — in file order, deduplicated.
//
// The names are DECLARATIONS, not values: this package never reads os.Environ,
// so resolving them is the caller's job. A caller that wants Compose's
// behaviour walks this list and applies its own source (os.Getenv, a secret
// store) with its own precedence — the same layering the package doc prescribes
// for loading in general.
func (f *File) Inherited() []string {
	seen := make(map[string]bool, len(f.entries))
	var out []string
	for _, e := range f.entries {
		if e.Kind == KindInherited && !seen[e.Key] {
			seen[e.Key] = true
			out = append(out, e.Key)
		}
	}
	return out
}

// Restore re-activates the last disabled entry for key, reporting whether one
// was found. It is the inverse of Unset with del=false.
//
// The exact comment marker recorded when the entry was disabled is removed, so a
// line the author wrote as "#DB_USER=admin" comes back without inventing a space
// that was never there.
//
// A commented-out MULTI-LINE value cannot be restored from a re-read file: each
// of its lines parses as a separate comment, so only the first is recognised as
// a disabled pair. Within one session, where Unset kept the block together,
// Restore reverses it completely.
func (f *File) Restore(key string) bool {
	for i := len(f.entries) - 1; i >= 0; i-- {
		e := f.entries[i]
		if e.Kind != KindDisabledPair || e.Key != key {
			continue
		}
		for j, line := range e.Raw {
			e.Raw[j] = strings.Replace(line, e.commentPrefix, "", 1)
		}
		e.Kind = KindPair
		e.commentPrefix = ""
		return true
	}
	return false
}

// Unset deactivates key, returning whether it was found. Like Set, it targets
// the last occurrence.
//
// With del=false every physical line of the entry is commented out: reversible,
// diff-friendly, and the documentation above it stays attached to something.
// With del=true the entry is removed outright.
func (f *File) Unset(key string, del bool) (found bool) {
	for i := len(f.entries) - 1; i >= 0; i-- {
		e := f.entries[i]
		if e.Kind != KindPair || e.Key != key {
			continue
		}
		if del {
			f.entries = append(f.entries[:i], f.entries[i+1:]...)
			return true
		}
		for j, line := range e.Raw {
			e.Raw[j] = commentMarker + line
		}
		e.Kind = KindDisabledPair
		e.commentPrefix = commentMarker
		return true
	}
	return false
}
