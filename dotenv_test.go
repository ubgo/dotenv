package dotenv

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// sample exercises every construct the parser understands, in one file, so the
// round-trip test below is a real guarantee rather than a happy-path check.
const sample = `# Top comment
# second line of it

APP_NAME=volt
export EXPORTED=yes
  SPACED   =   padded value
QUOTED="hello world"
LITERAL='no $expansion here'
WITH_HASH=a#b
INLINE=value # trailing note
EMPTY=
MULTI="line one
line two
line three"
ESCAPED="say \"hi\" and \\ too"
REF=${APP_NAME}-suffix

# A commented-out setting
# OLD_KEY=old
not a pair line
DUPLICATE=first
DUPLICATE=second
`

func load(t *testing.T, content string) *File {
	t.Helper()
	return Parse(content)
}

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// --- the package invariants -------------------------------------------------

// TestRoundTripByteIdentical is the guarantee the whole package exists for.
func TestRoundTripByteIdentical(t *testing.T) {
	cases := map[string]string{
		"full sample":         sample,
		"empty":               "",
		"no trailing newline": "A=1\nB=2",
		"only blanks":         "\n\n\n",
		// One newline is the edge case where every entry renders to an empty
		// string: the trailing terminator has to be re-added explicitly or the
		// file comes back as "" and the blank line is silently deleted.
		"single newline":         "\n",
		"crlf":                   "A=1\r\nB=2\r\n",
		"comment only":           "# just a note\n",
		"unterminated quote":     "A=\"never closes\nstill going\n",
		"unterminated single":    "A='never closes\nstill going\n",
		"whitespace only lines":  "   \n\t\n",
		"no newline single line": "A=1",
	}

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if got := Parse(content).Render(); got != content {
				t.Errorf("round trip changed the file:\n got: %q\nwant: %q", got, content)
			}
		})
	}
}

// TestSetSameValueIsByteNoop pins the second invariant: writing back a value
// that is already there must not reformat the author's line.
func TestSetSameValueIsByteNoop(t *testing.T) {
	f := load(t, sample)

	for _, key := range f.Keys() {
		v, _ := f.Get(key)
		if created := f.Set(key, v); created {
			t.Errorf("Set(%q, <current>) reported created", key)
		}
	}
	if got := f.Render(); got != sample {
		t.Errorf("setting every key to its current value changed the file:\n%s", got)
	}
}

// --- parsing ----------------------------------------------------------------

