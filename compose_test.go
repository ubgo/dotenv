package dotenv

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// --- constructors -----------------------------------------------------------

func TestConstructors(t *testing.T) {
	t.Run("NewComment decorates plain text", func(t *testing.T) {
		e := NewComment("----------", "DATABASE", "----------")

		want := []string{"# ----------", "# DATABASE", "# ----------"}
		if !reflect.DeepEqual(e.Raw, want) {
			t.Errorf("Raw = %q, want %q", e.Raw, want)
		}
		if e.Kind != KindComment {
			t.Errorf("Kind = %s, want %s", e.Kind, KindComment)
		}
	})

	t.Run("NewComment leaves pre-decorated lines alone", func(t *testing.T) {
		// Otherwise a caller passing "# x" would end up with "## x".
		e := NewComment("# already", "  # indented", "plain")

		want := []string{"# already", "  # indented", "# plain"}
		if !reflect.DeepEqual(e.Raw, want) {
			t.Errorf("Raw = %q, want %q", e.Raw, want)
		}
	})

	t.Run("NewComment with no lines is still a comment", func(t *testing.T) {
		// An empty Raw would render as nothing and silently vanish.
		if e := NewComment(); len(e.Raw) != 1 || e.Raw[0] != "#" {
			t.Errorf("Raw = %q, want a bare marker", e.Raw)
		}
	})

	t.Run("NewPair defers rendering", func(t *testing.T) {
		e := NewPair("K", "v")

		if e.Key != "K" || e.Value != "v" {
			t.Errorf("got %q=%q", e.Key, e.Value)
		}
		// Raw is empty until attached: the line ending belongs to the
		// destination file, which this entry does not know yet.
		if len(e.Raw) != 0 {
			t.Errorf("Raw = %q, want empty before attachment", e.Raw)
		}
	})

	t.Run("NewBlank", func(t *testing.T) {
		if e := NewBlank(); e.Kind != KindBlank || len(e.Raw) != 1 || e.Raw[0] != "" {
			t.Errorf("NewBlank() = %+v", e)
		}
	})
}

// --- Append -----------------------------------------------------------------

func TestAppend(t *testing.T) {
	t.Run("a generated section", func(t *testing.T) {
		f := Parse("EXISTING=1\n")
		f.Append(
			NewBlank(),
			NewComment("----------", "DATABASE", "----------"),
			NewPair("DB_HOST", "localhost"),
			NewPair("DB_PORT", "5432"),
		)

		want := "EXISTING=1\n\n# ----------\n# DATABASE\n# ----------\nDB_HOST=localhost\nDB_PORT=5432\n"
		if got := f.Render(); got != want {
			t.Errorf("Render() =\n%q\nwant\n%q", got, want)
		}
		if v, _ := f.Get("DB_HOST"); v != "localhost" {
			t.Errorf("Get(DB_HOST) = %q", v)
		}
	})

	t.Run("into an empty file", func(t *testing.T) {
		f := Parse("")
		f.Append(NewComment("header"), NewPair("A", "1"))

		if got := f.Render(); got != "# header\nA=1\n" {
			t.Errorf("Render() = %q", got)
		}
	})

	t.Run("is literal, not upsert", func(t *testing.T) {
		// Set guarantees one active entry; Append places exactly what it is
		// given. A generator depends on that difference.
		f := Parse("A=1\n")
		f.Append(NewPair("A", "2"))

		if got := f.Render(); got != "A=1\nA=2\n" {
			t.Errorf("Render() = %q, want the duplicate preserved", got)
		}
		if n := f.Count("A"); n != 2 {
			t.Errorf("Count(A) = %d, want 2", n)
		}
		// Last wins, so the duplicate is what a consumer sees.
		if v, _ := f.Get("A"); v != "2" {
			t.Errorf("Get(A) = %q, want %q", v, "2")
		}
	})

	t.Run("quotes values that need it", func(t *testing.T) {
		f := Parse("")
		f.Append(NewPair("K", "has space # and hash"))

		if v, _ := Parse(f.Render()).Get("K"); v != "has space # and hash" {
			t.Errorf("round trip = %q", v)
		}
	})

	t.Run("uses the file's line ending", func(t *testing.T) {
		f := Parse("A=1\r\n")
		f.Append(NewComment("note"), NewPair("B", "2"), NewBlank())

		got := f.Render()
		if strings.Count(got, "\r\n") != strings.Count(got, "\n") {
			t.Errorf("Render() = %q, want every terminator to be CRLF", got)
		}
	})

	t.Run("an already-attached entry is copied verbatim", func(t *testing.T) {
		// Re-rendering would discard the exact bytes this package preserves.
		src := Parse("  SPACED   =   padded value   # note\n")
		f := Parse("A=1\n")
		f.Append(src.Entries()[0])

		if !strings.Contains(f.Render(), "  SPACED   =   padded value   # note") {
			t.Errorf("the entry was re-rendered:\n%s", f.Render())
		}
	})

	t.Run("appending nothing changes nothing", func(t *testing.T) {
		f := Parse("A=1\n")
		f.Append()
		if got := f.Render(); got != "A=1\n" {
			t.Errorf("Render() = %q", got)
		}
	})
}

