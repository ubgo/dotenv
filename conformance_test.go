package dotenv

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TestSyntaxConformance walks every .env syntax rule this package claims to
// support and asserts its behaviour for each one.
//
// It exists because "do we support X" is otherwise answered by reading the code
// and guessing. A row documenting a deliberate divergence says so in its name.
func TestSyntaxConformance(t *testing.T) {
	tests := []struct {
		rule   string
		input  string
		key    string
		want   string
		expand bool
		opts   []Option
	}{
		// --- quoting ---
		{rule: "unquoted", input: "SIMPLE=xyz123\n", key: "SIMPLE", want: "xyz123"},
		{rule: "double-quoted", input: `VAR="VAL"` + "\n", key: "VAR", want: "VAL"},
		{rule: "single-quoted is literal", input: `VAR='VAL'` + "\n", key: "VAR", want: "VAL"},
		{rule: "backtick single line", input: "K = `has \"quotes\" and 'apostrophes'`\n",
			key: "K", want: `has "quotes" and 'apostrophes'`},
		{rule: "backtick multiline", input: "MULTILINE = `line one\nline two`\n",
			key: "MULTILINE", want: "line one\nline two"},
		{rule: "spaces around equals", input: "K = v\n", key: "K", want: "v"},
		{rule: "empty value", input: "K=\n", key: "K", want: ""},
		{rule: "export prefix", input: "export K=v\n", key: "K", want: "v"},

		// --- comments ---
		{rule: "comment after quoted value", input: `API_KEY="value" # comment` + "\n",
			key: "API_KEY", want: "value"},
		{rule: "hash without space is not a comment", input: "VAR=VAL# not a comment\n",
			key: "VAR", want: "VAL# not a comment"},
		{rule: "hash inside quotes is preserved", input: `SECRET="something-#-not-comment"` + "\n",
			key: "SECRET", want: "something-#-not-comment"},

		// --- escapes ---
		{rule: "escape backslash-quote", input: `K="say \"hi\""` + "\n", key: "K", want: `say "hi"`},
		{rule: "escape double backslash", input: `K="back\\slash"` + "\n", key: "K", want: `back\slash`},
		{rule: "escape newline", input: `K="a\nb"` + "\n", key: "K", want: "a\nb"},
		{rule: "escape tab", input: `K="a\tb"` + "\n", key: "K", want: "a\tb"},
		{rule: "escape carriage return", input: `K="a\rb"` + "\n", key: "K", want: "a\rb"},
		{rule: "unknown escape stays literal", input: `K="C:\Users\x"` + "\n", key: "K", want: `C:\Users\x`},
		{rule: "compose mode leaves backslash-n literal", input: `K="a\nb"` + "\n",
			key: "K", want: `a\nb`, opts: []Option{WithEscapes(EscapeCompose)}},
		{rule: "no escapes inside single quotes", input: `K='a\nb'` + "\n", key: "K", want: `a\nb`},

		// --- interpolation ---
		{rule: "braced reference", input: "A=x\nB=${A}y\n", key: "B", want: "xy", expand: true},
		{rule: "bare reference", input: "A=x\nB=$A/y\n", key: "B", want: "x/y", expand: true},
		{rule: "default when unset", input: "B=${NOPE:-fb}\n", key: "B", want: "fb", expand: true},
		{rule: "default when empty", input: "A=\nB=${A:-fb}\n", key: "B", want: "fb", expand: true},
		{rule: "dash default only when unset", input: "A=\nB=${A-fb}\n", key: "B", want: "", expand: true},
		{rule: "alternate when set", input: "A=set\nB=${A:+alt}\n", key: "B", want: "alt", expand: true},
		{rule: "alternate when empty", input: "A=\nB=${A:+alt}\n", key: "B", want: "", expand: true},
		{rule: "alternate when unset", input: "B=${NOPE:+alt}\n", key: "B", want: "", expand: true},
		{rule: "unresolved reference is empty", input: "B=${NOPE}\n", key: "B", want: "", expand: true},
		{rule: "chained references", input: "A=x\nB=${A}/y\nC=${B}/z\n", key: "C", want: "x/y/z", expand: true},

		// THE bug this suite caught: a literal value must never be interpolated.
		{rule: "single quotes suppress interpolation", input: "A=x\nB='${A}'\n",
			key: "B", want: "${A}", expand: true},
		{rule: "backticks suppress interpolation", input: "A=x\nB=`${A}`\n",
			key: "B", want: "${A}", expand: true},
		{rule: "double quotes allow interpolation", input: "A=x\nB=\"${A}\"\n",
			key: "B", want: "x", expand: true},

		// --- command substitution ---
		{rule: "command substitution is off by default", input: `K="hi $(echo there)"` + "\n",
			key: "K", want: "hi $(echo there)", expand: true},
		{rule: "command substitution when enabled", input: `K="hi $(echo there)"` + "\n",
			key: "K", want: "hi there", expand: true,
			opts: []Option{WithCommandRunner(func(c string) (string, error) {
				return strings.TrimPrefix(c, "echo "), nil
			})}},
		{rule: "failing command expands to empty", input: `K="a$(boom)b"` + "\n",
			key: "K", want: "ab", expand: true,
			opts: []Option{WithCommandRunner(func(string) (string, error) {
				return "", errors.New("exit 127")
			})}},
		{rule: "command substitution alongside a variable", input: "A=x\nK=\"$A-$(id)\"\n",
			key: "K", want: "x-42", expand: true,
			opts: []Option{WithCommandRunner(func(string) (string, error) { return "42", nil })}},

		// --- misc ---
		{rule: "encrypted prefix stored verbatim", input: "K=encrypted:BFt4x9\n",
			key: "K", want: "encrypted:BFt4x9"},
		{rule: "key with digits and underscores", input: "K1_A=v\n", key: "K1_A", want: "v"},
	}

	for _, tt := range tests {
		t.Run(tt.rule, func(t *testing.T) {
			f := Parse(tt.input, tt.opts...)

			got, ok := f.Get(tt.key)
			var expErr error
			if tt.expand {
				got, ok, expErr = f.GetExpanded(tt.key)
			}
			if expErr != nil {
				t.Fatalf("%s: unexpected expansion error: %v", tt.rule, expErr)
			}
			if !ok {
				t.Fatalf("key %q not found in:\n%s", tt.key, tt.input)
			}
			if got != tt.want {
				t.Errorf("%s: got %q, want %q", tt.rule, got, tt.want)
			}
		})
	}
}