func TestParseValues(t *testing.T) {
	f := load(t, sample)

	tests := []struct {
		key  string
		want string
	}{
		{"APP_NAME", "volt"},
		{"EXPORTED", "yes"},
		{"SPACED", "padded value"},
		{"QUOTED", "hello world"},
		{"LITERAL", "no $expansion here"},
		// A # with no preceding whitespace is part of the value, not a comment.
		{"WITH_HASH", "a#b"},
		{"INLINE", "value"},
		{"EMPTY", ""},
		{"MULTI", "line one\nline two\nline three"},
		{"ESCAPED", `say "hi" and \ too`},
		{"REF", "${APP_NAME}-suffix"},
		// Last occurrence wins, matching what a consumer would see.
		{"DUPLICATE", "second"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, ok := f.Get(tt.key)
			if !ok {
				t.Fatalf("Get(%q) not found", tt.key)
			}
			if got != tt.want {
				t.Errorf("Get(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}

	if _, ok := f.Get("OLD_KEY"); ok {
		t.Error("a commented-out key must not be active")
	}
	if _, ok := f.Get("MISSING"); ok {
		t.Error("Get of an absent key reported found")
	}
}

func TestParseKinds(t *testing.T) {
	f := load(t, "# c\n\nA=1\nnot a pair\n")

	want := []Kind{KindComment, KindBlank, KindPair, KindOther}
	got := make([]Kind, 0, len(f.Entries()))
	for _, e := range f.Entries() {
		got = append(got, e.Kind)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("kinds = %v, want %v", got, want)
	}
}

func TestKind_String(t *testing.T) {
	for k, want := range map[Kind]string{
		KindPair: "pair", KindComment: "comment", KindBlank: "blank",
		KindOther: "other", KindDisabledPair: "disabled-pair", Kind(99): "Kind(99)",
	} {
		if got := k.String(); got != want {
			t.Errorf("Kind(%d).String() = %q, want %q", int(k), got, want)
		}
	}
}

func TestParseMultilineSingleQuoted(t *testing.T) {
	f := load(t, "A='one\ntwo' # note\nB=after\n")

	if got, _ := f.Get("A"); got != "one\ntwo" {
		t.Errorf("Get(A) = %q, want %q", got, "one\ntwo")
	}
	// The entry after a multi-line block must still parse — proof the parser
	// consumed exactly the lines the value spanned and no more.
	if got, _ := f.Get("B"); got != "after" {
		t.Errorf("Get(B) = %q, want %q", got, "after")
	}
}

func TestParseUnterminatedQuotes(t *testing.T) {
	t.Run("double", func(t *testing.T) {
		f := load(t, "A=\"one\ntwo\n")
		if got, _ := f.Get("A"); got != "one\ntwo\n" && got != "one\ntwo" {
			t.Errorf("Get(A) = %q", got)
		}
	})
	t.Run("single", func(t *testing.T) {
		f := load(t, "A='one\ntwo\n")
		if got, _ := f.Get("A"); !strings.HasPrefix(got, "one") {
			t.Errorf("Get(A) = %q", got)
		}
	})
}

func TestScanDoubleQuoted(t *testing.T) {
	tests := []struct {
		in        string
		value     string
		remainder string
		closed    bool
	}{
		{`hello"`, "hello", "", true},
		{`hello" trailing`, "hello", " trailing", true},
		{`unterminated`, "unterminated", "", false},
		{`say \"hi\""`, `say "hi"`, "", true},
		{`back\\slash"`, `back\slash`, "", true},
		// Any other backslash sequence stays literal, so a Windows path
		// survives without the author doubling every separator.
		{`C:\Users\x"`, `C:\Users\x`, "", true},
		{`trailing\`, `trailing\`, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			v, rem, closed := scanDoubleQuoted(tt.in, EscapeCompose)
			if v != tt.value || rem != tt.remainder || closed != tt.closed {
				t.Errorf("scanDoubleQuoted(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.in, v, rem, closed, tt.value, tt.remainder, tt.closed)
			}
		})
	}
}

// --- expansion --------------------------------------------------------------

func TestGetExpanded(t *testing.T) {
	f := load(t, strings.Join([]string{
		"BASE=root",
		"BLANK=",
		"BRACED=${BASE}/sub",
		"BARE=$BASE/sub",
		"CHAIN=${BRACED}/deep",
		"DEF_UNSET=${NOPE:-fallback}",
		"DEF_EMPTY=${BLANK:-fallback}",
		"DASH_EMPTY=${BLANK-fallback}",
		"DASH_UNSET=${NOPE-fallback}",
		"NO_DEF=${NOPE}",
		"PLAIN=nothing to expand",
	}, "\n")+"\n")

	tests := []struct {
		key  string
		want string
	}{
		{"BRACED", "root/sub"},
		{"BARE", "root/sub"},
		{"CHAIN", "root/sub/deep"},
		{"DEF_UNSET", "fallback"},
		// `:-` substitutes when unset OR empty.
		{"DEF_EMPTY", "fallback"},
		// `-` substitutes only when unset, so an empty value stays empty.
		{"DASH_EMPTY", ""},
		{"DASH_UNSET", "fallback"},
		// An unresolved reference with no default expands to "", never errors.
		{"NO_DEF", ""},
		{"PLAIN", "nothing to expand"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, ok, _ := f.GetExpanded(tt.key)
			if !ok {
				t.Fatalf("GetExpanded(%q) not found", tt.key)
			}
			if got != tt.want {
				t.Errorf("GetExpanded(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}

	if _, ok, _ := f.GetExpanded("MISSING"); ok {
		t.Error("GetExpanded of an absent key reported found")
	}
}

// TestExpandCycleTerminates proves the depth bound: without it a self- or
// mutually-referential file would hang the caller.
func TestExpandCycleTerminates(t *testing.T) {
	f := load(t, "A=${B}\nB=${A}\n")

	done := make(chan string, 1)
	go func() {
		v, _, _ := f.GetExpanded("A")
		done <- v
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("expansion did not terminate on a reference cycle")
	}
}

func TestParseVarToken(t *testing.T) {
	tests := []struct {
		tok  string
		want varRef
	}{
		{"VAR", varRef{varName: "VAR"}},
		{"VAR:-d", varRef{varName: "VAR", op: opDefaultEmpty, arg: "d"}},
		{"VAR-d", varRef{varName: "VAR", op: opDefault, arg: "d"}},
		{"VAR:+a", varRef{varName: "VAR", op: opAlternateSet, arg: "a"}},
		{"VAR+a", varRef{varName: "VAR", op: opAlternate, arg: "a"}},
		{"VAR:?e", varRef{varName: "VAR", op: opRequiredSet, arg: "e"}},
		{"VAR?e", varRef{varName: "VAR", op: opRequired, arg: "e"}},
		// Colon forms must be recognised before their bare counterparts, or
		// ${VAR:-d} parses as a "-" default named ":-d".
		{"VAR:-", varRef{varName: "VAR", op: opDefaultEmpty}},
		{"VAR?", varRef{varName: "VAR", op: opRequired}},
	}

	for _, tt := range tests {
		t.Run(tt.tok, func(t *testing.T) {
			if got := parseVarToken(tt.tok); got != tt.want {
				t.Errorf("parseVarToken(%q) = %+v, want %+v", tt.tok, got, tt.want)
			}
		})
	}
}

// mustExpandMap fails the test on an expansion error, so callers testing the
// happy path stay readable.
func mustExpandMap(t *testing.T, f *File) map[string]string {
	t.Helper()
	m, err := f.ExpandedMap()
	if err != nil {
		t.Fatalf("ExpandedMap: %v", err)
	}
	return m
}

func TestExpandedMap(t *testing.T) {
	f := load(t, "BASE=root\nDERIVED=${BASE}/x\n")

	got := mustExpandMap(t, f)
	if got["DERIVED"] != "root/x" {
		t.Errorf("ExpandedMap()[DERIVED] = %q, want %q", got["DERIVED"], "root/x")
	}
}

// TestKindString pins every Kind's display name — the strings appear in test
// failures and logs, so a rename is a (minor) behavior change worth catching —
// plus the defensive fallback for a Kind the parser never produces.
func TestKindString(t *testing.T) {
	for kind, want := range map[Kind]string{
		KindPair:         "pair",
		KindComment:      "comment",
		KindBlank:        "blank",
		KindOther:        "other",
		KindDisabledPair: "disabled-pair",
		KindInherited:    "inherited",
		Kind(99):         "Kind(99)",
	} {
		if got := kind.String(); got != want {
			t.Errorf("Kind(%d).String() = %q, want %q", int(kind), got, want)
		}
	}
}
