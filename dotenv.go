// Package dotenv is a comment-preserving .env parser and editor.
//
// Parse-to-map dotenv libraries throw away everything that is not a key or a
// value, so writing a file back destroys its comments, blank lines, ordering,
// and quoting style. This package edits LINE-WISE instead: every byte outside
// the entry you touch survives verbatim. Two invariants, both pinned by tests:
//
//	parse → render with no changes  ⇒  byte-identical output
//	Set(key, <current value>)       ⇒  byte-identical output (a true no-op)
//
// That matters for a file a human wrote and a tool edits. A `.env` is
// documentation as much as configuration, and a tool that silently strips the
// comment explaining why a value exists has damaged the file even though every
// key survived.
//
// # What it understands
//
//	KEY=value                 a pair
//	KEY: value                a pair — Compose and Node dotenv accept the colon
//	                          delimiter too, and the author's choice is preserved
//	export KEY=value          the export prefix is preserved
//	KEY                       a name with no delimiter: a Compose "inherited"
//	                          declaration — recognised but never resolved (§ scope)
//	KEY="quoted value"        double quotes; \" and \\ are interpreted
//	KEY='literal value'       single quotes; fully literal
//	KEY="line one            a quoted value whose quote does not close keeps
//	line two"                 going, and the whole block is one logical entry
//	KEY=value # note          an inline comment, preserved on edit
//	# a comment               kept verbatim
//	<blank>                   kept verbatim
//
// # Escape policy
//
// Inside double quotes only `\"` and `\\` are interpreted. There is NO `\n`
// expansion — a newline in a value is a real newline in the file. Single-quoted
// values are entirely literal. This mirrors Docker Compose's .env handling
// rather than shell semantics, because a .env is far more often read by compose
// than sourced by a shell.
//
// # Scope: a parser and editor, not a loader
//
// This package NEVER touches os.Environ. Reading a .env and exporting it into
// the process are separate decisions, and conflating them is why godotenv.Load
// means something this package must not mean. A caller that wants the values in
// its environment applies its own precedence policy over f.Map().
//
// It also does not merge multiple files. Merging is a precedence policy — which
// file wins, per key — and the whole guarantee here is that one file maps to one
// set of bytes. Merged content has no single file to render back to, so it could
// not round-trip. Layer that above this package.
//
// It does not coerce values to types. Every value is a string, because a .env
// has none — PORT=8443 is four characters, and which Go type that becomes
// depends on the field it is bound to. A *File satisfies the Lookuper interface
// used by struct-binding packages with a three-line adapter, so typed
// configuration composes rather than being reimplemented here. See the README.
//
// It does not stream. Parse and ParseReader both hold the whole file, and three
// separate features depend on that: a forward reference resolves against a key
// defined further down, insertion needs an index into a complete entry list, and
// byte-exact rendering needs every original line. A streaming parser would have
// to abandon all three.
//
// It would also be dangerous rather than merely limited. Streaming could resolve
// BACKWARD references from a running map — but with a duplicated key, "last
// wins" cannot be known until the file ends, so the same bytes would expand to
// one value here and another there. Two APIs disagreeing about one file is worse
// than one API doing less.
//
// See docs/proposals/dotenv-streaming.md for the measurements behind this and
// the shape a Scan would take if the case ever arises.
//
// # Naming
//
// Named for the format, the way encoding/json and yaml are — not for the file it
// reads. The constructor is Open rather than Load because godotenv.Load sets
// os.Environ, and this package deliberately never touches the environment: it
// hands back a file you read, edit, and save. A Load here would mean the
// opposite of every other Load in the ecosystem.
//
// # Line endings and permissions
//
// CRLF files render back as CRLF. A file's existing mode is preserved; a new
// one is created 0600, because .env files hold credentials and a
// group-readable secret is a leak.
package dotenv

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

// defaultFileMode is used when creating a file that did not exist.
//
// 0600 rather than 0644: these files hold credentials. Restrictive by default
// is the only safe direction — a user who wants it wider can chmod, and Save
// preserves whatever mode an existing file already has.
const defaultFileMode fs.FileMode = 0o600

// tempSuffix marks the sibling file Save writes before renaming into place.
const tempSuffix = ".dotenv-tmp"

