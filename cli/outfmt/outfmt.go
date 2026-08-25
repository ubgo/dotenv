// Package outfmt is the single output seam for every dotenvctl verb.
//
// Why it exists: with one writer per mode (human vs --json), ten verbs cannot
// drift into ten envelope shapes. Every byte a verb emits goes through a
// Printer, and the JSON envelope is defined exactly once, here.
//
// The envelope is stable API for scripts:
//
//	{"ok":true,"data":<verb-specific>}
//	{"ok":false,"error":{"code":"<ErrCode>","message":"..."}}
//
// Changing a field name here is a breaking change for every consumer piping
// dotenvctl --json into jq — treat it with library-API discipline.
package outfmt

import (
	"encoding/json"
	"fmt"
	"io"
)

// ErrCode classifies a failure in the JSON envelope. Closed set — scripts
// branch on these strings, so adding a value is additive API and renaming one
// is a breaking change.
type ErrCode string

const (
	// CodeNotFound: the named key (or disabled/inherited entry) does not exist
	// in the file.
	CodeNotFound ErrCode = "not_found"

	// CodeIO: the file could not be read or written.
	CodeIO ErrCode = "io"

	// CodeRequired: a ${VAR:?msg} / ${VAR?msg} reference failed during
	// --expand — the library's RequiredError surfaced.
	CodeRequired ErrCode = "required"

	// CodeUsage: arguments did not parse (bad KEY=VALUE, missing operand).
	CodeUsage ErrCode = "usage"

	// CodeExec: the child process of `run` could not be started. A child that
	// starts and exits non-zero is NOT this — its exit code is passed through
	// verbatim instead.
	CodeExec ErrCode = "exec"

	// CodeDiff: reserved for `diff` when the files differ. Not an error in the
	// envelope (diff emits ok:true with the differences); it exists so the
	// exit-code mapping has a named cause rather than a bare 1.
	CodeDiff ErrCode = "diff"
)

// Printer renders verb output in exactly one of two modes, chosen once at
// root-command parse time.
//
// Invariant: verbs never write to their cobra command's writers directly —
// everything flows through Out so tests can capture it and so the JSON mode
// cannot be polluted by stray human-form prints.
type Printer struct {
	// Out receives ALL success output (human or JSON envelope alike).
	Out io.Writer

	// JSON selects the machine envelope. When false, verbs print their
	// human-readable form via Human/Humanf and OK emits nothing.
	JSON bool
}

// envelope is the wire shape of every --json response. Kept unexported: the
// only way to produce one is OK/Fail, which is what keeps the shape uniform.
type envelope struct {
	OK    bool       `json:"ok"`
	Data  any        `json:"data,omitempty"`
	Error *wireError `json:"error,omitempty"`
}

// wireError is the failure half of the envelope.
type wireError struct {
	Code    ErrCode `json:"code"`
	Message string  `json:"message"`
}

// OK emits the success envelope in JSON mode and is a no-op in human mode —
// human output is verb-specific and printed via Human/Humanf where the verb
// knows its own layout.
//
// data crosses into encoding/json untyped here; this is the one sanctioned
// serialization boundary (each verb passes a concrete named struct, never a
// bare map, so the shape stays greppable at the call site).
func (p *Printer) OK(data any) error {
	if !p.JSON {
		return nil
	}
	return p.emit(envelope{OK: true, Data: data})
}

// Fail emits the failure envelope in JSON mode. In human mode it prints
// "dotenvctl: <message>" so failures are never silent in either mode.
func (p *Printer) Fail(code ErrCode, message string) error {
	if !p.JSON {
		_, err := fmt.Fprintf(p.Out, "dotenvctl: %s\n", message)
		return err
	}
	return p.emit(envelope{OK: false, Error: &wireError{Code: code, Message: message}})
}

// Human prints a line of human-mode output; a no-op under --json so verbs can
// call it unconditionally without guarding every print site.
//
// Write errors are deliberately dropped in Human/Humanf: this path targets a
// terminal (or a test buffer), where a failed print has no recovery and no
// caller acts on it — unlike OK/Fail, whose error return guards the machine
// envelope that scripts depend on.
func (p *Printer) Human(line string) {
	if p.JSON {
		return
	}
	_, _ = fmt.Fprintln(p.Out, line)
}

// Humanf is Human with formatting; same deliberate write-error drop.
func (p *Printer) Humanf(format string, args ...any) {
	if p.JSON {
		return
	}
	_, _ = fmt.Fprintf(p.Out, format+"\n", args...)
}

// emit writes one envelope as a single JSON line — newline-terminated so
// stream consumers (jq, line readers) get exactly one object per invocation.
func (p *Printer) emit(e envelope) error {
	enc := json.NewEncoder(p.Out)
	return enc.Encode(e)
}