// --- Insert -----------------------------------------------------------------

func TestInsert(t *testing.T) {
	const src = "# db\nDB_HOST=h\nDB_PORT=p\n\n# auth\nAUTH_KEY=k\n"

	t.Run("after places a whole block", func(t *testing.T) {
		f := Parse(src)
		if err := f.InsertAfter("DB_PORT", NewComment("added"), NewPair("DB_PASSWORD", "s")); err != nil {
			t.Fatalf("InsertAfter: %v", err)
		}

		want := "# db\nDB_HOST=h\nDB_PORT=p\n# added\nDB_PASSWORD=s\n\n# auth\nAUTH_KEY=k\n"
		if got := f.Render(); got != want {
			t.Errorf("Render() =\n%q\nwant\n%q", got, want)
		}
	})

	t.Run("before places a whole block", func(t *testing.T) {
		f := Parse(src)
		if err := f.InsertBefore("DB_HOST", NewPair("DB_DRIVER", "pg"), NewComment("above")); err != nil {
			t.Fatalf("InsertBefore: %v", err)
		}

		want := "# db\nDB_DRIVER=pg\n# above\nDB_HOST=h\nDB_PORT=p\n\n# auth\nAUTH_KEY=k\n"
		if got := f.Render(); got != want {
			t.Errorf("Render() =\n%q\nwant\n%q", got, want)
		}
	})

	t.Run("order within the block is preserved", func(t *testing.T) {
		f := Parse("A=1\n")
		if err := f.InsertAfter("A", NewPair("B", "2"), NewPair("C", "3"), NewPair("D", "4")); err != nil {
			t.Fatal(err)
		}
		if got := f.Render(); got != "A=1\nB=2\nC=3\nD=4\n" {
			t.Errorf("Render() = %q", got)
		}
	})

	t.Run("a missing anchor errors and changes nothing", func(t *testing.T) {
		f := Parse(src)
		err := f.InsertAfter("NOPE", NewPair("X", "1"))

		if !errors.Is(err, ErrAnchorNotFound) {
			t.Fatalf("err = %v, want ErrAnchorNotFound", err)
		}
		if got := f.Render(); got != src {
			t.Errorf("the file changed despite the error:\n%s", got)
		}
	})

	t.Run("a disabled pair is not a valid anchor", func(t *testing.T) {
		f := Parse("# OLD=x\nA=1\n")
		if err := f.InsertAfter("OLD", NewPair("X", "1")); !errors.Is(err, ErrAnchorNotFound) {
			t.Errorf("err = %v, want ErrAnchorNotFound — a disabled entry is inactive", err)
		}
	})

	t.Run("after a multi-line value clears the block", func(t *testing.T) {
		f := Parse("KEY=\"one\ntwo\"\nNEXT=n\n")
		if err := f.InsertAfter("KEY", NewPair("X", "1")); err != nil {
			t.Fatal(err)
		}
		if got := f.Render(); got != "KEY=\"one\ntwo\"\nX=1\nNEXT=n\n" {
			t.Errorf("Render() =\n%s", got)
		}
	})
}

// --- disabled pairs ---------------------------------------------------------

