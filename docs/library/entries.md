# Entries and kinds

How the `dotenv` library sees a file line by line: the `Entry` type, the six `Kind` values, and how to walk every entry, comments and blanks included.

**On this page**

- [What an entry is](#what-an-entry-is)
- [Before you start](#before-you-start)
- [Step 1 — Know the six kinds](#step-1--know-the-six-kinds)
- [Step 2 — Read an entry's fields](#step-2--read-an-entrys-fields)
- [Step 3 — Walk every entry](#step-3--walk-every-entry)
- [Step 4 — Print a kind](#step-4--print-a-kind)
- [Try it](#try-it)
- [Pitfalls](#pitfalls)
- [Related](#related)

## What an entry is

A parsed `File` is a list of entries in file order. An entry is one logical unit: a pair (which can span several physical lines when a quoted value runs on), a comment line, a blank line, or a line the parser does not recognise. Nothing is dropped, which is why an untouched file renders back byte for byte.

The accessors in [reading.md](reading.md) (`Get`, `Keys`, `Map` and the rest) only see active pairs. Walk `f.Entries()` when you need the rest of the file: a linter that flags disabled settings or unrecognised lines, a tool that reports "DB_USER is disabled" rather than "DB_USER is missing", or a renderer that shows comments next to values.

## Before you start

```sh
go get github.com/ubgo/dotenv
```

You need a `*dotenv.File` from `Open`, `Parse` or `ParseReader` (see [reading.md](reading.md)).

## Step 1 — Know the six kinds

`Kind` is a closed set: the parser produces nothing else.

| Kind | `String()` | A line that produces it | Active? | `Key` | `Value` |
|---|---|---|---|---|---|
| `KindPair` | `pair` | `PORT=8080`, `PORT: 8080`, `export PORT=8080` | yes | set | decoded value |
| `KindComment` | `comment` | `# a note` (first non-space character is `#`, and the body is not a pair) | no | empty | empty |
| `KindBlank` | `blank` | an empty or whitespace-only line | no | empty | empty |
| `KindOther` | `other` | `not a setting!`, `MY KEY=value` — anything not recognised | no | empty | empty |
| `KindDisabledPair` | `disabled-pair` | `# DEBUG=true` — a comment whose body parses as a pair on one line | no | set | decoded value |
| `KindInherited` | `inherited` | `export AWS_REGION` or `HOME` — a name with no delimiter and no value | no | set | empty |

"Active" means `Get`, `Has`, `Count`, `Keys`, `Pairs` and `Map` see it. Only `KindPair` is active.

Here is each kind from the exact line that produces it:

<!-- snippet: example_read_test.go#read_Kind -->
```go
// ExampleKind shows the exact line that produces each of the six kinds.
func ExampleKind() {
	lines := []string{
		"PORT=8080",         // a pair
		"# a note",          // a comment
		"",                  // a blank line
		"not a setting!",    // unrecognised, kept verbatim
		"# DEBUG=true",      // a commented-out setting
		"export AWS_REGION", // a name with no value: inherited
	}

	f := dotenv.Parse(strings.Join(lines, "\n") + "\n")
	for _, e := range f.Entries() {
		fmt.Printf("%-13s %q\n", e.Kind, e.Raw[0])
	}
	// Output:
	// pair          "PORT=8080"
	// comment       "# a note"
	// blank         ""
	// other         "not a setting!"
	// disabled-pair "# DEBUG=true"
	// inherited     "export AWS_REGION"
}
```

What each kind is for:

- **`KindPair`** is a setting. A key may contain letters, digits, `_`, `.`, `-`, `[` and `]` (Docker Compose's set), and the delimiter may be `=` or `:`. The `export` prefix and the delimiter are kept when you edit the value.
- **`KindComment`** is prose, kept verbatim.
- **`KindBlank`** is kept verbatim, whitespace included.
- **`KindOther`** is a line the parser does not understand. It is preserved rather than rejected, so one stray line never makes a file unopenable. The cost is that a malformed pair is invisible to `Get` and friends instead of reported; see [Pitfalls](#pitfalls).
- **`KindDisabledPair`** is a setting somebody turned off. A line commented out by hand and one commented out by `Unset` parse the same way. `f.Disabled()` lists them and `f.Restore(key)` turns one back on (see [editing.md](editing.md)).
- **`KindInherited`** is the Compose way to say "take this variable from the process environment". The library records the name but never reads the environment; `f.Inherited()` lists the names so your program can decide.

Example: [`ExampleKind`](../../example_read_test.go).

## Step 2 — Read an entry's fields

`Entry` has four exported fields:

| Field | Type | Holds |
|---|---|---|
| `Kind` | `Kind` | which of the six kinds this is |
| `Raw` | `[]string` | the exact physical lines, without line terminators (a CRLF file keeps its `\r`); one element per line, several for a multi-line value |
| `Key` | `string` | the variable name, for `KindPair`, `KindDisabledPair` and `KindInherited`; empty otherwise |
| `Value` | `string` | the decoded value, for `KindPair` and `KindDisabledPair`: quotes stripped, escapes interpreted, lines of a multi-line value joined with `\n`; empty otherwise |

`Value` is raw in the expansion sense: `${REF}` is not resolved. Everything else the library needs to edit an entry without disturbing it (the `export` prefix, spacing, the delimiter, the inline comment, the quote style, the comment marker of a disabled pair) is kept in unexported fields.

<!-- snippet: example_read_test.go#read_Entry -->
```go
// ExampleEntry shows the four exported fields. Raw keeps the exact physical
// lines (a multi-line value spans several), Value is the decoded value with
// quotes stripped and lines joined by \n.
func ExampleEntry() {
	const content = "export CERT=\"line one\nline two\" # pem\n# DEBUG='true'\n"

	f := dotenv.Parse(content)
	for _, e := range f.Entries() {
		fmt.Println("Kind: ", e.Kind)
		fmt.Printf("Raw:   %q\n", e.Raw)
		fmt.Printf("Key:   %q\n", e.Key)
		fmt.Printf("Value: %q\n", e.Value)
	}
	// Output:
	// Kind:  pair
	// Raw:   ["export CERT=\"line one" "line two\" # pem"]
	// Key:   "CERT"
	// Value: "line one\nline two"
	// Kind:  disabled-pair
	// Raw:   ["# DEBUG='true'"]
	// Key:   "DEBUG"
	// Value: "true"
}
```

Treat entries from `Entries()` as read-only. The slice and the entries are the file's own, not copies; change the file through `Set`, `Unset`, `Restore`, `Append` and `Insert*` so `Raw` stays in step with `Key` and `Value` (see [editing.md](editing.md)).

Example: [`ExampleEntry`](../../example_read_test.go).

## Step 3 — Walk every entry

`f.Entries()` returns every entry in file order. Switch on `Kind` to handle each one.

<!-- snippet: example_read_test.go#read_Entries -->
```go
// ExampleFile_Entries walks every entry in file order, comments and blanks
// included. This is the view for tools that inspect structure, such as a
// linter that flags disabled settings or unrecognised lines.
func ExampleFile_Entries() {
	const content = `# database
DB_HOST=localhost
# DB_USER=admin

HOME
not a setting
`
	f := dotenv.Parse(content)

	for _, e := range f.Entries() {
		switch e.Kind {
		case dotenv.KindPair:
			fmt.Printf("%-13s %s=%s\n", e.Kind, e.Key, e.Value)
		case dotenv.KindDisabledPair:
			fmt.Printf("%-13s %s (off, would be %s)\n", e.Kind, e.Key, e.Value)
		case dotenv.KindInherited:
			fmt.Printf("%-13s %s (from the environment)\n", e.Kind, e.Key)
		default:
			fmt.Printf("%-13s %q\n", e.Kind, e.Raw[0])
		}
	}
	// Output:
	// comment       "# database"
	// pair          DB_HOST=localhost
	// disabled-pair DB_USER (off, would be admin)
	// blank         ""
	// inherited     HOME (from the environment)
	// other         "not a setting"
}
```

For a quick count of one kind you do not need to walk: `f.Disabled()` returns the disabled pairs as `[]Pair`, and `f.Inherited()` returns the inherited names, both in file order.

Example: [`ExampleFile_Entries`](../../example_read_test.go).

## Step 4 — Print a kind

`Kind` implements `fmt.Stringer`, so `%s`, `%v` and `fmt.Println` print its name instead of an integer. That keeps logs and test failures readable. A value outside the six kinds prints as `Kind(n)`.

<!-- snippet: example_read_test.go#read_KindString -->
```go
// ExampleKind_String prints a Kind as a name, which is what %s, %v and a log
// line show. A value outside the six kinds prints as Kind(n).
func ExampleKind_String() {
	kinds := []dotenv.Kind{
		dotenv.KindPair,
		dotenv.KindComment,
		dotenv.KindBlank,
		dotenv.KindOther,
		dotenv.KindDisabledPair,
		dotenv.KindInherited,
		dotenv.Kind(42),
	}
	for _, k := range kinds {
		fmt.Println(k)
	}
	// Output:
	// pair
	// comment
	// blank
	// other
	// disabled-pair
	// inherited
	// Kind(42)
}
```

Example: [`ExampleKind_String`](../../example_read_test.go).

## Try it

Every snippet on this page is an `Example` function in [`example_read_test.go`](../../example_read_test.go), so `go test` checks each printed line against its `// Output:` block. From the repository root:

```sh
go test -run '^Example(Kind|Kind_pitfalls|Kind_String|Entry|File_Entries)$' -v .
```

```text
=== RUN   ExampleFile_Entries
--- PASS: ExampleFile_Entries (0.00s)
=== RUN   ExampleKind
--- PASS: ExampleKind (0.00s)
=== RUN   ExampleKind_pitfalls
--- PASS: ExampleKind_pitfalls (0.00s)
=== RUN   ExampleKind_String
--- PASS: ExampleKind_String (0.00s)
=== RUN   ExampleEntry
--- PASS: ExampleEntry (0.00s)
PASS
ok  	github.com/ubgo/dotenv	0.010s
```

## Pitfalls

Each of these lines is parsed as documented, and each still renders back byte for byte. They are surprising only if you expect a different reading.

<!-- snippet: example_read_test.go#read_KindPitfalls -->
```go
// ExampleKind_pitfalls shows lines that parse as something other than what a
// reader might expect. Each is the documented behaviour, and each line still
// renders back byte for byte.
func ExampleKind_pitfalls() {
	const content = `# TODO=fix this
# Note: rotate monthly
# Note - rotate monthly
http://example.com
MY KEY=value
`
	f := dotenv.Parse(content)

	for _, e := range f.Entries() {
		fmt.Printf("%-13s key=%q value=%q\n", e.Kind, e.Key, e.Value)
	}
	fmt.Println("round trip:", f.Render() == content)
	// Output:
	// disabled-pair key="TODO" value="fix this"
	// disabled-pair key="Note" value="rotate monthly"
	// comment       key="" value=""
	// pair          key="http" value="//example.com"
	// other         key="" value=""
	// round trip: true
}
```

- **Prose shaped like an assignment is a disabled setting.** `# TODO=fix this` becomes `KindDisabledPair` with key `TODO`. This is deliberate: a human skimming the file reads it the same way. It shows up in `f.Disabled()`, and `f.Restore("TODO")` would turn it into a live pair.
- **The colon delimiter widens this.** Because `KEY: value` is a pair, `# Note: rotate monthly` is a disabled pair with key `Note`, and a commented URL such as `# http://example.com` is a disabled pair with key `http`. Prose with a space before the colon (`# See https://…`) or a non-key character is still a comment.
- **A bare URL line is a pair.** `http://example.com` on its own line parses as key `http`, value `//example.com`. Docker Compose reads it the same way, so the library does not special-case it (pinned by `TestColonDelimiter`).
- **A key with a space is not an error.** `MY KEY=value` is `KindOther`: kept, but invisible to `Get`, `Keys` and `Map`. If you want strict parsing, walk `Entries()` and treat `KindOther` as an error yourself.
- **A commented-out multi-line value is several comments.** Only a disabled pair whose quote closes on the same line is `KindDisabledPair`; `# KEY="one` followed by `# two"` is two `KindComment` entries, so it cannot be restored from a re-read file.
- **`KindInherited` is never resolved.** `HOME` has a `Key` but no `Value`, and a `${HOME}` reference to it expands to `""` like any unset variable. Apply `os.LookupEnv` over `f.Inherited()` if you want Compose's behaviour.
- **`Raw` and `Value` differ for quoted and multi-line values.** Compare against `Value` for the setting, against `Raw` for the bytes in the file.

Example: [`ExampleKind_pitfalls`](../../example_read_test.go).

## Related

- [reading.md](reading.md) — `Read`, `Open`, `Parse` and the read accessors.
- [editing.md](editing.md) — `Unset`, `Restore`, `Disabled`, `Inherited` and the entry constructors.
- [syntax.md](syntax.md) — every quoting, comment and escape rule.
- [Feature index](../reference/features.md#library-reading) — every library feature with its guide and example.
- Examples: [`example_read_test.go`](../../example_read_test.go).
