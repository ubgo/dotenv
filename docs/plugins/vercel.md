# Vercel plugin

Push values from a `.env` file to **Vercel environment variables**, list what is there, and prune what your file no longer defines, through the [`vercel` CLI](https://vercel.com/docs/cli).

On this page: [Before you start](#before-you-start) · [Commands](#commands) · [Vercel-specific flags](#vercel-specific-flags) · [Choosing the target](#choosing-the-target) · [The confirm banner](#the-confirm-banner) · [Authentication](#authentication) · [Step by step](#step-by-step) · [Keeping remote variables in sync](#keeping-remote-variables-in-sync) · [CI usage](#ci-usage) · [Failure behavior](#failure-behavior) · [Pitfalls](#pitfalls) · [A note on `list`/`prune` parsing](#a-note-on-listprune-parsing) · [Related](#related)

Use it when one `.env` file is the source of truth for a Vercel project and you want the dashboard to follow it, without pasting values into a browser and without putting them on a command line. The plugin owns selection, the safety guards and the confirm gate; the `vercel` CLI owns authentication, project linking and the actual API calls.

## Before you start

- `dotenvctl` installed ([Getting started](../getting-started.md#install)).
- The [`vercel` CLI](https://vercel.com/docs/cli) on your PATH, logged in (`vercel login`), or a token in `VERCEL_TOKEN` ([Authentication](#authentication)).
- The directory you run from linked to a project (`vercel link`, which writes `.vercel/project.json`). The plugin only reads that file to show you the project; the `vercel` CLI decides what an unlinked run does.

## Commands

```sh
dotenvctl vercel push [KEY...]     # upsert env vars (vercel env add --force, value via stdin)
dotenvctl vercel list              # remote variable names for the selected target(s)
dotenvctl vercel prune --yes       # delete remote names absent from your keep-set (skipped keys and --keep names are spared)
```

Under the hood each verb is a fixed `vercel` call per target, with the value on stdin and never in the argument list:

| Verb | `vercel` call (once per target) |
|---|---|
| `push` | `vercel env add NAME TARGET --force` (plus `--sensitive` when asked), value on stdin |
| `list` | `vercel env ls TARGET` |
| `prune` | `vercel env ls TARGET`, then `vercel env rm NAME TARGET --yes` per stale name |
| every mutating verb | `vercel whoami` first, for the banner |

All [shared selection and safety flags](../commands.md#selecting-keys--the-shared-grammar) apply to `push` and `prune`: `--prefix`, `--exclude-prefix`, `--strip-prefix`, `--include`, `--exclude` (both multi-value: repeat or comma-separate), `--expand` (default on), `--include-placeholders`/`--include-empty`, `--dry-run`, `--yes`, `--json`. `prune` also takes `--keep NAME`.

## Vercel-specific flags

| Flag | Verbs | Meaning |
|---|---|---|
| `--target production\|preview\|development` | `push`, `list`, `prune` | which Vercel environment(s) to act on — multi-value (`--target production --target preview` or `--target production,preview`), default [`development`](ds:block?id=verceldefaulttarget-w4c9y8nf) (the least dangerous place for an accident) |
| `--sensitive` | `push` only | store as a sensitive variable: encrypted, unreadable in the dashboard |

```sh
dotenvctl vercel push --target production --target preview DB_URL --sensitive --yes
```

## Choosing the target

Two things decide where values land, and the banner prints both before anything is written:

| What | Comes from | Banner line |
|---|---|---|
| Project | [`.vercel/project.json`](ds:block?id=vercelresolveproject-6w9rnap9) in the **current directory** (its `projectId`) | `project: prj_abc123 (from .vercel/project.json)` |
| Project, when that file is missing or unreadable | nothing is guessed; the `vercel` CLI resolves or prompts | `project: unlinked directory (vercel CLI will resolve or prompt)` |
| Environment(s) | `--target`, default `development` | `targets: production, preview` |
| Acting account | `vercel whoami`, labeled with the auth source | `account: you (stored vercel auth)` or `account: you (via VERCEL_TOKEN)` |

With several targets, `push` writes each key to every target in turn, `list` shows the union of names across them, and `prune` deletes each stale name from every selected target.

## The confirm banner

Every mutating verb prints where and as whom before writing, then prompts (or requires `--yes` non-interactively):

```
project: prj_abc123 (from .vercel/project.json)
account: you (stored vercel auth)
targets: production, preview
keys   : 4
proceed? [y/N]
```

`keys` counts the variables that would be written; guard-skipped keys are not counted. The prompt goes to stderr and accepts `y` or `yes`; anything else aborts. `--dry-run` prints the banner and the would-push list without prompting or writing. Runs with no terminal on stdin (CI, pipes) and every `--json` run refuse to write without `--yes`.

## Authentication

The plugin inherits the vercel CLI's auth; set **`VERCEL_TOKEN`** to act with a different identity, and the banner labels it — `account: you (via VERCEL_TOKEN)` — so a token exported yesterday cannot act invisibly today.

```sh
VERCEL_TOKEN="$(cat ~/.secrets/vercel-ci-token)" dotenvctl vercel push --target preview --dry-run
```

There is deliberately no `--token` flag (argv is visible in `ps` and shell history), and values reach the CLI via stdin, never as arguments.

## Step by step

The runs below were captured with a stand-in `vercel` on PATH that answers `whoami` with `you` and lists three remote variables (`API_KEY`, `DB_URL`, `OLD_FLAG`), so no account was touched. Every line shown is exactly what `dotenvctl` printed. The `.env`:

```env
DB_URL=postgres://${DB_HOST}/app
DB_HOST=db.internal
API_KEY=__YOU__
VERCEL_ANALYTICS_ID=abc123
EMPTY=
```

**Step 1 — preview.** Always start with `--dry-run`: it resolves the selection, prints the banner and the per-key actions, and writes nothing.

```console
$ dotenvctl vercel push --dry-run
project: unlinked directory (vercel CLI will resolve or prompt)
account: you (stored vercel auth)
targets: development
keys   : 3
API_KEY: skipped-placeholder
EMPTY: skipped-empty
DB_URL: would-push
DB_HOST: would-push
VERCEL_ANALYTICS_ID: would-push
```

The placeholder and the empty value are skipped by the default guards, and `DB_URL` will be pushed expanded (`postgres://db.internal/app`), because `--expand` is on for pushes.

**Step 2 — push to the environments you mean.** Name them explicitly; the default is `development`.

```console
$ dotenvctl vercel push --target production,preview --sensitive --yes
project: unlinked directory (vercel CLI will resolve or prompt)
account: you (stored vercel auth)
targets: production, preview
keys   : 3
API_KEY: skipped-placeholder
EMPTY: skipped-empty
DB_URL: pushed
DB_HOST: pushed
VERCEL_ANALYTICS_ID: pushed
```

**Step 3 — push only one audience.** If the file mixes app config and Vercel-only values, mark the Vercel ones with a prefix and strip it on the way out:

```console
$ dotenvctl vercel push --prefix VERCEL_ --strip-prefix --target preview --dry-run
project: unlinked directory (vercel CLI will resolve or prompt)
account: you (stored vercel auth)
targets: preview
keys   : 1
ANALYTICS_ID: would-push
```

## Keeping remote variables in sync

```console
$ dotenvctl vercel list --target production
API_KEY
DB_URL
OLD_FLAG
$ dotenvctl vercel prune --target production --dry-run
project: unlinked directory (vercel CLI will resolve or prompt)
account: you (stored vercel auth)
targets: production
keys   : 1
API_KEY: kept-skipped
OLD_FLAG: would-delete
$ dotenvctl vercel prune --target production --keep OLD_FLAG --dry-run
nothing to prune
API_KEY: kept-skipped
OLD_FLAG: kept-keep-flag
```

An empty target says which target it read, since each one is listed separately:

```console
$ dotenvctl vercel list --target preview
no environment variables found for target(s) preview (each target is listed separately — try another --target: production, preview or development)
```

`prune` computes *remote names minus the local keep-set* and deletes only those. The keep-set is wider than what `push` would push, on purpose: `API_KEY` is `__YOU__` locally, so push skips it and prune spares the real remote value (`kept-skipped`); `--keep NAME` (repeat or comma-separate) spares variables that another tool manages (`kept-keep-flag`). `DB_URL` is in the selection, so it is kept silently. Every other spare is reported with its reason.

## CI usage

```yaml
- name: Install the vercel CLI
  run: npm install --global vercel

- name: Sync Vercel env vars from the env file
  run: |
    dotenvctl -f .env.prod vercel push \
      --prefix VERCEL_ --strip-prefix \
      --target production --yes --json
  env:
    VERCEL_TOKEN: ${{ secrets.VERCEL_TOKEN }}
```

CI has no terminal, so `--yes` is required; without it the run stops before writing with `refusing to write without --yes in non-interactive mode` and exit `2`. `--json` emits one envelope with the resolved context and per-key actions, names only:

```console
$ dotenvctl vercel push --target production --yes --json
{"ok":true,"data":{"meta":{"account":"you (stored vercel auth)","project":"unlinked directory (vercel CLI will resolve or prompt)","targets":"production"},"results":[{"name":"API_KEY","action":"skipped-placeholder"},{"name":"EMPTY","action":"skipped-empty"},{"name":"DB_URL","action":"pushed"},{"name":"DB_HOST","action":"pushed"},{"name":"VERCEL_ANALYTICS_ID","action":"pushed"}],"dry_run":false}}
```

## Failure behavior

Every failure exits non-zero and names what failed. Nothing is written before the banner resolves.

- **`vercel` missing or not logged in.** The account lookup (`vercel whoami`) fails first, so nothing runs:

  ```console
  $ dotenvctl vercel push --yes; echo "exit=$?"
  dotenvctl: resolve account: vercel [whoami]: exec: "vercel": executable file not found in $PATH:  (is the vercel CLI installed and logged in? https://vercel.com/docs/cli)
  exit=1
  ```

- **No `--yes` without a terminal.** Exit `2` after the banner, before any write (shown under [CI usage](#ci-usage)).
- **A required variable is unset.** A failing `${VAR:?msg}` stops the run before the `vercel` CLI is called at all:

  ```console
  $ dotenvctl -f req.env vercel push --dry-run; echo "exit=$?"
  dotenvctl: envkit: expand: dotenv: required variable NEED is not set or is empty: set NEED
  exit=1
  ```

- **A push failing mid-way.** Everything already pushed is reported as `pushed`, then the failing key and target are named and the command exits `1`. Here the stand-in `vercel` refused `DB_HOST`:

  ```console
  $ dotenvctl vercel push --target production --yes; echo "exit=$?"
  project: unlinked directory (vercel CLI will resolve or prompt)
  account: you (stored vercel auth)
  targets: production
  keys   : 3
  API_KEY: skipped-placeholder
  EMPTY: skipped-empty
  DB_URL: pushed
  dotenvctl: push DB_HOST: target production: vercel [env add DB_HOST production --force]: exit status 1: Error: The Environment Variable already exists (simulated)
  exit=1
  ```

  Re-running completes the rest: every push is `vercel env add --force`, an upsert. `VERCEL_ANALYTICS_ID` was never attempted, and is not listed. The per-key lines are on stdout and the `dotenvctl:` line is on stderr. With `--json` the same run prints one `ok:false` envelope whose `data` is the partial record:

  ```console
  $ dotenvctl vercel push --target production --yes --json; echo "exit=$?"
  {"ok":false,"data":{"meta":{"account":"you (stored vercel auth)","project":"unlinked directory (vercel CLI will resolve or prompt)","targets":"production"},"results":[{"name":"API_KEY","action":"skipped-placeholder"},{"name":"EMPTY","action":"skipped-empty"},{"name":"DB_URL","action":"pushed"}],"dry_run":false},"error":{"code":"exec","message":"push DB_HOST: target production: vercel [env add DB_HOST production --force]: exit status 1: Error: The Environment Variable already exists (simulated)"}}
  exit=1
  ```
- **`list` failing.** If `vercel env ls` fails, `list` and `prune` fail too, rather than treating the store as empty (an empty answer would make prune mark every remote name stale).

## Pitfalls

- **`--sensitive` is a `push` flag only.** `vercel list --sensitive` and `vercel prune --sensitive` are usage errors (`unknown flag: --sensitive`, exit `2`).
- **`--sensitive` goes to every target you name, including the default.** `push --sensitive` without `--target` sends `vercel env add NAME development --force --sensitive`. The plugin passes the flag through unchanged and Vercel decides whether a target accepts sensitive variables; a refusal stops the push with the target named.
- **The plugin does not remember how a variable was created.** Each push sends exactly the flags you give it this time. Keep `--sensitive` in the script or Taskfile task that pushes, rather than typing it by hand.
- **Sensitive values cannot be read back from Vercel.** That is the point, so your `.env` (or the secret store it comes from) must stay the source of truth. `dotenvctl` never reads values from Vercel in any mode.
- **With several targets, one key can be half-written.** `push` writes a key to each target in order. If the second target fails, the first already has the new value, and the key is not reported as `pushed`. Re-run after fixing the cause.
- **The default target is `development`.** A forgotten `--target production` writes to development, not production. That is deliberate, but check the `targets:` line in the banner.
- **The project line comes from the current directory, not from `-f`.** Run `dotenvctl -f app/.env vercel push` from the repo root and the banner reads the root's `.vercel/project.json` (or says `unlinked directory`), even if `app/` is linked. `cd app` first, so the banner and the `vercel` CLI agree.
- **Names are not checked locally.** Unlike the [GitHub plugin](github.md#secret-names), this plugin does not validate key names before writing. A name Vercel rejects fails at that key, after the earlier keys were pushed.
- **A mid-way failure under `--json` is one `ok:false` envelope that still has `data`.** Read `.data.results` for what was written and `.error` for what stopped the run. Before cli v0.1.3 it printed two envelopes, the partial `{"ok":true,…}` record and then the error.
- **Values are expanded before pushing.** `--expand` is on by default, so `DB_URL=postgres://${DB_HOST}/app` arrives in Vercel resolved. Pass `--expand=false` only if you really want the literal `${…}` stored.

## A note on `list`/`prune` parsing

The vercel CLI has no stable machine-readable output for `vercel env ls`, so names are parsed from its table with a conservative heuristic: the first column of each row, skipping rows that start with `name`, `Vercel`, `>`, `Error` or `WARN`. A CLI format change would surface loudly as an odd listing — never as silent misdata for `push`, which doesn't consult the listing at all. Run `prune --dry-run` before every real prune. A direct-API mode is the planned upgrade path.

## Related

- [Plugins overview](../plugins.md) — the shared flags, guards and confirm gate every plugin uses
- [GitHub plugin](github.md) — the same verbs for GitHub Actions secrets
- [Selecting keys](../commands.md#selecting-keys--the-shared-grammar) — the selection grammar in full
- [Recipes](../recipes.md) — complete workflows
- [Go API guide](../go-api.md) — `vercelplugin.New()` and `providerkit`, to mount the plugin in your own tool
- [Feature index](../reference/features.md)
- Tests that pin this behavior: [`vercel_test.go`](../../cli/plugins/vercelplugin/vercel_test.go)
