package envkit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// envAssignSep joins name and value in a child environment entry.
const envAssignSep = "="

// RunOptions configures Run.
type RunOptions struct {
	// Base is the environment the file's values are overlaid ONTO. Pass
	// os.Environ() for the usual "inherit my environment" behavior, or a
	// curated slice for a hermetic child. Nil means an empty base — the
	// child then sees ONLY the file's values.
	Base []string
	// Select narrows which of the file's keys reach the child. The zero
	// value takes every active key with expansion ON (the sane default for
	// execution: a child cannot resolve ${refs} itself).
	Select *SelectOptions
	// Stdin, Stdout, Stderr are the child's streams; nil means the parent's.
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	// Dir is the child's working directory ("" means the parent's).
	Dir string
}

// ChildEnv builds the environment a child process should receive: Base
// overlaid with the file's selected values, FILE WINS.
//
// File-wins is the whole point of running a command through an env file — the
// caller is deliberately imposing configuration. Duplicate names are removed
// from the base rather than appended after it: os/exec would honor the last
// entry either way, but a duplicated name confuses anything that reads the
// raw environ block (`env | sort` inside the child).
//
// This function does NOT read or modify os.Environ — the caller passes the
// base explicitly, upholding the library's never-touch-the-environment rule.
func ChildEnv(path string, base []string, sel *SelectOptions) ([]string, error) {
	opts := SelectOptions{Expand: true}
	if sel != nil {
		opts = *sel
	}
	selection, err := SelectFile(path, opts)
	if err != nil {
		return nil, err
	}
	return MergeEnv(base, selection.Map()), nil
}

// MergeEnv overlays values onto a base environ slice, values winning. Pure —
// exported for callers who already hold the values.
func MergeEnv(base []string, values map[string]string) []string {
	replaced := make(map[string]bool, len(values))
	for k := range values {
		replaced[k] = true
	}

	out := make([]string, 0, len(base)+len(values))
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, envAssignSep)
		if ok && replaced[name] {
			continue
		}
		out = append(out, entry)
	}
	for k, v := range values {
		out = append(out, k+envAssignSep+v)
	}
	return out
}

// Run executes argv with the file's values in the child's environment and
// returns the child's EXIT CODE.
//
// The exit code is returned rather than turned into an error because a
// non-zero child is a normal outcome a caller must be able to pass through
// verbatim (that is what makes a wrapper transparent to CI). err is non-nil
// only for OUR failures: the file could not be read or expanded, or the child
// could not be started at all.
func Run(ctx context.Context, path string, argv []string, opts RunOptions) (int, error) {
	if len(argv) == 0 {
		return 0, fmt.Errorf("envkit: run: no command given")
	}

	env, err := ChildEnv(path, opts.Base, opts.Select)
	if err != nil {
		return 0, err
	}

	child := exec.CommandContext(ctx, argv[0], argv[1:]...)
	child.Env = env
	child.Dir = opts.Dir
	child.Stdin = orDefaultReader(opts.Stdin, os.Stdin)
	child.Stdout = orDefaultWriter(opts.Stdout, os.Stdout)
	child.Stderr = orDefaultWriter(opts.Stderr, os.Stderr)

	if err := child.Run(); err != nil {
		var xerr *exec.ExitError
		if errors.As(err, &xerr) {
			// The child ran and failed — its code is the answer, not an error.
			return xerr.ExitCode(), nil
		}
		return 0, fmt.Errorf("envkit: run %s: %w", argv[0], err)
	}
	return 0, nil
}

// orDefaultReader / orDefaultWriter keep the nil-means-inherit contract in one
// place instead of three nil checks inline.
func orDefaultReader(r, fallback io.Reader) io.Reader {
	if r == nil {
		return fallback
	}
	return r
}

func orDefaultWriter(w, fallback io.Writer) io.Writer {
	if w == nil {
		return fallback
	}
	return w
}