// TestSyntaxConformance_RoundTrips proves conformance did not cost the
// package's core guarantee: every rule above must still render byte-identically.
func TestSyntaxConformance_RoundTrips(t *testing.T) {
	inputs := []string{
		"K = `line one\nline two`\n",
		`K="a\nb"` + "\n",
		"B='${A}'\n",
		"B=${A:+alt}\n",
		`K="hi $(echo there)"` + "\n",
		"VAR=VAL# not a comment\n",
	}

	for _, in := range inputs {
		if got := Parse(in).Render(); got != in {
			t.Errorf("round trip changed the file:\n got: %q\nwant: %q", got, in)
		}
	}
}

func TestEscapeMode_String(t *testing.T) {
	for m, want := range map[EscapeMode]string{
		EscapeExtended: "extended", EscapeCompose: "compose", EscapeMode(9): "unknown",
	} {
		if got := m.String(); got != want {
			t.Errorf("EscapeMode(%d).String() = %q, want %q", int(m), got, want)
		}
	}
}

// TestShellRun exercises the real default runner, since every other test injects
// a fake one.
func TestShellRun(t *testing.T) {
	out, err := shellRun("printf 'hello\\n'")
	if err != nil {
		t.Fatalf("shellRun: %v", err)
	}
	// Trailing newlines are trimmed: $(whoami) in a connection string must not
	// embed a line break.
	if out != "hello" {
		t.Errorf("shellRun = %q, want %q", out, "hello")
	}

	if _, err := shellRun("exit 127"); err == nil {
		t.Error("shellRun of a failing command = nil error")
	}
}

// TestWithCommandSubstitution_UsesRealShell covers the flag-only path, where no
// custom runner was supplied.
func TestWithCommandSubstitution_UsesRealShell(t *testing.T) {
	f := Parse("K=\"a$(printf b)c\"\n", WithCommandSubstitution(true))

	if got, _, _ := f.GetExpanded("K"); got != "abc" {
		t.Errorf("GetExpanded(K) = %q, want %q", got, "abc")
	}
}