// TestDisabledPair_SameRegardlessOfAuthor is the consistency this kind exists
// for: a setting commented out by hand and one commented out by Unset are the
// same thing, so they must parse the same way.
func TestDisabledPair_SameRegardlessOfAuthor(t *testing.T) {
	byHand := Parse("# DB_USER=admin\n").Entries()[0]

	f := Parse("DB_USER=admin\n")
	f.Unset("DB_USER", false)
	byUnset := f.Entries()[0]

	for _, e := range []*Entry{byHand, byUnset} {
		if e.Kind != KindDisabledPair {
			t.Errorf("Kind = %s, want %s", e.Kind, KindDisabledPair)
		}
		if e.Key != "DB_USER" || e.Value != "admin" {
			t.Errorf("got %q=%q, want DB_USER=admin", e.Key, e.Value)
		}
	}
	if byHand.Raw[0] != byUnset.Raw[0] {
		t.Errorf("rendered differently: %q vs %q", byHand.Raw[0], byUnset.Raw[0])
	}
}

func TestDisabledPair_Classification(t *testing.T) {
	tests := []struct {
		name  string
		input string
		kind  Kind
		key   string
		value string
	}{
		{"plain prose", "# just a note\n", KindComment, "", ""},
		{"banner", "# --------\n", KindComment, "", ""},
		{"disabled setting", "# DB_USER=admin\n", KindDisabledPair, "DB_USER", "admin"},
		{"no space after the marker", "#DB_USER=admin\n", KindDisabledPair, "DB_USER", "admin"},
		{"indented", "   #  DB_USER=admin\n", KindDisabledPair, "DB_USER", "admin"},
		{"quoted value", `# K="a b"` + "\n", KindDisabledPair, "K", "a b"},
		{"with an inline comment", "# K=v # why\n", KindDisabledPair, "K", "v"},
		{"empty value", "# K=\n", KindDisabledPair, "K", ""},
		// A quote that does not close cannot be reassembled from separate
		// comment lines, so it stays prose rather than becoming a broken pair.
		{"unterminated quote stays prose", `# K="one` + "\n", KindComment, "", ""},
		// A digit-first key is valid under the Compose charset, so a commented
		// one is a disabled setting like any other.
		{"digit-first key is a disabled pair", "# 1K=v\n", KindDisabledPair, "1K", "v"},
		{"invalid key stays prose", "# K!=v\n", KindComment, "", ""},
		{"no equals sign", "# just words\n", KindComment, "", ""},
		// The documented false positive: prose shaped like an assignment.
		{"prose shaped like a pair", "# TODO=fix this\n", KindDisabledPair, "TODO", "fix this"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := Parse(tt.input).Entries()[0]

			if e.Kind != tt.kind {
				t.Fatalf("Kind = %s, want %s", e.Kind, tt.kind)
			}
			if e.Key != tt.key || e.Value != tt.value {
				t.Errorf("got %q=%q, want %q=%q", e.Key, e.Value, tt.key, tt.value)
			}
			if out := Parse(tt.input).Render(); out != tt.input {
				t.Errorf("round trip changed the line: %q", out)
			}
		})
	}
}

// TestDisabledPair_IsInactive: recognising a disabled setting must not make it
// take effect, or commenting a key out would stop working.
func TestDisabledPair_IsInactive(t *testing.T) {
	f := Parse("# DB_USER=admin\nDB_HOST=h\n")

	if f.Has("DB_USER") {
		t.Error("Has = true for a disabled setting")
	}
	if _, ok := f.Get("DB_USER"); ok {
		t.Error("Get found a disabled setting")
	}
	if n := f.Count("DB_USER"); n != 0 {
		t.Errorf("Count = %d, want 0", n)
	}
	if _, ok := f.Map()["DB_USER"]; ok {
		t.Error("Map included a disabled setting")
	}
	if keys := f.Keys(); len(keys) != 1 || keys[0] != "DB_HOST" {
		t.Errorf("Keys() = %v, want only the active key", keys)
	}
	// It is visible where it should be.
	if d := f.Disabled(); len(d) != 1 || d[0].Key != "DB_USER" || d[0].Value != "admin" {
		t.Errorf("Disabled() = %v", d)
	}
}

