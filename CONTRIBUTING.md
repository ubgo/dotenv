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

## Coverage, and the seven statements that are not covered

```sh
task test:cover          # library
task cli:test:cover      # CLI
task test:uncovered      # every function below 100%
```

Today: **100.0% of the library, 99.4% of the CLI.** The badge rounds the second to 99%.

The library is at 100% and should stay there — it is a parser whose defining promise is byte-exact round-tripping, and an untested branch in it is a byte somebody loses.

The CLI's **seven uncovered statements are listed here rather than rounded away**, because a number with no explanation invites the assumption that what is missing does not matter. Two of these are real gaps, and saying so is the point of the list:

| Where | Why it is uncovered |
|---|---|
| `cmd/dotenvctl/main.go:15` | the binary entrypoint. `Execute` exists as a separate function precisely so tests drive the CLI without a process; `main` is the four lines that cannot be reached that way |
| `dotenvcmd/execplugin.go:85` | `filepath.Abs` failing, which needs `os.Getwd` to fail |
| `dotenvcmd/execplugin.go:236` | the `isWindows()` arm of the executable-bit check — unreachable on any other platform |
| `dotenvcmd/root.go:184` | a plugin failing with `providerkit.CmdError`; needs a real exec-plugin binary that exits non-zero |
| **`dotenvcmd/get.go:52`** | **a real gap.** Reachable, not defensive — a `Lookuper` plugin whose lookup fails makes expansion return a non-`RequiredError` (`expand.go:291`) |
| **`dotenvcmd/list.go:138`** | **the same gap**, on the list path |

The last two would be closed by a fixture plugin that fails a lookup. They are written down as *untested* rather than *unreachable* because they are not the same thing, and the difference is exactly what a coverage note is for.

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
