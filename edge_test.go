package dotenv

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestEdgeCases_Parsing asserts behaviour for inputs that are unusual, malformed,
// or ambiguous.
//
// These are the cases where a parser quietly does something surprising. Pinning
// them means a future change to the scanner cannot alter any of these silently.
func TestEdgeCases_Parsing(t *testing.T) {
	tests := []struct {
		name  string
		input string
		key   string
		want  string
		// found is false when the line must NOT be recognised as a pair.
		found bool
		kinds []Kind
	}{
		// --- keys the spec rejects: kept verbatim, never guessed at ---
		{name: "empty key", input: "=value\n", key: "", found: false, kinds: []Kind{KindOther}},
		// --- keys the wider Compose charset accepts (digits-first, dash, dot,
		// brackets) — real pairs everywhere in the ecosystem, so real pairs here ---
		{name: "key starting with a digit", input: "1K=v\n", key: "1K", want: "v", found: true},
		{name: "key containing a dash", input: "A-B=v\n", key: "A-B", want: "v", found: true},
		{name: "key containing dots", input: "spring.datasource.url=jdbc:x\n", key: "spring.datasource.url", want: "jdbc:x", found: true},
		{name: "key containing brackets", input: "SERVICES[0]=api\n", key: "SERVICES[0]", want: "api", found: true},
		{name: "non-ascii key", input: "Ké=v\n", key: "Ké", found: false, kinds: []Kind{KindOther}},
		{name: "no equals sign at all", input: "just some text\n", key: "", found: false, kinds: []Kind{KindOther}},

		// --- values ---
		{name: "equals signs inside the value", input: "K=a=b=c\n", key: "K", want: "a=b=c", found: true},
		{name: "non-ascii value", input: "K=héllo→\n", key: "K", want: "héllo→", found: true},
		{name: "double quote inside a bare value", input: `K=a"b` + "\n", key: "K", want: `a"b`, found: true},
		{name: "apostrophe inside a bare value", input: "K=it's\n", key: "K", want: "it's", found: true},
		{name: "single quotes inside double quotes", input: `K="say 'hi'"` + "\n", key: "K", want: "say 'hi'", found: true},
		{name: "double quotes inside single quotes", input: `K='say "hi"'` + "\n", key: "K", want: `say "hi"`, found: true},
		// Trailing whitespace on a bare value is not part of it; quote to keep it.
		{name: "bare value of only spaces", input: "K=   \n", key: "K", want: "", found: true},
		{name: "quoted value of only spaces", input: `K="   "` + "\n", key: "K", want: "   ", found: true},
		{name: "trailing backslash in a bare value", input: `K=ends\` + "\n", key: "K", want: `ends\`, found: true},
		{name: "equals and an inline comment", input: "K=a=b #c\n", key: "K", want: "a=b", found: true},

		// --- comments and prefixes ---
		{name: "comment with no space after hash", input: "#comment\nK=v\n", key: "K", want: "v", found: true,
			kinds: []Kind{KindComment, KindPair}},
		{name: "indented comment", input: "   # indented\nK=v\n", key: "K", want: "v", found: true,
			kinds: []Kind{KindComment, KindPair}},
		{name: "tab between export and key", input: "export\tK=v\n", key: "K", want: "v", found: true},
		{name: "spaces around the equals sign", input: "  K  =  v\n", key: "K", want: "v", found: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Parse(tt.input)

			got, ok := f.Get(tt.key)
			if ok != tt.found {
				t.Fatalf("Get(%q) found = %v, want %v (value %q)", tt.key, ok, tt.found, got)
			}
			if tt.found && got != tt.want {
				t.Errorf("Get(%q) = %q, want %q", tt.key, got, tt.want)
			}
			if tt.kinds != nil {
				if len(f.Entries()) != len(tt.kinds) {
					t.Fatalf("got %d entries, want %d", len(f.Entries()), len(tt.kinds))
				}
				for i, want := range tt.kinds {
					if got := f.Entries()[i].Kind; got != want {
						t.Errorf("entry %d kind = %s, want %s", i, got, want)
					}
				}
			}
			// Every edge case must still round-trip: an input the parser does
			// not understand must survive untouched rather than be normalised.
			if out := f.Render(); out != tt.input {
				t.Errorf("round trip changed the file:\n got: %q\nwant: %q", out, tt.input)
			}
		})
	}
}

// TestEdgeCases_Expansion pins the degenerate reference forms.
func TestEdgeCases_Expansion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		key   string
		want  string
	}{
		{"empty braces", "K=${}\n", "K", ""},
		{"empty name with a default", "K=${:-d}\n", "K", "d"},
		// A cycle resolves to "" — the same answer an unset variable gives,
		// and the only one that terminates. The previous behaviour leaked the
		// raw text back, which was an artefact of hitting the depth bound.
		{"self reference", "A=${A}\n", "A", ""},
		{"mutual cycle", "A=${B}\nB=${A}\n", "A", ""},
		// Two SEPARATE references to one variable are not a cycle.
		{"same variable referenced twice", "A=x\nK=${A}-${A}\n", "K", "x-x"},
		// The case fuzzing found. Each of the two $A references expands once
		// and is then cut on re-entry, so this terminates with a deterministic
		// result; without cycle detection it is 2^depth expansions and hangs.
		{"branching self reference terminates", "A=$A$A{\n", "A", "{{{"},
		{"forward reference to a later key", "A=${B}\nB=later\n", "A", "later"},
		{"reference to a commented-out key uses the default", "# B=x\nK=${B:-fb}\n", "K", "fb"},
		{"reference to a duplicated key takes the last", "B=one\nB=two\nK=${B}\n", "K", "two"},
		{"three dollars", "K=$$$\n", "K", "$$"},
		{"unterminated brace stays literal", "K=${\n", "K", "${"},
		{"unterminated brace mid-value", "K=a${B\n", "K", "a${B"},
		{"deep nesting", "K=${A:-${B:-${C:-deep}}}\n", "K", "deep"},
		{"nesting resolves the innermost that is set", "C=found\nK=${A:-${B:-${C}}}\n", "K", "found"},
		{"expansion inside a multiline value", "A=x\nK=\"one ${A}\ntwo\"\n", "K", "one x\ntwo"},
		{"adjacent references", "A=1\nB=2\nK=${A}${B}\n", "K", "12"},
		{"reference immediately followed by text", "A=1\nK=${A}abc\n", "K", "1abc"},
		// A bare reference ends at the first non-name character.
		{"bare reference ends at a dot", "A=1\nK=$A.tld\n", "K", "1.tld"},
		{"bare reference ends at a slash", "A=1\nK=$A/x\n", "K", "1/x"},
		{"bare reference is case sensitive", "a=lower\nA=upper\nK=$A\n", "K", "upper"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Parse(tt.input)

			got, ok, err := f.GetExpanded(tt.key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !ok {
				t.Fatalf("key %q not found", tt.key)
			}
			if got != tt.want {
				t.Errorf("GetExpanded(%q) = %q, want %q", tt.key, got, tt.want)
			}
			if out := f.Render(); out != tt.input {
				t.Errorf("round trip changed the file:\n got: %q\nwant: %q", out, tt.input)
			}
		})
	}
}

// TestEdgeCases_Editing covers operation sequences, where state carried between
// calls is most likely to go wrong.
func TestEdgeCases_Editing(t *testing.T) {
	t.Run("set then unset then set", func(t *testing.T) {
		f := Parse("A=1\n")
		f.Set("A", "2")
		f.Unset("A", false)
		created := f.Set("A", "3")

		if !created {
			t.Error("after Unset the key is inactive, so Set must append a new entry")
		}
		if v, _ := f.Get("A"); v != "3" {
			t.Errorf("Get(A) = %q, want %q", v, "3")
		}
		if !strings.Contains(f.Render(), "# A=2") {
			t.Errorf("the commented-out entry was lost:\n%s", f.Render())
		}
	})

	t.Run("unset removes only the last duplicate", func(t *testing.T) {
		f := Parse("A=1\nA=2\nA=3\n")
		f.Unset("A", true)

		if v, _ := f.Get("A"); v != "2" {
			t.Errorf("Get(A) = %q, want %q — only the last entry should go", v, "2")
		}
		if n := f.Count("A"); n != 2 {
			t.Errorf("Count(A) = %d, want 2", n)
		}
	})

	t.Run("set never touches an unrecognised line", func(t *testing.T) {
		const in = "not a pair\nA=1\n"
		f := Parse(in)
		f.Set("A", "2")

		if !strings.HasPrefix(f.Render(), "not a pair\n") {
			t.Errorf("the unrecognised line was disturbed:\n%s", f.Render())
		}
	})

	t.Run("set on an empty file", func(t *testing.T) {
		f := Parse("")
		f.Set("A", "1")

		if got := f.Render(); got != "A=1\n" {
			t.Errorf("Render() = %q", got)
		}
	})

	t.Run("set preserves a missing trailing newline", func(t *testing.T) {
		f := Parse("A=1")
		f.Set("A", "2")

		if got := f.Render(); got != "A=2" {
			t.Errorf("Render() = %q, want no trailing newline added", got)
		}
	})

	t.Run("set preserves CRLF", func(t *testing.T) {
		f := Parse("A=1\r\nB=2\r\n")
		f.Set("A", "changed")

		got := f.Render()
		if !strings.Contains(got, "\r\n") || strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
			t.Errorf("Render() = %q, want CRLF throughout", got)
		}
	})

	t.Run("set a multiline value into a CRLF file", func(t *testing.T) {
		f := Parse("A=1\r\n")
		f.Set("A", "one\ntwo")

		got := f.Render()
		if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
			t.Errorf("Render() = %q, want every terminator to be CRLF", got)
		}
		if v, _ := Parse(got).Get("A"); v != "one\ntwo" {
			t.Errorf("round trip = %q, want %q", v, "one\ntwo")
		}
	})
}

// TestEdgeCases_QuotingRoundTrip is the strongest guarantee the editor offers:
// whatever Set writes, the parser reads back identically. Every value here would
// be corrupted by naive quoting.
func TestEdgeCases_QuotingRoundTrip(t *testing.T) {
	values := []string{
		"", " ", "  spaced  ", "tab\there", "new\nline", "multi\nline\nvalue",
		`"quoted"`, "'single'", "`backtick`", `back\slash`, `\\double`,
		"#hash", "with # hash", "a=b", "$VAR", "${VAR}", "$$", "$(cmd)",
		"héllo→", "trailing\\", "\"", "'", "\\", "\n", "a\n\nb",
		"line1\nline2\n", "  leading", "trailing  ",
	}

	for _, v := range values {
		t.Run(strings.ReplaceAll(v, "\n", "\\n"), func(t *testing.T) {
			f := Parse("K=placeholder\n")
			f.Set("K", v)

			back, ok := Parse(f.Render()).Get("K")
			if !ok {
				t.Fatalf("key lost after Set(%q):\n%s", v, f.Render())
			}
			if back != v {
				t.Errorf("Set(%q) → Render → Get = %q\nrendered as: %q", v, back, f.Render())
			}
		})
	}
}

// TestEdgeCases_SetIsIdempotent: applying the same edit twice must be identical
// to applying it once, or repeated tooling runs produce churn in version control.
func TestEdgeCases_SetIsIdempotent(t *testing.T) {
	values := []string{"simple", "with space", "multi\nline", `"quoted"`, ""}

	for _, v := range values {
		f := Parse("K=start\n# note\n")
		f.Set("K", v)
		once := f.Render()
		f.Set("K", v)

		if twice := f.Render(); twice != once {
			t.Errorf("Set(%q) applied twice differs:\n once: %q\ntwice: %q", v, once, twice)
		}
	}
}

// TestEdgeCases_LineEndings pins the per-line terminator handling that fuzzing
// forced. A single crlf flag for the whole file normalised mixed endings into
// one style — a whole-file diff nobody asked for.
func TestEdgeCases_LineEndings(t *testing.T) {
	t.Run("mixed endings round-trip verbatim", func(t *testing.T) {
		for _, in := range []string{
			"\r\n\n", "\n\r\n", "A=1\r\nB=2\n", "A=1\nB=2\r\n",
			"# c\r\n\nA=1\r\n", "A=\"x\r\ny\"\n",
		} {
			if got := Parse(in).Render(); got != in {
				t.Errorf("round trip changed the file:\n got: %q\nwant: %q", got, in)
			}
		}
	})

	t.Run("a CRLF file gets CRLF on new lines", func(t *testing.T) {
		f := Parse("A=1\r\n")
		f.Set("B", "2")

		if got := f.Render(); got != "A=1\r\nB=2\r\n" {
			t.Errorf("Render() = %q, want the appended line to use CRLF", got)
		}
	})

	t.Run("an LF file gets LF on new lines", func(t *testing.T) {
		f := Parse("A=1\n")
		f.Set("B", "2")

		if got := f.Render(); got != "A=1\nB=2\n" {
			t.Errorf("Render() = %q", got)
		}
	})

	t.Run("CRLF values decode without a stray carriage return", func(t *testing.T) {
		f := Parse("A=1\r\nB=\"x\r\ny\"\r\n")

		if v, _ := f.Get("A"); v != "1" {
			t.Errorf("Get(A) = %q, want %q — the terminator must not leak into the value", v, "1")
		}
		if v, _ := f.Get("B"); v != "x\ny" {
			t.Errorf("Get(B) = %q, want %q", v, "x\ny")
		}
	})

	t.Run("editing a CRLF multiline value keeps CRLF", func(t *testing.T) {
		f := Parse("A=old\r\n")
		f.Set("A", "one\ntwo\nthree")

		got := f.Render()
		if strings.Count(got, "\r\n") != strings.Count(got, "\n") {
			t.Errorf("Render() = %q, want every terminator to be CRLF", got)
		}
		if v, _ := Parse(got).Get("A"); v != "one\ntwo\nthree" {
			t.Errorf("round trip = %q", v)
		}
	})
}

// TestEdgeCases_CycleDetection covers the termination guarantee fuzzing exposed:
// a depth bound alone cannot stop a value that branches at every level.
func TestEdgeCases_CycleDetection(t *testing.T) {
	inputs := []string{
		"A=$A$A{\n",                    // the input fuzzing found
		"A=${A}${A}\n",                 // braced form of the same
		"A=${B}\nB=${A}\n",             // mutual
		"A=${B}\nB=${C}\nC=${A}\n",     // three-way
		"A=${A:-${A:-${A}}}\n",         // nested self-reference
		"A=$A$A$A$A$A$A$A$A$A$A$A$A\n", // wide branching
	}

	for _, in := range inputs {
		done := make(chan struct{})
		go func() {
			f := Parse(in)
			for _, k := range f.Keys() {
				_, _, _ = f.GetExpanded(k)
			}
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("expansion did not terminate for %q", in)
		}
	}
}

// TestNeverTouchesEnviron pins the package's scope boundary mechanically.
//
// Reading a .env and exporting it into the process are separate decisions.
// Conflating them is why godotenv.Load means something this package must not
// mean, and a stray os.Setenv anywhere in here would make that distinction a
// lie — silently, since nothing else would fail.
func TestNeverTouchesEnviron(t *testing.T) {
	before := os.Environ()

	f := Parse(sample, WithCommandRunner(func(string) (string, error) { return "x", nil }))
	for _, k := range f.Keys() {
		_, _, _ = f.GetExpanded(k)
	}
	_, _ = f.ExpandedMap()
	f.Set("NEW_KEY", "value")
	f.Unset("APP_NAME", false)
	_, _ = f.SetAfter("DB_PORT", "ANOTHER", "v")
	_ = f.Render()

	path := filepath.Join(t.TempDir(), ".env")
	f.Path = path
	if err := f.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := Open(path); err != nil {
		t.Fatalf("Open: %v", err)
	}

	after := os.Environ()
	if len(before) != len(after) {
		t.Fatalf("the environment changed size: %d -> %d", len(before), len(after))
	}
	sort.Strings(before)
	sort.Strings(after)
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("the environment was modified: %q -> %q", before[i], after[i])
		}
	}
}

// TestParse_TakesAnySource documents that no filesystem is involved, so content
// from a stream, an embedded fixture, or a secret store parses the same way.
func TestParse_TakesAnySource(t *testing.T) {
	const src = "A=1\n# note\nB=2\n"

	fromString := Parse(src)

	// What a caller with an io.Reader writes — deliberately their line, not an
	// API of ours: it keeps the read-it-all cost visible where it happens.
	b, err := io.ReadAll(strings.NewReader(src))
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	fromReader := Parse(string(b))

	if fromString.Render() != fromReader.Render() {
		t.Error("source of the bytes changed the result")
	}
	if len(fromReader.Keys()) != 2 {
		t.Errorf("Keys() = %v", fromReader.Keys())
	}
}

// --- ParseReader ------------------------------------------------------------

func TestParseReader(t *testing.T) {
	t.Run("matches Parse for the same bytes", func(t *testing.T) {
		f, err := ParseReader(strings.NewReader(sample))
		if err != nil {
			t.Fatalf("ParseReader: %v", err)
		}
		if f.Render() != Parse(sample).Render() {
			t.Error("ParseReader and Parse disagree")
		}
	})

	t.Run("options are honoured", func(t *testing.T) {
		f, err := ParseReader(strings.NewReader(`K="a\nb"`+"\n"), WithEscapes(EscapeCompose))
		if err != nil {
			t.Fatalf("ParseReader: %v", err)
		}
		if v, _ := f.Get("K"); v != `a\nb` {
			t.Errorf("Get(K) = %q, want the compose dialect", v)
		}
	})

	t.Run("empty reader", func(t *testing.T) {
		f, err := ParseReader(strings.NewReader(""))
		if err != nil {
			t.Fatalf("ParseReader: %v", err)
		}
		if got := f.Render(); got != "" {
			t.Errorf("Render() = %q", got)
		}
	})

	t.Run("a read failure is reported, not swallowed", func(t *testing.T) {
		_, err := ParseReader(&failingReader{err: errors.New("network died")})
		if err == nil {
			t.Fatal("ParseReader = nil error on a failing reader")
		}
		if !strings.Contains(err.Error(), "network died") {
			t.Errorf("err = %v, want the cause preserved", err)
		}
	})

	// A multi-megabyte .env is real once a value holds a PEM block or a base64
	// certificate, which is the case ParseReader exists for.
	t.Run("a multi-megabyte value round-trips", func(t *testing.T) {
		blob := strings.Repeat("QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo=", 40_000) // ~1.4 MB
		src := "SMALL=1\nCERT=\"" + blob + "\"\nAFTER=2\n"

		f, err := ParseReader(strings.NewReader(src))
		if err != nil {
			t.Fatalf("ParseReader: %v", err)
		}
		if v, _ := f.Get("CERT"); v != blob {
			t.Errorf("the large value did not survive: len %d, want %d", len(v), len(blob))
		}
		// The entries around it must still parse — proof the scanner did not
		// lose its place inside a very long line.
		if v, _ := f.Get("AFTER"); v != "2" {
			t.Errorf("Get(AFTER) = %q, want %q", v, "2")
		}
		if f.Render() != src {
			t.Error("a multi-megabyte file did not round-trip")
		}
	})
}

// failingReader fails on the first read, so the error path is exercised without
// a real I/O failure.
type failingReader struct{ err error }

func (r *failingReader) Read([]byte) (int, error) { return 0, r.err }

// BenchmarkParseLarge compares the two ways of getting a reader into the parser.
// Run with -benchmem: ReadAll+string allocates roughly twice the file size,
// because the []byte→string conversion copies; ParseReader copies once.
func BenchmarkParseLarge(b *testing.B) {
	src := "CERT=\"" + strings.Repeat("QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo=", 40_000) + "\"\n"

	b.Run("ReadAll then Parse", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			raw, err := io.ReadAll(strings.NewReader(src))
			if err != nil {
				b.Fatal(err)
			}
			_ = Parse(string(raw))
		}
	})

	b.Run("ParseReader", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := ParseReader(strings.NewReader(src)); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// TestOpen_PermissionError covers the open failure that is NOT "missing file" —
// a missing file bootstraps an empty File, anything else must be reported.
func TestOpen_PermissionError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("A=1\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	_, err := Open(path)
	if err == nil {
		t.Fatal("Open of an unreadable file = nil error")
	}
	if !strings.Contains(err.Error(), "open") {
		t.Errorf("err = %v, want it to name the failed operation", err)
	}
}

// TestOpen_ErrorMessagesAreNotDoubled: each layer attaches its own context, so
// wrapping twice would read "dotenv: read x: dotenv: read: ...".
func TestOpen_ErrorMessagesAreNotDoubled(t *testing.T) {
	_, err := Open(t.TempDir()) // a directory opens, then fails to read
	if err == nil {
		t.Fatal("Open of a directory = nil error")
	}
	if n := strings.Count(err.Error(), "dotenv:"); n != 1 {
		t.Errorf("err = %q\ncontains %d %q prefixes, want 1", err, n, "dotenv:")
	}
}