func TestDisabled_Empty(t *testing.T) {
	if d := Parse("A=1\n# prose\n").Disabled(); len(d) != 0 {
		t.Errorf("Disabled() = %v, want none", d)
	}
}

// --- Restore ----------------------------------------------------------------

func TestRestore(t *testing.T) {
	t.Run("reverses Unset exactly", func(t *testing.T) {
		const src = "# keep this comment\nDB_USER=admin\nOTHER=1\n"
		f := Parse(src)

		f.Unset("DB_USER", false)
		if !f.Restore("DB_USER") {
			t.Fatal("Restore reported not found")
		}
		if got := f.Render(); got != src {
			t.Errorf("disable → enable did not return the original:\n got: %q\nwant: %q", got, src)
		}
		if v, _ := f.Get("DB_USER"); v != "admin" {
			t.Errorf("Get = %q, want the setting active again", v)
		}
	})

	t.Run("preserves the author's own marker spacing", func(t *testing.T) {
		// "#K=v" must not come back as "K=v" with an invented space removed, nor
		// "#  K=v" lose only one of its spaces.
		for _, tt := range []struct{ src, want string }{
			{"#DB=1\n", "DB=1\n"},
			{"#  DB=1\n", "DB=1\n"},
			{"   # DB=1\n", "   DB=1\n"},
		} {
			f := Parse(tt.src)
			if !f.Restore("DB") {
				t.Fatalf("Restore(%q) reported not found", tt.src)
			}
			if got := f.Render(); got != tt.want {
				t.Errorf("Restore(%q) = %q, want %q", tt.src, got, tt.want)
			}
		}
	})

	t.Run("restores a hand-commented setting", func(t *testing.T) {
		f := Parse("# DB_USER=admin\n")
		if !f.Restore("DB_USER") {
			t.Fatal("Restore reported not found")
		}
		if v, _ := f.Get("DB_USER"); v != "admin" {
			t.Errorf("Get = %q", v)
		}
	})

	t.Run("a multi-line block restores within one session", func(t *testing.T) {
		const src = "KEY=\"one\ntwo\"\n"
		f := Parse(src)
		f.Unset("KEY", false)
		if !f.Restore("KEY") {
			t.Fatal("Restore reported not found")
		}
		if got := f.Render(); got != src {
			t.Errorf("Render() = %q, want %q", got, src)
		}
	})

	t.Run("targets the last disabled entry", func(t *testing.T) {
		f := Parse("# A=1\n# A=2\n")
		f.Restore("A")

		if got := f.Render(); got != "# A=1\nA=2\n" {
			t.Errorf("Render() = %q", got)
		}
	})

	t.Run("a missing or active key returns false", func(t *testing.T) {
		f := Parse("A=1\n")
		if f.Restore("A") {
			t.Error("Restore = true for an already-active key")
		}
		if f.Restore("NOPE") {
			t.Error("Restore = true for a key that is not there")
		}
		if got := f.Render(); got != "A=1\n" {
			t.Errorf("the file changed: %q", got)
		}
	})

	t.Run("restore then unset then restore", func(t *testing.T) {
		const src = "# A=1\n"
		f := Parse(src)
		f.Restore("A")
		f.Unset("A", false)
		f.Restore("A")

		if v, _ := f.Get("A"); v != "1" {
			t.Errorf("Get(A) = %q after a full cycle", v)
		}
	})
}

// --- documented hazards -----------------------------------------------------

// TestAppend_UnterminatedQuoteSwallowsWhatFollows records a real hazard found by
// fuzzing: appending to a file whose last value has an unclosed quote puts the
// new content INSIDE that value.
//
// Nothing here can fix it — the open quote is the source file's problem, and a
// parser that guessed where it should have closed would corrupt a legitimate
// multi-line value. Callers writing to files they did not author should check
// the result, which is what this test demonstrates.
func TestAppend_UnterminatedQuoteSwallowsWhatFollows(t *testing.T) {
	f := Parse(`A="`)
	f.Append(NewComment("note"), NewPair("APPENDED", "1"))

	rendered := f.Render()
	if _, ok := Parse(rendered).Get("APPENDED"); ok {
		t.Error("APPENDED was readable — the hazard this test documents has gone away, so update the docs")
	}
	// The appended text is inside A's value, not lost.
	v, _ := Parse(rendered).Get("A")
	if !strings.Contains(v, "APPENDED=1") {
		t.Errorf("Get(A) = %q, want the appended lines swallowed into it", v)
	}
	// A well-formed source has no such problem.
	g := Parse("A=\"closed\"\n")
	g.Append(NewPair("APPENDED", "1"))
	if _, ok := Parse(g.Render()).Get("APPENDED"); !ok {
		t.Error("APPENDED not found after appending to a well-formed file")
	}
}