// TestWithCommandRunner_NilFallsBackToShell covers the guard in newOptions: a
// caller passing nil must not leave the runner unset, which would panic on the
// first substitution rather than reporting anything useful.
func TestWithCommandRunner_NilFallsBackToShell(t *testing.T) {
	f := Parse("K=\"a$(printf b)c\"\n", WithCommandRunner(nil))

	if got, _, _ := f.GetExpanded("K"); got != "abc" {
		t.Errorf("GetExpanded(K) = %q, want the default shell runner to be used", got)
	}
}

// TestComposeConformance walks every interpolation form documented at
// https://docs.docker.com/reference/compose-file/interpolation/
//
// Compose defines the broadest interpolation set in common use — the bare `+`
// and `?` operators, the `$$` literal-dollar escape, and nesting — so matching
// it means a .env written for any other tool also reads correctly here.
func TestComposeConformance(t *testing.T) {
	tests := []struct {
		rule  string
		input string
		key   string
		want  string
	}{
		// --- direct ---
		{"braced", "A=x\nB=${A}\n", "B", "x"},
		{"bare", "A=x\nB=$A\n", "B", "x"},

		// --- default ---
		{"colon-dash when unset", "B=${NOPE:-d}\n", "B", "d"},
		{"colon-dash when empty", "A=\nB=${A:-d}\n", "B", "d"},
		{"colon-dash when set", "A=x\nB=${A:-d}\n", "B", "x"},
		{"dash when unset", "B=${NOPE-d}\n", "B", "d"},
		{"dash keeps an empty value", "A=\nB=${A-d}\n", "B", ""},

		// --- alternate ---
		{"colon-plus when set and non-empty", "A=x\nB=${A:+alt}\n", "B", "alt"},
		{"colon-plus when empty", "A=\nB=${A:+alt}\n", "B", ""},
		{"colon-plus when unset", "B=${NOPE:+alt}\n", "B", ""},
		{"plus when set", "A=x\nB=${A+alt}\n", "B", "alt"},
		// The bare + fires even for an empty value; only the colon form cares.
		{"plus when empty but set", "A=\nB=${A+alt}\n", "B", "alt"},
		{"plus when unset", "B=${NOPE+alt}\n", "B", ""},

		// --- required, satisfied ---
		{"colon-question when set", "A=x\nB=${A:?msg}\n", "B", "x"},
		{"question when set", "A=x\nB=${A?msg}\n", "B", "x"},
		{"question satisfied by an empty value", "A=\nB=${A?msg}\n", "B", ""},

		// --- literal dollar ---
		{"double dollar escapes a braced reference", "A=x\nB=$${A}\n", "B", "${A}"},
		{"double dollar escapes a bare reference", "A=x\nB=$$A\n", "B", "$A"},
		{"double dollar alone", "B=cost is 5$$\n", "B", "cost is 5$"},
		{"trailing dollar is literal", "B=ends with $\n", "B", "ends with $"},
		{"dollar before a non-name is literal", "B=100$ or so\n", "B", "100$ or so"},

		// --- nesting ---
		{"nested default", "B=${NOPE:-${ALSO_NOPE:-fallback}}\n", "B", "fallback"},
		{"nested default resolves the inner", "F=inner\nB=${NOPE:-${F}}\n", "B", "inner"},
		{"nested alternate", "A=x\nF=y\nB=${A:+${F}}\n", "B", "y"},
		{"reference inside a longer string", "A=x\nB=pre-${A}-post\n", "B", "pre-x-post"},
		{"two references in one value", "A=x\nC=y\nB=${A}${C}\n", "B", "xy"},

		// --- malformed input degrades rather than panics ---
		{"unbalanced brace is literal", "B=${UNCLOSED\n", "B", "${UNCLOSED"},
		{"empty braces", "B=${}\n", "B", ""},
	}

	for _, tt := range tests {
		t.Run(tt.rule, func(t *testing.T) {
			got, ok, err := Parse(tt.input).GetExpanded(tt.key)
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", tt.rule, err)
			}
			if !ok {
				t.Fatalf("key %q not found", tt.key)
			}
			if got != tt.want {
				t.Errorf("%s: got %q, want %q", tt.rule, got, tt.want)
			}
		})
	}
}

