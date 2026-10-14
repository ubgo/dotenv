# Changelog

All notable changes to **dotenv** are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Release prerequisite: `cli/go.mod` still requires `github.com/ubgo/dotenv v0.1.1`. The library fixes below reach the CLI only through `go.work` in this repository, so a CLI release that ships them needs the library tagged first and `cli/go.mod` bumped to that tag.

### Added

- **`outfmt.Printer` fields `Err` and `Program`, plus `ProgramName`, `DefaultProgram` and `FailWith`.** `Err` is where human-mode failures go (nil means `os.Stderr`), `Program` sets the failure-line prefix (default `dotenvctl`), and `FailWith(code, msg, data)` prints one failure envelope that still carries partial data.
- **`dotenvcmd.WithProgramName`, an option for `dotenvcmd.Execute`.** A program that embeds the CLI passes `Execute(args, stdout, stderr, dotenvcmd.WithProgramName("myapp env"))` to put its own name in failure lines, the `Run '<name> --help'` hint, the help Usage lines and `--version`; without it they still say `dotenvctl`. `Execute` takes the option variadically, so existing calls compile unchanged (`ExampleExecute_programName`).
- **`providerkit.EmptyListHinter` and `providerkit.DefaultEmptyListHint`.** A plugin's lister can supply its own hint for an empty `list`; the GitHub plugin keeps its `--environment` / `--org` hint and the Vercel plugin names its targets.

### Changed

- **`Set` and `NewPair` store a value containing `$` literally.** It is written in single quotes (backticks if the value has a `'`, double quotes only if it has both, where `$` stays a reference), so `GetExpanded` returns what was set, before and after a reload. `Set` can therefore no longer write a reference: `Set("URL", "${HOST}/v1")` writes `URL='${HOST}/v1'`, and so does `dotenvctl set`. To write a reference, `Append` a parsed line (`ExampleFile_Append_verbatim`).
- **Embedders that set only `Printer.Out` now get human-mode failures on stderr.** Set `Printer.Err` to capture them; under `--json` the failure envelope stays on `Out`.
- **A commented `KEY: two words` reads back as a plain comment after a reload.** In a comment the `:` form counts as a disabled setting only when its value is one token, so a value with a space that was `Unset`, saved and read again cannot be found by `Restore`. Within one `File`, `Unset` then `Restore` still works. Quote such values, or use `=`.

- **The library's minimum Go is now 1.22** (was 1.26.4, the toolchain that happened to be installed). The real floor is range-over-int in `parse.go`; nothing in the standard library newer than 1.20 is used. Three benchmarks that used `b.Loop` (Go 1.24) are back to the classic loop. Build and vet were checked with a real go1.22.0 toolchain, and the full suite passes on 1.23.

### Documentation

- **Step-by-step guides for every feature, with runnable examples.** New library guides under `docs/library/` (reading, entries and kinds, syntax, editing, saving, expansion, plugins, typed configuration); the CLI pages (`commands`, `getting-started`, `multi-env`, `recipes`, `plugins`, the GitHub and Vercel plugin pages) and the Go API page were expanded, and the Vercel page now matches the GitHub one. 148 new `Example` functions with checked output back the guides (94 in the library, 54 in the CLI module): `example_*_test.go` at the repository root and an `example_test.go` in each `cli/` package. All 122 features in the new [feature index](docs/reference/features.md) have a guide section, and 109 have a runnable example.
- **A learning path and decision guides.** `docs/README.md` opens with a 12-step learning path and a "start from your goal" table; new `docs/choosing/` pages answer which call, option, plugin hook, tool or sync backend to use, and which options suit which kind of app. The README links them, embeds the architecture diagram, and its code blocks are now tested examples too.
- **Docs gate.** `task docs:check` checks links and anchors, that every Go code block matches a tested `Example` (`task docs:snippets:fix` rewrites drifted blocks), hard-wrapped lines, local paths and banned words, and runs every `Example` in both modules.
- **Corrected claims.** New lines use CRLF as soon as a file contains any CRLF (not the dominant style); a failing `CommandRunner` expands to `""` rather than failing; go-envconfig v1 takes `envconfig.ProcessWith(ctx, &envconfig.Config{Target: &cfg, Lookuper: l})` and stops at its first error. The README's `task dotenv:bench` / `task dotenv:fuzz*` commands are now `task bench` / `task fuzz` / `task fuzz:all` / `task fuzz:list`, and `task fuzz:all` runs all eight properties.

