# Feature index

Every feature of the `github.com/ubgo/dotenv` library and the `dotenvctl` CLI, one row each, with the guide section that explains it and a runnable example where one exists. The list is built from the exported Go API and the CLI's command and flag definitions, not from other docs.

- **Package** — the Go package (or `dotenvctl` for a command-line feature).
- **Guide** — the section that documents the feature. A row marked *(brief)* would be covered only by a table row or a one-line mention; none is today.
- **Example** — runnable `Example` functions (run with `go test -run Example` in the module) whose code uses the feature; `go test` compiles each one and checks its output. A dash means the feature is a CLI or process behaviour shown in the guide with real output rather than an `Example`.

**Areas:** [Library: reading](#library-reading) · [Library: writing and generating](#library-writing-and-generating) · [Library: syntax (parse)](#library-syntax-parse) · [Library: expansion](#library-expansion) · [Library: plugins and hooks](#library-plugins-and-hooks) · [CLI: global](#cli-global) · [CLI: core verbs](#cli-core-verbs) · [CLI: multi-environment](#cli-multi-environment) · [CLI: plugins](#cli-plugins) · [Go API: cli module](#go-api-cli-module)

## Library: reading

| Feature | Package | Guide | Example |
|---|---|---|---|
| `Read(path)` — straight to an expanded `map[string]string` | `dotenv` | [reading › Step 1 — Read the values in one call](../library/reading.md#step-1--read-the-values-in-one-call) | [`ExampleRead`](../../example_read_test.go) |
| `Open(path)` — open from disk; a missing file is not an error | `dotenv` | [reading › Step 2 — Open a file you may edit](../library/reading.md#step-2--open-a-file-you-may-edit) | [`ExampleOpen`](../../example_read_test.go), [`ExampleOpen_missing`](../../example_read_test.go) |
| `Parse(content)` — parse from a string | `dotenv` | [reading › Step 3 — Parse a string or a reader](../library/reading.md#step-3--parse-a-string-or-a-reader) | [`ExampleParse`](../../example_read_test.go) |
| `ParseReader(r)` — parse from any `io.Reader`, fewer copies | `dotenv` | [reading › Step 3 — Parse a string or a reader](../library/reading.md#step-3--parse-a-string-or-a-reader) | [`ExampleParseReader`](../../example_read_test.go) |
| `f.Get(key)` — raw value, last duplicate wins | `dotenv` | [reading › Step 4 — Query the file](../library/reading.md#step-4--query-the-file) | [`ExampleFile_Get`](../../example_read_test.go) |
| `f.Has(key)` — active-pair check | `dotenv` | [reading › Step 4 — Query the file](../library/reading.md#step-4--query-the-file) | [`ExampleFile_Has`](../../example_read_test.go) |
| `f.Count(key)` — how many active entries share a key | `dotenv` | [reading › Step 4 — Query the file](../library/reading.md#step-4--query-the-file) | [`ExampleFile_Count`](../../example_read_test.go) |
| `f.Keys()` — active keys, file order, deduplicated | `dotenv` | [reading › Step 4 — Query the file](../library/reading.md#step-4--query-the-file) | [`ExampleFile_Keys`](../../example_read_test.go) |
| `f.Pairs()` — active pairs, duplicates included | `dotenv` | [reading › Step 4 — Query the file](../library/reading.md#step-4--query-the-file) | [`ExampleFile_Pairs`](../../example_read_test.go) |
| `f.Map()` — effective last-wins map | `dotenv` | [reading › Step 4 — Query the file](../library/reading.md#step-4--query-the-file) | [`ExampleFile_Map`](../../example_read_test.go) |
| `f.Entries()` — every entry incl. comments and blanks | `dotenv` | [entries › Step 3 — Walk every entry](../library/entries.md#step-3--walk-every-entry) | [`ExampleFile_Entries`](../../example_read_test.go) |
| `Entry` struct (`Kind`, `Raw`, `Key`, `Value`) | `dotenv` | [entries › Step 2 — Read an entry's fields](../library/entries.md#step-2--read-an-entrys-fields) | [`ExampleEntry`](../../example_read_test.go) |
| `Kind` — six entry kinds and `Kind.String()` | `dotenv` | [entries › Step 1 — Know the six kinds](../library/entries.md#step-1--know-the-six-kinds) | [`ExampleKind`](../../example_read_test.go), [`ExampleKind_String`](../../example_read_test.go) |
| `f.Existed()` / `f.Mode()` / `f.Path` — file facts | `dotenv` | [reading › Step 5 — Check where the file came from](../library/reading.md#step-5--check-where-the-file-came-from) | [`ExampleFile_Existed`](../../example_read_test.go), [`ExampleFile_Mode`](../../example_read_test.go), [`ExampleFile_Path`](../../example_read_test.go) |
| Parse cost at scale (linear, ~4x memory) | `dotenv` | [reading › Cost at scale](../library/reading.md#cost-at-scale) | — |

## Library: writing and generating

| Feature | Package | Guide | Example |
|---|---|---|---|
| `f.Set(key, value)` — upsert in place or append | `dotenv` | [editing › Step 1 — Change or add a value with Set](../library/editing.md#step-1--change-or-add-a-value-with-set) | [`ExampleFile_Set`](../../example_edit_test.go), [`ExampleFile_Set_quoting`](../../example_edit_test.go), [`ExampleFile_Set_duplicates`](../../example_edit_test.go) |
| `f.SetAfter` / `f.SetBefore` — anchored placement of a new key | `dotenv` | [editing › Step 2 — Place a new key with SetAfter and SetBefore](../library/editing.md#step-2--place-a-new-key-with-setafter-and-setbefore) | [`ExampleFile_SetAfter`](../../example_edit_test.go), [`ExampleFile_SetBefore`](../../example_edit_test.go) |
| `ErrAnchorNotFound` — missing anchor leaves the file untouched | `dotenv` | [editing › Step 2 — Place a new key with SetAfter and SetBefore](../library/editing.md#step-2--place-a-new-key-with-setafter-and-setbefore) | [`ExampleFile_SetAfter_anchorNotFound`](../../example_edit_test.go) |
| `f.Unset(key, del)` — comment out (reversible) or delete | `dotenv` | [editing › Step 3 — Turn a setting off with Unset](../library/editing.md#step-3--turn-a-setting-off-with-unset) | [`ExampleFile_Unset`](../../example_edit_test.go) |
| `f.Restore(key)` — re-enable a disabled setting byte-exactly | `dotenv` | [editing › Step 4 — Turn it back on with Restore](../library/editing.md#step-4--turn-it-back-on-with-restore) | [`ExampleFile_Restore`](../../example_edit_test.go) |
| `f.Disabled()` — commented-out settings as pairs | `dotenv` | [editing › Step 5 — List disabled settings](../library/editing.md#step-5--list-disabled-settings) | [`ExampleFile_Disabled`](../../example_edit_test.go) |
| `f.Inherited()` — Compose name-only declarations | `dotenv` | [editing › Step 6 — Handle inherited declarations](../library/editing.md#step-6--handle-inherited-declarations) | [`ExampleFile_Inherited`](../../example_edit_test.go) |
| `f.Append(entries...)` — literal placement at end | `dotenv` | [editing › Step 8 — Place blocks with Append, InsertAfter and InsertBefore](../library/editing.md#step-8--place-blocks-with-append-insertafter-and-insertbefore) | [`ExampleFile_Append`](../../example_edit_test.go), [`ExampleFile_Append_verbatim`](../../example_edit_test.go) |
| `f.InsertAfter` / `f.InsertBefore` — literal anchored blocks | `dotenv` | [editing › Step 8 — Place blocks with Append, InsertAfter and InsertBefore](../library/editing.md#step-8--place-blocks-with-append-insertafter-and-insertbefore) | [`ExampleFile_InsertAfter`](../../example_edit_test.go), [`ExampleFile_InsertBefore`](../../example_edit_test.go) |
| `NewPair` / `NewComment` / `NewBlank` constructors | `dotenv` | [editing › Step 7 — Generate content with the constructors](../library/editing.md#step-7--generate-content-with-the-constructors) | [`ExampleNewPair`](../../example_edit_test.go), [`ExampleNewComment`](../../example_edit_test.go) |
| Upsert vs literal families | `dotenv` | [editing › Two families: upsert and literal](../library/editing.md#two-families-upsert-and-literal) | [`ExampleFile_Append_vsSet`](../../example_edit_test.go) |
| `f.Clone()` — independent deep copy | `dotenv` | [editing › Step 9 — Derive another environment with Clone](../library/editing.md#step-9--derive-another-environment-with-clone) | [`ExampleFile_Clone`](../../example_edit_test.go), [`ExampleFile_Clone_baked`](../../example_edit_test.go) |
| `f.SaveAs(path)` — write elsewhere, `Path` untouched, mode rules | `dotenv` | [saving › Step 4 — Write a copy with SaveAs](../library/saving.md#step-4--write-a-copy-with-saveas) | [`ExampleFile_SaveAs`](../../example_edit_test.go) |
| `f.Render()` — serialize; never expands | `dotenv` | [README › Why references survive: Render never expands](../../README.md#why-references-survive-render-never-expands) | [`ExampleParse`](../../example_read_test.go) |
| `f.Save()` — atomic temp-0600 + rename | `dotenv` | [saving › How a save works](../library/saving.md#how-a-save-works) | [`ExampleFile_Save`](../../example_edit_test.go), [`ExampleFile_Save_noPath`](../../example_edit_test.go) |
| Mode preservation; new files 0600 | `dotenv` | [saving › Step 2 — Know which mode the file gets](../library/saving.md#step-2--know-which-mode-the-file-gets) | [`ExampleFile_Save_mode`](../../example_edit_test.go) |
| Line endings preserved per line; CRLF for new lines once the file has any CRLF | `dotenv` | [saving › Step 3 — Keep line endings and the trailing newline](../library/saving.md#step-3--keep-line-endings-and-the-trailing-newline) | [`ExampleFile_Save_lineEndings`](../../example_edit_test.go), [`Example_lineEndings`](../../example_readme_test.go) |
| Missing trailing newline preserved | `dotenv` | [saving › Step 3 — Keep line endings and the trailing newline](../library/saving.md#step-3--keep-line-endings-and-the-trailing-newline) | [`ExampleFile_Save_trailingNewline`](../../example_edit_test.go) |
| Byte-exact round trip and no-op `Set` invariants | `dotenv` | [README › Why not a dotenv library](../../README.md#why-not-a-dotenv-library) | [`ExampleFile_Set_noop`](../../example_edit_test.go) |
| Never reads or writes `os.Environ` | `dotenv` | [README › Scope](../../README.md#scope) | — |

## Library: syntax (parse)

| Feature | Package | Guide | Example |
|---|---|---|---|
| Unquoted, double-quoted values | `dotenv` | [syntax › Step 1 — Quoting: bare, double, single, backtick](../library/syntax.md#step-1--quoting-bare-double-single-backtick) | [`ExampleParse_syntaxQuotes`](../../example_syntax_test.go) |
| Single-quoted and backtick literal values | `dotenv` | [syntax › Step 1 — Quoting: bare, double, single, backtick](../library/syntax.md#step-1--quoting-bare-double-single-backtick) | [`ExampleParse_syntaxQuotes`](../../example_syntax_test.go) |
| Multi-line double-quoted and backtick values | `dotenv` | [syntax › Step 2 — Multi-line values](../library/syntax.md#step-2--multi-line-values) | [`ExampleParse_syntaxMultiline`](../../example_syntax_test.go), [`ExampleParse_syntaxUnterminated`](../../example_syntax_test.go) |
| `export` prefix and spaces around `=` | `dotenv` | [syntax › Step 1 — Quoting: bare, double, single, backtick](../library/syntax.md#step-1--quoting-bare-double-single-backtick) | [`ExampleParse_syntaxQuotes`](../../example_syntax_test.go) |
| `KEY: value` colon delimiter (preserved on edit) | `dotenv` | [syntax › Step 4 — The : delimiter](../library/syntax.md#step-4--the--delimiter) | [`ExampleParse_syntaxDelimiter`](../../example_syntax_test.go) |
| Compose key charset (`.` `-` `[` `]`, digit-first) | `dotenv` | [syntax › Step 5 — Key names and lines that are not pairs](../library/syntax.md#step-5--key-names-and-lines-that-are-not-pairs) | [`ExampleParse_syntaxKeys`](../../example_syntax_test.go) |
| Inline comments (`#` needs leading whitespace) | `dotenv` | [syntax › Step 3 — Comments and where a value ends](../library/syntax.md#step-3--comments-and-where-a-value-ends) | [`ExampleParse_syntaxComments`](../../example_syntax_test.go) |
| Unrecognised lines kept verbatim (`KindOther`) | `dotenv` | [syntax › Step 5 — Key names and lines that are not pairs](../library/syntax.md#step-5--key-names-and-lines-that-are-not-pairs) | [`ExampleParse_syntaxKeys`](../../example_syntax_test.go), [`ExampleKind`](../../example_read_test.go) |
| Disabled-pair detection (`# KEY=value` → `KindDisabledPair`) | `dotenv` | [entries › Step 1 — Know the six kinds](../library/entries.md#step-1--know-the-six-kinds) | [`ExampleKind`](../../example_read_test.go), [`ExampleKind_pitfalls`](../../example_read_test.go) |
| Name-only inherited lines (`KindInherited`) | `dotenv` | [entries › Step 1 — Know the six kinds](../library/entries.md#step-1--know-the-six-kinds) | [`ExampleKind`](../../example_read_test.go), [`ExampleParse_syntaxKinds`](../../example_syntax_test.go) |
| Duplicate keys: last wins | `dotenv` | [syntax › Step 7 — Duplicate keys](../library/syntax.md#step-7--duplicate-keys) | [`ExampleParse_syntaxDuplicates`](../../example_syntax_test.go) |
| `encrypted:` values stored verbatim | `dotenv` | [syntax › Step 8 — encrypted: values](../library/syntax.md#step-8--encrypted-values) | [`ExampleParse_syntaxEncrypted`](../../example_syntax_test.go) |
| `WithEscapes(EscapeExtended | EscapeCompose)` escape dialects | `dotenv` | [syntax › Step 9 — Escape sequences: EscapeExtended vs EscapeCompose](../library/syntax.md#step-9--escape-sequences-escapeextended-vs-escapecompose) | [`ExampleWithEscapes`](../../example_syntax_test.go), [`ExampleEscapeCompose`](../../example_syntax_test.go) |

## Library: expansion

| Feature | Package | Guide | Example |
|---|---|---|---|
| `f.GetExpanded(key)` — interpolated read with error | `dotenv` | [expansion › Step 1 — Get vs GetExpanded](../library/expansion.md#step-1--get-vs-getexpanded) | [`ExampleFile_GetExpanded`](../../example_expand_test.go) |
| `f.ExpandedMap()` — all values expanded, stops at first error | `dotenv` | [expansion › Step 7 — Expand the whole file with ExpandedMap](../library/expansion.md#step-7--expand-the-whole-file-with-expandedmap) | [`ExampleFile_ExpandedMap`](../../example_expand_test.go) |
| `${VAR}` / `$VAR` substitution | `dotenv` | [expansion › Step 1 — Get vs GetExpanded](../library/expansion.md#step-1--get-vs-getexpanded) | [`ExampleFile_GetExpanded`](../../example_expand_test.go), [`ExampleFile_GetExpanded_names`](../../example_expand_test.go) |
| `${VAR:-d}` / `${VAR-d}` defaults | `dotenv` | [expansion › Step 2 — The ${…} operators](../library/expansion.md#step-2--the--operators) | [`ExampleFile_GetExpanded_operators`](../../example_expand_test.go) |
| `${VAR:+alt}` / `${VAR+alt}` alternates | `dotenv` | [expansion › Step 2 — The ${…} operators](../library/expansion.md#step-2--the--operators) | [`ExampleFile_GetExpanded_operators`](../../example_expand_test.go) |
| `${VAR:?msg}` / `${VAR?msg}` required → `*RequiredError` | `dotenv` | [expansion › Step 8 — Handle RequiredError](../library/expansion.md#step-8--handle-requirederror) | [`ExampleRequiredError`](../../example_expand_test.go), [`ExampleFile_GetExpanded_operators`](../../example_expand_test.go) |
| `$$` literal dollar and `$` dispatch rules | `dotenv` | [expansion › Step 3 — Literal dollars and how $ is read](../library/expansion.md#step-3--literal-dollars-and-how--is-read) | [`ExampleFile_GetExpanded_dollar`](../../example_expand_test.go) |
| Nested defaults and chained references | `dotenv` | [expansion › Step 4 — Nesting, chains and cycles](../library/expansion.md#step-4--nesting-chains-and-cycles) | [`ExampleFile_GetExpanded_nesting`](../../example_expand_test.go) |
| Cycle and depth guard (the re-entering reference resolves to empty) | `dotenv` | [expansion › Step 4 — Nesting, chains and cycles](../library/expansion.md#step-4--nesting-chains-and-cycles) | [`ExampleFile_GetExpanded_nesting`](../../example_expand_test.go) |
| Single quotes / backticks suppress interpolation | `dotenv` | [expansion › Step 6 — Quotes that turn expansion off](../library/expansion.md#step-6--quotes-that-turn-expansion-off) | [`ExampleFile_GetExpanded_literal`](../../example_expand_test.go) |
| `WithCommandSubstitution(true)` — opt-in `$(cmd)` | `dotenv` | [expansion › Step 9 — Command substitution (opt-in)](../library/expansion.md#step-9--command-substitution-opt-in) | [`ExampleWithCommandSubstitution_off`](../../example_expand_test.go), [`ExampleWithCommandSubstitution_order`](../../example_expand_test.go) |
| `WithCommandRunner(fn)` — sandboxed runner for `$(cmd)` | `dotenv` | [expansion › Step 9 — Command substitution (opt-in)](../library/expansion.md#step-9--command-substitution-opt-in) | [`ExampleWithCommandRunner_allowList`](../../example_expand_test.go), [`ExampleWithCommandRunner`](../../example_plugin_test.go) |

## Library: plugins and hooks

| Feature | Package | Guide | Example |
|---|---|---|---|
| `WithPlugin(p...)` and the `Plugin` interface | `dotenv` | [plugins › Step 5 — Write a plugin type with several capabilities](../library/plugins.md#step-5--write-a-plugin-type-with-several-capabilities) | [`ExampleWithPlugin`](../../example_plugin_test.go) |
| `Lookuper` capability / `WithLookup(fn)` | `dotenv` | [plugins › Step 1 — Fill missing references with WithLookup](../library/plugins.md#step-1--fill-missing-references-with-withlookup) | [`ExampleWithLookup`](../../example_plugin_test.go), [`ExampleWithPlugin_lookupError`](../../example_plugin_test.go) |
| `ValueTransformer` / `WithValueTransform(fn)` | `dotenv` | [plugins › Step 2 — Decrypt values with WithValueTransform](../library/plugins.md#step-2--decrypt-values-with-withvaluetransform) | [`ExampleWithValueTransform`](../../example_plugin_test.go) |
| `SaveGuard` / `WithSaveGuard(fn)` | `dotenv` | [plugins › Step 3 — Refuse bad writes with WithSaveGuard](../library/plugins.md#step-3--refuse-bad-writes-with-withsaveguard) | [`ExampleWithSaveGuard`](../../example_plugin_test.go), [`ExampleWithPlugin_guard`](../../example_plugin_test.go) |
| `CommandRunner` capability interface | `dotenv` | [plugins › Step 8 — A CommandRunner as a type](../library/plugins.md#step-8--a-commandrunner-as-a-type) | [`ExampleCommandRunner`](../../example_plugin_test.go) |
| `EntryObserver` capability | `dotenv` | [plugins › Step 9 — Audit every entry with an EntryObserver](../library/plugins.md#step-9--audit-every-entry-with-an-entryobserver) | [`ExampleEntryObserver`](../../example_plugin_test.go) |
| `ExpandObserver` capability | `dotenv` | [plugins › Step 10 — Trace references with an ExpandObserver](../library/plugins.md#step-10--trace-references-with-an-expandobserver) | [`ExampleExpandObserver`](../../example_plugin_test.go) |
| Hook ordering rules (first non-miss, chained, last wins) | `dotenv` | [plugins › Ordering: what happens with several plugins](../library/plugins.md#ordering-what-happens-with-several-plugins) | [`ExampleFile_Plugins_ordering`](../../example_plugin_test.go) |
| The firewall: hooks never change written bytes | `dotenv` | [plugins › The firewall: plugins never change saved bytes](../library/plugins.md#the-firewall-plugins-never-change-saved-bytes) | [`ExampleFile_Plugins_firewall`](../../example_plugin_test.go) |
| `f.Plugins()` / `PluginInfo` / `Capability` constants | `dotenv` | [plugins › Checking what was detected: Plugins, PluginInfo, Capability](../library/plugins.md#checking-what-was-detected-plugins-plugininfo-capability) | [`ExampleFile_Plugins`](../../plugin_test.go), [`ExamplePluginInfo`](../../example_plugin_test.go), [`ExampleCapability`](../../example_plugin_test.go) |
| Typed config via the standard library, go-envconfig, caarlos0/env or cfgkit | `dotenv` | [typed-config › Step 2 — Bind them with the standard library](../library/typed-config.md#step-2--bind-them-with-the-standard-library) | [`Example_typedConfig`](../../example_plugin_test.go) |

## CLI: global

| Feature | Package | Guide | Example |
|---|---|---|---|
| `-f, --file` target file (default `./.env`) | `dotenvctl` | [getting-started › Choosing the file: -f](../getting-started.md#choosing-the-file--f) | [`ExampleExecute_get`](../../cli/dotenvcmd/example_test.go) |
| `--json` `{ok,data|error}` envelope | `dotenvctl` | [getting-started › Scripting: --json and exit codes](../getting-started.md#scripting---json-and-exit-codes) | [`ExampleExecute_json`](../../cli/dotenvcmd/example_test.go) |
| Exit codes 0 / 1 / 2 | `dotenvctl` | [getting-started › Scripting: --json and exit codes](../getting-started.md#scripting---json-and-exit-codes) | [`ExampleExecute`](../../cli/dotenvcmd/example_test.go), [`ExampleExecute_json`](../../cli/dotenvcmd/example_test.go) |
| `--version` | `dotenvctl` | [commands › Global flags and --version](../commands.md#global-flags-and---version) | — |
| `completion` shell completions | `dotenvctl` | [commands › completion — shell completions](../commands.md#completion--shell-completions) | — |

## CLI: core verbs

| Feature | Package | Guide | Example |
|---|---|---|---|
| `get KEY` (`--expand`) | `dotenvctl` | [commands › get — print one value](../commands.md#get--print-one-value) | [`ExampleExecute_get`](../../cli/dotenvcmd/example_test.go) |
| `set KEY=VALUE...` (`--after`, `--before`, `--dry-run`) | `dotenvctl` | [commands › set — upsert values, byte-preserving](../commands.md#set--upsert-values-byte-preserving) | [`ExampleExecute_set`](../../cli/dotenvcmd/example_test.go) |
| `unset KEY...` (`--delete`, `--dry-run`) | `dotenvctl` | [commands › unset — deactivate a key](../commands.md#unset--deactivate-a-key) | [`ExampleExecute_unset`](../../cli/dotenvcmd/example_test.go) |
| `restore KEY...` | `dotenvctl` | [commands › restore — the inverse of unset](../commands.md#restore--the-inverse-of-unset) | [`ExampleExecute_unset`](../../cli/dotenvcmd/example_test.go) |
| `list` (`--expand`, `--disabled`, `--inherited`) | `dotenvctl` | [commands › list — the file's effective contents](../commands.md#list--the-files-effective-contents) | [`ExampleExecute_list`](../../cli/dotenvcmd/example_test.go) |
| `keys` — names only | `dotenvctl` | [commands › keys — names only, pipe-friendly](../commands.md#keys--names-only-pipe-friendly) | [`ExampleExecute_keys`](../../cli/dotenvcmd/example_test.go) |
| Selection flags `--prefix` `--exclude-prefix` `--include` `--exclude` `--strip-prefix` | `dotenvctl` | [commands › Selecting keys — the shared grammar](../commands.md#selecting-keys--the-shared-grammar) | [`ExampleSelectFile`](../../cli/envkit/example_test.go), [`ExampleSelect`](../../cli/envkit/example_test.go) |
| `run -- CMD` — child env from the file, exit code passed through | `dotenvctl` | [commands › run — execute a command with the file's values](../commands.md#run--execute-a-command-with-the-files-values) | [`ExampleRun`](../../cli/envkit/example_test.go), [`ExampleChildEnv`](../../cli/envkit/example_test.go) |

## CLI: multi-environment

| Feature | Package | Guide | Example |
|---|---|---|---|
| `envs` (`--dir`) — discover the `.env` family | `dotenvctl` | [multi-env › envs — discover the family](../multi-env.md#envs--discover-the-family) | [`ExampleExecute_envs`](../../cli/dotenvcmd/example_test.go) |
| `matrix [FILE...]` drift table and state symbols | `dotenvctl` | [multi-env › matrix — the drift table](../multi-env.md#matrix--the-drift-table) | [`ExampleExecute_matrix`](../../cli/dotenvcmd/example_test.go) |
| `matrix --values` / `--reveal` masking | `dotenvctl` | [multi-env › matrix — the drift table](../multi-env.md#matrix--the-drift-table) | [`ExampleBuildMatrix`](../../cli/envkit/example_test.go) |
| `matrix --only-drift` | `dotenvctl` | [multi-env › matrix — the drift table](../multi-env.md#matrix--the-drift-table) | [`ExampleDriftRows`](../../cli/envkit/example_test.go) |
| `matrix --contract FILE` CI gate | `dotenvctl` | [recipes › CI: gate the build on env completeness](../recipes.md#ci-gate-the-build-on-env-completeness) | [`ExampleBuildMatrix`](../../cli/envkit/example_test.go) |
| `matrix --placeholder REGEX` | `dotenvctl` | [commands › Changing what counts as a placeholder: --placeholder](../commands.md#changing-what-counts-as-a-placeholder---placeholder) | [`ExampleExecute_matrix_placeholder`](../../cli/dotenvcmd/example_test.go) |
| `diff A B` (`--expand`, `--reveal`) | `dotenvctl` | [commands › diff — compare two files' effective configuration](../commands.md#diff--compare-two-files-effective-configuration) | [`ExampleExecute_diff`](../../cli/dotenvcmd/example_test.go) |
| `--format human|json|html` and `-o, --output` HTML reports | `dotenvctl` | [multi-env › HTML reports](../multi-env.md#html-reports) | [`ExampleMatrixHTML`](../../cli/report/example_test.go), [`ExampleDiffHTML`](../../cli/report/example_test.go) |

## CLI: plugins

| Feature | Package | Guide | Example |
|---|---|---|---|
| Exec plugins: `dotenvctl-<name>` on `PATH` | `dotenvctl` | [plugins › A complete exec plugin: dotenvctl-export](../plugins.md#a-complete-exec-plugin-dotenvctl-export) | — |
| Exec plugin env contract `DOTENVCTL_FILE` / `_JSON` / `_PLACEHOLDER` / `_BIN` / `_VERSION` | `dotenvctl` | [plugins › Writing your own plugin (exec plugins)](../plugins.md#writing-your-own-plugin-exec-plugins) | — |
| `plugins` — list discovered exec plugins and shadowing | `dotenvctl` | [commands › plugins — discover exec plugins](../commands.md#plugins--discover-exec-plugins) | — |
| Store-verb safety flags `--expand` `--include-placeholders` `--include-empty` `--dry-run` `--yes` | `dotenvctl` | [plugins › What every plugin shares](../plugins.md#what-every-plugin-shares) | [`ExampleNewPushCmd`](../../cli/providerkit/example_test.go) |
| `github push` / `list` / `prune` | `dotenvctl` | [github › Commands](../plugins/github.md#commands) | [`ExamplePlugin_Command`](../../cli/plugins/githubplugin/example_test.go) |
| `github --repo/-R`, `--environment/--env`, `--org` target | `dotenvctl` | [github › Choosing the target](../plugins/github.md#choosing-the-target) | — |
| `github env-create NAME` (`--yes`) | `dotenvctl` | [github › Creating the environment first](../plugins/github.md#creating-the-environment-first) | — |
| `github prune --keep` remote sync | `dotenvctl` | [github › Keeping remote secrets in sync](../plugins/github.md#keeping-remote-secrets-in-sync) | — |
| `GH_TOKEN` / `GITHUB_TOKEN` auth override | `dotenvctl` | [github › Authentication and token override](../plugins/github.md#authentication-and-token-override) | — |
| `vercel push` / `list` / `prune` | `dotenvctl` | [vercel › Step by step](../plugins/vercel.md#step-by-step) | [`ExamplePlugin_Command`](../../cli/plugins/vercelplugin/example_test.go) |
| `vercel --target` (repeatable) and `push --sensitive` | `dotenvctl` | [vercel › Choosing the target](../plugins/vercel.md#choosing-the-target) | — |
| `VERCEL_TOKEN` auth and the confirm banner | `dotenvctl` | [vercel › The confirm banner](../plugins/vercel.md#the-confirm-banner) | — |

## Go API: cli module

| Feature | Package | Guide | Example |
|---|---|---|---|
| `envkit.Select` / `SelectFile` / `SelectOptions` / `Skip` | `cli/envkit` | [go-api › Selecting keys](../go-api.md#selecting-keys) | [`ExampleSelectFile`](../../cli/envkit/example_test.go), [`ExampleSelect`](../../cli/envkit/example_test.go) |
| `envkit.Discover` / `Inventories` / `EnvNameForPath` | `cli/envkit` | [go-api › Discovering environment files](../go-api.md#discovering-environment-files) | [`ExampleDiscover`](../../cli/envkit/example_test.go), [`ExampleInventories`](../../cli/envkit/example_test.go), [`ExampleEnvNameForPath`](../../cli/envkit/example_test.go) |
| `envkit.BuildMatrix` / `DriftRows` / `SparseColumn` / `StateSymbols` | `cli/envkit` | [go-api › The drift matrix](../go-api.md#the-drift-matrix) | [`ExampleBuildMatrix`](../../cli/envkit/example_test.go), [`ExampleDriftRows`](../../cli/envkit/example_test.go), [`ExampleSparseColumn`](../../cli/envkit/example_test.go) |
| `envkit.DisambiguateColumns` | `cli/envkit` | [go-api › Unique column labels](../go-api.md#unique-column-labels) | [`ExampleDisambiguateColumns`](../../cli/envkit/example_test.go) |
| `envkit.Diff` / `DiffMaps` | `cli/envkit` | [go-api › Diffing two files](../go-api.md#diffing-two-files) | [`ExampleDiff`](../../cli/envkit/example_test.go), [`ExampleDiffMaps`](../../cli/envkit/example_test.go) |
| `envkit.Set` / `Unset` / `Restore` (`EditOptions`, `ErrKeyNotFound`) | `cli/envkit` | [go-api › Editing, byte-preservingly](../go-api.md#editing-byte-preservingly) | [`ExampleSet`](../../cli/envkit/example_test.go), [`ExampleUnset`](../../cli/envkit/example_test.go), [`ExampleRestore`](../../cli/envkit/example_test.go) |
| `envkit.Run` / `ChildEnv` / `MergeEnv` | `cli/envkit` | [go-api › Running a command with the file's values](../go-api.md#running-a-command-with-the-files-values) | [`ExampleRun`](../../cli/envkit/example_test.go), [`ExampleChildEnv`](../../cli/envkit/example_test.go), [`ExampleMergeEnv`](../../cli/envkit/example_test.go) |
| `providerkit.Plugin` / `Deps` / `Runner` / `ExecRunner` | `cli/providerkit` | [go-api › providerkit — plugins](../go-api.md#providerkit--plugins) | [`ExamplePlugin`](../../cli/providerkit/example_test.go), [`ExampleRunner`](../../cli/providerkit/example_test.go) |
| `providerkit.NewPushCmd` / `NewListCmd` / `NewPruneCmd` + `Secret*` interfaces | `cli/providerkit` | [go-api › Step 2 — generate the verbs](../go-api.md#step-2--generate-the-verbs) | [`ExampleNewPushCmd`](../../cli/providerkit/example_test.go), [`ExampleNewListCmd`](../../cli/providerkit/example_test.go), [`ExampleNewPruneCmd`](../../cli/providerkit/example_test.go), [`ExampleSecretWriter`](../../cli/providerkit/example_test.go) |
| `providerkit.VerbConfig` / `Gate` / `Confirm` / `GateInfo` | `cli/providerkit` | [go-api › Step 3 — add a gate and metadata with VerbConfig](../go-api.md#step-3--add-a-gate-and-metadata-with-verbconfig) | [`ExampleVerbConfig`](../../cli/providerkit/example_test.go), [`ExampleConfirm`](../../cli/providerkit/example_test.go) |
| `providerkit.AddSelectionFlags` / `SelectionActive` | `cli/providerkit` | [go-api › Selection flags on your own commands](../go-api.md#selection-flags-on-your-own-commands) | [`ExampleAddSelectionFlags`](../../cli/providerkit/example_test.go) |
| `providerkit.CmdError` / `Fail` / `Action` / `SkipAction` | `cli/providerkit` | [go-api › Failing with the right exit code: Fail and CmdError](../go-api.md#failing-with-the-right-exit-code-fail-and-cmderror) | [`ExampleCmdError`](../../cli/providerkit/example_test.go), [`ExampleFail`](../../cli/providerkit/example_test.go), [`ExampleSkipAction`](../../cli/providerkit/example_test.go) |
| `discover.Scan` / `Classify` / `EnvDefault` | `cli/discover` | [go-api › discover](../go-api.md#discover) | [`ExampleScan`](../../cli/discover/example_test.go), [`ExampleClassify`](../../cli/discover/example_test.go) |
| `report.DiffHTML` / `MatrixHTML` / `MaskGlyph` | `cli/report` | [go-api › report](../go-api.md#report) | [`ExampleMatrixHTML`](../../cli/report/example_test.go), [`ExampleDiffHTML`](../../cli/report/example_test.go) |
| `textdiff.Lines` | `cli/textdiff` | [go-api › textdiff](../go-api.md#textdiff) | [`ExampleLines`](../../cli/textdiff/example_test.go) |
| `outfmt.Printer` / `ErrCode` | `cli/outfmt` | [go-api › outfmt](../go-api.md#outfmt) | [`ExamplePrinter`](../../cli/outfmt/example_test.go), [`ExamplePrinter_Fail`](../../cli/outfmt/example_test.go), [`ExampleErrCode`](../../cli/outfmt/example_test.go) |
| `dotenvcmd.Execute` — embed the whole CLI | `cli/dotenvcmd` | [go-api › Embedding the CLI](../go-api.md#embedding-the-cli) | [`ExampleExecute`](../../cli/dotenvcmd/example_test.go) |
| `githubplugin.New` / `vercelplugin.New` — mount a built-in plugin | `cli/plugins/*` | [go-api › Mounting the github and vercel plugins](../go-api.md#mounting-the-github-and-vercel-plugins) | [`ExampleNew`](../../cli/plugins/githubplugin/example_test.go), [`ExampleNew`](../../cli/plugins/vercelplugin/example_test.go) |