// TestRequiredError covers the two forms whose entire purpose is to fail loudly.
// Yielding "" for them would defeat the only reason an author writes one.
func TestRequiredError(t *testing.T) {
	tests := []struct {
		rule      string
		input     string
		wantKey   string
		wantEmpty bool
		wantMsg   string
	}{
		{"colon-question when unset", "B=${NOPE:?set me}\n", "NOPE", true, "set me"},
		{"colon-question when empty", "A=\nB=${A:?set me}\n", "A", true, "set me"},
		{"question when unset", "B=${NOPE?set me}\n", "NOPE", false, "set me"},
		{"no message", "B=${NOPE:?}\n", "NOPE", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.rule, func(t *testing.T) {
			_, ok, err := Parse(tt.input).GetExpanded("B")
			if !ok {
				t.Fatal("key B not found")
			}
			var re *RequiredError
			if !errors.As(err, &re) {
				t.Fatalf("err = %v, want a *RequiredError", err)
			}
			if re.Key != tt.wantKey || re.Empty != tt.wantEmpty || re.Message != tt.wantMsg {
				t.Errorf("got %+v, want key=%q empty=%v msg=%q", re, tt.wantKey, tt.wantEmpty, tt.wantMsg)
			}
			// The message must name the variable, or a user cannot act on it.
			if !strings.Contains(re.Error(), tt.wantKey) {
				t.Errorf("Error() = %q, does not name the variable", re.Error())
			}
			if tt.wantMsg != "" && !strings.Contains(re.Error(), tt.wantMsg) {
				t.Errorf("Error() = %q, drops the author's message", re.Error())
			}
		})
	}
}

// TestExpandedMap_StopsOnRequiredError: a half-expanded map is worse than an
// error, because the caller cannot tell which values are trustworthy.
func TestExpandedMap_StopsOnRequiredError(t *testing.T) {
	m, err := Parse("A=ok\nB=${NOPE:?required}\n").ExpandedMap()

	if err == nil {
		t.Fatal("ExpandedMap = nil error despite an unsatisfied required variable")
	}
	if m != nil {
		t.Errorf("ExpandedMap returned a partial map: %v", m)
	}
}

// TestExpand_LiteralQuotesSuppressEverything: literal delimiters must survive
// even the escape and substitution machinery untouched.
func TestExpand_LiteralQuotesSuppressEverything(t *testing.T) {
	f := Parse("A=x\nS='${A} $$ $(echo hi)'\nB=`${A} $$`\n", WithCommandSubstitution(true))

	for _, key := range []string{"S", "B"} {
		got, _, err := f.GetExpanded(key)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if !strings.Contains(got, "${A}") || !strings.Contains(got, "$$") {
			t.Errorf("%s = %q, want the literal text untouched", key, got)
		}
	}
}

// TestExpand_CycleTerminatesWithNesting re-checks the depth bound against the
// new expander, which recurses differently from the previous one.
func TestExpand_CycleTerminates(t *testing.T) {
	done := make(chan struct{})
	go func() {
		_, _, _ = Parse("A=${B}\nB=${A}\n").GetExpanded("A")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("expansion did not terminate on a reference cycle")
	}
}

func TestMatchers(t *testing.T) {
	t.Run("matchBrace handles nesting", func(t *testing.T) {
		if got := matchBrace("{a${b}c}"); got != 7 {
			t.Errorf("matchBrace = %d, want 7 (the OUTER brace)", got)
		}
		if got := matchBrace("{unclosed"); got != -1 {
			t.Errorf("matchBrace = %d, want -1", got)
		}
	})
	t.Run("matchParen handles nesting", func(t *testing.T) {
		if got := matchParen("(a(b)c)"); got != 6 {
			t.Errorf("matchParen = %d, want 6", got)
		}
		if got := matchParen("(unclosed"); got != -1 {
			t.Errorf("matchParen = %d, want -1", got)
		}
	})
}

// TestExpand_ErrorPropagation drives a RequiredError up through every nested
// path, so a failure deep inside a default or a command cannot be swallowed on
// the way out.
func TestExpand_ErrorPropagation(t *testing.T) {
	tests := []struct {
		rule  string
		input string
		opts  []Option
	}{
		{rule: "inside a default", input: "B=${NOPE:-${ALSO:?required}}\n"},
		{rule: "inside an alternate", input: "A=x\nB=${A:+${ALSO:?required}}\n"},
		{rule: "inside a bare-dash default", input: "B=${NOPE-${ALSO:?required}}\n"},
		{rule: "inside a bare-plus alternate", input: "A=x\nB=${A+${ALSO:?required}}\n"},
		{rule: "through a chained reference", input: "C=${ALSO:?required}\nB=${C}\n"},
		{rule: "inside a command substitution", input: "B=\"$(echo ${ALSO:?required})\"\n",
			opts: []Option{WithCommandRunner(func(string) (string, error) { return "", nil })}},
		{rule: "in a bare reference", input: "C=${ALSO:?required}\nB=$C\n"},
	}

	for _, tt := range tests {
		t.Run(tt.rule, func(t *testing.T) {
			_, _, err := Parse(tt.input, tt.opts...).GetExpanded("B")
			var re *RequiredError
			if !errors.As(err, &re) {
				t.Errorf("err = %v, want the RequiredError to propagate", err)
			}
		})
	}
}

