# dotenvctl

A comment-preserving `.env` control tool — the command-line companion to [`github.com/ubgo/dotenv`](../README.md). Read, edit, compare, and sync env files from the shell without ever destroying the formatting a human wrote: every byte outside the entry you touch survives verbatim — comments, blank lines, ordering, quoting style, even the author's `=` vs `:` delimiter. It never modifies its own process environment.

```sh
go install github.com/ubgo/dotenv/cli/cmd/dotenvctl@latest
```

**Docs:** [getting started](../docs/getting-started.md) · [command reference](../docs/commands.md) · [multi-env tools](../docs/multi-env.md) · [recipes](../docs/recipes.md) · [plugins](../docs/plugins.md) · [GitHub](../docs/plugins/github.md) / [Vercel](../docs/plugins/vercel.md) · [Go API](../docs/go-api.md)

## The pitch, in one diff

Given a hand-formatted file:

```env
# ──────────────────────────────
# DATABASE
# ──────────────────────────────
DB_HOST=localhost
DB_PORT=5432
DB_USER=admin

# ──────────────────────────────
# APP — tuned by ops, do not touch
# ──────────────────────────────
API_URL=https://api.${DOMAIN}
DOMAIN=acme.dev
DEBUG=false
```

```sh
dotenvctl set DB_PASSWORD=s3cret --after DB_USER
```

The **entire** change to the file, per `diff`:

```diff
 DB_USER=admin
+DB_PASSWORD=s3cret
```

One line added, in the right section. Every banner, blank line, and comment intact; the `${DOMAIN}` reference untouched. A parse-to-map tool (or a hand-rolled `sed`/`awk` script) rewriting this file loses the banners, reorders keys, normalizes quoting — a whole-file diff nobody asked for, on a file that is documentation as much as configuration. Here that is impossible by construction: the guarantee is inherited from the library and pinned by its fuzz tests.

Not sure what a command will do? Every mutating verb takes `--dry-run` and shows you the exact line diff first:

```sh
$ dotenvctl set DEBUG=true --dry-run
-DEBUG=false
+DEBUG=true
```

## The 60-second tour

Real commands, real output:

```sh
$ dotenvctl get API_URL
https://api.${DOMAIN}
$ dotenvctl get API_URL --expand          # full Docker Compose interpolation
https://api.acme.dev

$ dotenvctl unset DEBUG                   # disable — comments it out, reversible
$ grep DEBUG .env
# DEBUG=false
$ dotenvctl restore DEBUG                 # …and back, byte-identical
$ grep DEBUG .env
DEBUG=false

$ dotenvctl list
DB_HOST      localhost
DB_PORT      5432
DB_USER      admin
DB_PASSWORD  s3cret
API_URL      https://api.${DOMAIN}
DOMAIN       acme.dev
DEBUG        false

$ dotenvctl run -- sh -c 'echo "child sees: $DB_HOST"'
child sees: localhost                     # the child gets the file's values; YOUR shell env is untouched
```

Built for scripts as much as humans — every command takes `--json` and returns one stable envelope:

```sh
$ dotenvctl get DB_HOST --json
{"ok":true,"data":{"key":"DB_HOST","value":"localhost","expanded":false}}

$ dotenvctl get NOPE --json; echo "exit=$?"
{"ok":false,"error":{"code":"not_found","message":"key \"NOPE\" not found in .env"}}
exit=1
```

## Multi-environment: see the whole family at once

A real project holds `.env`, `.env.staging`, `.env.prod`, `.env.example` — and the questions that hurt are *cross-file*: what drifted, what's missing, what's still a placeholder. `dotenvctl` answers them directly:

```sh
$ dotenvctl envs
ENV                 FILE          KEYS  DISABLED  INHERITED  PLACEHOLDERS
default             .env          7     0         0          0
stag                .env.staging  6     0         0          0
prod                .env.prod     4     1         0          1
example (contract)  .env.example  4     0         0          0
```

```sh
$ dotenvctl matrix .env.staging .env.prod --only-drift
KEY                        stag  prod
DB_PASS                    ✓     !
EXTRA                      —     ✓
FEATURE_X                  ✓     #
GITHUB_SECRET_DEPLOY_PATH  ✓     —
GITHUB_SECRET_GHCR_PAT     ✓     —

✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
```

That one table says: prod's `DB_PASS` is still a `__YOU__` placeholder, `FEATURE_X` is commented out in prod (not missing — different problem, different fix), and the deploy secrets exist only in staging. No other dotenv tool distinguishes *disabled* from *missing* from *placeholder* — they parse to a map and all three collapse into "absent".

```sh
$ dotenvctl diff .env.staging .env.prod
+ EXTRA=only-here
- FEATURE_X=on
- GITHUB_SECRET_GHCR_PAT=ghp_xxxx
~ DB_HOST: stag.db.internal -> prod.db.internal
~ DOMAIN: staging.acme.io -> acme.io
```

