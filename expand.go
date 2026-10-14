package dotenv

import (
	"fmt"
	"strings"
)

// ds:def id=maxexpanddepth-hr5dfhv2 owner=@maintainers stability=stable

// maxExpandDepth bounds how deep a chain of references may nest.
//
// Real chains are one hop deep — a per-service variable pointing at a shared
// global — so 32 is astronomically generous.
//
// Depth alone is NOT enough to guarantee termination. A value that references
// itself twice, such as A=$A$A, branches at every level: bounded at depth 32
// that is still 2^32 expansions, which hangs rather than returns. Cycles are
// therefore broken by name via the visiting set below, and the depth bound is
// only a backstop for deep-but-acyclic chains.
const maxExpandDepth = 32

// visiting tracks the variables currently being expanded, so a reference that
// re-enters one already on the stack is cut immediately instead of unfolding.
//
// Each name maps to WHERE its value came from: the index of the entry being
// expanded, or atLookup when a Lookuper supplied it. The index is what gives a
// re-entering reference Docker Compose's answer. Compose reads a .env file top
// to bottom and expands each line against the values read so far, so in
// `A=1` then `A=${A}2` the reference sees the definition ABOVE it ("12"), and
// in `ZONE=${ZONE:-eu}` it sees nothing in the file and asks the lookup
// function, then takes the default. A re-entering reference here therefore
// resolves to the last definition above the entry on the stack, then to the
// Lookupers, then to unset — see source. The starting key is on the stack from
// the first call, so a cycle back to it is cut on its first return.
type visiting map[string]int

// atLookup marks a name on the stack whose value a Lookuper supplied rather
// than a file entry. A reference back into it has nothing earlier to fall back
// to and is unset; asking the Lookuper again would loop.
const atLookup = -1

// ds:def id=visitingenter-r4jp8kdd owner=@maintainers stability=stable

// enter records that name is being expanded from at (an entry index or
// atLookup) and returns the function that restores the previous state. A name
// already on the stack is re-pointed rather than refused: source only lets the
// index move strictly up the file, so every re-entry chain ends.
func (v visiting) enter(name string, at int) (leave func()) {
	prev, had := v[name]
	v[name] = at
	return func() {
		if had {
			v[name] = prev
		} else {
			delete(v, name)
		}
	}
}

// RequiredError reports a ${VAR:?message} or ${VAR?message} reference whose
// variable was not satisfied.
//
// These forms exist precisely to fail loudly: silently yielding "" would defeat
// the only reason an author writes one.
type RequiredError struct {
	// Key is the referenced variable that was unset or empty.
	Key string
	// Message is the text after the operator. Empty when the author wrote none.
	Message string
	// Empty distinguishes ":?" (unset OR empty) from "?" (unset only).
	Empty bool
}

func (e *RequiredError) Error() string {
	reason := "is not set"
	if e.Empty {
		reason = "is not set or is empty"
	}
	if e.Message == "" {
		return fmt.Sprintf("dotenv: required variable %s %s", e.Key, reason)
	}
	return fmt.Sprintf("dotenv: required variable %s %s: %s", e.Key, reason, e.Message)
}

// operator is the modifier following a variable name inside ${...}.
type operator int

// ds:def id=operators-2jt6akwa owner=@maintainers stability=stable

const (
	opNone         operator = iota // ${VAR}
	opDefault                      // ${VAR-default}   default when unset
	opDefaultEmpty                 // ${VAR:-default}  default when unset or empty
	opAlternate                    // ${VAR+alt}       alt when set
	opAlternateSet                 // ${VAR:+alt}      alt when set and non-empty
	opRequired                     // ${VAR?err}       error when unset
	opRequiredSet                  // ${VAR:?err}      error when unset or empty
)

// varRef is a parsed ${...} reference.
type varRef struct {
	varName string
	op      operator
	// arg is the text after the operator: a default, an alternate, or an error
	// message, depending on op.
	arg string
}

