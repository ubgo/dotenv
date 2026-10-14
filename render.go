package dotenv

import "strings"

// renderPair builds the physical lines for a pair and reports the quote style
// it chose, so the caller can record it on the entry.
//
// Recording the style is what keeps an edit honest: GetExpanded decides
// whether to interpolate from the entry's quote style, so an entry whose bytes
// say one style while the entry remembers another reads differently in memory
// and after a reload. See renderStyle for how the style is chosen.
func renderPair(prefix, value, inlineComment, eol string) ([]string, quoteStyle) {
	q := renderStyle(value, inlineComment)
	open, close := q.delimiters()

	parts := strings.Split(value, "\n")
	lines := make([]string, len(parts))
	for i, p := range parts {
		if q == quoteDouble {
			p = escapeDouble(p)
		}
		line := p
		if i == 0 {
			line = prefix + open + line
		}
		if i == len(parts)-1 {
			line += close + inlineComment
		}
		lines[i] = line + eol
	}
	return lines, q
}

// renderStyle chooses how a value is quoted so that it reads back as exactly
// the same value AND the same meaning.
//
//   - A value containing '$' is stored literally: single-quoted, or
//     backtick-quoted when it holds a single quote. Written bare or
//     double-quoted, "a$b" would be read back as "a" plus a reference to b,
//     so a password could never be stored with Set. Only a value holding both
//     a single quote and a backtick has no literal delimiter left; it is
//     double-quoted and its '$' stays a reference.
//   - A value with a newline becomes a double-quoted block.
//   - A value with a quoting trigger is double-quoted.
//   - A value followed by an inline comment that does not start with
//     whitespace (`K="v"# note`), or an empty value followed by any comment,
//     is double-quoted, because bare it would absorb the comment.
//   - Anything else is bare.
func renderStyle(value, inlineComment string) quoteStyle {
	if strings.ContainsRune(value, '$') {
		switch {
		case !strings.ContainsRune(value, '\''):
			return quoteSingle
		case !strings.ContainsRune(value, '`'):
			return quoteBacktick
		}
		return quoteDouble
	}
	if strings.ContainsRune(value, '\n') || needsQuoting(value) {
		return quoteDouble
	}
	if inlineComment != "" && (value == "" || !startsWithBlank(inlineComment)) {
		return quoteDouble
	}
	return quoteNone
}

// startsWithBlank reports whether s begins with a space or a tab — the only
// way a bare value's inline comment is recognised (see inlineCommentRe).
func startsWithBlank(s string) bool {
	return s != "" && (s[0] == ' ' || s[0] == '\t')
}

// delimiters returns the opening and closing text for a value written in this
// style.
func (q quoteStyle) delimiters() (open, close string) {
	switch q {
	case quoteDouble:
		return `"`, `"`
	case quoteSingle:
		return "'", "'"
	case quoteBacktick:
		return "`", "`"
	default:
		return "", ""
	}
}

// ds:def id=quotingtriggers-xsur5zum owner=@maintainers stability=stable

// quotingTriggers are the characters whose presence makes a bare rendering parse
// back as something else: whitespace and `#` end the value early, the three
// quote characters would be read as delimiters, and a backslash would be read as
// an escape. '$' is not listed because double quotes do not help it — it needs
// a literal delimiter, which renderStyle checks first.
//
// The backtick belongs here for the same reason the other two quotes do — it is
// a value delimiter. Omitting it silently corrupted any value containing one,
// which is exactly the class of bug the round-trip property test exists to catch.
const quotingTriggers = " \t#\"'`\\"

// needsQuoting reports whether rendering value bare would parse back differently.
func needsQuoting(value string) bool {
	return strings.ContainsAny(value, quotingTriggers)
}

// escapeDouble escapes the only two characters the double-quote syntax
// interprets. Order matters: backslashes first, or the backslash added for a
// quote would itself be escaped.
func escapeDouble(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}
