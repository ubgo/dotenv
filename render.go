package dotenv

import "strings"

// renderPair builds the physical lines for a pair, choosing canonical quoting:
// bare when that round-trips safely, double-quoted otherwise, and a
// double-quoted block when the value contains newlines.
func renderPair(prefix, value, inlineComment, eol string) []string {
	if !strings.ContainsRune(value, '\n') {
		return []string{prefix + renderScalar(value) + inlineComment + eol}
	}

	parts := strings.Split(value, "\n") // len >= 2, guaranteed by the check above
	lines := make([]string, 0, len(parts))
	for i, p := range parts {
		esc := escapeDouble(p)
		switch {
		case i == 0:
			lines = append(lines, prefix+`"`+esc+eol)
		case i == len(parts)-1:
			lines = append(lines, esc+`"`+inlineComment+eol)
		default:
			lines = append(lines, esc+eol)
		}
	}
	return lines
}

// renderScalar renders a single-line value: bare when it holds no character
// that would change meaning, double-quoted with escaping otherwise.
func renderScalar(value string) string {
	if value == "" {
		return ""
	}
	if needsQuoting(value) {
		return `"` + escapeDouble(value) + `"`
	}
	return value
}

// quotingTriggers are the characters whose presence makes a bare rendering parse
// back as something else: whitespace and `#` end the value early, the three
// quote characters would be read as delimiters, and a backslash would be read as
// an escape.
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
