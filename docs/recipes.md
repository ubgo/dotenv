# Recipes

Complete workflows, end to end. Each one is copy-paste-able; output shown is real. Flags are documented flag-by-flag in the [command reference](commands.md) and [multi-env tools](multi-env.md) — this page is about composing them.

On this page: [CI gate](#ci-gate-the-build-on-env-completeness) · [Derive `.env.prod`](#derive-envprod-from-envstaging) · [One file, several audiences](#one-file-several-audiences-the-monorepo-split) · [Sync to GitHub Actions](#sync-deploy-secrets-to-github-actions-end-to-end) · [Sync to Vercel](#sync-env-vars-to-vercel-end-to-end) · [Run against an env file](#run-testsservices-against-a-specific-env-file) · [Drift report](#a-weekly-drift-report-for-the-team) · [Load a file into your shell](#load-a-file-into-your-shell) · [jq cookbook](#jq-cookbook) · [Safe edits](#safe-edits-in-automation) · [Related](#related)

## CI: gate the build on env completeness

The problem: someone adds `SENTRY_DSN` to the code and to `.env.example`, and three weeks later staging crashes because nobody added it to `.env.staging`. Make `.env.example` a **contract** and let CI enforce it:

```yaml
# .github/workflows/ci.yml
- name: env files satisfy the contract
  run: dotenvctl matrix --contract .env.example --json > /dev/null
```

`--contract` exits `1` when any environment misses a contract key, with the gaps named. With the family from [multi-env tools](multi-env.md) (`.env`, `.env.staging`, `.env.prod`, `.env.example`):

```console
$ dotenvctl matrix --contract .env.example; echo "exit=$?"
KEY         default  stag  prod
API_URL     ✓        —     —
DB_HOST     ✓        ✓     ✓
DB_PASS     —        ✓     !
DB_PORT     ✓        —     —
DOMAIN      ✓        ✓     ✓
FEATURE_X   —        ✓     #
SENTRY_DSN  —        —     ✓

✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing

contract: default is missing 2 key(s): [DB_PASS SENTRY_DSN]

contract: stag is missing 1 key(s): [SENTRY_DSN]
exit=1
```

If your local `.env` is not meant to satisfy the contract, name the deployed files instead: `dotenvctl matrix .env.staging .env.prod --contract .env.example`.

Add a second gate for placeholders — a `__YOU__` that reaches prod is a config value that was never filled in. The contract gate does not catch it (a placeholder counts as present), and `matrix` needs two files, so read the PLACEHOLDERS column of `envs`:

```yaml
- name: no placeholder values left
  run: dotenvctl envs | awk 'NR > 1 && $NF > 0 { print "placeholders left:", $0; bad = 1 } END { exit bad }'
```

```console
$ dotenvctl envs | awk 'NR > 1 && $NF > 0 { print "placeholders left:", $0; bad = 1 } END { exit bad }'; echo "exit=$?"
placeholders left: prod                .env.prod     4     1         0          1
exit=1
```

Do not gate with `matrix … | grep '!'`: the legend line under every matrix contains `!`, so that check always fires, and `--values` masks the placeholder marker away.

## Derive `.env.prod` from `.env.staging`

Copy the file, change only what differs — every comment, banner, and `${reference}` survives, so derived values (`API_URL=https://api.${DOMAIN}`) keep tracking automatically:

```sh
cp .env.staging .env.prod
dotenvctl -f .env.prod set DOMAIN=acme.io DB_HOST=prod.db.internal
dotenvctl -f .env.prod set DB_PASS=__YOU__          # placeholder: visible in matrix as '!', never pushed by plugins
dotenvctl diff .env.staging .env.prod               # review exactly what now differs
```

```
~ DB_HOST: stag.db.internal -> prod.db.internal
~ DB_PASS: stag-pass -> __YOU__
~ DOMAIN: staging.acme.io -> acme.io
```

Three changed lines, nothing else — that *is* the whole review. Do the same from Go with `Clone` + `SaveAs` ([library docs](../README.md#cloning-between-environments)).

## One file, several audiences (the monorepo split)

A single `.env.prod` holds app config **and** deploy credentials, separated by a prefix your repo chose:

```env
DB_HOST=db.internal
JWT_SECRET=...
GITHUB_SECRET_GHCR_PAT=ghp_...
GITHUB_SECRET_DEPLOY_PATH=/srv/app
```

Read each audience without hand-rolled line parsing (which breaks on multiline values and inline comments — silently):

```sh
# deploy view: prefixed keys, renamed, machine-readable
$ dotenvctl -f .env.prod list --prefix GITHUB_SECRET_ --strip-prefix --json
{"ok":true,"data":{"pairs":[{"key":"GHCR_PAT","value":"ghp_..."},{"key":"DEPLOY_PATH","value":"/srv/app"}],"disabled":[],"inherited":[],"expanded":false}}

# app view: everything EXCEPT the deploy keys
dotenvctl -f .env.prod list --exclude-prefix GITHUB_SECRET_

# names only, for scripting
dotenvctl -f .env.prod keys --prefix GITHUB_SECRET_ --strip-prefix
```

The same selection flags drive the plugins, so previewing with `list` and pushing with `github push` can never disagree about which keys match.

## Sync deploy secrets to GitHub Actions, end to end

One-time setup, then one command per push. Requires an authenticated [`gh`](https://cli.github.com) — the plugin resolves the target repo from the cwd's git remote and shows you both before writing:

```sh
dotenvctl github env-create prod --yes                                        # idempotent
dotenvctl -f .env.prod github push --prefix GITHUB_SECRET_ --strip-prefix --env prod --dry-run   # preview first
dotenvctl -f .env.prod github push --prefix GITHUB_SECRET_ --strip-prefix --env prod --yes
```

```
target : yourorg/yourrepo (from cwd git remote)
account: you (stored gh auth)
keys   : 3
VPS_HOST: skipped-placeholder
GHCR_PAT: pushed
DEPLOY_PATH: pushed
```

Note what did *not* happen: `DB_HOST` was never considered (no prefix), and the `__YOU__` placeholder was skipped with a warning instead of becoming a production secret. Then keep the remote side from accumulating dead names:

```sh
dotenvctl -f .env.prod github prune --prefix GITHUB_SECRET_ --strip-prefix --env prod --dry-run
dotenvctl -f .env.prod github prune --prefix GITHUB_SECRET_ --strip-prefix --env prod --keep ENV_B64 --yes
```

`prune` deletes remote names absent from your local keep-set — and the keep-set is deliberately wider than what push pushes: guard-skipped keys are kept, `--keep` spares out-of-band names, every spare is reported with its reason. Full semantics: [GitHub plugin](plugins/github.md).

## Sync env vars to Vercel, end to end

Requires an authenticated [`vercel`](https://vercel.com/docs/cli) CLI and a linked directory (`vercel link`). Mark the Vercel-bound keys with a prefix, preview, then push to the environments you mean (the default is `development`):

```sh
cd apps/web                                                                          # the linked directory: the banner reads .vercel/project.json here
dotenvctl -f .env.prod vercel push --prefix VERCEL_ --strip-prefix --target production --dry-run
dotenvctl -f .env.prod vercel push --prefix VERCEL_ --strip-prefix --target production --sensitive --yes
dotenvctl -f .env.prod vercel prune --prefix VERCEL_ --strip-prefix --target production --dry-run
```

The banner names the project, the acting account and the targets before anything is written; placeholders and empty values are skipped and, on prune, spared. Real output for every step, plus the `--sensitive` pitfalls: [Vercel plugin](plugins/vercel.md#step-by-step).

## Run tests/services against a specific env file

```sh
dotenvctl -f .env.test run -- go test ./...
dotenvctl run -- npm start
```

The child's environment is your shell's overlaid with the file's values (file wins); your own shell is never modified; the child's exit code passes through verbatim, so CI wrappers behave transparently. A failing `${VAR:?msg}` aborts before the child starts — a required variable fails the run instead of starting the service half-configured.

## A weekly drift report for the team

```sh
dotenvctl matrix --only-drift --format html -o reports/env-drift.html
```

Self-contained HTML, no JavaScript, light/dark via OS preference — and masked **structurally**: without `--reveal`, secret values are never placed in the document at all, so the report is safe to attach to a ticket or publish on an internal wiki. With `--reveal` it embeds real values and stamps a red "CONTAINS SECRETS" banner.

## Load a file into your shell

`dotenvctl run` covers most cases, because the child gets the values and your shell stays clean. When you really want the values in the current shell (an interactive debugging session, say), install the [`dotenvctl-export` exec plugin](plugins.md#a-complete-exec-plugin-dotenvctl-export) and `eval` its output:

```console
$ eval "$(dotenvctl export --prefix DB_URL)"; echo "$DB_URL"
postgres://db.internal/app
```

Values arrive expanded, placeholders are skipped with a note on stderr, and a failing `${VAR:?msg}` stops the plugin with exit `1`. Lines already printed for earlier keys still reach `eval`, so in scripts capture the output first and `eval` it only when the plugin succeeded.

## jq cookbook

Every command emits one `{"ok":…}` envelope under `--json`; error `code` values are stable API (`not_found` · `io` · `required` · `usage` · `exec`):

```sh
dotenvctl get DB_HOST --json | jq -r .data.value
dotenvctl keys --json | jq -r '.data.keys[]'
dotenvctl list --expand --json | jq -r '.data.pairs[] | "\(.key)=\(.value)"'
dotenvctl diff .env.staging .env.prod --json | jq -r '.data.removed[].key'    # keys prod is missing
dotenvctl envs --json | jq -r '.data.files[].file'
dotenvctl matrix --json | jq '.data'                                          # cell states only — values require --reveal
```

## Safe edits in automation

Patterns that make unattended edits trustworthy:

```sh
dotenvctl set DEBUG=true --dry-run          # shows the exact -old/+new line diff, writes nothing
dotenvctl set DB_PASSWORD=x --after DB_USER # a MISSING anchor exits 1 without writing — no silent append
dotenvctl unset OLD_KEY                     # comment-out, not delete: reversible, diff-friendly
dotenvctl restore OLD_KEY                   # byte-identical undo
```

Two properties you get for free: re-setting a key to its current value is a true no-op (same bytes, same mtime — file watchers see nothing), and a `set` on a missing file creates it `0600`, because env files hold credentials.

One trap when scripting reads: a missing key and an empty value both leave the variable empty. A failing verb prints its `dotenvctl: …` message on stderr, so capture the value and check the exit code together:

```sh
if ! DB_HOST=$(dotenvctl -f .env.prod get DB_HOST); then
  exit 1                   # the reason is already on stderr
fi
```

Before cli v0.1.3, human-mode failures went to stdout, so the error text ended up in the captured variable.

## Related

- [Command reference](commands.md) and [multi-env tools](multi-env.md) — every flag used above
- [Plugins](plugins.md), [GitHub plugin](plugins/github.md), [Vercel plugin](plugins/vercel.md)
- [Getting started](getting-started.md) — install and the first commands
- Runnable examples of every verb: [`cli/dotenvcmd/example_test.go`](../cli/dotenvcmd/example_test.go)
- [Feature index](reference/features.md)