// parseVarToken splits the interior of a ${...} reference — or the name after a
// bare $ — into a variable name and its operator.
//
// Colon-prefixed forms are tested first because all six operators share their
// trailing character; checking ":-" before "-" is what keeps ${VAR:-d} from
// parsing as a "-" default named ":-d". Variable names contain none of
// ':', '-', '+', or '?', so the first such run unambiguously begins the operator.
func parseVarToken(tok string) varRef {
	for _, o := range []struct {
		sep string
		op  operator
	}{
		{":-", opDefaultEmpty},
		{":+", opAlternateSet},
		{":?", opRequiredSet},
		{"-", opDefault},
		{"+", opAlternate},
		{"?", opRequired},
	} {
		if n, a, ok := strings.Cut(tok, o.sep); ok {
			return varRef{varName: n, op: o.op, arg: a}
		}
	}
	return varRef{varName: tok}
}

// expand resolves $ references in s.
//
// Hand-written rather than delegating to os.Expand, which cannot express two
// things Compose requires: `$$` as a literal dollar, and nested references such
// as ${VAR:-${FALLBACK:-x}} — os.Expand stops at the FIRST closing brace, so the
// inner default swallows the outer one.
// expandFor is expand, carrying the key whose value is being expanded so an
// ExpandObserver can report which entry a reference came from.
func (f *File) expandFor(key, s string, depth int, seen visiting) (string, error) {
	return f.expand(key, s, depth, seen)
}

func (f *File) expand(key, s string, depth int, seen visiting) (string, error) {
	if depth >= maxExpandDepth || !strings.ContainsRune(s, '$') {
		return s, nil
	}

	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '$' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 >= len(s) {
			b.WriteByte('$') // a trailing $ is just a dollar sign
			continue
		}

		switch next := s[i+1]; {
		case next == '$':
			// $$ is the documented escape for a literal dollar.
			b.WriteByte('$')
			i++

		case next == '(':
			consumed, out, err := f.expandCommand(key, s[i:], depth, seen)
			if err != nil {
				return "", err
			}
			b.WriteString(out)
			i += consumed - 1

		case next == '{':
			end := matchBrace(s[i+1:])
			if end < 0 {
				// Unbalanced: emit verbatim rather than guessing where the
				// author meant it to close.
				b.WriteByte('$')
				continue
			}
			out, err := f.resolve(key, s[i+2:i+1+end], depth, seen)
			if err != nil {
				return "", err
			}
			b.WriteString(out)
			i += end + 1

		case isNameStart(next):
			n := 1
			for i+1+n < len(s) && isNameChar(s[i+1+n]) {
				n++
			}
			out, err := f.resolve(key, s[i+1:i+1+n], depth, seen)
			if err != nil {
				return "", err
			}
			b.WriteString(out)
			i += n

		default:
			// $ followed by anything else is a literal dollar.
			b.WriteByte('$')
		}
	}
	return b.String(), nil
}

// ds:def id=expandcommand-a4tbu6wa owner=@maintainers stability=stable

// expandCommand handles a $(command) substitution starting at s[0].
//
// Returns how many bytes of s it consumed. When substitution is disabled the
// text is emitted verbatim, so a value survives a round trip unchanged rather
// than being silently blanked.
func (f *File) expandCommand(key, s string, depth int, seen visiting) (consumed int, out string, err error) {
	end := matchParen(s[1:])
	if end < 0 {
		return 1, "$", nil // unbalanced: a literal dollar
	}
	whole := s[:end+2]
	if !f.opts.allowShell {
		return len(whole), whole, nil
	}

	// The command may itself reference variables.
	cmd, err := f.expand(key, s[2:end+1], depth+1, seen)
	if err != nil {
		return 0, "", err
	}
	result, runErr := f.opts.runCommand(cmd)
	if runErr != nil {
		// A failed substitution yields "", as a shell does inside a value,
		// rather than failing the entire read.
		return len(whole), "", nil
	}
	return len(whole), result, nil
}

// varSource is where a referenced variable's value came from.
type varSource struct {
	name  string
	value string
	// set is false when neither the file nor a Lookuper knows the name.
	set bool
	// at is the entry index the value came from, or atLookup.
	at int
}

