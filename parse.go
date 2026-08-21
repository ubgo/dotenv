package dotenv

import (
	"regexp"
	"strings"
)

// keyClass is the set of characters a variable name may contain — Docker
// Compose's charset (compose-go accepts letters, digits, '_', '.', '-', '[',
// ']'), which is a superset of Node dotenv's and godotenv's `[\w.-]`. Matching
// the widest mainstream implementation means a file any of those tools reads
// loses no keys here: dotted keys (`spring.datasource.url`), hyphenated keys
// (`MY-SERVICE-TOKEN`), digit-first keys (`2FA_SECRET`), and bracketed keys
// (`SERVICES[0]`) are all real pairs, not unrecognised lines.
//
// Deliberately NOT the interpolation-name charset: what `${VAR}` may reference
// is a separate, narrower grammar owned by expand.go. Widening one must not
// widen the other.
//
// A key containing anything OUTSIDE this class (`HAS KEY=v`, `K!=v`) is not an
// error here, unlike compose-go which refuses the whole file: the line becomes
// KindOther and survives verbatim. See KindOther for why an editor must prefer
// preservation over rejection.
const keyClass = `[A-Za-z0-9_.\-\[\]]`

// pairRe matches the head of a KEY=value (or Compose/Node-style KEY: value)
// line.
//
// The whole prefix — optional `export`, spacing, key, spacing, and the
// delimiter — is captured as one group so a value edit can put it back exactly
// as the author wrote it, colon and all. Reconstructing it from parts would
// normalise somebody's chosen spacing (or delimiter) into a diff they did not
// ask for.
//
// Both `=` and `:` are accepted because both are in the Compose env-file
// grammar and in Node dotenv's LINE regex; a file written for either tool must
// parse to the same set of keys here. No whitespace is required after the
// colon — Compose requires none, and following Node's stricter `: ` form
// instead would make this package and Compose disagree about the same bytes.
//
// Known consequence, shared with compose-go: a bare URL line
// (`http://example.com`) parses as key `http`, value `//example.com`. That
// looks surprising but is Compose's own reading, and the line still
// round-trips verbatim — do NOT special-case it. Pinned by
// TestColonDelimiter.
var pairRe = regexp.MustCompile(`^((?:export[ \t]+)?[ \t]*(` + keyClass + `+)[ \t]*[=:])(.*)$`)

// inheritedRe matches a name-only line: `HOME`, or `export HOME` — no
// delimiter, no value. In the Compose env-file grammar this declares the
// variable as INHERITED from the process environment rather than defined by
// the file.
//
// This package recognises the declaration (KindInherited) but never resolves
// it — reading os.Environ is exactly what this package promises not to do.
// See KindInherited for the contract.
var inheritedRe = regexp.MustCompile(`^[ \t]*(?:export[ \t]+)?(` + keyClass + `+)[ \t]*$`)

// inlineCommentRe finds where an unquoted value ends: at whitespace followed by
// a #. Requiring the whitespace is deliberate — a # inside a value such as
// PASSWORD=a#b is part of the password, not the start of a comment.
var inlineCommentRe = regexp.MustCompile(`[ \t]#`)