// TestRestore_TargetsTheLastDisabledEntry records the second hazard fuzzing
// found: with more than one disabled entry for a key, Restore brings back the
// LAST, which is not necessarily the one Unset just created.
//
// "Last wins" is the rule everywhere else in this package — Get, Set, Unset — so
// Restore following it is consistent rather than surprising. The usual layout,
// where an old disabled entry sits ABOVE the active one, behaves as expected.
func TestRestore_TargetsTheLastDisabledEntry(t *testing.T) {
	t.Run("the usual layout reverses cleanly", func(t *testing.T) {
		const src = "# A=old\nA=new\n"
		f := Parse(src)

		f.Unset("A", false)
		f.Restore("A")

		if got := f.Render(); got != src {
			t.Errorf("Render() = %q, want %q", got, src)
		}
	})

	t.Run("a later disabled entry is restored instead", func(t *testing.T) {
		f := Parse("A=active\n# A=stale\n")

		f.Unset("A", false) // -> "# A=active", then "# A=stale"
		f.Restore("A")      // the LAST disabled entry is "# A=stale"

		if got := f.Render(); got != "# A=active\nA=stale\n" {
			t.Errorf("Render() = %q", got)
		}
		if v, _ := f.Get("A"); v != "stale" {
			t.Errorf("Get(A) = %q, want the last disabled entry to win", v)
		}
	})
}

// TestValueEndsOnThisLine covers each quote style's continuation check directly.
// A commented-out block cannot be reassembled from separate comment lines, so
// only a self-contained assignment may become a disabled pair.
func TestValueEndsOnThisLine(t *testing.T) {
	tests := map[string]bool{
		`plain`:        true,
		`  spaced`:     true,
		``:             true,
		`"closed"`:     true,
		`"open`:        false,
		`'closed'`:     true,
		`'open`:        false,
		"`closed`":     true,
		"`open":        false,
		`"escaped \""`: true,
	}

	for rhs, want := range tests {
		if got := valueEndsOnThisLine(rhs); got != want {
			t.Errorf("valueEndsOnThisLine(%q) = %v, want %v", rhs, got, want)
		}
	}
}

// TestDisabledPair_EveryQuoteStyle proves the check above is reached for each
// style through the parser, not just called directly.
func TestDisabledPair_EveryQuoteStyle(t *testing.T) {
	tests := []struct {
		input string
		kind  Kind
		value string
	}{
		{`# K='lit'` + "\n", KindDisabledPair, "lit"},
		{"# K=`tick`\n", KindDisabledPair, "tick"},
		{`# K='open` + "\n", KindComment, ""},
		{"# K=`open\n", KindComment, ""},
	}

	for _, tt := range tests {
		t.Run(strings.TrimSpace(tt.input), func(t *testing.T) {
			e := Parse(tt.input).Entries()[0]
			if e.Kind != tt.kind {
				t.Errorf("Kind = %s, want %s", e.Kind, tt.kind)
			}
			if e.Value != tt.value {
				t.Errorf("Value = %q, want %q", e.Value, tt.value)
			}
		})
	}
}

// --- scaling ----------------------------------------------------------------