// resolve evaluates one reference token against the file's own entries.
func (f *File) resolve(key, tok string, depth int, seen visiting) (string, error) {
	ref := parseVarToken(tok)

	src, err := f.source(ref.varName, seen)
	if err != nil {
		return "", err
	}
	hasValue := src.set && src.value != ""

	// useArg picks the operator's argument (a default or an alternate) over the
	// variable's own value. The argument is part of the REFERENCING value, so
	// it is expanded in the referencing context, not as the variable.
	useArg := false
	switch ref.op {
	case opDefault:
		useArg = !src.set
	case opDefaultEmpty:
		useArg = !hasValue
	case opAlternate:
		if !src.set {
			return f.observed(key, ref.varName, "", nil)
		}
		useArg = true
	case opAlternateSet:
		if !hasValue {
			return f.observed(key, ref.varName, "", nil)
		}
		useArg = true
	case opRequired:
		if !src.set {
			return "", &RequiredError{Key: ref.varName, Message: ref.arg}
		}
	case opRequiredSet:
		if !hasValue {
			return "", &RequiredError{Key: ref.varName, Message: ref.arg, Empty: true}
		}
	}

	// An unresolved reference with no operator expands to "", never an error —
	// Compose treats an unset variable as empty.
	var out string
	if useArg {
		out, err = f.expand(key, ref.arg, depth+1, seen)
	} else {
		out, err = f.expandSource(key, src, depth, seen)
	}
	return f.observed(key, ref.varName, out, err)
}

// expandSource expands a variable's own value with the variable on the stack,
// so a reference back into it is recognised as re-entry.
func (f *File) expandSource(key string, src varSource, depth int, seen visiting) (string, error) {
	if !src.set {
		return "", nil
	}
	leave := seen.enter(src.name, src.at)
	defer leave()
	return f.expand(key, src.value, depth+1, seen)
}

// source finds a variable's value: the file first, then any installed
// Lookuper.
//
// The file wins. Adding an environment fallback or a secret store must not
// silently override a value somebody wrote down. (Compose's own lookup runs the
// other way round, process environment first; this package never reads the
// environment, and a Lookuper is a fallback by contract.)
//
// Which file entry: the LAST definition of name, matching Get — except while
// name is already being expanded. Then the reference is a self-reference or a
// cycle, and it sees only the definitions ABOVE the entry on the stack, as in
// Compose's top-to-bottom read; with none above, it falls through to the
// Lookupers and then to unset. See visiting.
//
// A Lookuper error aborts expansion rather than being treated as a miss: a
// secret store that is unreachable must not look like a variable that is unset,
// or a deploy quietly proceeds with an empty password.
func (f *File) source(name string, seen visiting) (varSource, error) {
	below := len(f.entries)
	if at, onStack := seen[name]; onStack {
		if at == atLookup {
			return varSource{name: name}, nil
		}
		below = at
	}
	for i := below - 1; i >= 0; i-- {
		if e := f.entries[i]; e.Kind == KindPair && e.Key == name {
			return varSource{name: name, value: e.Value, set: true, at: i}, nil
		}
	}

	for i, l := range f.opts.lookupers {
		v, ok, err := l.Lookup(name)
		if err != nil {
			return varSource{}, pluginError(f.opts.pluginNameFor(CapLookup, i), "lookup "+name, err)
		}
		if ok {
			// First to answer wins.
			return varSource{name: name, value: v, set: true, at: atLookup}, nil
		}
	}
	return varSource{name: name}, nil
}

// observed reports a completed resolution to every ExpandObserver, then passes
// the result through unchanged.
//
// Wrapping the return keeps the reporting at one site rather than at each of the
// operator branches, where one would eventually be forgotten.
func (f *File) observed(key, name, resolved string, err error) (string, error) {
	if err != nil || len(f.opts.expandObs) == 0 {
		return resolved, err
	}
	for _, o := range f.opts.expandObs {
		o.ObserveExpand(key, name, resolved)
	}
	return resolved, nil
}

// matchBrace returns the index of the `}` closing the `{` at s[0], accounting
// for nesting, or -1 when unbalanced.
//
// Nesting is the whole reason this exists: ${A:-${B:-c}} needs the OUTER brace,
// and any scan for the first `}` finds the inner one.
func matchBrace(s string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// matchParen returns the index of the `)` closing the `(` at s[0], accounting
// for nesting, or -1 when unbalanced.
func matchParen(s string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// isNameStart reports whether c may begin a variable name.
func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isNameChar reports whether c may continue a variable name.
func isNameChar(c byte) bool {
	return isNameStart(c) || (c >= '0' && c <= '9')
}
