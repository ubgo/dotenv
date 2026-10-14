# Choosing a sync backend

`dotenvctl` pushes a selection of a `.env` file to a remote store. Two backends are built in; for anything else you write a plugin, in any language or in Go.

**On this page:** [Decision table](#decision-table) · [If X, use Y](#if-x-use-y) · [Exec plugin or providerkit](#exec-plugin-or-providerkit) · [Pitfalls](#pitfalls) · [Related](#related)

## Decision table

| Backend | Stores into | Needs | Targets | Guide | Example |
|---|---|---|---|---|---|
| `dotenvctl github` | GitHub Actions secrets (repository, environment or organisation) | the `gh` CLI, logged in, or `GH_TOKEN` / `GITHUB_TOKEN` | `--repo`, `--environment`, `--org`; `env-create` makes an environment | [GitHub plugin](../plugins/github.md) | [`githubplugin.ExamplePlugin_Command`](../../cli/plugins/githubplugin/example_test.go) |
| `dotenvctl vercel` | Vercel project environment variables | the `vercel` CLI, logged in, or `VERCEL_TOKEN`; a linked directory (`.vercel/project.json`) | `--target production\|preview\|development`, repeatable; default `development` | [Vercel plugin](../plugins/vercel.md) | [`vercelplugin.ExamplePlugin_Command`](../../cli/plugins/vercelplugin/example_test.go) |
| An exec plugin, `dotenvctl-<name>` on `PATH` | anything you can reach | your program, in any language | yours | [writing an exec plugin](../plugins.md#writing-your-own-plugin-exec-plugins) | [the `dotenvctl-export` plugin](../plugins.md#a-complete-exec-plugin-dotenvctl-export) |
| A Go plugin built with `providerkit` | anything you can reach | Go; you mount it in your own binary | yours | [Go API, providerkit](../go-api.md#providerkit--plugins) | [`ExamplePlugin`](../../cli/providerkit/example_test.go), [`ExampleNewPushCmd`](../../cli/providerkit/example_test.go) |

The built-in backends share one set of verbs and flags: `push`, `list`, `prune`, the selection flags (`--prefix`, `--strip-prefix`, `--include`, `--exclude`, `--exclude-prefix`), and the safety flags (`--include-placeholders`, `--include-empty`, `--dry-run`, `--yes`, `--json`) ([plugins overview](../plugins.md#what-every-plugin-shares)). Values travel to `gh` and `vercel` on stdin, never as arguments.

## If X, use Y

- **If CI runs on GitHub Actions**, use `github push`. Keep the CI-only keys under a prefix (`GITHUB_SECRET_`) and push them with `--prefix GITHUB_SECRET_ --strip-prefix` ([recipe](../recipes.md#sync-deploy-secrets-to-github-actions-end-to-end)).
- **If the app is deployed on Vercel**, use `vercel push --target production`, and add `--sensitive` for values nobody should read back in the dashboard ([recipe](../recipes.md#sync-env-vars-to-vercel-end-to-end)).
- **If the remote must match the file exactly**, follow `push` with `prune --yes`: it deletes remote names your selection no longer contains; `--keep NAME` spares a name ([GitHub](../plugins/github.md#keeping-remote-secrets-in-sync), [Vercel](../plugins/vercel.md#keeping-remote-variables-in-sync)).
- **If you use another store** (a cloud secret manager, a team vault, Fly, Render) and want a small tool in any language, write an exec plugin. It calls `dotenvctl` back for names and values, so selection, expansion and the placeholder guard behave exactly as in the built-in verbs.
- **If you ship your own Go binary**, build the backend with `providerkit`: implement the store's capabilities and generate the `push`, `list` and `prune` commands with the same gate and banner the built-ins use.
- **If you only want to see what would change**, add `--dry-run` to any `push` or `prune`.

## Exec plugin or providerkit

| | Exec plugin | `providerkit` plugin |
|---|---|---|
| Language | any | Go |
| Ships as | a separate executable on `PATH` | part of your own binary |
| Gets the selection | by calling `dotenvctl` back (`DOTENVCTL_BIN`) | as Go values, from the shared selection flags |
| Confirm banner, placeholder guard, `--json` envelope | you implement them | generated for you |
| Discovered by `dotenvctl plugins` | yes | no; it is a command in your binary |

## Pitfalls

- **A placeholder value is never pushed by default.** Values matching `^__[A-Z0-9_]+__$` are skipped unless you pass `--include-placeholders`; empty values need `--include-empty`.
- **GitHub secret names are stricter than `.env` keys.** A key with a dot, a hyphen or a leading digit fails the whole push before any write ([secret names](../plugins/github.md#secret-names)).
- **GitHub secrets are write-only.** `github list` shows names, never values.
- **Vercel's default target is `development`.** A push without `--target` never reaches production.
- **The Vercel plugin reads `.vercel/project.json` from the working directory**, not from the `-f` file's directory.
- **With `--json`, a push or prune that fails part-way prints one `ok:false` envelope whose `data` holds what was written.** In cli v0.1.2 and earlier it printed two envelopes.
- **An empty `list` prints a hint from the plugin**: GitHub names `--environment` / `--org`, Vercel names its targets. In cli v0.1.2 and earlier every backend printed the GitHub hint.

## Related

- [Plugins overview](../plugins.md) · [GitHub plugin](../plugins/github.md) · [Vercel plugin](../plugins/vercel.md) · [Go API](../go-api.md)
- [CLI, library or embedding](cli-or-library.md) · [Options for your kind of app](app-setups.md)
- [All choosing guides](README.md) · [Feature index](../reference/features.md)
