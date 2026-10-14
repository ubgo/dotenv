# Changelog

All notable changes to **dotenv** are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
### Changed

- **The library's minimum Go is now 1.22** (was 1.26.4, the toolchain that happened to be installed). The real floor is range-over-int in `parse.go`; nothing in the standard library newer than 1.20 is used. Three benchmarks that used `b.Loop` (Go 1.24) are back to the classic loop. Build and vet were checked with a real go1.22.0 toolchain, and the full suite passes on 1.23.

### Documentation

- **Step-by-step guides for every feature, with runnable examples.** New library guides under `docs/library/` (reading, entries and kinds, syntax, editing, saving, expansion, plugins, typed configuration); the CLI pages (`commands`, `getting-started`, `multi-env`, `recipes`, `plugins`, the GitHub and Vercel plugin pages) and the Go API page were expanded, and the Vercel page now matches the GitHub one. 148 new `Example` functions with checked output back the guides (94 in the library, 54 in the CLI module): `example_*_test.go` at the repository root and an `example_test.go` in each `cli/` package. All 122 features in the new [feature index](docs/reference/features.md) have a guide section, and 109 have a runnable example.
- **A learning path and decision guides.** `docs/README.md` opens with a 12-step learning path and a "start from your goal" table; new `docs/choosing/` pages answer which call, option, plugin hook, tool or sync backend to use, and which options suit which kind of app. The README links them, embeds the architecture diagram, and its code blocks are now tested examples too.
- **Docs gate.** `task docs:check` checks links and anchors, that every Go code block matches a tested `Example` (`task docs:snippets:fix` rewrites drifted blocks), hard-wrapped lines, local paths and banned words, and runs every `Example` in both modules.
- **Corrected claims.** New lines use CRLF as soon as a file contains any CRLF (not the dominant style); `Set` keeps the spacing before the delimiter but not after it; a failing `CommandRunner` expands to `""` rather than failing; single quotes stop expansion only when that key is read directly; a cycle's re-entering reference resolves to `""` while the rest of the value stays; go-envconfig v1 takes `envconfig.ProcessWith(ctx, &envconfig.Config{Target: &cfg, Lookuper: l})` and stops at its first error. The README's `task dotenv:bench` / `task dotenv:fuzz*` commands are now `task bench` / `task fuzz` / `task fuzz:all` / `task fuzz:list`, and `task fuzz:all` runs all eight properties.

### Deprecated
### Removed
### Fixed

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
