# GitHub plugin

Pushes env values to **GitHub Actions secrets** through the [`gh` CLI](https://cli.github.com), which supplies authentication and secret encryption. Requires `gh` on your PATH, logged in (`gh auth login`) or overridden by a token (below).

## Commands

```sh
dotenvctl github push [KEY...]      # upsert secrets
dotenvctl github list               # remote secret names (GitHub secrets are write-only)
dotenvctl github prune --yes        # delete remote secrets absent from your local selection
```

All [shared plugin flags](../plugins.md#what-every-plugin-shares) apply: `--prefix`, `--strip-prefix`, `--expand` (default on), `--dry-run`, `--yes`, `--json`, plus the guards that skip placeholder and empty values.

## Choosing the target

| Flag | Target |
|---|---|
| *(none)* | the repository of your current directory's git remote, exactly as `gh` resolves it |
| `--repo owner/name` | that repository |
| `--environment production` | a deployment environment's secrets (combinable with `--repo`) |
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
# What exists remotely?
dotenvctl github list

# Preview a full sync: what would push, what would be skipped
dotenvctl github push --prefix GITHUB_SECRET_ --strip-prefix --dry-run

# Delete remote secrets your file no longer defines (after the same selection)
dotenvctl github prune --prefix GITHUB_SECRET_ --strip-prefix --dry-run
dotenvctl github prune --prefix GITHUB_SECRET_ --strip-prefix --yes
```

`prune` computes *remote names minus local selection* and deletes only those; `--dry-run` shows the would-delete list first.

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