### Deprecated
### Removed
### Fixed

- **A key that refers to itself now sees the line above it, as in Docker Compose.** `A=1` then `A=${A}2` gives `12` (was `22`), and `ZONE=${ZONE:-d}` with no earlier `ZONE` asks the Lookuper and otherwise gives `d` (was `""`, with the Lookuper never asked). The decision follows Compose: compose-go v2.16.1 `dotenv/parser.go` expands each line against the lines read so far; its fixture `dotenv/fixtures/substitutions-default.env` starts with `OPTION_A=${OPTION_A:-1}`, which `TestSubstitutionsWithUnsetVarEnvFileDefaultValuePrecedence` expects to be `1` and `TestSubstitutionsWithEnvFileDefaultValuePrecedence` expects to take the lookup's value.
- **Cycles stop where Compose stops.** A reference back into a variable being expanded sees only the lines above, then the Lookuper, then nothing. `A=a${B}` with `B=b${A}` now gives `ab` for `A` (was `aba`) and `ba` for `B`, as Compose does. A deliberate divergence remains: a reference to a later line resolves here, while Compose leaves it unset, so Compose gives `a` for `A`.
- **A single-quoted value stays literal when another key refers to it.** With `A='${B}'`, `B=x` and `C=${A}`, `C` is now `${B}` (was `x`), matching compose-go's `TestExpanding` case "does not expand variables in single quoted strings".
- **`ExpandedMap` (and so `Read`) expands only the line that wins for each key.** An overridden duplicate holding a failing `${VAR:?…}` no longer fails the whole map. This deliberately differs from Compose, which expands every line and fails on it.
- **`Set` round-trips.** A value containing `$` is no longer written bare, a single-quoted value keeps a quote style that stays literal after the edit, and an empty value or one before a no-space inline comment (`K="v"# note`) is no longer written in a form that absorbs the comment on re-read. `GetExpanded` after `Set` equals `GetExpanded` after re-parsing the rendered file, pinned by `TestSet_RoundTrip` and `FuzzSetRoundTrip`.
- **`Set` keeps the spacing after the delimiter.** `A = 1` set to `2` is now `A = 2` (was `A =2`).
- **No plugin can change the saved bytes.** A `SaveGuard` now receives a copy of the file and an `EntryObserver` a copy of the entry, so a guard that calls `Set` or an observer that edits its entry no longer changes what is parsed or saved. `TestPlugins_NeverAffectBytes` covers both hooks.
- **A commented colon line is read as prose unless its value is one token.** `# Note: rotate monthly`, `# TODO: fix this later` and `# http://example.com` are comments again, no longer disabled settings that `Restore` could turn into live keys; `# KEY: value`, `# KEY: "a b"` and `# DB_URL: postgres://db/app` are still disabled settings.
- **`dotenvctl` prints human-mode failures on stderr.** `VALUE=$(dotenvctl get NOPE)` no longer captures the error text as the value.
- **`--json` prints exactly one envelope per run.** `plugins --json` no longer prints the human table first, a push or prune that fails part-way prints one `ok:false` envelope whose `data` holds the partial results (was two envelopes), and usage errors print a `usage` envelope with exit code 2 (was plain text). `run` and exec plugins still pass through output that is not dotenvctl's.
- **An empty `list` prints the plugin's own hint.** The Vercel plugin no longer prints GitHub's `--environment` / `--org` hint, and an empty store is `"names":[]` under `--json` (was `null`).
- **`--version` reports dotenvctl's own module version when the CLI is embedded** with `dotenvcmd.Execute`, read from the binary's dependency list (replace-aware), instead of the host program's version. `DOTENVCTL_VERSION` for exec plugins follows.
- **A push or prune looks up the account and target once.** `gh api user` (and Vercel's `whoami`) ran twice per run; the metadata lookup now runs only under `--json` and reuses the banner's answer.
- **`matrix --contract` prints its contract lines without blank lines between them.**
- **`TestParse_ScalesLinearly` no longer fails under `-race`.** It used a fixed 5 second budget, which the race detector's slowdown exceeds with the code unchanged. It now compares parse time at two input sizes (4x the input should cost about 4x, quadratic would cost about 16x), a ratio the slowdown cancels out of.

### Security

## [0.1.2] - 2026-10-14

CLI-only release, tagged `cli/v0.1.2`. There is no new library release; the CLI still pins `github.com/ubgo/dotenv v0.1.1`.

### Fixed

- **`dotenvctl github push` stored every secret as the single character `-`.** It ran `gh secret set NAME --body -`, but gh has no "`-` means stdin" convention: a non-empty `--body` is the literal value. The flag is gone; with no `--body` and stdin not a terminal, gh reads the value from stdin and trims only trailing `\r\n`. A new test drives the real runner against a fake `gh` on `PATH` and pins the argv and the exact stdin bytes. `--dry-run` never calls gh, so it could not show the problem. Anyone who pushed with 0.1.0 or 0.1.1 should re-push (#1).

## [0.1.1] - 2026-09-09

No library or CLI behaviour changed in this release. Every Go file outside the test suites is byte-identical to 0.1.0 — it is a licence, documentation and test-coverage release, which is why it is a patch.

### Changed

- **Licence: MIT to Apache-2.0.** Apache-2.0 adds an explicit patent grant and a clearer contribution clause; it does not restrict anything MIT permitted. Copies obtained as 0.1.0 remain under MIT, since a published version's terms cannot be withdrawn.
- README restructured so both deliverables — the library and `dotenvctl` — are visible up front, with an install line and a documentation route for each.
- Documentation reports **measured** coverage with statement counts (100.00% library, 99.52% CLI) rather than rounded claims.

### Fixed

- The coverage note in CONTRIBUTING carried three errors: it claimed seven uncovered statements while listing six, omitted the `os.Executable` fallback entirely, and labelled two arms in `get` and `list` as reachable gaps. They are not reachable — the CLI never attaches a library plugin, and a plugin is the only thing that makes expansion fail with something other than a `RequiredError`. The arms remain in place because they guard a documented library contract.
- Every documented command now shows real captured output rather than illustrative text.

### Added

- Coverage tasks across both modules (`task cover`, `test:cover`, `test:uncovered`) and a pinning test for the root dispatcher's non-quiet `CmdError` arm, verified by breaking the arm it covers.

### Note on 0.1.0

The `v0.1.0` and `cli/v0.1.0` tags were published to the Go module proxy and then deleted from the repository. Deleting a tag does not withdraw it: both versions remain permanently recorded in the checksum database and are still installable. They are therefore **never reused** — this release skips forward to 0.1.1 rather than re-tagging.

<!--
Release process:
  1. Move the relevant [Unreleased] entries under a new version heading below.
  2. Date it: ## [1.2.0] - YYYY-MM-DD
  3. Tag the release (e.g. v1.2.0) and update the link refs at the bottom.
-->

[Unreleased]: https://github.com/ubgo/dotenv/compare/cli/v0.1.2...main
[0.1.2]: https://github.com/ubgo/dotenv/releases/tag/cli/v0.1.2
[0.1.1]: https://github.com/ubgo/dotenv/releases/tag/v0.1.1
