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

## Branches & commits

- Branch off `main`. Use a short descriptive branch name (`fix/...`, `feat/...`, `docs/...`).
- Keep commits focused; one logical change per commit where practical.

## Pull request checklist

- [ ] The change is scoped and described (link the issue it closes).
- [ ] `task ci` passes.
- [ ] New behavior has a pinning test; round-trip-affecting changes also ran the fuzz targets.
- [ ] The library stays stdlib-only (dependencies belong in the `cli/` module, if anywhere).
- [ ] Docs/README/CHANGELOG updated if the change is user-facing.
- [ ] No unrelated files or formatting churn.

## Changelog

User-facing changes go under `[Unreleased]` in [CHANGELOG.md](CHANGELOG.md), following [Keep a Changelog](https://keepachangelog.com/).

## Questions

Open a [discussion or issue](https://github.com/ubgo/dotenv/issues). We're happy to help you land your first contribution.
