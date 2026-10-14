package dotenv

import (
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