// Kind classifies a parsed entry. Closed set — the parser produces nothing else.
type Kind int

const (
	// KindPair is a KEY=value entry, possibly multi-line and possibly
	// `export`-prefixed.
	KindPair Kind = iota

	// KindComment is a full-line comment: the first non-space character is #.
	KindComment

	// KindBlank is an empty or whitespace-only line.
	KindBlank

	// KindOther is any line the parser does not recognise. Preserved verbatim
	// and never touched — an unparseable line is far more likely to be
	// something the parser has not learned yet than something safe to discard.
	//
	// This is a DELIBERATE divergence from compose-go, which errors on an
	// invalid character in a key (`unexpected character "!" in variable name`)
	// and refuses the whole file. An editor cannot afford that: erroring would
	// make an otherwise-valid file unopenable — and so uneditable — because of
	// one stray line, and "parse, edit, save" must never be blocked by content
	// it was never going to touch. Preservation over rejection. The cost is
	// that a malformed pair is silently invisible to Get/Keys/Map rather than
	// loudly reported; a caller that wants strictness can walk Entries and
	// treat KindOther as its own error.
	KindOther

	// KindDisabledPair is a commented-out setting: "# DB_USER=admin". Key and
	// Value are populated, but the entry is INACTIVE — Get, Has, Count, and Map
	// all ignore it, exactly as a consumer of the file would.
	//
	// It is a distinct kind rather than a plain comment because the two mean
	// different things to anything inspecting a file: one is a setting somebody
	// turned off, the other is prose. Restore turns it back on.
	//
	// The parser assigns this to any comment whose body parses as a pair, so a
	// line the author commented out by hand is indistinguishable from one Unset
	// produced. The cost is a false positive on prose shaped like an
	// assignment — "# TODO=fix this" reads as a disabled setting, which is also
	// what a human skimming the file would assume.
	KindDisabledPair

	// KindInherited is a name-only line — `HOME`, or `export HOME` — with no
	// delimiter and no value. In the Compose env-file grammar it declares that
	// the variable's value comes from the process environment, and it is how a
	// file whitelists host variables (an AWS key, a proxy setting) without
	// hardcoding their values.
	//
	// This package recognises the declaration but NEVER resolves it: doing so
	// would read os.Environ, which this package promises not to touch. So the
	// entry is INACTIVE — Get, Has, Count, Keys, Pairs, and Map all ignore it,
	// and an unresolved ${reference} to it expands to "" like any other unset
	// variable. Inherited lists the declared names so a caller that wants
	// Compose's behaviour can apply os.Getenv (or any other source) itself —
	// the decision to read the environment stays with the application.
	KindInherited
)

// String implements fmt.Stringer, so a Kind in a test failure or a log line
// reads as a name rather than an integer.
func (k Kind) String() string {
	switch k {
	case KindPair:
		return "pair"
	case KindComment:
		return "comment"
	case KindBlank:
		return "blank"
	case KindOther:
		return "other"
	case KindDisabledPair:
		return "disabled-pair"
	case KindInherited:
		return "inherited"
	default:
		return fmt.Sprintf("Kind(%d)", int(k))
	}
}

// Entry is one logical unit of the file: a pair (possibly spanning several
// physical lines), a comment, a blank, or an unrecognised line.
type Entry struct {
	// Kind classifies the entry.
	Kind Kind

	// Raw holds the exact physical lines, without terminators. For an untouched
	// entry these render back byte-identically — this field is what makes the
	// preservation invariant possible.
	Raw []string

	// Key is meaningful for KindPair, KindDisabledPair, and KindInherited;
	// empty for every other kind.
	Key string

	// Value is the DECODED value for KindPair: quotes stripped, escapes
	// interpreted, multi-line joined with \n.
	Value string

	// prefix is everything left of the value on the first line, including the
	// delimiter (`=` or the Compose/Node-style `:`) and any `export ` —
	// preserved exactly when the value is edited. Keeping the delimiter inside
	// the prefix is what makes delimiter preservation automatic: renderPair
	// re-emits the prefix verbatim, so a colon-delimited pair can never be
	// silently rewritten to `=` by an edit.
	prefix string

	// inlineComment is whatever followed the value on its line, preserved
	// exactly when the value is edited.
	inlineComment string

	// commentPrefix is the exact leading text up to and including the comment
	// marker, for a KindDisabledPair — "# ", "#", "   #  ". Retained so Restore
	// removes precisely what was added and nothing else.
	commentPrefix string

	// attached reports whether this entry belongs to a File and already holds
	// rendered Raw lines. Constructor-made entries are false until Append or
	// Insert renders them, which is when the file's line ending is known.
	attached bool

	// quote records how the value was delimited in the source. It has to be
	// kept because decoding is lossy: once the quotes are stripped, nothing
	// distinguishes a literal '${A}' from an interpolatable "${A}". Without
	// this, expansion silently rewrites values the author marked as literal.
	quote quoteStyle
}

