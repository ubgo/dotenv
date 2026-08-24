# Vercel plugin

Pushes env values to **Vercel environment variables** through the [`vercel` CLI](https://vercel.com/docs/cli). Requires `vercel` on your PATH, logged in — or overridden by a token (below).

## Commands

```sh
dotenvctl vercel push [KEY...]     # upsert env vars (vercel env add --force, value via stdin)
dotenvctl vercel list              # remote variable names for the selected target(s)
dotenvctl vercel prune --yes       # delete remote names absent from your keep-set (skipped keys and --keep names are spared)
```

All [shared selection and safety flags](../commands.md#selecting-keys--the-shared-grammar) apply to `push` and `prune`: `--prefix`, `--exclude-prefix`, `--strip-prefix`, `--include`, `--exclude` (both multi-value: repeat or comma-separate), `--expand` (default on), `--include-placeholders`/`--include-empty`, `--dry-run`, `--yes`, `--json`.

## Vercel-specific flags

| Flag | Meaning |
|---|---|
| `--target production\|preview\|development` | which Vercel environment(s) to write — multi-value (`--target production --target preview` or `--target production,preview`), default `development` (the least dangerous place for an accident); also on `list`/`prune` to pick the scope |
| `--sensitive` | store as a sensitive variable: encrypted, unreadable in the dashboard (push only) |

```sh
dotenvctl vercel push --target production --target preview DB_URL --sensitive --yes
```

## The confirm banner

Every mutating verb prints where and as whom before writing, then prompts (or requires `--yes` non-interactively):

```
project: prj_abc123 (from .vercel/project.json)
account: you (stored vercel auth)
targets: production, preview
keys   : 4
proceed? [y/N]
```

The project line comes from the directory's `.vercel/project.json` link; an unlinked directory says so honestly (`unlinked directory (vercel CLI will resolve or prompt)`) rather than guessing.

## Authentication

The plugin inherits the vercel CLI's auth; set **`VERCEL_TOKEN`** to act with a different identity, and the banner labels it — `account: you (via VERCEL_TOKEN)`. There is deliberately no `--token` flag (argv is visible in `ps` and shell history), and values reach the CLI via stdin, never as arguments.

## A note on `list`/`prune` parsing

The vercel CLI has no stable machine-readable output for `vercel env ls`, so names are parsed from its table with a conservative heuristic. A CLI format change would surface loudly as an odd listing — never as silent misdata for `push`, which doesn't consult the listing at all. A direct-API mode is the planned upgrade path.
