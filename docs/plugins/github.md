# GitHub plugin

Pushes env values to **GitHub Actions secrets** through the [`gh` CLI](https://cli.github.com), which supplies authentication and secret encryption. Requires `gh` on your PATH, logged in (`gh auth login`) or overridden by a token (below).

## Commands

```sh
dotenvctl github push [KEY...]      # upsert secrets
dotenvctl github list               # remote secret names (GitHub secrets are write-only)
dotenvctl github prune --yes        # delete remote secrets absent from your local selection
dotenvctl github env-create NAME    # create a deployment environment, idempotently
```

All [shared selection and safety flags](../commands.md#selecting-keys--the-shared-grammar) apply to `push` and `prune`: `--prefix`, `--exclude-prefix`, `--strip-prefix`, `--include`, `--exclude` (both multi-value: repeat or comma-separate), `--expand` (default on), `--include-placeholders`/`--include-empty`, `--dry-run`, `--yes`, `--json`.

```sh
# push the prefixed keys except two act-only specials — both spellings equivalent
dotenvctl github push --prefix GITHUB_SECRET_ --strip-prefix   --exclude GITHUB_SECRET_GITHUB_TOKEN,GITHUB_SECRET_GITHUB_ACTOR --env prod --yes
dotenvctl github push --prefix GITHUB_SECRET_ --strip-prefix   --exclude GITHUB_SECRET_GITHUB_TOKEN --exclude GITHUB_SECRET_GITHUB_ACTOR --env prod --yes
```

## Choosing the target

| Flag | Target |
|---|---|
| *(none)* | the repository of your current directory's git remote, exactly as `gh` resolves it |
| `--repo owner/name` | that repository |
| `--environment production` (alias `--env`) | a deployment environment's secrets (combinable with `--repo`) |
| `--org acme` | organization secrets — mutually exclusive with `--repo`/`--environment` |

Before anything writes, the plugin prints where and as whom, then asks:

```
$ dotenvctl github push --prefix GITHUB_SECRET_ --strip-prefix
target : acme/api (from cwd git remote)
account: you (stored gh auth)
keys   : 10
proceed? [y/N]
```

`--dry-run` prints the banner and the would-push list without prompting or writing. Non-interactive runs (CI, pipes, `--json`) require `--yes`. The banner exists because "wrong directory + inferred repo + irreversible write" must never combine silently.

## Authentication and token override

The plugin inherits `gh`'s own auth chain: **`GH_TOKEN` → `GITHUB_TOKEN` → your stored `gh auth login`** (plus `GH_HOST` / `GH_ENTERPRISE_TOKEN` for GitHub Enterprise). To act with a different identity than your login — say your account lacks access but you hold a PAT that doesn't:

```sh
GH_TOKEN=ghp_yourtoken dotenvctl github push --repo someorg/private-repo --yes
```

The banner labels the source — `account: you (via GH_TOKEN)` — so a token exported yesterday can't act invisibly today. There is deliberately **no `--token` flag**: argv is visible to every process via `ps` and persists in shell history. For the same reason, secret values reach `gh` via stdin, never as arguments.

## Creating the environment first

Environment-scoped secrets require the deployment environment to exist — the step everyone forgets and then does by hand in the web UI:

```sh
dotenvctl github env-create prod
dotenvctl github env-create staging --repo owner/name --yes
```

Idempotent: an existing environment is success, so it belongs at the top of any setup task. The usual confirm gate applies (`--yes` for scripts). A typical bootstrap for a fresh repo:

```sh
dotenvctl github env-create prod --yes
dotenvctl -f .env.prod github push --prefix GITHUB_SECRET_ --strip-prefix --env prod --yes
```

## Secret names

GitHub requires `[A-Za-z_][A-Za-z0-9_]*`. Env keys that are legal in `.env` files but not as secret names (dots, hyphens, digit-first) fail the **whole push before any write**, with the offending keys listed — no silent renaming, no partial pushes.

## The prefix workflow

If your env files mark deploy secrets with a prefix, one command syncs exactly those:

```sh
dotenvctl -f .env.staging github push --prefix GITHUB_SECRET_ --strip-prefix
```

`GITHUB_SECRET_GHCR_PAT=…` becomes repo secret `GHCR_PAT`; unprefixed keys are never considered; placeholder values (`__YOU__`) are skipped with a per-key warning.

## Keeping remote secrets in sync

```sh
# What exists remotely? (repo-level secrets)
dotenvctl github list

# Environment-scoped secrets are listed separately — if `list` reports
# "no secrets found for this scope", yours probably live in an environment:
dotenvctl github list --env prod

# Preview a full sync: what would push, what would be skipped
dotenvctl github push --prefix GITHUB_SECRET_ --strip-prefix --dry-run

# Delete remote secrets your file no longer defines (after the same selection)
dotenvctl github prune --prefix GITHUB_SECRET_ --strip-prefix --dry-run
dotenvctl github prune --prefix GITHUB_SECRET_ --strip-prefix --yes
```

`prune` computes *remote names minus the local keep-set* and deletes only those; `--dry-run` shows the would-delete list first. The keep-set is wider than the pushable selection, on purpose:

- keys skipped as placeholders/empty locally are **spared** (`GITHUB_ACTOR: kept-skipped`) — a `__YOU__` in your file never deletes the real secret someone set upstream;
- `--keep NAME` (repeat or comma-separate) spares secrets managed outside this selection — e.g. a bundle or key-file secret another pipeline maintains:

```sh
dotenvctl github prune --prefix GITHUB_SECRET_ --strip-prefix \
  --keep ENV_B64,VPS_DEPLOY_KEY_B64 --env prod --dry-run
```

Every spared name is reported with its reason, so the decision is auditable.

## CI usage

```yaml
- name: Sync Actions secrets from the env template
  run: |
    dotenvctl -f .env.staging github push \
      --prefix GITHUB_SECRET_ --strip-prefix \
      --repo ${{ github.repository }} --yes --json
  env:
    GH_TOKEN: ${{ secrets.ADMIN_PAT }}
```

`--json` emits `{"ok":true,"data":{"meta":{"target":…,"account":…},"results":[{"name":"GHCR_PAT","action":"pushed"}]}}` — names and actions only; values never appear in any output.

## Failure behavior

- `gh` missing or unauthenticated: a clear error pointing at `https://cli.github.com` / `gh auth login`; nothing runs.
- A push failing mid-way: everything already pushed is reported as `pushed`, the command exits `1`, and re-running completes the rest — pushes are idempotent upserts.
