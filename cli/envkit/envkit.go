// Package envkit is dotenvctl's operations API: everything the CLI does,
// callable from Go.
//
// Why it exists: the verbs (select, matrix, diff, run, edit) are useful well
// beyond a terminal — a deploy tool wants the selection pipeline, a CI check
// wants the drift matrix, a service wants the child-environment builder. Those
// operations used to live inside cobra RunE closures, reachable only by
// spawning a subprocess. They live here instead, and the cobra layer
// (dotenvcmd) is a thin translator: flags in, envkit call, formatted output.
//
// Contract for everything in this package:
//
//   - Pure-ish functions over typed options and results. No printing, no
//     os.Exit, no cobra — callers decide how to render and how to fail.
//   - Errors are plain wrapped errors. The CLI maps them to exit codes; a
//     library caller inspects them with errors.Is/As.
//   - The env file is opened, parsed, and closed inside each call unless an
//     already-parsed *dotenv.File overload exists (Select vs SelectFile) —
//     so a caller holding a File never pays to re-parse.
//   - This package NEVER touches os.Environ, upholding the underlying
//     library's core promise. Run builds a CHILD environment explicitly.
package envkit

import (
	"fmt"
	"regexp"

	"github.com/ubgo/dotenv"
)

// DefaultPlaceholderPattern marks values that are stand-ins rather than real
// configuration — the `__YOU__` / `__CONFIRM__` convention. Exported because
// callers embedding envkit need the same default the CLI uses, and because a
// caller with a different convention needs to know what it is overriding.
const DefaultPlaceholderPattern = `^__[A-Z0-9_]+__$`

// defaultPlaceholderRe is the compiled default, shared so the common path
// compiles the pattern once per process rather than once per call.
var defaultPlaceholderRe = regexp.MustCompile(DefaultPlaceholderPattern)

// placeholderMatcher resolves an optional caller pattern to a compiled regexp,
// falling back to the shared default. Returns a usable error rather than
// panicking on a bad pattern — the pattern is user input in every caller.
func placeholderMatcher(pattern string) (*regexp.Regexp, error) {
	if pattern == "" || pattern == DefaultPlaceholderPattern {
		return defaultPlaceholderRe, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("envkit: placeholder pattern %q: %w", pattern, err)
	}
	return re, nil
}

// openExisting loads a file that MUST exist.
//
// The underlying library treats a missing file as an empty one (correct for an
// editor that may create it), but for read/compare operations that leniency
// lies: an absent file would read as "everything is missing" instead of "you
// pointed at the wrong path". Callers that want the lenient behavior use
// dotenv.Open directly.
func openExisting(path string) (*dotenv.File, error) {
	f, err := dotenv.Open(path)
	if err != nil {
		return nil, fmt.Errorf("envkit: open %s: %w", path, err)
	}
	if !f.Existed() {
		return nil, fmt.Errorf("envkit: %s does not exist", path)
	}
	return f, nil
}

// effectiveValues returns the file's last-wins view, expanded when asked.
// Expansion failures (an unsatisfied ${VAR:?message}) surface here, before any
// caller side effect — that ordering is the whole point of resolving values up
// front rather than lazily at use.
func effectiveValues(f *dotenv.File, expand bool) (map[string]string, error) {
	if !expand {
		return f.Map(), nil
	}
	m, err := f.ExpandedMap()
	if err != nil {
		return nil, fmt.Errorf("envkit: expand: %w", err)
	}
	return m, nil
}