// TestExpandCommand_Degradations covers the substitution paths that are not the
// happy one.
func TestExpandCommand_Degradations(t *testing.T) {
	t.Run("unbalanced paren is a literal dollar", func(t *testing.T) {
		got, _, err := Parse("B=cost $(unclosed\n", WithCommandSubstitution(true)).GetExpanded("B")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(got, "$(unclosed") {
			t.Errorf("got %q, want the text preserved", got)
		}
	})

	t.Run("disabled substitution preserves the text", func(t *testing.T) {
		const in = "B=\"a$(echo b)c\"\n"
		got, _, _ := Parse(in).GetExpanded("B")
		if got != "a$(echo b)c" {
			t.Errorf("got %q, want the substitution left verbatim", got)
		}
		// It must also still round-trip, not just read back.
		if out := Parse(in).Render(); out != in {
			t.Errorf("round trip = %q, want %q", out, in)
		}
	})

	t.Run("command may reference a variable", func(t *testing.T) {
		f := Parse("NAME=world\nB=\"$(greet ${NAME})\"\n",
			WithCommandRunner(func(c string) (string, error) { return strings.ToUpper(c), nil }))
		got, _, err := f.GetExpanded("B")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "GREET WORLD" {
			t.Errorf("got %q, want the variable expanded before the command ran", got)
		}
	})
}

// TestExpandedMap_LiteralEntries covers the non-interpolating branch of the map
// walk, and the non-pair skip.
func TestExpandedMap_LiteralEntries(t *testing.T) {
	m, err := Parse("# c\n\nA=x\nB='${A}'\n").ExpandedMap()
	if err != nil {
		t.Fatalf("ExpandedMap: %v", err)
	}
	if m["B"] != "${A}" {
		t.Errorf("ExpandedMap()[B] = %q, want the literal preserved", m["B"])
	}
	if len(m) != 2 {
		t.Errorf("ExpandedMap() = %v, want only the two pairs", m)
	}
}

// TestColonDelimiter pins the second delimiter of the Compose grammar: both
// compose-go and Node dotenv accept `KEY: value` alongside `KEY=value`, so a
// file written for either tool must parse to the same keys here.
func TestColonDelimiter(t *testing.T) {
	tests := []struct {
		name, input, key, want string
	}{
		{"colon with a space", "FOO: bar\n", "FOO", "bar"},
		{"colon without a space", "FOO:bar\n", "FOO", "bar"},
		{"space before the colon", "FOO : bar\n", "FOO", "bar"},
		{"quoted value", `FOO: "a b"` + "\n", "FOO", "a b"},
		{"export prefix", "export FOO: bar\n", "FOO", "bar"},
		{"empty value", "FOO:\n", "FOO", ""},
		// A URL line parses as key `http`, value `//example.com` — exactly what
		// compose-go does with it. Documented rather than special-cased: the line
		// still round-trips verbatim, and inventing an exception would mean this
		// package and Compose disagree about the same bytes.
		{"bare URL becomes a pair, as in Compose", "http://example.com\n", "http", "//example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Parse(tt.input)
			got, ok := f.Get(tt.key)
			if !ok {
				t.Fatalf("Get(%q) not found in %q", tt.key, tt.input)
			}
			if got != tt.want {
				t.Errorf("Get(%q) = %q, want %q", tt.key, got, tt.want)
			}
			if out := f.Render(); out != tt.input {
				t.Errorf("round trip changed the line: %q", out)
			}
		})
	}
}