// parse ingests whole-file content. Separate from Load so parsing is testable
// without a filesystem, and so Parse can reuse it.
func (f *File) parse(content string) {
	// crlf records the DOMINANT style, used only for lines this package writes
	// later. Existing lines keep their own terminator verbatim: a file with
	// mixed endings must round-trip as mixed, not be silently normalised to one
	// style — that would be a whole-file diff nobody asked for.
	f.crlf = strings.Contains(content, "\r\n")
	f.trailingNewline = strings.HasSuffix(content, "\n") || content == ""

	lines := strings.Split(content, "\n")
	if f.trailingNewline && len(lines) > 0 {
		// Split leaves an empty tail element after a final terminator; keeping
		// it would invent a blank line that was never in the file.
		lines = lines[:len(lines)-1]
	}

	// observe fires EntryObserver for one finished entry. Hoisted so every
	// append site reports, rather than three of them remembering to.
	observe := func(e *Entry) {
		for _, o := range f.opts.entryObs {
			o.ObserveEntry(e)
		}
	}

	for i := 0; i < len(lines); i++ {
		// raw keeps any trailing \r so Render reproduces the exact bytes;
		// everything downstream parses the trimmed form.
		raw := lines[i]
		line := strings.TrimSuffix(raw, "\r")
		trimmed := strings.TrimSpace(line)

		switch {
		case trimmed == "":
			e := &Entry{Kind: KindBlank, Raw: []string{raw}, attached: true}
			f.entries = append(f.entries, e)
			observe(e)

		case strings.HasPrefix(trimmed, "#"):
			e := f.parseComment(line, raw)
			f.entries = append(f.entries, e)
			observe(e)

		default:
			m := pairRe.FindStringSubmatch(line)
			if m == nil {
				// Not a pair. A bare name is a Compose "inherited" declaration
				// (see KindInherited); anything else is unrecognised and kept
				// verbatim.
				if im := inheritedRe.FindStringSubmatch(line); im != nil {
					inh := &Entry{Kind: KindInherited, Key: im[1], Raw: []string{raw}, attached: true}
					f.entries = append(f.entries, inh)
					observe(inh)
					continue
				}
				other := &Entry{Kind: KindOther, Raw: []string{raw}, attached: true}
				f.entries = append(f.entries, other)
				observe(other)
				continue
			}
			e := &Entry{Kind: KindPair, Key: m[2], prefix: m[1], Raw: []string{raw}, attached: true}
			// A quoted value may run past this physical line; whatever it
			// consumes joins this entry's Raw so the block edits as one unit.
			// The lookahead is passed UNTRIMMED and each line is trimmed only
			// if a multi-line value actually consumes it. Pre-trimming copied
			// the whole remaining file for every pair, which made parsing
			// quadratic: a 5 MB file took 23 seconds.
			consumed := parseValue(m[3], lines[i+1:], e, f.opts)
			for range consumed {
				i++
				e.Raw = append(e.Raw, lines[i])
			}
			f.entries = append(f.entries, e)
			observe(e)
		}
	}
}

// parseComment classifies a comment line, recognising a commented-out setting as
// a KindDisabledPair rather than prose.
//
// Without this, "# DB_USER=admin" would mean something different depending on
// who commented it: Unset records the key, a human editor does not. Same text,
// same intent, so they must parse the same way.
func (f *File) parseComment(line, raw string) *Entry {
	marker := strings.Index(line, "#")
	body := line[marker+1:]

	// The recorded prefix starts AT the marker, not at the start of the line:
	// Restore must remove the comment marker and the spaces that follow it,
	// while leaving any indentation the line already had. Stripping from column
	// zero would silently re-align an indented setting.
	spaces := len(body) - len(strings.TrimLeft(body, " \t"))
	prefix := line[marker : marker+1+spaces]
	body = body[spaces:]

	m := pairRe.FindStringSubmatch(body)
	if m == nil || !valueEndsOnThisLine(m[3]) {
		return &Entry{Kind: KindComment, Raw: []string{raw}, attached: true}
	}

	e := &Entry{
		Kind:          KindDisabledPair,
		Key:           m[2],
		prefix:        m[1],
		commentPrefix: prefix,
		Raw:           []string{raw},
		attached:      true,
	}
	parseValue(m[3], nil, e, f.opts)
	return e
}

// valueEndsOnThisLine reports whether a quoted value closes on the line it opens.
//
// A commented-out multi-line block cannot be reassembled — each of its lines is
// a separate comment entry — so only a self-contained assignment is treated as a
// disabled pair. Otherwise "# KEY=\"one" would become a pair whose value is an
// unterminated quote.
func valueEndsOnThisLine(rhs string) bool {
	v := strings.TrimLeft(rhs, " \t")
	switch {
	case strings.HasPrefix(v, `"`):
		_, _, closed := scanDoubleQuoted(v[1:], EscapeCompose)
		return closed
	case strings.HasPrefix(v, "'"):
		return strings.IndexByte(v[1:], '\'') >= 0
	case strings.HasPrefix(v, "`"):
		return strings.IndexByte(v[1:], '`') >= 0
	default:
		return true
	}
}

// trimEOL strips a CRLF terminator from one lookahead line, so a multi-line
// value's decoded content never carries a stray \r.
func trimEOL(line string) string {
	return strings.TrimSuffix(line, "\r")
}

