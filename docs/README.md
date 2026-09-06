# dotenvctl documentation

`dotenvctl` is the command-line companion to the [`github.com/ubgo/dotenv`](../README.md) library: every edit is comment-preserving and byte-exact — comments, blank lines, ordering, and quoting style survive anything you do — and the tool never modifies its own process environment.

## Install

```sh
go install github.com/ubgo/dotenv/cli/cmd/dotenvctl@main
```

Or build from a clone: `task cli:build` produces `cli/bin/dotenvctl`.

## Guides

| Guide | Covers |
|---|---|
| [Getting started](getting-started.md) | install, first commands, the `-f` flag, `--json`, exit codes |
| [Command reference](commands.md) | **every verb** — `get` · `set` · `unset` · `restore` · `list` · `keys` · `run` · `diff` · `envs` · `matrix` · `github` · `vercel` · `plugins` · `completion` — every flag, with real output |
| [Multi-environment tools](multi-env.md) | `envs` discovery, the `matrix` drift table, `diff`, HTML reports, secret masking |
| [Recipes](recipes.md) | complete workflows: CI contract gates, staging→prod cloning, the monorepo prefix split, GitHub sync end to end, jq cookbook |
| [Plugins overview](plugins.md) | how plugin namespaces work, shared flags, safety rules |
| [Go API](go-api.md) | **importing dotenvctl as a library** — envkit operations, providerkit plugins, helper packages |
| [GitHub plugin](plugins/github.md) | pushing env values to GitHub Actions secrets via `gh` |

## Design promises (hold everywhere)

- **Byte preservation.** Mutating commands change only the entries you name; re-setting a key to its current value is a true no-op — the file is not even rewritten, so watchers see no phantom mtime change.
- **Your process environment is never touched.** `run` builds the child's environment; nothing else goes near `os.Environ`.
- **Secrets stay out of output.** Values are masked (`••••••`) in shareable surfaces (matrix `--values`, JSON, HTML) unless you explicitly pass `--reveal`, and plugin output prints names and actions, never values.
- **Stable machine interface.** `--json` emits one `{"ok":true,"data":…}` / `{"ok":false,"error":{"code","message"}}` object per invocation; exit codes are `0` success, `1` operation failed, `2` usage error (`run` passes the child's exit code through verbatim; `diff` and `matrix --contract` use `1` to signal differences/gaps, like `diff(1)`).