// TestParse_ScalesLinearly guards against quadratic parsing.
//
// An earlier version copied the entire remaining line slice for every pair, to
// pre-trim carriage returns from the lookahead. That made a 5 MB file take 23
// seconds; trimming lazily brought it to 0.14. Nothing in the unit tests noticed,
// because they all operate on a handful of lines.
//
// The bound is deliberately loose — this asserts the right complexity class, not
// a performance target, so it stays meaningful on a loaded CI machine.
func TestParse_ScalesLinearly(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a multi-megabyte input")
	}

	var b strings.Builder
	for i := 0; b.Len() < 5*1024*1024; i++ {
		fmt.Fprintf(&b, "KEY_%d=QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo=\n", i)
	}
	src := b.String()

	start := time.Now()
	f := Parse(src)
	elapsed := time.Since(start)

	// Quadratic parsing took ~24s here; linear takes ~0.15s.
	const budget = 5 * time.Second
	if elapsed > budget {
		t.Errorf("parsing %d entries took %v, over the %v budget — parsing has likely gone quadratic again",
			len(f.Entries()), elapsed.Round(time.Millisecond), budget)
	}
	if len(f.Keys()) < 100_000 {
		t.Errorf("parsed %d keys, want the whole file", len(f.Keys()))
	}
	if f.Render() != src {
		t.Error("a multi-megabyte file did not round-trip")
	}
}

// BenchmarkParseManyEntries measures the per-entry cost, which is where the
// quadratic bug lived. BenchmarkParseLarge covers the few-entries-one-huge-value
// shape instead; both matter and they stress different paths.
func BenchmarkParseManyEntries(b *testing.B) {
	var sb strings.Builder
	for i := 0; sb.Len() < 1024*1024; i++ {
		fmt.Fprintf(&sb, "KEY_%d=value_%d\n", i, i)
	}
	src := sb.String()

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		_ = Parse(src)
	}
}

// --- SaveAs and Clone -------------------------------------------------------

func TestSaveAs(t *testing.T) {
	const src = "# banner\nDOMAIN=staging\nAPI=https://api.${DOMAIN}\n"

	t.Run("writes elsewhere without mutating Path", func(t *testing.T) {
		dir := t.TempDir()
		from := write(t, dir, ".env.staging", src)
		to := filepath.Join(dir, ".env.prod")

		f, err := Open(from)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.SaveAs(to); err != nil {
			t.Fatalf("SaveAs: %v", err)
		}

		if f.Path != from {
			t.Errorf("Path = %q, want it unchanged at %q", f.Path, from)
		}
		got, err := os.ReadFile(to)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != src {
			t.Errorf("destination =\n%s\nwant\n%s", got, src)
		}
		// The source must not have been touched.
		orig, _ := os.ReadFile(from)
		if string(orig) != src {
			t.Error("SaveAs modified the source file")
		}
	})

	t.Run("emits several variants from one source", func(t *testing.T) {
		dir := t.TempDir()
		f := Parse(src)

		f.Set("DOMAIN", "acme.io")
		if err := f.SaveAs(filepath.Join(dir, ".env.prod")); err != nil {
			t.Fatal(err)
		}
		f.Set("DOMAIN", "qa.acme.io")
		if err := f.SaveAs(filepath.Join(dir, ".env.qa")); err != nil {
			t.Fatal(err)
		}

		for name, want := range map[string]string{".env.prod": "acme.io", ".env.qa": "qa.acme.io"} {
			g, err := Open(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if v, _ := g.Get("DOMAIN"); v != want {
				t.Errorf("%s: DOMAIN = %q, want %q", name, v, want)
			}
			// The reference must have survived in both.
			if v, _ := g.Get("API"); v != "https://api.${DOMAIN}" {
				t.Errorf("%s: API = %q, want the reference intact", name, v)
			}
		}
	})

	t.Run("a new destination gets this file's mode", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("unix permissions")
		}
		dir := t.TempDir()
		from := write(t, dir, ".env", src)
		if err := os.Chmod(from, 0o640); err != nil {
			t.Fatal(err)
		}
		f, _ := Open(from)

		to := filepath.Join(dir, ".env.new")
		if err := f.SaveAs(to); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(to)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o640 {
			t.Errorf("mode = %o, want 640 carried from the source", info.Mode().Perm())
		}
	})

	t.Run("an existing destination keeps its own mode", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("unix permissions")
		}
		dir := t.TempDir()
		f := Parse(src) // mode defaults to 0600

		to := write(t, dir, ".env.prod", "OLD=1\n")
		if err := os.Chmod(to, 0o640); err != nil {
			t.Fatal(err)
		}
		if err := f.SaveAs(to); err != nil {
			t.Fatal(err)
		}

		info, err := os.Stat(to)
		if err != nil {
			t.Fatal(err)
		}
		// Overwriting must not silently change permissions a user chose.
		if info.Mode().Perm() != 0o640 {
			t.Errorf("mode = %o, want the destination's own 640", info.Mode().Perm())
		}
	})

	t.Run("an empty path is rejected", func(t *testing.T) {
		if err := Parse(src).SaveAs(""); err == nil {
			t.Error("SaveAs(\"\") = nil error")
		}
	})

	t.Run("a write failure is reported", func(t *testing.T) {
		if err := Parse(src).SaveAs(filepath.Join(t.TempDir(), "missing-dir", ".env")); err == nil {
			t.Error("SaveAs into a missing directory = nil error")
		}
	})
}