Exit `1` when the files differ, like `diff(1)` — so `dotenvctl diff` *is* the CI assertion. Two more one-liners that drop straight into a pipeline:

```sh
dotenvctl matrix --contract .env.example    # exit 1 when any env misses a key the template requires
dotenvctl matrix --format html -o envs.html # self-contained shareable report — secrets masked by default
```

The HTML report masks every value structurally (secrets are never placed in the document unless you pass `--reveal`), so the default report is safe to attach to a ticket or commit to a wiki.

## Sync secrets outward, safely

One env file often serves several audiences — app config for the container, deploy credentials for CI — split only by a naming convention. Plugins push a *selection* of the file to a backend, and the selection grammar is shared with `list`/`keys`, so what you preview is exactly what pushes:

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

The safety rules hold across every plugin: placeholder (`__YOU__`) and empty values are skipped with a warning, never pushed silently; secret values reach backend CLIs via **stdin, never argv**; output prints names and actions, never values; `--dry-run` everywhere; non-interactive runs refuse to write without `--yes`; `prune` keeps more than push pushes, because delete has no undo. Ships with [GitHub Actions secrets](../docs/plugins/github.md) and [Vercel env vars](../docs/plugins/vercel.md); any `dotenvctl-<name>` executable on PATH becomes a third-party subcommand, git-style ([write your own](../docs/plugins.md#writing-your-own-plugin-exec-plugins)).

## Why this over the usual suspects

Most dotenv tooling does one of two jobs: **load** a file into a process (dotenv-cli, direnv, godotenv's CLI) or **encrypt** it (dotenvx, sops). `dotenvctl`'s job is different — it treats the `.env` file itself as the artifact to *manage*: edit it without destroying it, compare it across environments, gate CI on its completeness, and fan selections of it out to secret stores.

| Job | Typical answer today | `dotenvctl` |
|---|---|---|
| Run a command with a file's values | dotenv-cli / direnv — solved | `run --` (file wins over shell, exit code passes through) |
| Edit a value from a script | `sed -i` / hand-editing — destroys formatting, breaks on quoting and multiline values | `set`/`unset`/`restore`, byte-preserving, `--dry-run`, real parser |
| Disable a key temporarily | delete the line and hope git remembers | `unset` = comment out; `restore` = byte-identical undo; `list --disabled` shows what's off |
| Spot drift between staging and prod | eyeball two files / ad-hoc `diff` full of formatting noise | `matrix --only-drift`, `diff` over *effective* config, exit-code CI gates |
| Check every env defines what `.env.example` promises | nothing, until prod crashes | `matrix --contract` — exit 1 on any gap |
| Push deploy secrets to GitHub/Vercel | a hand-rolled bash loop with `gh secret set` that silently mangles multiline values | `github push` / `vercel push` with selection, placeholder guards, stdin-only values |
| Hand the team a readable env report | copy-paste values into a wiki (yikes) | `matrix --format html` — masked by default, structurally |

And one boring advantage: the name `dotenvctl` collides with nothing on npm, PyPI, Homebrew, crates, or RubyGems — no PATH roulette with the Node/Python/Ruby dotenv tools.

## Use it as a Go library

Every verb is an exported function — the CLI is a thin translator over `envkit`:

```go
import "github.com/ubgo/dotenv/cli/envkit"

sel, err := envkit.SelectFile(".env.prod", envkit.SelectOptions{Prefix: "GITHUB_SECRET_", StripPrefix: true, Expand: true})
m, err := envkit.BuildMatrix(envkit.MatrixOptions{Dir: ".", ContractPath: ".env.example"})
d, err := envkit.Diff(".env.staging", ".env.prod", envkit.DiffOptions{Expand: true})
code, err := envkit.Run(ctx, ".env.test", []string{"go", "test", "./..."}, envkit.RunOptions{Base: os.Environ()})
```

Plugins are importable too — mount ours in your own binary, or build one for your backend with `providerkit`. Full reference: [Go API](../docs/go-api.md).

## Development

This directory is its own Go module, so cobra and CLI dependencies never enter the library's dependency graph — the library stays stdlib-only.

```sh
task cli:build      # → cli/bin/dotenvctl        (from the repo root)
task cli:test       # race-enabled tests
task cli:ci         # fmt-check + vet + race tests
task ci             # the full gate: library AND cli
```

Layout: `cmd/dotenvctl` (main) · `dotenvcmd` (cobra verb tree) · **`envkit` (the operations API)** · `providerkit` (plugin seam) · `plugins/{githubplugin,vercelplugin}` · helpers `discover`, `outfmt`, `report`, `textdiff` — all exported, nothing under `internal/`.
