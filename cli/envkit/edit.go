package envkit

import (
	"errors"
	"fmt"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/textdiff"
)

// EditOptions configures the mutating operations.
type EditOptions struct {
	// DryRun computes the result and returns the would-be diff WITHOUT
	// writing. Every mutating operation supports it; nothing else about the
	// call changes, so a preview and the real run cannot disagree.
	DryRun bool
	// Anchor places a NEW key immediately after (or before, with
	// AnchorBefore) this existing key. An EXISTING key is updated in place
	// and never moved — relocating it would rewrite two regions of the file
	// for a one-value change. Set only for Set.
	Anchor string
	// AnchorBefore flips Anchor's side.
	AnchorBefore bool
	// Delete makes Unset remove entries outright instead of commenting them
	// out. Comment-out is the default because it is reversible and keeps the
	// documentation above a setting attached to something.
	Delete bool
}

// EditResult reports what an edit did.
type EditResult struct {
	// Changed is false when the operation was a byte-level no-op — setting a
	// key to its current value. The file is then not rewritten at all, so its
	// mtime is untouched and watchers see no phantom event.
	Changed bool
	// Written is true when the file was actually saved (false for DryRun and
	// for no-ops).
	Written bool
	// Diff is the minimal `-old`/`+new` line diff of the change, always
	// computed — a caller can log it, show it, or ignore it.
	Diff []string
	// Created lists keys that did not exist and were added.
	Created []string
	// Affected lists every key the operation touched, in order.
	Affected []string
}

// ErrAnchorNotFound is returned when EditOptions.Anchor names a key with no
// active entry. A dedicated error rather than a silent append: the whole point
// of anchoring is WHERE the key lands, so quietly putting it at the end would
// defeat the call while reporting success.
var ErrAnchorNotFound = errors.New("envkit: anchor key not found")

// ErrKeyNotFound is returned when an operation names a key that has no
// matching entry (Unset on an absent key, Restore with nothing disabled).
var ErrKeyNotFound = errors.New("envkit: key not found")

// Set upserts pairs into the file at path, preserving every byte outside the
// entries it touches — comments, blank lines, ordering, quoting style, and the
// author's `=` vs `:` delimiter all survive.
//
// A missing file is CREATED (0600, because env files hold credentials).
// Setting a key to its current value is a true no-op: same bytes, same mtime.
func Set(path string, pairs []dotenv.Pair, opts EditOptions) (*EditResult, error) {
	if opts.Anchor != "" && len(pairs) == 0 {
		return nil, fmt.Errorf("envkit: set: no pairs given")
	}

	f, err := dotenv.Open(path)
	if err != nil {
		return nil, fmt.Errorf("envkit: open %s: %w", path, err)
	}
	before := f.Render()

	res := &EditResult{}
	for _, kv := range pairs {
		created, err := applySet(f, kv, opts)
		if err != nil {
			return nil, err
		}
		if created {
			res.Created = append(res.Created, kv.Key)
		}
		res.Affected = append(res.Affected, kv.Key)
	}
	return finish(f, before, res, opts)
}

// applySet routes one assignment through the plain or anchored setter.
func applySet(f *dotenv.File, kv dotenv.Pair, opts EditOptions) (bool, error) {
	switch {
	case opts.Anchor != "" && opts.AnchorBefore:
		created, err := f.SetBefore(opts.Anchor, kv.Key, kv.Value)
		if errors.Is(err, dotenv.ErrAnchorNotFound) {
			return false, fmt.Errorf("%w: %q", ErrAnchorNotFound, opts.Anchor)
		}
		return created, err
	case opts.Anchor != "":
		created, err := f.SetAfter(opts.Anchor, kv.Key, kv.Value)
		if errors.Is(err, dotenv.ErrAnchorNotFound) {
			return false, fmt.Errorf("%w: %q", ErrAnchorNotFound, opts.Anchor)
		}
		return created, err
	default:
		return f.Set(kv.Key, kv.Value), nil
	}
}

// Unset deactivates keys — commenting them out by default (reversible via
// Restore) or removing them with EditOptions.Delete.
func Unset(path string, keys []string, opts EditOptions) (*EditResult, error) {
	f, err := openExisting(path)
	if err != nil {
		return nil, err
	}
	before := f.Render()

	res := &EditResult{}
	for _, key := range keys {
		if !f.Unset(key, opts.Delete) {
			return nil, fmt.Errorf("%w: %q", ErrKeyNotFound, key)
		}
		res.Affected = append(res.Affected, key)
	}
	return finish(f, before, res, opts)
}

// Restore re-activates commented-out settings — the inverse of Unset. The
// exact comment marker recorded when the entry was disabled is removed, so a
// line written as "#KEY=v" comes back without inventing a space.
func Restore(path string, keys []string, opts EditOptions) (*EditResult, error) {
	f, err := openExisting(path)
	if err != nil {
		return nil, err
	}
	before := f.Render()

	res := &EditResult{}
	for _, key := range keys {
		if !f.Restore(key) {
			return nil, fmt.Errorf("%w: no disabled entry for %q", ErrKeyNotFound, key)
		}
		res.Affected = append(res.Affected, key)
	}
	return finish(f, before, res, opts)
}

// finish computes the diff and saves when the render actually changed —
// extending the library's no-op invariant to the filesystem.
func finish(f *dotenv.File, before string, res *EditResult, opts EditOptions) (*EditResult, error) {
	res.Diff = textdiff.Lines(before, f.Render())
	res.Changed = res.Diff != nil
	if opts.DryRun || !res.Changed {
		return res, nil
	}
	if err := f.Save(); err != nil {
		return nil, fmt.Errorf("envkit: save %s: %w", f.Path, err)
	}
	res.Written = true
	return res, nil
}
