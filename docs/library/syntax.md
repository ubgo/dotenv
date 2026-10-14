# .env syntax

Every rule the parser applies to a `.env` file, shown as input → `Get` → `GetExpanded`, with a tested example for each.

**On this page**

- [Before you start](#before-you-start)
- [Step 1 — Quoting: bare, double, single, backtick](#step-1--quoting-bare-double-single-backtick)
- [Step 2 — Multi-line values](#step-2--multi-line-values)
- [Step 3 — Comments and where a value ends](#step-3--comments-and-where-a-value-ends)
- [Step 4 — The `:` delimiter](#step-4--the--delimiter)
- [Step 5 — Key names and lines that are not pairs](#step-5--key-names-and-lines-that-are-not-pairs)
- [Step 6 — Every entry kind](#step-6--every-entry-kind)
- [Step 7 — Duplicate keys](#step-7--duplicate-keys)
- [Step 8 — `encrypted:` values](#step-8--encrypted-values)
- [Step 9 — Escape sequences: `EscapeExtended` vs `EscapeCompose`](#step-9--escape-sequences-escapeextended-vs-escapecompose)
- [Quick reference](#quick-reference)
- [Try it](#try-it)
- [Pitfalls](#pitfalls)
- [Related](#related)

`Get` returns the **decoded** value: quotes removed, escapes turned into characters, multi-line values joined with `\n`. `GetExpanded` returns that value with `$` references resolved, as described in [Variable expansion](expansion.md). Showing both side by side is the fastest way to see what each rule does, because quoting decides both how a value is decoded and whether it is expanded.

The parser never fails. A line it cannot read is kept as it is and the rest of the file still parses, and `Render` writes an untouched file back byte for byte.

## Before you start

You need Go 1.22 or later and the library:

```sh
go get github.com/ubgo/dotenv
```

Every snippet below is a complete `Example` function from [`example_syntax_test.go`](../../example_syntax_test.go). The examples read an in-memory string with `dotenv.Parse`, so nothing touches your disk. `Open(path)` and `ParseReader(r)` apply exactly the same rules.

## Step 1 — Quoting: bare, double, single, backtick

A value can be written four ways. Pick by what the value contains and whether `$` in it should be a reference:

| Style | Example | Escapes decoded | `$` expanded | Whitespace |
|---|---|---|---|---|
| Bare | `K=hello $NAME` | no | yes | leading and trailing spaces trimmed |
| Double quotes | `K="hello $NAME"` | yes (see [Step 9](#step-9--escape-sequences-escapeextended-vs-escapecompose)) | yes | kept exactly |
| Single quotes | `K='hello $NAME'` | no | no | kept exactly |
| Backticks | ``K=`hello $NAME` `` | no | no | kept exactly |

`export ` in front of a key is accepted and ignored, and so are spaces or tabs around the `=`. An empty value (`K=` or `K=""`) is a real pair whose value is `""`.

The helper `synShow` prints `Get` and `GetExpanded` for one key with `%q`, so trailing spaces and newlines are visible. The later examples on this page reuse it.

<!-- snippet: example_syntax_test.go#syn_Quotes -->
```go
// synShow prints what Get and GetExpanded return for one key, side by side.
// Values are printed with %q so the characters that matter here (newlines,
// tabs, trailing spaces, backslashes) are visible instead of invisible.
func synShow(f *dotenv.File, key string) {
	raw, ok := f.Get(key)
	if !ok {
		fmt.Printf("%s: not a pair\n", key)
		return
	}
	expanded, _, err := f.GetExpanded(key)
	if err != nil {
		fmt.Printf("%s: Get=%q GetExpanded error: %v\n", key, raw, err)
		return
	}
	fmt.Printf("%s: Get=%q GetExpanded=%q\n", key, raw, expanded)
}

// ExampleParse_syntaxQuotes shows the four ways to write a value and what
// each one does to whitespace and to a $reference. Bare and double-quoted
// values interpolate; single-quoted and backtick values never do.
func ExampleParse_syntaxQuotes() {
	src := "NAME=world\n" +
		"BARE=hello $NAME   \n" +
		"DOUBLE=\"hello $NAME  \"\n" +
		"SINGLE='hello $NAME'\n" +
		"BACKTICK=`it's \"hi\" $NAME`\n" +
		"export EXPORTED=yes\n" +
		"  SPACED  =   padded value  \n" +
		"EMPTY=\n" +
		"QUOTED_EMPTY=\"\"\n"

	f := dotenv.Parse(src)
	for _, key := range f.Keys() {
		synShow(f, key)
	}
	// Output:
	// NAME: Get="world" GetExpanded="world"
	// BARE: Get="hello $NAME" GetExpanded="hello world"
	// DOUBLE: Get="hello $NAME  " GetExpanded="hello world  "
	// SINGLE: Get="hello $NAME" GetExpanded="hello $NAME"
	// BACKTICK: Get="it's \"hi\" $NAME" GetExpanded="it's \"hi\" $NAME"
	// EXPORTED: Get="yes" GetExpanded="yes"
	// SPACED: Get="padded value" GetExpanded="padded value"
	// EMPTY: Get="" GetExpanded=""
	// QUOTED_EMPTY: Get="" GetExpanded=""
}
```

Note `BARE` versus `DOUBLE`: the bare value lost its three trailing spaces, the double-quoted one kept its two. Quote a value whenever its edges matter.

## Step 2 — Multi-line values

A double-quoted, single-quoted or backtick value continues across lines until its closing quote. The lines are joined with `\n` (a CRLF file's `\r` is dropped from the value). The whole block is one entry, so editing it rewrites the block as a unit. A bare value always ends at the end of its line.

Backticks are the easy way to paste a PEM key or anything else that contains both `'` and `"`.

<!-- snippet: example_syntax_test.go#syn_Multiline -->
```go
// ExampleParse_syntaxMultiline shows values that span several physical lines.
// A double-quoted, single-quoted or backtick value runs until its closing
// quote; the lines are joined with "\n". A bare value always ends at the end
// of its line.
func ExampleParse_syntaxMultiline() {
	src := "GREETING=\"line one\n" +
		"line two\"\n" +
		"PEM=`-----BEGIN KEY-----\n" +
		"abc'\"def\n" +
		"-----END KEY-----`\n" +
		"LITERAL='a\n" +
		"b'\n" +
		"AFTER=still parsed\n"

	f := dotenv.Parse(src)
	for _, key := range f.Keys() {
		synShow(f, key)
	}
	// Four keys, eight physical lines: each multi-line value is one entry.
	fmt.Println("entries:", len(f.Entries()))
	// Output:
	// GREETING: Get="line one\nline two" GetExpanded="line one\nline two"
	// PEM: Get="-----BEGIN KEY-----\nabc'\"def\n-----END KEY-----" GetExpanded="-----BEGIN KEY-----\nabc'\"def\n-----END KEY-----"
	// LITERAL: Get="a\nb" GetExpanded="a\nb"
	// AFTER: Get="still parsed" GetExpanded="still parsed"
	// entries: 4
}
```

If the closing quote is missing, the value runs to the end of the file and swallows every line after it. The file still renders back unchanged, but the keys below the broken line disappear from `Get` and `Keys`:

<!-- snippet: example_syntax_test.go#syn_Unterminated -->
```go
// ExampleParse_syntaxUnterminated shows what a missing closing quote does: the
// value keeps going to the end of the file and swallows every line after it.
// Nothing is lost on disk (Render is still byte-identical), but the keys
// after the bad line are no longer pairs.
func ExampleParse_syntaxUnterminated() {
	src := "OK=1\n" +
		"BROKEN=\"no closing quote\n" +
		"LATER=2\n"

	f := dotenv.Parse(src)
	fmt.Println("keys:", f.Keys())
	synShow(f, "BROKEN")
	synShow(f, "LATER")
	fmt.Println("round trip intact:", f.Render() == src)
	// Output:
	// keys: [OK BROKEN]
	// BROKEN: Get="no closing quote\nLATER=2" GetExpanded="no closing quote\nLATER=2"
	// LATER: not a pair
	// round trip intact: true
}
```

## Step 3 — Comments and where a value ends

A line whose first non-space character is `#` is a comment. In a bare value, [`#` starts an inline comment only when a space or tab comes right before it](ds:block?id=inlinecommentre-5km4985g), so `PASSWORD=a#b` keeps the whole password. Inside any quotes, `#` is part of the value.

After a closing quote the value is finished. Whatever follows on that line, a `# comment` or any other text, is not part of the value. It is kept on disk and re-emitted when the value is edited, but `Get` never returns it.

A comment that holds a complete assignment, such as `# DISABLED=off`, is a **disabled pair**: a setting that is switched off. It is not visible to `Get`, but `Disabled()` lists it and `Restore` turns it back on. See [Editing](editing.md) for those calls.

<!-- snippet: example_syntax_test.go#syn_Comments -->
```go
// ExampleParse_syntaxComments shows where a value ends and a comment begins.
// In a bare value a # starts a comment only when whitespace (space or tab)
// comes before it, so a password like a#b survives. Inside quotes a # is
// always part of the value.
func ExampleParse_syntaxComments() {
	src := "# a whole-line comment\n" +
		"   # an indented comment\n" +
		"SPACE=value # comment\n" +
		"TAB=value\t# comment\n" +
		"NOSPACE=a#b\n" +
		"QUOTED=\"a # b\" # comment\n" +
		"SINGLE='a#b'#comment\n" +
		"TRAILING=\"kept\" this text is dropped\n" +
		"# DISABLED=off\n"

	f := dotenv.Parse(src)
	for _, key := range f.Keys() {
		synShow(f, key)
	}
	for _, e := range f.Entries() {
		if e.Kind != dotenv.KindPair {
			fmt.Printf("%s %q\n", e.Kind, e.Raw[0])
		}
	}
	// Output:
	// SPACE: Get="value" GetExpanded="value"
	// TAB: Get="value" GetExpanded="value"
	// NOSPACE: Get="a#b" GetExpanded="a#b"
	// QUOTED: Get="a # b" GetExpanded="a # b"
	// SINGLE: Get="a#b" GetExpanded="a#b"
	// TRAILING: Get="kept" GetExpanded="kept"
	// comment "# a whole-line comment"
	// comment "   # an indented comment"
	// disabled-pair "# DISABLED=off"
}
```

## Step 4 — The `:` delimiter

[`KEY: value`](ds:block?id=pairre-v3u356bt) is read exactly like `KEY=value`, because both Docker Compose's env-file grammar and Node dotenv accept it. A space after the colon is optional, and `export` and quoting work the same way. The author's delimiter is kept when the value is edited; new keys are always written with `=`.

A key cannot contain `:` or `/`, so `URL=http://example.com:8080` is an ordinary `=` pair. The catch is a line that holds **only** a URL: `http://example.com` reads as key `http` with value `//example.com`. That is also how Compose reads it, and the line still renders back unchanged. If a file has stray URL lines, comment them out with `#`.

<!-- snippet: example_syntax_test.go#syn_Delimiter -->
```go
// ExampleParse_syntaxDelimiter shows the colon delimiter from the Docker
// Compose and Node dotenv grammars. KEY: value and KEY=value read the same.
// The catch: a line holding only a URL is also a colon pair, exactly as
// Compose reads it.
func ExampleParse_syntaxDelimiter() {
	src := "EQUALS=a\n" +
		"COLON: b\n" +
		"TIGHT:c\n" +
		"export SPACED : \"d e\"\n" +
		"URL=http://example.com:8080\n" +
		"http://example.com\n"

	f := dotenv.Parse(src)
	for _, key := range f.Keys() {
		synShow(f, key)
	}
	// Output:
	// EQUALS: Get="a" GetExpanded="a"
	// COLON: Get="b" GetExpanded="b"
	// TIGHT: Get="c" GetExpanded="c"
	// SPACED: Get="d e" GetExpanded="d e"
	// URL: Get="http://example.com:8080" GetExpanded="http://example.com:8080"
	// http: Get="//example.com" GetExpanded="//example.com"
}
```

## Step 5 — Key names and lines that are not pairs

A key may use letters, digits, `_`, `.`, `-`, `[` and `]`, and may start with a digit. That is [Docker Compose's charset](ds:block?id=keyclass-46et9zsb), which is wider than Node dotenv's and godotenv's, so dotted (`spring.datasource.url`), dashed (`MY-SERVICE-TOKEN`), digit-first (`2FA_SECRET`) and bracketed (`SERVICES[0]`) keys are all real pairs. Keys are case sensitive and ASCII only.

A line that does not fit (a space inside the key, a `!`, no key before the `=`, a non-ASCII letter, free text) becomes a `KindOther` entry. It is not an error: the line is kept verbatim, `Get` ignores it, and the rest of the file parses normally. Docker Compose would refuse such a file; this package keeps it so an editor can still open and save around it.

A line that is only a name, such as `HOME` or `export HOME`, is a Compose **inherited** declaration (`KindInherited`). The package records it but never reads the process environment to fill it in. See [Entries](entries.md) for `Inherited()`.

<!-- snippet: example_syntax_test.go#syn_Keys -->
```go
// ExampleParse_syntaxKeys shows which keys are pairs and what happens to a
// line that is not one. Keys use Docker Compose's charset: letters, digits,
// _ . - [ ]. Anything else, or a line with no delimiter and more than one
// word, is kept verbatim as KindOther and never becomes a key. A single bare
// name is a Compose inherited declaration.
func ExampleParse_syntaxKeys() {
	src := "2FA_SECRET=a\n" +
		"spring.datasource.url=b\n" +
		"MY-SERVICE-TOKEN=c\n" +
		"SERVICES[0]=d\n" +
		"HAS KEY=e\n" +
		"K!=f\n" +
		"=no key\n" +
		"Ké=g\n" +
		"just some text\n" +
		"HOME\n"

	f := dotenv.Parse(src)
	fmt.Println("keys:", f.Keys())
	for _, e := range f.Entries() {
		fmt.Printf("%-9s key=%q raw=%q\n", e.Kind, e.Key, e.Raw[0])
	}
	// Output:
	// keys: [2FA_SECRET spring.datasource.url MY-SERVICE-TOKEN SERVICES[0]]
	// pair      key="2FA_SECRET" raw="2FA_SECRET=a"
	// pair      key="spring.datasource.url" raw="spring.datasource.url=b"
	// pair      key="MY-SERVICE-TOKEN" raw="MY-SERVICE-TOKEN=c"
	// pair      key="SERVICES[0]" raw="SERVICES[0]=d"
	// other     key="" raw="HAS KEY=e"
	// other     key="" raw="K!=f"
	// other     key="" raw="=no key"
	// other     key="" raw="Ké=g"
	// other     key="" raw="just some text"
	// inherited key="HOME" raw="HOME"
}
```

Being a valid key does not mean a reference can reach it. `${MY-SERVICE-TOKEN}` is read as a default operator, not a name with dashes. See [Expansion › Which names a reference can reach](expansion.md#step-5--which-names-a-reference-can-reach).

## Step 6 — Every entry kind

Each line, or block of lines for a multi-line value, becomes one `Entry` with a [`Kind`](ds:block?id=kinds-77m444t8). Only `KindPair` entries count as settings; everything else is kept so the file renders back exactly.

| Kind | String | Example line | Seen by `Get` |
|---|---|---|---|
| `KindPair` | `pair` | `KEY=value` | yes |
| `KindComment` | `comment` | `# note` | no |
| `KindBlank` | `blank` | (empty or only spaces) | no |
| `KindDisabledPair` | `disabled-pair` | `# OLD=value` | no (see `Disabled()`) |
| `KindInherited` | `inherited` | `HOME` | no (see `Inherited()`) |
| `KindOther` | `other` | `not a pair` | no |

<!-- snippet: example_syntax_test.go#syn_Kinds -->
```go
// ExampleParse_syntaxKinds lists every Kind the parser assigns, one line each.
// Only KindPair entries are visible to Get, Keys, Map and GetExpanded; the
// rest are kept so Render can write the file back byte for byte.
func ExampleParse_syntaxKinds() {
	src := "# note\n" +
		"\n" +
		"KEY=value\n" +
		"# OLD=value\n" +
		"HOME\n" +
		"not a pair\n"

	f := dotenv.Parse(src)
	for _, e := range f.Entries() {
		fmt.Printf("%-13s %q\n", e.Kind, e.Raw[0])
	}
	_, ok := f.Get("OLD")
	fmt.Println("OLD visible to Get:", ok)
	fmt.Println("round trip intact:", f.Render() == src)
	// Output:
	// comment       "# note"
	// blank         ""
	// pair          "KEY=value"
	// disabled-pair "# OLD=value"
	// inherited     "HOME"
	// other         "not a pair"
	// OLD visible to Get: false
	// round trip intact: true
}
```

## Step 7 — Duplicate keys

When a key appears more than once, the last occurrence wins for `Get`, `GetExpanded`, `Map` and `ExpandedMap`, and `Set` updates that last one. This matches every mainstream dotenv tool and Compose. The earlier lines are not hidden: `Count` reports how many there are and `Pairs` lists them all, so a lint step can flag the mistake.

<!-- snippet: example_syntax_test.go#syn_Duplicates -->
```go
// ExampleParse_syntaxDuplicates shows duplicate keys. The last one wins for
// Get, GetExpanded and Map, matching every dotenv tool and Compose. Count and
// Pairs still show the earlier ones, so an authoring mistake stays visible.
func ExampleParse_syntaxDuplicates() {
	src := "PORT=3000\n" +
		"HOST=localhost\n" +
		"PORT=8080\n"

	f := dotenv.Parse(src)
	synShow(f, "PORT")
	fmt.Println("count:", f.Count("PORT"))
	fmt.Println("keys:", f.Keys())
	fmt.Println("pairs:", f.Pairs())
	// Output:
	// PORT: Get="8080" GetExpanded="8080"
	// count: 2
	// keys: [PORT HOST]
	// pairs: [{PORT 3000} {HOST localhost} {PORT 8080}]
}
```

## Step 8 — `encrypted:` values

A value such as `encrypted:BFt4x9` is an ordinary string to the parser. It is stored and returned verbatim; this package never decrypts anything. To decrypt on read, install a value transformer (`WithValueTransform` or a `ValueTransformer` plugin, see [Plugins](plugins.md)); it runs after expansion and also on single-quoted values.

If the ciphertext can contain `$`, single-quote it. A bare value is expanded, so `$cd` in `BARE` below is read as a reference to an unset key and vanishes:

<!-- snippet: example_syntax_test.go#syn_Encrypted -->
```go
// ExampleParse_syntaxEncrypted shows that an encrypted: value is stored and
// read verbatim. This package never decrypts; plug a ValueTransformer in for
// that. Single quotes keep a $ inside ciphertext from being read as a
// reference.
func ExampleParse_syntaxEncrypted() {
	src := "TOKEN=encrypted:BFt4x9\n" +
		"BARE=encrypted:ab$cd\n" +
		"SAFE='encrypted:ab$cd'\n"

	f := dotenv.Parse(src)
	for _, key := range f.Keys() {
		synShow(f, key)
	}
	// Output:
	// TOKEN: Get="encrypted:BFt4x9" GetExpanded="encrypted:BFt4x9"
	// BARE: Get="encrypted:ab$cd" GetExpanded="encrypted:ab"
	// SAFE: Get="encrypted:ab$cd" GetExpanded="encrypted:ab$cd"
}
```

## Step 9 — Escape sequences: `EscapeExtended` vs `EscapeCompose`

[Backslash escapes](ds:block?id=decodeescape-w7neknhj) are decoded **only inside double quotes**. Single-quoted, backtick and bare values keep every backslash as written. Which sequences are decoded depends on the [escape mode](ds:block?id=escapemodes-pqfu7w85) you pass with `WithEscapes`:

| In the file | `EscapeExtended` (default) | `EscapeCompose` |
|---|---|---|
| `\"` | `"` | `"` |
| `\\` | `\` | `\` |
| `\n` | newline | `\n` kept as two characters |
| `\r` | carriage return | `\r` kept |
| `\t` | tab | `\t` kept |
| anything else (`\d`, `\U`, `\$`) | kept, backslash included | kept, backslash included |

The two modes disagree about `\n`, `\r` and `\t`, so the right one depends on who wrote the file. Use `EscapeExtended` for files written for Node dotenv, godotenv and most application tooling. Use `EscapeCompose` for files Docker Compose reads, and for values with Windows paths or regexes. The mode is part of parsing, so pass the same option to every `Open`/`Parse` of a file.

<!-- snippet: example_syntax_test.go#syn_WithEscapes -->
```go
// synEscapesSrc holds one double-quoted value per escape sequence, plus one
// single-quoted value, so both modes are compared on exactly the same input.
const synEscapesSrc = `QUOTE="say \"hi\""` + "\n" +
	`BACKSLASH="a\\b"` + "\n" +
	`NEWLINE="a\nb"` + "\n" +
	`TAB="a\tb"` + "\n" +
	`WINPATH="C:\Users\dev"` + "\n" +
	`REGEX="^\d+$"` + "\n" +
	`SINGLE='a\nb'` + "\n"

// ExampleWithEscapes compares the two escape dialects on the same input.
// EscapeExtended (the default) decodes \n \r \t \" \\. EscapeCompose decodes
// only \" and \\, like Docker Compose. Unknown sequences such as \U or \d are
// kept as written in both. Single-quoted values never decode anything.
func ExampleWithEscapes() {
	for _, mode := range []dotenv.EscapeMode{dotenv.EscapeExtended, dotenv.EscapeCompose} {
		fmt.Println("mode:", mode)
		f := dotenv.Parse(synEscapesSrc, dotenv.WithEscapes(mode))
		for _, key := range f.Keys() {
			v, _ := f.Get(key)
			fmt.Printf("  %s=%q\n", key, v)
		}
	}
	// Output:
	// mode: extended
	//   QUOTE="say \"hi\""
	//   BACKSLASH="a\\b"
	//   NEWLINE="a\nb"
	//   TAB="a\tb"
	//   WINPATH="C:\\Users\\dev"
	//   REGEX="^\\d+$"
	//   SINGLE="a\\nb"
	// mode: compose
	//   QUOTE="say \"hi\""
	//   BACKSLASH="a\\b"
	//   NEWLINE="a\\nb"
	//   TAB="a\\tb"
	//   WINPATH="C:\\Users\\dev"
	//   REGEX="^\\d+$"
	//   SINGLE="a\\nb"
}
```

The case that bites in practice is a Windows path with a folder starting with `n` or `t`:

<!-- snippet: example_syntax_test.go#syn_EscapeCompose -->
```go
// ExampleEscapeCompose shows the trap EscapeCompose exists for: a Windows
// path whose folder starts with n or t. Under the default EscapeExtended the
// \n and \t become a newline and a tab; under EscapeCompose the path is kept.
func ExampleEscapeCompose() {
	const src = `DIR="C:\new\tools"` + "\n"

	extended, _ := dotenv.Parse(src).Get("DIR")
	compose, _ := dotenv.Parse(src, dotenv.WithEscapes(dotenv.EscapeCompose)).Get("DIR")
	fmt.Printf("extended: %q\n", extended)
	fmt.Printf("compose:  %q\n", compose)
	// Output:
	// extended: "C:\new\tools"
	// compose:  "C:\\new\\tools"
}
```

`EscapeMode` implements `fmt.Stringer`: `EscapeExtended` prints as `extended` and `EscapeCompose` as `compose`.

## Quick reference

Each row is pinned by an example on this page or by `conformance_test.go` / `edge_test.go`. `GetExpanded` assumes `NAME=world` is also in the file.

| Line in the file | `Get` | `GetExpanded` |
|---|---|---|
| `K=hello $NAME   ` | `hello $NAME` | `hello world` |
| `K="hello $NAME  "` | `hello $NAME␣␣` | `hello world␣␣` |
| `K='hello $NAME'` | `hello $NAME` | `hello $NAME` |
| ``K=`hello $NAME` `` | `hello $NAME` | `hello $NAME` |
| `export K=v` | `v` | `v` |
| `  K  =  v  ` | `v` | `v` |
| `K=` | (empty) | (empty) |
| `K: v` | `v` | `v` |
| `K=a=b=c` | `a=b=c` | `a=b=c` |
| `K=a#b` | `a#b` | `a#b` |
| `K=a #b` | `a` | `a` |
| `K="a # b" # c` | `a # b` | `a # b` |
| `K="a\nb"` (default mode) | `a`, newline, `b` | same |
| `K="a\nb"` (`EscapeCompose`) | `a\nb` | `a\nb` |
| `K='a\nb'` | `a\nb` | `a\nb` |
| `K=a\nb` (bare) | `a\nb` | `a\nb` |
| `K="C:\Users\x"` | `C:\Users\x` | `C:\Users\x` |
| `K=encrypted:BFt4x9` | `encrypted:BFt4x9` | `encrypted:BFt4x9` |
| `HAS KEY=v` | not a pair (`KindOther`) | not a pair |
| `http://example.com` | key `http` = `//example.com` | same |

## Try it

Run the examples on this page from the repository root:

```sh
go test -run '^(ExampleParse_syntaxQuotes|ExampleWithEscapes)$' -v .
```

```text
=== RUN   ExampleParse_syntaxQuotes
--- PASS: ExampleParse_syntaxQuotes (0.00s)
=== RUN   ExampleWithEscapes
--- PASS: ExampleWithEscapes (0.00s)
PASS
ok  	github.com/ubgo/dotenv	0.006s
```

A pass means the printed output matched the `// Output:` block in each snippet above exactly. Change a line of `src` in [`example_syntax_test.go`](../../example_syntax_test.go) and run it again to see how the parser reads your own input.

## Pitfalls

- **A missing closing quote eats the rest of the file.** Every key below the broken line stops being a pair, with no error. Check `Keys()` or `Count` after loading a file you did not write. See [`ExampleParse_syntaxUnterminated`](../../example_syntax_test.go).
- **Text after a closing quote is silently ignored.** `K="kept" dropped` gives `kept`. The extra text stays on disk but is not part of the value.
- **Bare values lose their trailing whitespace.** Quote a value whose spaces matter.
- **`#` without a space before it is part of a bare value.** `K=value#note` is the value `value#note`. Put a space before the `#` to make it a comment.
- **`\n` means different things in the two escape modes.** A file written for Docker Compose and read with the default `EscapeExtended` turns `C:\new` into a newline. Choose the mode by who wrote the file.
- **A backslash never escapes `$`.** `"\$NAME"` keeps the backslash and still expands `$NAME`. Write `$$NAME` for a literal `$NAME`, or single-quote the value. See [Expansion › `$$` and how `$` is read](expansion.md#step-3--literal-dollars-and-how--is-read).
- **A bare URL line becomes a pair named `http`.** Compose reads it the same way. Comment such lines out.
- **Bad keys do not raise errors.** `HAS KEY=v` is silently a `KindOther` line. If you need to reject malformed files, walk `Entries()` and fail on `KindOther`.
- **Unquoted ciphertext or passwords containing `$` get expanded.** Single-quote them.

## Related

- [Variable expansion](expansion.md): `${VAR}`, operators, `$$`, `RequiredError`, command substitution.
- [Reading](reading.md) and [Entries](entries.md): `Open`, `Parse`, `Get`, `Keys`, `Pairs`, `Entries`, `Inherited`.
- [Editing](editing.md): how quoting is chosen when a value is written back, and `Disabled`/`Restore`.
- [Plugins](plugins.md): value transformers for `encrypted:` values.
- [Feature index](../reference/features.md#library-syntax-parse)
- Examples in [`example_syntax_test.go`](../../example_syntax_test.go): `ExampleParse_syntaxQuotes`, `ExampleParse_syntaxMultiline`, `ExampleParse_syntaxUnterminated`, `ExampleParse_syntaxComments`, `ExampleParse_syntaxDelimiter`, `ExampleParse_syntaxKeys`, `ExampleParse_syntaxKinds`, `ExampleParse_syntaxDuplicates`, `ExampleParse_syntaxEncrypted`, `ExampleWithEscapes`, `ExampleEscapeCompose`.
