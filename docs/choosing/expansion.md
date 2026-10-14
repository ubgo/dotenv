# Choosing an expansion setup

Expansion is how `${VAR}` references become values. You choose three things: which read call, which operator in the file, and which options (escape dialect, environment fallback, command substitution).

**On this page:** [Which read call](#which-read-call) · [Which operator](#which-operator) · [Which options](#which-options) · [If X, use Y](#if-x-use-y) · [Pitfalls](#pitfalls) · [Related](#related)

## Which read call

| Call | Returns | On a failing `${VAR:?…}` | Use for | Guide | Example |
|---|---|---|---|---|---|
| `f.Get(key)` | the raw value, references intact | never fails | anything you write back | [expansion, step 1](../library/expansion.md#step-1--get-vs-getexpanded) | [`ExampleFile_Get`](../../example_read_test.go) |
| `f.GetExpanded(key)` | one value, resolved; `(value, found, err)` | returns a `*RequiredError` | reading one value the way a consumer sees it | [expansion, step 1](../library/expansion.md#step-1--get-vs-getexpanded) | [`ExampleFile_GetExpanded`](../../example_expand_test.go) |
| `f.ExpandedMap()` | every active key, resolved | stops at the first error, returns no map | handing the whole configuration to a binder | [expansion, step 7](../library/expansion.md#step-7--expand-the-whole-file-with-expandedmap) | [`ExampleFile_ExpandedMap`](../../example_expand_test.go) |
| `dotenv.Read(path)` | `Open` + `ExpandedMap` | same as `ExpandedMap` | a program that only consumes configuration | [reading, step 1](../library/reading.md#step-1--read-the-values-in-one-call) | [`ExampleRead`](../../example_read_test.go) |

## Which operator

All operators are Docker Compose's. "Empty" means set to `""`.

| You want | Write | Unset | Empty | Set to `x` | Example |
|---|---|---|---|---|---|
| the value | `${VAR}` or `$VAR` | `""` | `""` | `x` | [`ExampleFile_GetExpanded`](../../example_expand_test.go) |
| a default when it is missing or empty | `${VAR:-d}` | `d` | `d` | `x` | [`ExampleFile_GetExpanded_operators`](../../example_expand_test.go) |
| a default only when it is missing | `${VAR-d}` | `d` | `""` | `x` | same |
| an alternative when it has a value | `${VAR:+alt}` | `""` | `""` | `alt` | same |
| an alternative when it is set at all | `${VAR+alt}` | `""` | `alt` | `alt` | same |
| to fail loudly when missing or empty | `${VAR:?msg}` | error | error | `x` | [`ExampleRequiredError`](../../example_expand_test.go) |
| to fail loudly only when missing | `${VAR?msg}` | error | `""` | `x` | same |
| a literal `$` | `$$` | — | — | — | [`ExampleFile_GetExpanded_dollar`](../../example_expand_test.go) |
| no expansion at all for this value | single quotes or backticks: `'${VAR}'` | — | — | — | [`ExampleFile_GetExpanded_literal`](../../example_expand_test.go) |

Operators nest: `${A:-${B:-c}}` works, and so do chains (`C=${B}` where `B=${A}`) ([`ExampleFile_GetExpanded_nesting`](../../example_expand_test.go)).

## Which options

| Option | Default | Turn it on when | Guide | Example |
|---|---|---|---|---|
| `WithEscapes(EscapeExtended)` | **on** | your files use `\n`, `\t`, `\r` inside double quotes as escapes | [syntax, step 9](../library/syntax.md#step-9--escape-sequences-escapeextended-vs-escapecompose) | [`ExampleWithEscapes`](../../example_syntax_test.go) |
| `WithEscapes(EscapeCompose)` | off | Docker Compose reads the same file, or values hold Windows paths or regexes where `\n` must stay two characters | [syntax, step 9](../library/syntax.md#step-9--escape-sequences-escapeextended-vs-escapecompose) | [`ExampleEscapeCompose`](../../example_syntax_test.go) |
| `WithLookup(fn)` | off | references the file does not define should come from somewhere else, such as `os.LookupEnv`; the file always wins | [plugins, step 1](../library/plugins.md#step-1--fill-missing-references-with-withlookup) | [`ExampleWithLookup`](../../example_plugin_test.go) |
| `WithCommandSubstitution(true)` | off | you trust the file and want `$(command)` to run through `sh -c` | [expansion, step 9](../library/expansion.md#step-9--command-substitution-opt-in) | [`ExampleWithCommandSubstitution_order`](../../example_expand_test.go) |
| `WithCommandRunner(fn)` | off | you want `$(command)` but only through your own runner (an allow-list, a sandbox, a fake in tests); installing it turns substitution on | [expansion, step 9](../library/expansion.md#step-9--command-substitution-opt-in) | [`ExampleWithCommandRunner_allowList`](../../example_expand_test.go) |

## If X, use Y

- **If you write the value back to a file**, use `Get`. `GetExpanded` output saved to disk flattens the reference for good.
- **If a value must be present for the program to work**, write `${VAR:?message}` in the file, and match the error with `errors.As(err, &re)` where `re` is a `*dotenv.RequiredError`. The two `?` forms are the only operators that produce an error.
- **If a value may legitimately be empty**, use the forms without a colon (`-`, `+`, `?`); the colon forms treat empty like unset.
- **If the file comes from somewhere you do not control** (a repository, an image, a teammate), keep command substitution off. With it on, reading the file runs shell commands.
- **If you need `$(command)` in tests**, use `WithCommandRunner` with a fake runner, never the real shell ([`ExampleWithCommandRunner`](../../example_plugin_test.go)).
- **If Docker Compose reads the same file**, use `EscapeCompose`; see [Options for your kind of app](app-setups.md#docker-compose-compatibility).

## Pitfalls

- **A failing command expands to `""` with no error**, as a shell does inside a value. A command that must succeed needs a check in your runner.
- **`WithCommandRunner(nil)` turns on the real `sh -c`**, the same as `WithCommandSubstitution(true)`.
- **A key that refers to itself sees the line above it**, as in Docker Compose. `A=1` followed by `A=${A}2` gives `12`; `ZONE=${ZONE:-x}` with no earlier `ZONE` asks your Lookuper, then gives `x` ([`ExampleFile_GetExpanded_selfReference`](../../example_expand_test.go)). In library v0.1.1 and earlier these gave `22` and `""`.
- **Cycles terminate but can leave text.** A reference that re-enters a variable already being expanded sees only the lines above, then the Lookuper, else `""`; the rest of each value stays, so `A=a${B}` with `B=b${A}` gives `A` = `ab` ([`ExampleFile_GetExpanded_nesting`](../../example_expand_test.go)).
- **Forward references differ from Compose.** A reference to a key defined further down resolves here; Compose reads top to bottom and leaves it unset.
- **Names stop at characters that are not part of a bare name.** `$spring.url` reads `$spring` followed by `.url`; use `${spring.url}`. `${MY-KEY}` reads as the `-` operator on `MY` ([`ExampleFile_GetExpanded_names`](../../example_expand_test.go)).
- **A backslash never escapes a `$`.** Use `$$`.

## Related

- [Variable expansion](../library/expansion.md) · [.env syntax](../library/syntax.md) · [Plugins](../library/plugins.md)
- [How to read a file](reading.md) · [Which plugin hook](plugins.md)
- [All choosing guides](README.md) · [Feature index](../reference/features.md)