// quoteStyle records the delimiter a value was written with.
type quoteStyle int

const (
	// quoteNone is a bare value: interpolated.
	quoteNone quoteStyle = iota
	// quoteDouble is "..." — escapes decoded, interpolated.
	quoteDouble
	// quoteSingle is '...' — fully literal, never interpolated.
	quoteSingle
	// quoteBacktick is `...` — fully literal, never interpolated. The
	// ergonomic way to paste a PEM or an SSH key, which contain both quote
	// characters.
	quoteBacktick
)

// interpolates reports whether a value written with this delimiter takes part
// in variable expansion. Only bare and double-quoted values do, matching every
// mainstream dotenv implementation.
func (q quoteStyle) interpolates() bool {
	return q == quoteNone || q == quoteDouble
}

// NewPair builds an unattached KEY=value entry.
//
// New entries always use `=` — the canonical delimiter every dotenv dialect
// reads. The colon form is only ever PRESERVED from source, never authored:
// see Entry.prefix.
//
// The value is rendered when the entry is attached by Append or Insert, not
// here: quoting is settled at construction, but the line ending is a property of
// the destination file, which this entry does not yet have.
func NewPair(key, value string) *Entry {
	return &Entry{Kind: KindPair, Key: key, Value: value, prefix: key + "="}
}

// NewComment builds an unattached comment spanning one line per argument.
//
//	NewComment("----------", "DATABASE", "----------")
//
// Each line is prefixed with "# " unless it already begins with "#", so a caller
// can pass either plain text or pre-decorated lines without ending up with "##".
func NewComment(lines ...string) *Entry {
	raw := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimLeft(l, " \t"), "#") {
			raw = append(raw, l)
			continue
		}
		raw = append(raw, "# "+l)
	}
	if len(raw) == 0 {
		raw = []string{"#"}
	}
	return &Entry{Kind: KindComment, Raw: raw}
}

// NewBlank builds an unattached empty line, for separating sections.
func NewBlank() *Entry {
	return &Entry{Kind: KindBlank, Raw: []string{""}}
}

// File is a parsed .env plus the formatting facts needed to render it back
// byte-identically.
type File struct {
	// Path is where the file was loaded from, and where Save writes.
	Path string

	entries []*Entry
	mode    fs.FileMode

	// crlf records that the source used \r\n terminators.
	crlf bool

	// trailingNewline records whether the source ended with a line terminator.
	// Adding or removing one would be a spurious diff on every save.
	trailingNewline bool

	// existed records whether the file was present at Open time.
	existed bool

	// opts is the dialect this file was parsed with, retained so expansion
	// behaves consistently with parsing.
	opts options
}

// Open reads and parses path.
//
// A missing file is NOT an error: it loads as an empty File so a caller can
// bootstrap a fresh .env, and Save will create it at 0600. Callers that need to
// distinguish the two cases ask Existed.
func Open(path string, opts ...Option) (*File, error) {
	f := &File{Path: path, mode: defaultFileMode, trailingNewline: true, opts: newOptions(opts)}

	handle, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return nil, fmt.Errorf("dotenv: open %s: %w", path, err)
	}
	// Close error deliberately dropped: this handle is READ-only, so Close
	// cannot signal a lost write the way it can for a writer — and the content
	// was already fully read (or reading failed loudly) by the time it runs.
	defer func() { _ = handle.Close() }()

	// Stat the OPEN handle rather than the path: it cannot race with the file
	// being replaced between the two calls, and it is one syscall fewer.
	if info, statErr := handle.Stat(); statErr == nil {
		f.mode = info.Mode().Perm()
	}

	content, err := readAll(handle)
	if err != nil {
		return nil, fmt.Errorf("dotenv: read %s: %w", path, err)
	}

	f.existed = true
	f.parse(content)
	return f, nil
}

