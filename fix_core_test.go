package dotenv

import (
	"fmt"
	"strings"
	"testing"
)

// expandCase is one GetExpanded expectation against an in-memory file.
type expandCase struct {
	name  string
	src   string
	key   string
	want  string
	opts  []Option
	isErr bool
}

// runExpandCases checks every case's GetExpanded result.
func runExpandCases(t *testing.T, cases []expandCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok, err := Parse(tc.src, tc.opts...).GetExpanded(tc.key)
			if !ok {
				t.Fatalf("key %q not found", tc.key)
			}
			if tc.isErr {
				if err == nil {
					t.Fatalf("GetExpanded(%q) = %q, want an error", tc.key, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetExpanded(%q): %v", tc.key, err)
			}
			if got != tc.want {
				t.Errorf("GetExpanded(%q) = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

// lookupOnly answers exactly one name, so a test can tell whether the Lookuper
// was asked.
func lookupOnly(name, value string) Option {
	return WithLookup(func(n string) (string, bool) {
		if n == name {
			return value, true
		}
		return "", false
	})
}

// TestExpand_SelfReference pins Docker Compose's reading of a key that refers
// to itself: the reference sees the definition ABOVE the line (compose-go
// expands each line against the values read so far), then the Lookuper, then
// nothing. Compose evidence: compose-go v2.16.1 dotenv/fixtures/
// substitutions-default.env (`OPTION_A=${OPTION_A:-1}`) under
// TestSubstitutionsWithUnsetVarEnvFileDefaultValuePrecedence ("1") and
// TestSubstitutionsWithEnvFileDefaultValuePrecedence (the lookup's "5").
func TestExpand_SelfReference(t *testing.T) {
	runExpandCases(t, []expandCase{
		{name: "extends the earlier definition", src: "A=1\nA=${A}2\n", key: "A", want: "12"},
		{name: "extends twice", src: "A=1\nA=${A}2\nA=${A}3\n", key: "A", want: "123"},
		{name: "path-style append", src: "P=/a\nP=${P}:/b\nB=${P}\n", key: "B", want: "/a:/b"},
		{name: "default when nothing earlier", src: "ZONE=${ZONE:-default-zone}\n", key: "ZONE", want: "default-zone"},
		{name: "dash default when nothing earlier", src: "ZONE=${ZONE-d}\n", key: "ZONE", want: "d"},
		{name: "Lookuper answers a self-referencing default", src: "ZONE=${ZONE:-default-zone}\n", key: "ZONE", want: "eu-west", opts: []Option{lookupOnly("ZONE", "eu-west")}},
		{name: "earlier definition beats the Lookuper", src: "ZONE=us\nZONE=${ZONE:-d}\n", key: "ZONE", want: "us", opts: []Option{lookupOnly("ZONE", "eu-west")}},
		{name: "bare self-reference is empty", src: "A=${A}\n", key: "A", want: ""},
		{name: "doubled self-reference is empty", src: "A=$A$A\n", key: "A", want: ""},
		{name: "required self-reference errors", src: "A=${A:?set A}\n", key: "A", isErr: true},
		{name: "required self-reference satisfied above", src: "A=x\nA=${A:?set A}\n", key: "A", want: "x"},
		{name: "required self-reference satisfied by the Lookuper", src: "A=${A:?set A}\n", key: "A", want: "L", opts: []Option{lookupOnly("A", "L")}},
		{name: "a later definition is not seen by the earlier one", src: "A=${A:-first}\nB=${A}\nA=second\n", key: "B", want: "second"},
		{name: "reference through another key reaches the earlier definition", src: "A=1\nA=${B}2\nB=${A}\n", key: "A", want: "12"},
	})
}

// TestExpand_Cycle pins the cycle rule: a reference back into a key that is
// still being expanded resolves the same way a self-reference does — the
// definition above, then the Lookuper, then empty — so the starting key is cut
// on its FIRST return rather than going round once more.
func TestExpand_Cycle(t *testing.T) {
	runExpandCases(t, []expandCase{
		{name: "two-key cycle from A", src: "CYCLE_A=a${CYCLE_B}\nCYCLE_B=b${CYCLE_A}\n", key: "CYCLE_A", want: "ab"},
		{name: "two-key cycle from B", src: "CYCLE_A=a${CYCLE_B}\nCYCLE_B=b${CYCLE_A}\n", key: "CYCLE_B", want: "ba"},
		{name: "three-key cycle", src: "A=a${B}\nB=b${C}\nC=c${A}\n", key: "A", want: "abc"},
		{name: "empty cycle", src: "A=${B}\nB=${A}\n", key: "A", want: ""},
		{name: "cycle cut falls back to the Lookuper", src: "A=${B}\nB=${A:-dflt}\n", key: "A", want: "L", opts: []Option{lookupOnly("A", "L")}},
		{name: "cycle cut uses the default", src: "A=${B}\nB=${A:-dflt}\n", key: "A", want: "dflt"},
		{name: "two separate references are not a cycle", src: "D=x\nT=${D}|${D}\n", key: "T", want: "x|x"},
	})
}

// TestExpand_LiteralThroughReference pins that a single-quoted or backtick
// value is literal everywhere, including when another key refers to it. That
// is Compose's reading: compose-go stores '${B}' as the text ${B} and never
// expands a stored value again (TestExpanding, "does not expand variables in
// single quoted strings").
func TestExpand_LiteralThroughReference(t *testing.T) {
	runExpandCases(t, []expandCase{
		{name: "single-quoted reference text", src: "A='${B}'\nB=x\nC=${A}\n", key: "C", want: "${B}"},
		{name: "single-quoted double dollar", src: "A='$$'\nC=${A}\n", key: "C", want: "$$"},
		{name: "backtick reference text", src: "A=`${B}`\nB=x\nC=${A}\n", key: "C", want: "${B}"},
		{name: "literal reached through a chain", src: "A='$x'\nB=${A}\nC=${B}\n", key: "C", want: "$x"},
		{name: "literal as an alternate's variable is still tested for emptiness", src: "A=''\nC=${A:-d}\n", key: "C", want: "d"},
		{name: "a default is still expanded", src: "B=x\nC=${NOPE:-${B}}\n", key: "C", want: "x"},
		{name: "self-reference to a literal earlier line", src: "A='$1'\nA=${A}2\n", key: "A", want: "$12"},
		{name: "double-quoted value is still expanded", src: "A=\"${B}\"\nB=x\nC=${A}\n", key: "C", want: "x"},
	})
}

// TestExpandedMap_SkipsOverriddenDuplicates pins that ExpandedMap expands only
// the effective (last) entry for each key, so it agrees with GetExpanded: a
// dead earlier line can no longer fail the whole map.
func TestExpandedMap_SkipsOverriddenDuplicates(t *testing.T) {
	f := Parse("B=${NOPE:?x}\nB=ok\nC=${B}\n")
	m, err := f.ExpandedMap()
	if err != nil {
		t.Fatalf("ExpandedMap: %v", err)
	}
	for _, key := range []string{"B", "C"} {
		want, _, err := f.GetExpanded(key)
		if err != nil {
			t.Fatalf("GetExpanded(%q): %v", key, err)
		}
		if m[key] != want {
			t.Errorf("ExpandedMap[%q] = %q, GetExpanded = %q", key, m[key], want)
		}
	}

	// The effective line still fails, and the error is reported in file order.
	if _, err := Parse("B=ok\nB=${NOPE:?x}\n").ExpandedMap(); err == nil {
		t.Error("ExpandedMap = nil error for an unsatisfied effective line")
	}
}

// setSources are entries written every way an author can write one: each
// quote style, the colon delimiter, export, and an inline comment.
var setSources = []string{
	"K=old\n",
	"K=\"old\"\n",
	"K='old'\n",
	"K=`old`\n",
	"K: old\n",
	"export K=old\n",
	"K=old # note\n",
	"K='old' # note\n",
	"K=\"old\"# tight\n",
}

// setValues cover the characters that change how a value is read back: a
// reference, a lone and a doubled dollar, every quote character, comments,
// spaces, newlines, and the empty value.
var setValues = []string{
	"plain", "", "${A}y", "a$b", "$$", "cost 5$", "$", "with space $x",
	"multi\nline $x", "it's $x", "tick ` $x", "both ' and ` $x",
	"both ' and `\nlines $x", "#hash", "x # not a comment", `back\slash $x`,
	`"dq" $x`, "it's", "multi\nline",
}

// TestSet_RoundTrip pins the editor's promise across every source style and
// value: after Set, Get returns the value, re-parsing the rendered file gives
// the same Get and the same GetExpanded, and a value containing '$' is stored
// literally so GetExpanded returns it unchanged — unless it holds both a single
// quote and a backtick, which no literal delimiter can carry.
func TestSet_RoundTrip(t *testing.T) {
	for _, src := range setSources {
		for _, value := range setValues {
			t.Run(fmt.Sprintf("%q<-%q", src, value), func(t *testing.T) {
				f := Parse("A=x\n" + src)
				f.Set("K", value)
				rendered := f.Render()
				back := Parse(rendered)

				if got, _ := f.Get("K"); got != value {
					t.Errorf("Get after Set = %q, want %q", got, value)
				}
				if got, _ := back.Get("K"); got != value {
					t.Errorf("Get after re-parse = %q, want %q\nrendered: %q", got, value, rendered)
				}
				mem, _, memErr := f.GetExpanded("K")
				disk, _, diskErr := back.GetExpanded("K")
				if mem != disk || (memErr == nil) != (diskErr == nil) {
					t.Errorf("GetExpanded in memory = %q (%v), after re-parse = %q (%v)\nrendered: %q", mem, memErr, disk, diskErr, rendered)
				}
				literalFits := !strings.ContainsRune(value, '\'') || !strings.ContainsRune(value, '`')
				if strings.ContainsRune(value, '$') && literalFits && disk != value {
					t.Errorf("GetExpanded = %q, want the literal %q\nrendered: %q", disk, value, rendered)
				}
				// Setting it again is a byte-level no-op.
				f.Set("K", value)
				if again := f.Render(); again != rendered {
					t.Errorf("second Set changed the file:\n once: %q\ntwice: %q", rendered, again)
				}
			})
		}
	}
}

// TestSet_NewKeyStoresDollarLiterally covers the append path, which renders
// through the same code as an in-place edit.
func TestSet_NewKeyStoresDollarLiterally(t *testing.T) {
	f := Parse("")
	f.Set("PASSWORD", "a$b")
	if got := f.Render(); got != "PASSWORD='a$b'\n" {
		t.Errorf("Render = %q, want PASSWORD='a$b'", got)
	}
	if got, _, _ := f.GetExpanded("PASSWORD"); got != "a$b" {
		t.Errorf("GetExpanded = %q, want a$b", got)
	}
}
