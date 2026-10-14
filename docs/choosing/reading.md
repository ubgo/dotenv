# Choosing how to read a file

Pick the entry point by two questions: where the content comes from, and whether you will write it back.

**On this page:** [Decision table](#decision-table) · [If X, use Y](#if-x-use-y) · [Which accessor](#which-accessor) · [Pitfalls](#pitfalls) · [Related](#related)

## Decision table

| Entry point | Input | Missing file | Values come back | Can `Save` | Guide | Example |
|---|---|---|---|---|---|---|
| `Read(path, opts...)` | a path | **error** | expanded, as a `map[string]string` | no (you get a map, not a file) | [reading, step 1](../library/reading.md#step-1--read-the-values-in-one-call) | [`ExampleRead`](../../example_read_test.go) |
| `Open(path, opts...)` | a path | empty `*File`, no error; `Existed()` is `false` | raw (`Get`) or expanded (`GetExpanded`) | yes, to `Path`; creates the file at `0600` | [reading, step 2](../library/reading.md#step-2--open-a-file-you-may-edit) | [`ExampleOpen`](../../example_read_test.go), [`ExampleOpen_missing`](../../example_read_test.go) |
| `Parse(content, opts...)` | a string | not applicable; never fails | raw or expanded | only with `SaveAs`, or after you set `f.Path` | [reading, step 3](../library/reading.md#step-3--parse-a-string-or-a-reader) | [`ExampleParse`](../../example_read_test.go) |
| `ParseReader(r, opts...)` | any `io.Reader` | not applicable; returns the reader's error | raw or expanded | only with `SaveAs`, or after you set `f.Path` | [reading, step 3](../library/reading.md#step-3--parse-a-string-or-a-reader) | [`ExampleParseReader`](../../example_read_test.go) |

All four take the same options (`WithEscapes`, `WithLookup`, `WithPlugin` and the rest), so the choice of entry point never limits which features you can use.

## If X, use Y

- **If your program only consumes configuration**, use `Read`. It is `Open` followed by `ExpandedMap`: one call, references resolved, comments and disabled lines skipped, duplicates collapsed to the last one.
- **If the file must exist** (a service that cannot start without it), use `Read`: a missing file is an error. With `Open`, check `f.Existed()` yourself.
- **If you will edit and save**, use `Open`. Values stay raw, so saving never flattens a `${reference}` into a copy of its target.
- **If you are creating a file that may not exist yet**, use `Open`. A missing file opens empty, and `Save` creates it with mode `0600`.
- **If the content is already in memory** (an embedded fixture, a test input, a value from a secret store), use `Parse`.
- **If the content arrives as a stream** (an HTTP body, a pipe, an archive member), or the file is large, use `ParseReader` rather than `io.ReadAll` + `Parse`: it skips one full copy of the content. It still holds the whole file in memory; nothing here streams.
- **If you need comments, blank lines or disabled settings**, any entry point except `Read` gives you a `*File`; walk `f.Entries()`.

## Which accessor

| Question | Accessor | Duplicates | Disabled lines | Example |
|---|---|---|---|---|
| What is on disk for this key? | `f.Get(key)` | last wins | ignored | [`ExampleFile_Get`](../../example_read_test.go) |
| What does a consumer see? | `f.GetExpanded(key)` | last wins | ignored | [`ExampleFile_GetExpanded`](../../example_expand_test.go) |
| Is the key active? | `f.Has(key)` | — | ignored (a disabled key is not active) | [`ExampleFile_Has`](../../example_read_test.go) |
| Is the key duplicated? | `f.Count(key)` | counted | ignored | [`ExampleFile_Count`](../../example_read_test.go) |
| Which keys are set? | `f.Keys()` | once each, file order | ignored | [`ExampleFile_Keys`](../../example_read_test.go) |
| Every active assignment, in order? | `f.Pairs()` | all kept | ignored | [`ExampleFile_Pairs`](../../example_read_test.go) |
| The effective values, raw? | `f.Map()` | last wins | ignored | [`ExampleFile_Map`](../../example_read_test.go) |
| The effective values, expanded? | `f.ExpandedMap()` | last wins | ignored | [`ExampleFile_ExpandedMap`](../../example_expand_test.go) |
| Which settings are turned off? | `f.Disabled()` | all kept | **only these** | [`ExampleFile_Disabled`](../../example_edit_test.go) |
| Which names are inherited from the environment? | `f.Inherited()` | — | — | [`ExampleFile_Inherited`](../../example_edit_test.go) |
| Every line, including comments? | `f.Entries()` | all kept | included, as `KindDisabledPair` | [`ExampleFile_Entries`](../../example_read_test.go) |
| Did the file exist, with which mode and path? | `f.Existed()`, `f.Mode()`, `f.Path` | — | — | [`ExampleFile_Existed`](../../example_read_test.go) |

## Pitfalls

- **`Read` and `Open` differ on a missing file.** `Read` fails; `Open` succeeds with an empty file. Code that switches from one to the other changes its error behaviour.
- **`Read` stops at the first expansion error.** One `${VAR:?message}` that is unset fails the whole call, with a `*RequiredError`. That is deliberate: a half-expanded map would hide which values are trustworthy. See [expansion, step 8](../library/expansion.md#step-8--handle-requirederror).
- **`ExpandedMap` (and so `Read`) expands every occurrence, including duplicates that a later line overrides.** If an early duplicate holds a `${VAR:?…}` that fails, the call fails even though `GetExpanded` on that key returns the later, valid value. Remove dead duplicates.
- **A parsed file has no path.** `Save` on a file from `Parse` or `ParseReader` returns an error until you set `f.Path`, or call `SaveAs(path)` ([`ExampleFile_Save_noPath`](../../example_edit_test.go)).
- **Nothing here reads `os.Environ`.** Name-only lines such as `HOME` are listed by `Inherited()` but never resolved, and a `${VAR}` the file does not define expands to `""` unless you install `WithLookup(os.LookupEnv)`.
- **Prose comments with `=` or `:` read as disabled settings.** `# TODO=fix` and `# Note: rotate monthly` both appear in `Disabled()` ([`ExampleKind_pitfalls`](../../example_read_test.go)).

## Related

- [Reading a .env file](../library/reading.md) · [Entries and kinds](../library/entries.md)
- [Which expansion setup](expansion.md) · [How to edit a file](editing.md)
- [All choosing guides](README.md) · [Feature index](../reference/features.md)