// Parse parses content directly, with no filesystem involved.
//
// Useful for parsing a .env that arrived over a pipe, out of an embedded
// fixture, or from a secret store — and it makes the parser testable without
// touching disk.
func Parse(content string, opts ...Option) *File {
	f := &File{mode: defaultFileMode, trailingNewline: true, opts: newOptions(opts)}
	f.parse(content)
	return f
}

// ParseReader parses content read from r.
//
// Prefer this over reading the bytes yourself. io.ReadAll returns a []byte and
// converting that to a string copies the whole thing again, on top of the
// doubling io.ReadAll does as its buffer grows. Copying into a strings.Builder
// hands its buffer to the string directly, so both costs disappear.
//
// Measured on a ~1.4 MB file by BenchmarkParseLarge: 9.8 MB and 67 allocations
// against 13.0 MB and 93 for ReadAll-then-Parse. Re-run it rather than trusting
// these numbers — that is what the benchmark is for.
//
// This matters once a .env carries a PEM block or a base64 certificate and runs
// to megabytes, which is common enough to design for.
//
// The whole content is still held in memory, unavoidably: preserving a file byte
// for byte means keeping every line. This lowers the cost; it does not stream.
func ParseReader(r io.Reader, opts ...Option) (*File, error) {
	content, err := readAll(r)
	if err != nil {
		return nil, fmt.Errorf("dotenv: read: %w", err)
	}
	return Parse(content, opts...), nil
}

// readAll drains r into a string with a single copy, returning the underlying
// error unwrapped so each caller attaches its own context — Open names the path,
// ParseReader has none to name. Wrapping here as well would produce
// "dotenv: read x: dotenv: read: ..." at the call site.
func readAll(r io.Reader) (string, error) {
	var b strings.Builder
	if _, err := io.Copy(&b, r); err != nil {
		return "", err
	}
	return b.String(), nil
}

// Read is the whole-file shortcut: open, parse, expand, and hand back the
// values.
//
//	m, err := dotenv.Read(".env")
//
// Equivalent to Open followed by ExpandedMap, which is what most callers want
// when they only need the configuration and will never write the file back.
//
// The values are EXPANDED, because that is what a consumer sees: a value stored
// as "https://api.${DOMAIN}" arrives resolved. Use Open and Map instead when the
// file will be edited and saved — persisting an expanded value bakes the
// reference and silently breaks the cascade.
//
// A missing file is an error here, unlike Open. Open exists partly to bootstrap
// a file that does not yet exist; Read is for consuming one that should.
//
// It does not touch os.Environ. Nothing in this package does.
func Read(path string, opts ...Option) (map[string]string, error) {
	f, err := Open(path, opts...)
	if err != nil {
		return nil, err
	}
	if !f.Existed() {
		return nil, fmt.Errorf("dotenv: read %s: file does not exist", path)
	}
	return f.ExpandedMap()
}

// Existed reports whether the file was present when Open ran. False means Save
// will create it.
func (f *File) Existed() bool { return f.existed }

// Mode returns the file mode Save will apply.
func (f *File) Mode() fs.FileMode { return f.mode }

// Entries returns the parsed entries in file order, including comments and
// blanks — the view a linear read of the file would give.
func (f *File) Entries() []*Entry { return f.entries }

// Render produces the file content. For untouched entries this is byte-identical
// to the source, which is the guarantee the whole package exists to provide.
func (f *File) Render() string {
	var lines []string
	for _, e := range f.entries {
		lines = append(lines, e.Raw...)
	}

	// Joined with "\n" alone: each line already carries its own "\r" when the
	// source had one, so a file with mixed terminators comes back mixed rather
	// than normalised to a single style.
	out := strings.Join(lines, "\n")
	if f.trailingNewline && out != "" {
		out += "\n"
	}
	// A file of nothing but blank lines still ends in a terminator.
	if out == "" && f.trailingNewline && len(f.entries) > 0 {
		out = "\n"
	}
	return out
}

