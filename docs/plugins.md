# Plugins

Plugins move env values *outward* — into secret stores and deployment platforms — and each one is a command namespace: `dotenvctl github …`, with its own verbs and flags. Backends genuinely differ (GitHub secrets are write-only, other stores have environments or sensitivity levels), so plugins are not forced into one rigid interface; instead they share the pieces that must be uniform.

## What every plugin shares

**The value pipeline.** Plugins never parse your env file themselves. The host parses once and hands them a resolved selection — the same [core selection grammar](commands.md#selecting-keys--the-shared-grammar) that `list` and `keys` use, so reading a selection and pushing it can never disagree:

| Flag | Meaning |
|---|---|
| `[KEY...]` args | push exactly these keys; naming a key that doesn't exist fails loudly |
| `--prefix P` | select only keys starting with `P` |
| `--strip-prefix` | remove the prefix from the pushed names (`GITHUB_SECRET_PAT` → `PAT`) |
| `--expand` | resolve `${references}` first — **on by default** for pushes; a literal `${DB_HOST}` stored in a remote secret is nearly always wrong |
| `--exclude-prefix P` | drop keys starting with `P` — the inverse selector ("everything except the deploy keys") |
| `--include KEY` | force-include a key even when the prefix filters would drop it |
| `--exclude KEY` | drop a specific key from the selection (excluding an absent key is a no-op) |

Both take multiple values — repeat the flag (`--exclude A --exclude B`) or pass a comma list (`--exclude A,B`); the [core selection guide](commands.md#selecting-keys--the-shared-grammar) documents every rule (matching is against pre-strip names, exclude beats include, named keys must exist).
| `--include-placeholders` / `--include-empty` | lift the default guards (below) |

Resolution order is fixed and predictable: explicit `KEY...` args are the selection as given (prefix filters don't second-guess them) — otherwise all keys → `--prefix` keeps → `--exclude-prefix` drops → `--include` force-adds → `--exclude` drops → the guards run last. Any key you *name* (`KEY...` args or `--include`) must exist, or the command fails loudly.

## Why selection matters: one file, several audiences

The pattern these flags exist for: a single `.env.prod` holds *everything* — app config the container needs AND deploy credentials meant for a secret store — separated only by a naming convention:

```env
# app config → stays local / goes to the container
DB_HOST=db.internal
JWT_SECRET=...

# deploy credentials → GitHub Actions secrets (a prefix marks them)
GITHUB_SECRET_GHCR_PAT=ghp_...
GITHUB_SECRET_DEPLOY_PATH=/srv/app
```

Teams typically maintain a custom script that splits this file with hand-rolled line parsing — and hand-rolled dotenv parsing breaks on multiline values and inline comments in ways that ship credentials to the wrong audience *silently*. The selection flags make the split declarative, backed by a real parser:

```sh
# the deploy credentials, renamed, into GitHub — nothing else leaves the machine
dotenvctl github push --prefix GITHUB_SECRET_ --strip-prefix --env prod --yes

# a local-testing special that lives outside the prefix? force it in:
dotenvctl github push --prefix GITHUB_SECRET_ --strip-prefix --include ACTOR --env prod --yes

# a key that must NEVER push, wherever it appears:
dotenvctl github push --prefix GITHUB_SECRET_ --strip-prefix --exclude GITHUB_SECRET_LOCAL_ONLY --env prod --yes
```

The convention (`GITHUB_SECRET_`, or any other) belongs to *your repo* — the binary ships only the mechanism.

**The safety rules.**

- Values matching the placeholder pattern (`__YOU__`-style) are **skipped with a warning**, never pushed silently — a stand-in landing in production secrets is a disaster. Same for empty values.
- Secret values travel to backend CLIs via **stdin, never argv** — argv is visible to every process via `ps` and lands in shell history.
- Output prints **names and actions only**, never values: `pushed` · `would-push` · `skipped-placeholder` · `skipped-empty` · `deleted` · `would-delete` · `kept-skipped` · `kept-keep-flag`.
- Every mutating verb supports `--dry-run`, and prints the resolved target and acting account **before** anything writes.
- Interactive runs prompt `proceed? [y/N]`; non-interactive runs (CI, pipes, `--json`) refuse to write without `--yes`.
- A failing `${VAR:?msg}` reference aborts before the plugin touches any network.

**The standard verbs** (where the backend supports them): `push [KEY...]` upserts, `list` shows remote secret names (values are write-only in most stores), `prune --yes` deletes remote names absent from your local keep-set.

**Prune's keep-set is deliberately wider than what push would push**, because deleting is the one verb with no undo:

- **Guard-skipped keys are kept.** A local `KEY=__PLACEHOLDER__` is skipped by push — and prune spares the remote `KEY` too, reporting `KEY: kept-skipped`. "Not filled in locally" never means "delete remotely".
- **`--keep NAME` spares out-of-band names** (repeat or comma-separate): secrets created by other mechanisms or other tools that live in the same scope. `prune --keep ENV_B64,VPS_DEPLOY_KEY_B64` reports each as `kept-keep-flag`.
- Every spare is reported with its reason — an invisible keep would be as surprising as an invisible delete. Start any new prune usage with `--dry-run`. Backend-specific concepts appear as extra flags on these verbs, or as extra verbs in the namespace — the shared ones never change meaning.

## Available plugins

| Plugin | Backend | Guide |
|---|---|---|
| `github` | GitHub Actions secrets, via the `gh` CLI | [plugins/github.md](plugins/github.md) |
| `vercel` | Vercel environment variables, via the `vercel` CLI | [plugins/vercel.md](plugins/vercel.md) |

Run `dotenvctl plugins` to also see third-party exec plugins discovered on your PATH.

## Writing your own plugin (exec plugins)

Any executable named `dotenvctl-<name>` on your PATH becomes a subcommand: `dotenvctl acme push` runs `dotenvctl-acme push`, git-style, in any language. Its stdout/stderr stream through and its exit code becomes dotenvctl's. Built-ins always win — a PATH binary can never shadow `github`; `dotenvctl plugins` reports such collisions as `shadowed by built-in`.

Your plugin receives context as environment variables (facts only — secret values are never passed in the environment):

| Variable | Meaning |
|---|---|
| `DOTENVCTL_FILE` | absolute path of the `-f` file the user chose |
| `DOTENVCTL_JSON` | `1` when the user passed `--json`, else `0` |
| `DOTENVCTL_PLACEHOLDER` | the active placeholder regexp |
| `DOTENVCTL_BIN` | absolute path of the running dotenvctl — your callback handle |
| `DOTENVCTL_VERSION` | host version, for feature gating |

To read values, call the host back — this inherits dotenvctl's exact parsing, expansion, required-var, and placeholder semantics forever:

```sh
#!/usr/bin/env bash            # dotenvctl-acme
set -euo pipefail
"$DOTENVCTL_BIN" -f "$DOTENVCTL_FILE" list --expand --json |
  jq -r '.data.pairs[] | "\(.key)\t\(.value)"' |
  while IFS=$'\t' read -r key value; do
    [[ $value =~ $DOTENVCTL_PLACEHOLDER ]] && { echo "skip $key (placeholder)"; continue; }
    acme-cli secret set "$key" "$value"
    echo "$key: pushed"
  done
```

Trust model, stated plainly: an exec plugin is exactly as trustworthy as anything else on your PATH — dotenvctl adds discovery (`dotenvctl plugins`) and the no-shadowing rule, not a sandbox.

**Writing a plugin in Go instead?** `providerkit` is importable: implement the capability interfaces for your backend and the standard verbs are generated with the shared flags, guards, and confirm gate. See the [Go API guide](go-api.md#providerkit--plugins).

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
