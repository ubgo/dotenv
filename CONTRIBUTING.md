# Contributing to dotenv

Thanks for your interest in improving **dotenv**. This guide covers how to get set up, the conventions we follow, and what a good pull request looks like.

By participating, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

## Getting started

```sh
git clone https://github.com/ubgo/dotenv.git
cd dotenv
task test        # library + CLI tests
task ci          # the full gate: fmt-check + vet + race tests, both modules
```

The repo is two Go modules — the stdlib-only library at the root and the CLI under `cli/` — stitched together for local development by `go.work`, so changes to the library are visible to the CLI immediately. Everything runs through the [Taskfile](Taskfile.yml): `task --list` shows every command.

## The two invariants (non-negotiable)

- `parse → render` with no changes ⇒ **byte-identical** output.
- `Set(key, <current value>)` ⇒ **byte-identical** output (a true no-op).

Every byte outside the entry you touch survives verbatim. Both invariants are pinned by tests and fuzz targets — a change that trips them is wrong, not the test. If your change affects round-tripping, run the fuzz properties before opening the PR:

```sh
task fuzz:all                     # every property, 30s each
task fuzz -- FuzzParseRender 60s  # or one property, longer
```

## Ways to contribute

- **Report a bug** — open an issue using the bug template; include steps to reproduce, your OS/version, and what you expected.
- **Request a feature** — open an issue using the feature template; describe the problem first, then your proposed solution. Note the deliberate non-goals in the README (no `os.Environ`, no multi-file merging, no type coercion, no streaming) — PRs adding those will be declined with thanks.
- **Send a pull request** — for anything non-trivial, open an issue first so we can agree on the approach before you write code.

## Coverage, and the six statements that are not covered

```sh
task cover               # both modules
task test:cover          # library
task cli:test:cover      # CLI
task test:uncovered      # every function below 100%
```

Today: **100.00% of the library (611/611 statements), 99.52% of the CLI (1237/1243).**

The library is at 100% and should stay there — it is a parser whose defining promise is byte-exact round-tripping, and an untested branch in it is a byte somebody loses.

The CLI's **six uncovered statements are listed here rather than rounded away**, because a number with no explanation invites the assumption that what is missing does not matter:

| Where | Why it is uncovered |
|---|---|
| `cmd/dotenvctl/main.go:15` | the binary entrypoint. `Execute` exists as a separate function precisely so tests drive the whole CLI in-process with buffers; `main` is the one `os.Exit` line that cannot be reached that way |
| `dotenvcmd/execplugin.go:85` | `filepath.Abs` failing, which requires `os.Getwd` to fail — a deleted or unreadable working directory |
| `dotenvcmd/execplugin.go:89` | `os.Executable` failing. Same shape: the fallback exists so a host that cannot name itself still dispatches |
| `dotenvcmd/execplugin.go:236` | the `isWindows()` arm of the executable-bit check — unreachable on any other platform |
| `dotenvcmd/get.go:52` | expansion returning a non-`RequiredError`. The library can produce one (`expand.go:291`, a `Lookuper` plugin whose lookup fails), but **the CLI never attaches a library plugin**, so its own wiring cannot reach this arm |
| `dotenvcmd/list.go:138` | the same arm on the list path, for the same reason |

The last two were previously written down here as *reachable gaps a fixture plugin would close*. That was wrong, and the correction is worth keeping rather than quietly deleting: the plugin seam exists in the **library**, and `dotenvcmd` constructs every `dotenv.File` through a bare `dotenv.Open` with no plugins attached. No fixture reachable from the CLI can make expansion fail that way. The arms stay because they guard a documented library contract — if the CLI ever does attach a `Lookuper`, deleting them today would turn a plugin failure into a silently wrong value.

The rule the list follows: *unreachable* and *untested* are different words, and a coverage note that blurs them is worse than no note. Anything genuinely reachable gets a test instead of a table row — `root.go:184` was in this table until it turned out to need only a printer whose writes fail, which is entirely testable in-process.

## Branches & commits

- Branch off `main`. Use a short descriptive branch name (`fix/...`, `feat/...`, `docs/...`).
- Keep commits focused; one logical change per commit where practical.

## Pull request checklist

- [ ] The change is scoped and described (link the issue it closes).
- [ ] `task ci` passes.
- [ ] New behavior has a pinning test; round-trip-affecting changes also ran the fuzz targets.
- [ ] The library stays stdlib-only (dependencies belong in the `cli/` module, if anywhere).
- [ ] Library changes ran `task cli:bump` (after the library commit is pushed) so `go install …@main` builds the CLI against the new library — the workspace hides a stale pin locally.
- [ ] Docs/README/CHANGELOG updated if the change is user-facing.
- [ ] No unrelated files or formatting churn.

## Changelog

User-facing changes go under `[Unreleased]` in [CHANGELOG.md](CHANGELOG.md), following [Keep a Changelog](https://keepachangelog.com/).

## Questions

Open a [discussion or issue](https://github.com/ubgo/dotenv/issues). We're happy to help you land your first contribution.