// eol returns the terminator this package appends to lines it writes: the
// file's dominant style, so an edit does not introduce a stray LF into a CRLF
// file.
func (f *File) eol() string {
	if f.crlf {
		return "\r"
	}
	return ""
}

// chmod is a test seam. A chmod failure on a file we just created cannot be
// triggered portably, but the error branch still has to be verified.
var chmod = os.Chmod

// rename is a test seam, for the same reason as chmod.
var rename = os.Rename

// Save writes the rendered file atomically.
//
// A sibling temp file is written at 0600 — never a wider window, even briefly —
// then chmod'd to the target mode and renamed over the destination. A rename
// within a directory is atomic, so a reader never observes a half-written
// credentials file, and a crash mid-save leaves the original intact.
//
// The explicit chmod is required because os.WriteFile's mode argument is
// filtered by the process umask, so it cannot be relied on to reproduce the
// original permissions.
func (f *File) Save() error {
	if f.Path == "" {
		return fmt.Errorf("dotenv: no path set; use Open, or set Path before Save")
	}
	return f.writeTo(f.Path, f.mode)
}

// SaveAs writes the file to a different path, leaving f.Path unchanged.
//
// Not mutating f is what makes emitting several variants from one source read
// cleanly:
//
//	f, _ := dotenv.Open(".env.staging")
//	f.Set("DOMAIN", "acme.io")
//	f.SaveAs(".env.prod")
//	f.Set("DOMAIN", "qa.acme.io")
//	f.SaveAs(".env.qa")
//
// An EXISTING destination keeps its own permissions; a new one is created with
// this file's mode. Overwriting must never widen a file a user has deliberately
// locked down, and must never quietly loosen one it is about to fill with
// secrets.
func (f *File) SaveAs(path string) error {
	if path == "" {
		return fmt.Errorf("dotenv: SaveAs needs a path")
	}

	mode := f.mode
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return f.writeTo(path, mode)
}

// writeTo renders and writes atomically.
//
// A sibling temp file is written at 0600 — never a wider window, even briefly —
// then chmod'd to the target mode and renamed over the destination. A rename
// within a directory is atomic, so a reader never observes a half-written
// credentials file, and a crash mid-write leaves the original intact.
//
// The explicit chmod is required because os.WriteFile's mode argument is
// filtered by the process umask, so it cannot reproduce a mode on its own.
func (f *File) writeTo(path string, mode fs.FileMode) error {
	// Guards run before anything is written, so a rejected file leaves the
	// destination exactly as it was — not half-replaced by a temp file.
	for i, g := range f.opts.guards {
		if err := g.GuardSave(f); err != nil {
			return pluginError(f.opts.pluginNameFor(CapSaveGuard, i), "guard save "+path, err)
		}
	}

	tmp := path + tempSuffix
	if err := os.WriteFile(tmp, []byte(f.Render()), defaultFileMode); err != nil {
		return fmt.Errorf("dotenv: write temp file: %w", err)
	}
	// Best-effort cleanup on every failure path; the error is deliberately
	// dropped because after a successful rename the temp name no longer exists
	// and Remove ALWAYS fails — surfacing that would turn every successful
	// save into a spurious error.
	defer func() { _ = os.Remove(tmp) }()

	if err := chmod(tmp, mode); err != nil {
		return fmt.Errorf("dotenv: set mode on temp file: %w", err)
	}
	if err := rename(tmp, path); err != nil {
		return fmt.Errorf("dotenv: rename temp file into place: %w", err)
	}
	return nil
}

// Clone returns an independent copy: editing either leaves the other untouched.
//
// Cheaper and more faithful than Parse(f.Render()) — no re-parsing, and entries
// keep the exact Raw bytes they were read with, including any the parser would
// classify differently on a second pass.
func (f *File) Clone() *File {
	out := *f
	out.entries = make([]*Entry, len(f.entries))
	for i, e := range f.entries {
		clone := *e
		clone.Raw = append([]string(nil), e.Raw...)
		out.entries[i] = &clone
	}
	return &out
}
