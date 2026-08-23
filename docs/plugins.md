# Plugins

Plugins move env values *outward* — into secret stores and deployment platforms — and each one is a command namespace: `dotenvctl github …`, with its own verbs and flags. Backends genuinely differ (GitHub secrets are write-only, other stores have environments or sensitivity levels), so plugins are not forced into one rigid interface; instead they share the pieces that must be uniform.

## What every plugin shares

**The value pipeline.** Plugins never parse your env file themselves. The host parses once and hands them a resolved selection, so these behave identically in every plugin:

| Flag | Meaning |
|---|---|
| `[KEY...]` args | push exactly these keys; naming a key that doesn't exist fails loudly |
| `--prefix P` | select only keys starting with `P` |
| `--strip-prefix` | remove the prefix from the pushed names (`GITHUB_SECRET_PAT` → `PAT`) |
| `--expand` | resolve `${references}` first — **on by default** for pushes; a literal `${DB_HOST}` stored in a remote secret is nearly always wrong |
| `--include-placeholders` / `--include-empty` | lift the default guards (below) |

**The safety rules.**

- Values matching the placeholder pattern (`__YOU__`-style) are **skipped with a warning**, never pushed silently — a stand-in landing in production secrets is a disaster. Same for empty values.
- Secret values travel to backend CLIs via **stdin, never argv** — argv is visible to every process via `ps` and lands in shell history.
- Output prints **names and actions only**, never values: `pushed` · `would-push` · `skipped-placeholder` · `skipped-empty` · `deleted` · `would-delete`.
- Every mutating verb supports `--dry-run`, and prints the resolved target and acting account **before** anything writes.
- Interactive runs prompt `proceed? [y/N]`; non-interactive runs (CI, pipes, `--json`) refuse to write without `--yes`.
- A failing `${VAR:?msg}` reference aborts before the plugin touches any network.

**The standard verbs** (where the backend supports them): `push [KEY...]` upserts, `list` shows remote secret names (values are write-only in most stores), `prune --yes` deletes remote names absent from your local selection. Backend-specific concepts appear as extra flags on these verbs, or as extra verbs in the namespace — the shared ones never change meaning.

## Available plugins

| Plugin | Backend | Guide |
|---|---|---|
| `github` | GitHub Actions secrets, via the `gh` CLI | [plugins/github.md](plugins/github.md) |

Planned: `vercel` (Vercel environment variables), and exec plugins — third-party `dotenvctl-<name>` binaries on your PATH dispatched git-style, in any language. Not shipped yet; this page will document the authoring contract when they land.

## A worked example

Your env file follows the convention of prefixing deploy secrets:

```env
# .env.staging
DB_HOST=stag.db.internal
GITHUB_SECRET_GHCR_PAT=ghp_xxxx
GITHUB_SECRET_DEPLOY_PATH=/srv/apps/api-stag
GITHUB_SECRET_VPS_HOST=__YOU__          # placeholder — not filled in yet
```

One command pushes exactly the prefixed keys, renamed without the prefix, skipping the placeholder:

```sh
$ dotenvctl -f .env.staging github push --prefix GITHUB_SECRET_ --strip-prefix
target : yourorg/yourrepo (from cwd git remote)
account: you (stored gh auth)
keys   : 3
proceed? [y/N] y
VPS_HOST: skipped-placeholder
GHCR_PAT: pushed
DEPLOY_PATH: pushed
```

`DB_HOST` was never considered (no prefix), the placeholder never left your machine, and the repo secrets are now `GHCR_PAT` and `DEPLOY_PATH`.