// parseValue decodes the right-hand side of a pair into e.Value and
// e.inlineComment.
//
// rest holds the FOLLOWING physical lines; the return value is how many of them
// a multi-line quoted value consumed.
func parseValue(rhs string, rest []string, e *Entry, opts options) int {
	s := strings.TrimLeft(rhs, " \t")

	switch {
	case strings.HasPrefix(s, `"`):
		e.quote = quoteDouble
		return parseDoubleQuoted(s[1:], rest, e, opts)
	case strings.HasPrefix(s, `'`):
		e.quote = quoteSingle
		return parseLiteralQuoted(s[1:], rest, e, '\'')
	case strings.HasPrefix(s, "`"):
		e.quote = quoteBacktick
		return parseLiteralQuoted(s[1:], rest, e, '`')
	default:
		e.quote = quoteNone
		return parseBare(s, e)
	}
}

// parseDoubleQuoted handles a value opening with a double quote, spanning lines
// until the quote closes.
func parseDoubleQuoted(s string, rest []string, e *Entry, opts options) int {
	value, remainder, closed := scanDoubleQuoted(s, opts.escapes)
	if closed {
		e.Value = value
		e.inlineComment = remainder
		return 0
	}

	parts := []string{value}
	for n, raw := range rest {
		line := trimEOL(raw)
		v, rem, ok := scanDoubleQuoted(line, opts.escapes)
		parts = append(parts, v)
		if ok {
			e.Value = strings.Join(parts, "\n")
			e.inlineComment = rem
			return n + 1
		}
	}

	// Unterminated at EOF. Everything consumed becomes the value; preservation
	// still holds because Raw is verbatim, so rendering an untouched file is
	// unaffected by the malformed quote.
	e.Value = strings.Join(parts, "\n")
	return len(rest)
}

// parseLiteralQuoted handles a fully literal value delimited by quote — single
// quotes or backticks. No escape processing at all, so the closing delimiter is
// simply the next occurrence of it, and the value may span lines.
func parseLiteralQuoted(s string, rest []string, e *Entry, quote byte) int {
	if idx := strings.IndexByte(s, quote); idx >= 0 {
		e.Value = s[:idx]
		e.inlineComment = s[idx+1:]
		return 0
	}

	parts := []string{s}
	for n, raw := range rest {
		line := trimEOL(raw)
		if idx := strings.IndexByte(line, quote); idx >= 0 {
			parts = append(parts, line[:idx])
			e.Value = strings.Join(parts, "\n")
			e.inlineComment = line[idx+1:]
			return n + 1
		}
		parts = append(parts, line)
	}

	e.Value = strings.Join(parts, "\n")
	return len(rest)
}

// parseBare handles an unquoted value, which ends at an inline comment or at
// end of line, with trailing whitespace trimmed.
func parseBare(s string, e *Entry) int {
	if loc := inlineCommentRe.FindStringIndex(s); loc != nil {
		e.Value = strings.TrimRight(s[:loc[0]], " \t")
		e.inlineComment = s[loc[0]:]
		return 0
	}
	e.Value = strings.TrimRight(s, " \t")
	return 0
}

// scanDoubleQuoted scans s — the content AFTER an opening double quote — for the
// closing unescaped quote.
//
// Returns the decoded value, whatever followed the closing quote, and whether
// the quote closed within s. Which backslash sequences are decoded depends on
// mode; see EscapeMode. An unrecognised sequence is always kept literally,
// backslash included, so a Windows path or a regex survives without the author
// doubling every separator.
func scanDoubleQuoted(s string, mode EscapeMode) (value, remainder string, closed bool) {
	var b strings.Builder

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			if decoded, ok := decodeEscape(s[i+1], mode); ok {
				b.WriteString(decoded)
				i++
				continue
			}
		}
		if c == '"' {
			return b.String(), s[i+1:], true
		}
		b.WriteByte(c)
	}
	return b.String(), "", false
}

// decodeEscape maps the character following a backslash to its replacement,
// reporting whether the sequence is recognised in this mode.
func decodeEscape(c byte, mode EscapeMode) (string, bool) {
	switch c {
	case '"':
		return `"`, true
	case '\\':
		return `\`, true
	}
	if mode != EscapeExtended {
		return "", false
	}
	switch c {
	case 'n':
		return "\n", true
	case 'r':
		return "\r", true
	case 't':
		return "\t", true
	}
	return "", false
}