// TestColonDelimiter_EditPreservesDelimiter asserts the byte-preservation
// contract extends to the author's delimiter choice: editing a colon-delimited
// pair must not silently rewrite it to `=`.
func TestColonDelimiter_EditPreservesDelimiter(t *testing.T) {
	f := Parse("FOO: bar\nKEEP=1\n")

	// The no-op contract first: setting the current value changes nothing.
	f.Set("FOO", "bar")
	if got := f.Render(); got != "FOO: bar\nKEEP=1\n" {
		t.Fatalf("no-op Set changed bytes: %q", got)
	}

	f.Set("FOO", "baz")
	if got := f.Render(); got != "FOO:baz\nKEEP=1\n" {
		t.Errorf("Set rewrote the delimiter: %q", got)
	}
	if got, _ := Parse(f.Render()).Get("FOO"); got != "baz" {
		t.Errorf("edited colon pair reads back %q, want %q", got, "baz")
	}
}

// TestInherited pins the middle-ground contract for Compose name-only lines:
// the declaration is recognised (KindInherited, listed by Inherited) but the
// value is NEVER resolved — this package does not read os.Environ, so the
// entry stays invisible to Get, Keys, Map, and expansion.
func TestInherited(t *testing.T) {
	src := "A=1\nHOME\nexport AWS_SECRET_ACCESS_KEY\nHOME\n"
	f := Parse(src)

	if got := f.Inherited(); len(got) != 2 || got[0] != "HOME" || got[1] != "AWS_SECRET_ACCESS_KEY" {
		t.Errorf("Inherited() = %v, want [HOME AWS_SECRET_ACCESS_KEY] (file order, deduplicated)", got)
	}
	if e := f.Entries()[1]; e.Kind != KindInherited || e.Key != "HOME" {
		t.Errorf("entry 1 = %s %q, want inherited HOME", e.Kind, e.Key)
	}

	// Inactive everywhere a value would be needed.
	if _, ok := f.Get("HOME"); ok {
		t.Error("Get(HOME) found a value for an inherited declaration")
	}
	if f.Has("HOME") {
		t.Error("Has(HOME) = true for an inherited declaration")
	}
	if ks := f.Keys(); len(ks) != 1 || ks[0] != "A" {
		t.Errorf("Keys() = %v, want only the real pair", ks)
	}
	if m := f.Map(); len(m) != 1 {
		t.Errorf("Map() = %v, want only the real pair", m)
	}

	if out := f.Render(); out != src {
		t.Errorf("round trip changed the file: %q", out)
	}
}

// TestInherited_ReferenceExpandsToEmpty asserts a ${reference} to an inherited
// name behaves like any other unset variable: it expands to "", because
// resolving it from the process environment is exactly what this package
// promises never to do.
func TestInherited_ReferenceExpandsToEmpty(t *testing.T) {
	f := Parse("HOME\nB=${HOME:-fallback}\nC=${HOME}\n")

	if v, _, _ := f.GetExpanded("B"); v != "fallback" {
		t.Errorf("GetExpanded(B) = %q, want the default to fire for an unresolved inherited name", v)
	}
	if v, _, _ := f.GetExpanded("C"); v != "" {
		t.Errorf("GetExpanded(C) = %q, want \"\" for an unresolved inherited name", v)
	}
}

// TestKeyCharset_ComposeSuperset pins the widened key charset against each
// character class it adds, and the characters that remain rejected.
func TestKeyCharset_ComposeSuperset(t *testing.T) {
	accepted := []struct{ input, key, want string }{
		{"2FA_SECRET=x\n", "2FA_SECRET", "x"},
		{"MY-SERVICE-TOKEN=x\n", "MY-SERVICE-TOKEN", "x"},
		{"spring.datasource.url=x\n", "spring.datasource.url", "x"},
		{"SERVICES[0]=x\n", "SERVICES[0]", "x"},
	}
	for _, tt := range accepted {
		if got, ok := Parse(tt.input).Get(tt.key); !ok || got != tt.want {
			t.Errorf("Get(%q) = %q, %v — want %q accepted", tt.key, got, ok, tt.want)
		}
	}

	// Space, '!', and '@' stay outside the charset: the line is preserved
	// verbatim as unrecognised, exactly as before the widening.
	for _, input := range []string{"HAS KEY=v\n", "K!=v\n", "K@HOST=v\n"} {
		f := Parse(input)
		if len(f.Keys()) != 0 {
			t.Errorf("Parse(%q) produced keys %v, want none", input, f.Keys())
		}
		if out := f.Render(); out != input {
			t.Errorf("round trip changed the rejected line: %q", out)
		}
	}
}
