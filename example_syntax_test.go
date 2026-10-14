package dotenv_test

import (
	"fmt"

	"github.com/ubgo/dotenv"
)

// snippet:start syn_Quotes

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

// snippet:end syn_Quotes

// snippet:start syn_Multiline

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

// snippet:end syn_Multiline

// snippet:start syn_Unterminated

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

// snippet:end syn_Unterminated

// snippet:start syn_Comments

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

// snippet:end syn_Comments

// snippet:start syn_Delimiter

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

// snippet:end syn_Delimiter

// snippet:start syn_Keys

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

// snippet:end syn_Keys

// snippet:start syn_Duplicates

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

// snippet:end syn_Duplicates

// snippet:start syn_Encrypted

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

// snippet:end syn_Encrypted

// snippet:start syn_WithEscapes

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

// snippet:end syn_WithEscapes

// snippet:start syn_EscapeCompose

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

// snippet:end syn_EscapeCompose

// snippet:start syn_Kinds

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

// snippet:end syn_Kinds