func TestClone(t *testing.T) {
	const src = "# banner\nA=1\nB=${A}\n# C=disabled\n\nnot a pair\n"

	t.Run("is byte-identical", func(t *testing.T) {
		f := Parse(src)
		if got := f.Clone().Render(); got != src {
			t.Errorf("Clone().Render() = %q, want %q", got, src)
		}
	})

	t.Run("editing the clone leaves the original alone", func(t *testing.T) {
		f := Parse(src)
		c := f.Clone()

		c.Set("A", "changed")
		c.Append(NewPair("ADDED", "1"))
		c.Unset("B", false)

		if got := f.Render(); got != src {
			t.Errorf("the original changed:\n%s", got)
		}
		if v, _ := f.Get("A"); v != "1" {
			t.Errorf("original A = %q, want %q", v, "1")
		}
	})

	t.Run("editing the original leaves the clone alone", func(t *testing.T) {
		f := Parse(src)
		c := f.Clone()

		f.Set("A", "changed")

		if v, _ := c.Get("A"); v != "1" {
			t.Errorf("clone A = %q, want the value at clone time", v)
		}
		if got := c.Render(); got != src {
			t.Errorf("clone changed:\n%s", got)
		}
	})

	t.Run("carries the file's facts across", func(t *testing.T) {
		dir := t.TempDir()
		f, err := Open(write(t, dir, ".env", "A=1\r\n"))
		if err != nil {
			t.Fatal(err)
		}
		c := f.Clone()

		if c.Path != f.Path || c.Existed() != f.Existed() || c.Mode() != f.Mode() {
			t.Errorf("clone facts differ: %q/%v/%o vs %q/%v/%o",
				c.Path, c.Existed(), c.Mode(), f.Path, f.Existed(), f.Mode())
		}
		// The line-ending style must survive, or an edit to the clone would
		// introduce a stray LF.
		c.Set("B", "2")
		if got := c.Render(); got != "A=1\r\nB=2\r\n" {
			t.Errorf("Render() = %q, want CRLF preserved", got)
		}
	})

	t.Run("clone plus SaveAs is the whole clone-a-file flow", func(t *testing.T) {
		dir := t.TempDir()
		f := Parse("DOMAIN=staging\nAPI=https://api.${DOMAIN}\n")

		prod := f.Clone()
		prod.Set("DOMAIN", "acme.io")
		if err := prod.SaveAs(filepath.Join(dir, ".env.prod")); err != nil {
			t.Fatal(err)
		}

		// The source in memory is untouched, so it can seed another variant.
		if v, _ := f.Get("DOMAIN"); v != "staging" {
			t.Errorf("source DOMAIN = %q, want it untouched", v)
		}
		g, _ := Open(filepath.Join(dir, ".env.prod"))
		exp, _, _ := g.GetExpanded("API")
		if exp != "https://api.acme.io" {
			t.Errorf("API expands to %q, want the reference to follow the new DOMAIN", exp)
		}
	})

	t.Run("options carry across", func(t *testing.T) {
		f := Parse(`K="a\nb"`+"\n", WithEscapes(EscapeCompose))
		c := f.Clone()

		// A clone that lost the dialect would decode differently on re-expansion.
		if v, _ := c.Get("K"); v != `a\nb` {
			t.Errorf("clone Get(K) = %q, want the compose dialect preserved", v)
		}
	})
}
