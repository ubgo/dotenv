# Command reference

Every verb, with real output. Global flags on every command: `-f/--file PATH` (default `./.env`) and `--json` (machine envelope). Exit codes: `0` ok · `1` operation failed · `2` usage — exceptions noted per command. Mutating verbs all take `--dry-run`.

| Verb | Job |
|---|---|
| [`get`](#get--print-one-value) | print one value, optionally `${expanded}` |
| [`set`](#set--upsert-values-byte-preserving) | upsert `KEY=VALUE`s, byte-preserving, anchored placement |
| [`unset`](#unset--deactivate-a-key) | comment out (default) or delete keys |
| [`restore`](#restore--the-inverse-of-unset) | reactivate a commented-out key, byte-identical |
| [`list`](#list--the-files-effective-contents) | the file's contents as a table or JSON |
| [`keys`](#keys--names-only-pipe-friendly) | names only, for piping |
| [`run`](#run--execute-a-command-with-the-files-values) | run a child process with the file's values |
| [`diff`](#diff--compare-two-files-effective-configuration) | compare two files' effective config |
| [`envs`](#envs--discover-the-env-family) | discover the directory's `.env` family |
| [`matrix`](#matrix--the-keys--environments-drift-table) | keys × environments drift table, contract gate, HTML reports |
| [`github`](#github--github-actions-secrets) | push/list/prune GitHub Actions secrets, create environments |
| [`vercel`](#vercel--vercel-environment-variables) | push/list/prune Vercel env vars |
| [`plugins`](#plugins--discover-exec-plugins) | list third-party exec plugins on PATH |
| [`completion`](#completion--shell-completions) | shell completions (bash, zsh, fish, powershell) |

## get — print one value

```sh
dotenvctl get DATABASE_URL
dotenvctl get DATABASE_URL --expand        # resolve ${references} against the file
dotenvctl -f .env.prod get PORT --json
```

```
$ dotenvctl get API_URL
https://api.${DOMAIN}
$ dotenvctl get API_URL --expand
https://api.acme.dev
$ dotenvctl get DB_HOST --json
{"ok":true,"data":{"key":"DB_HOST","value":"localhost","expanded":false}}
$ dotenvctl get NOPE --json; echo "exit=$?"
{"ok":false,"error":{"code":"not_found","message":"key \"NOPE\" not found in .env"}}
exit=1
```

Exits `1` when the key has no active entry. A commented-out setting (`# KEY=…`) and a name-only declaration (`KEY` alone) do not count — they are inactive, and `list --disabled` / `list --inherited` is how you see them. A failing `${VAR:?message}` under `--expand` also exits `1`, with the message — that is the point of the `:?` form.

## set — upsert values, byte-preserving

```sh
dotenvctl set DB_HOST=localhost DB_PORT=5432   # multiple KEY=VALUE args
dotenvctl set CONN="postgres://u:p@h/db?x=1"   # values may contain '=' — only the first splits
dotenvctl set DB_PASSWORD=secret --after DB_PORT     # place a NEW key after an anchor
dotenvctl set DB_DRIVER=postgres --before DB_HOST    # …or before one
dotenvctl set DEBUG=true --dry-run             # print the would-be line diff, write nothing
```

`--dry-run` output is a minimal `-old`/`+new` line diff — the *entire* change the write would make:

```
$ dotenvctl set DEBUG=true --dry-run
-DEBUG=false
+DEBUG=true
```

Anchored placement, and what a missing anchor does:

```
$ dotenvctl set DB_PASSWORD=s3cret --after DB_USER    # lands right after DB_USER, inside its section
$ dotenvctl set X=1 --after NOPE; echo "exit=$?"
dotenvctl: envkit: anchor key not found: "NOPE"
exit=1                                                # nothing was written — no silent append
```

Rules worth knowing:

- An existing key updates in place and never moves; only a new key is placed by `--after`/`--before`. A missing anchor exits `1` without writing anything.
- Setting a key to its current value is a full no-op — same bytes, same mtime, file watchers see nothing.
- Everything the author wrote around the value survives: `export` prefixes, spacing, inline comments, and the `=` vs `:` delimiter style.
- A missing file is created with `0600` permissions (env files hold credentials — restrictive by default).

## unset — deactivate a key

```sh
dotenvctl unset OLD_KEY              # default: comments it out — '# OLD_KEY=…', reversible
dotenvctl unset OLD_KEY --delete     # removes the line entirely
dotenvctl unset A B C --dry-run      # preview, write nothing
```

```
$ dotenvctl unset DB_HOST DB_PORT --dry-run
-DB_HOST=localhost
-DB_PORT=5432
+# DB_HOST=localhost
+# DB_PORT=5432

$ dotenvctl unset DB_HOST --delete --dry-run
-DB_HOST=localhost
```

Comment-out is the default on purpose: it is reversible, diff-friendly, and documentation above the setting stays attached to something.

## restore — the inverse of unset

```sh
dotenvctl restore OLD_KEY
dotenvctl list --disabled            # see what is restorable
```

```
$ dotenvctl unset DEBUG && grep DEBUG .env
# DEBUG=false
$ dotenvctl restore DEBUG && grep DEBUG .env
DEBUG=false
$ dotenvctl restore NOPE; echo "exit=$?"
dotenvctl: envkit: key not found: no disabled entry for "NOPE"
exit=1
```

Removes exactly the comment marker that was added, so `#KEY=v` comes back without inventing a space. Unset followed by restore returns the file to its original bytes — a fuzz-pinned property of the underlying library.

## Selecting keys — the shared grammar

`list`, `keys`, and every plugin store verb accept the same selection flags, resolved by one core pipeline (so `list --prefix X` and `github push --prefix X` can never disagree about which keys match):

| Flag | Meaning |
|---|---|
| `--prefix P` | keep only keys starting with `P` |
| `--exclude-prefix P` | drop keys starting with `P` (the inverse selector) |
| `--strip-prefix` | remove the matched prefix from the emitted names |
| `--include KEY` | force-include a key past the prefix filters |
| `--exclude KEY` | drop a specific key (excluding an absent key is a no-op) |

**Multiple values** — `--include` and `--exclude` accept any mix of two equivalent spellings, repeat the flag or pass a comma-separated list:

```sh
--exclude GITHUB_SECRET_GITHUB_TOKEN --exclude GITHUB_SECRET_GITHUB_ACTOR   # repeated
--exclude GITHUB_SECRET_GITHUB_TOKEN,GITHUB_SECRET_GITHUB_ACTOR             # comma list
--exclude A,B --exclude C                                                    # mixed — union of all
```

Resolution order: explicit `KEY...` args are taken as given → otherwise all keys → `--prefix` keeps → `--exclude-prefix` drops → `--include` adds → `--exclude` drops. Rules that follow from it:

- Any key you *name* (`KEY...` args or `--include`) must exist in the file, or the command fails loudly — a silently skipped named key would report success for work that never happened.
- `--exclude` and `--include` match the key's **original** name as written in the file (`GITHUB_SECRET_GHCR_PAT`), never the post-`--strip-prefix` name (`GHCR_PAT`).
- `--exclude` beats `--include` when both name the same key.
- `--prefix` and `--exclude-prefix` compose: prefix narrows first, exclude-prefix removes from that result.
- `--strip-prefix` renames only keys that actually carry the `--prefix`; force-included keys without it keep their names untouched.

The use case this exists for: one env file serving several audiences, split by a naming convention your repo chose. Read just one audience — as JSON, without pushing anything anywhere:

```sh
# the deploy-secrets view, renamed, machine-readable
dotenvctl -f .env.prod list --prefix GITHUB_SECRET_ --strip-prefix --json

# the app-config view — everything EXCEPT the deploy keys
dotenvctl -f .env.prod list --exclude-prefix GITHUB_SECRET_

# just the names, for scripting
dotenvctl keys --prefix GITHUB_SECRET_ --strip-prefix
```

One semantic difference from store verbs: on reads the placeholder/empty guards are OFF — `list` shows you `WHO=__YOU__` (you're reading, you want to see it), while `github push` skips it (a stand-in must not become a production secret).

## list — the file's effective contents

```sh
dotenvctl list                       # active pairs, aligned table, file order
dotenvctl list --expand              # values with ${references} resolved
dotenvctl list --prefix GITHUB_SECRET_ --strip-prefix --json   # a selected, renamed view
dotenvctl list --disabled            # add the commented-out settings section
dotenvctl list --inherited           # add name-only declarations (values come from the environment)
dotenvctl list --json                # all sections always present in JSON, regardless of flags
```

```
$ dotenvctl list
DB_HOST      localhost
DB_PORT      5432
DB_USER      admin
API_URL      https://api.${DOMAIN}
DOMAIN       acme.dev

$ dotenvctl list --prefix GITHUB_SECRET_ --strip-prefix --json
{"ok":true,"data":{"pairs":[{"key":"GHCR_PAT","value":"ghp_xxxx"},{"key":"DEPLOY_PATH","value":"/srv/apps/api-stag"}],"disabled":[],"inherited":[],"expanded":false}}
```

Without selection flags, `list` shows the file as a linear read would: file order, duplicate keys included. With any selection flag it switches to the effective (last-wins) view — selected, optionally renamed.

## keys — names only, pipe-friendly

```sh
dotenvctl keys
dotenvctl keys --exclude-prefix GITHUB_SECRET_   # app-config names only
dotenvctl keys | grep '^DB_'
dotenvctl keys --json | jq -r '.data.keys[]'
```

```
$ dotenvctl keys
DB_HOST
DB_PORT
DB_USER
API_URL
DOMAIN
$ dotenvctl keys --json
{"ok":true,"data":{"keys":["DB_HOST","DB_PORT","DB_USER","API_URL","DOMAIN"]}}
```

## run — execute a command with the file's values

```sh
dotenvctl run -- npm start
dotenvctl -f .env.test run -- go test ./...
dotenvctl run -- sh -c 'echo $DATABASE_URL'
```

```
$ dotenvctl run -- sh -c 'echo "child sees: $DB_HOST"'
child sees: localhost

$ dotenvctl run -- sh -c 'exit 3'; echo "exit=$?"
exit=3                                # the child's exit code, verbatim

$ dotenvctl -f .env.req run -- echo hi; echo "exit=$?"     # .env.req has NEED=${MISSING:?fill me}
dotenvctl: envkit: expand: dotenv: required variable MISSING is not set or is empty: fill me
exit=1                                # the child never started
```

Semantics:

- The child's environment = your shell's environment overlaid with the file's values, **file wins** — that is why you ran it.
- Values arrive expanded; a failing `${VAR:?msg}` aborts before the child ever starts.
- The tool's own environment is never modified — the merge exists only in the child.
- The child's exit code passes through verbatim, so CI wrappers behave transparently.
- Name-only lines in the file (`HOME`) need no handling: the child inherits the parent environment anyway, which is exactly what those declarations mean.

## diff — compare two files' effective configuration

```sh
dotenvctl diff .env.staging .env.prod
dotenvctl diff .env .env.example           # am I missing keys the template defines?
dotenvctl diff a.env b.env --expand        # compare resolved values instead of raw text
dotenvctl diff a.env b.env --json | jq .data.changed
```

```
$ dotenvctl diff .env.staging .env.prod; echo "exit=$?"
+ EXTRA=only-here
- FEATURE_X=on
- GITHUB_SECRET_GHCR_PAT=ghp_xxxx
~ DB_HOST: stag.db.internal -> prod.db.internal
~ DB_PASS: stag-pass -> __YOU__
~ DOMAIN: staging.acme.io -> acme.io
exit=1

$ dotenvctl diff .env.staging .env.prod --json
{"ok":true,"data":{"added":[{"key":"EXTRA","value":"only-here"}],"removed":[{"key":"FEATURE_X","value":"on"},…],"changed":[{"key":"DB_HOST","from":"stag.db.internal","to":"prod.db.internal"},…]}}
```

Output: `+ KEY=…` only in the second file · `- KEY=…` only in the first · `~ KEY: a -> b` changed. Exit `0` identical, `1` different (like `diff(1)`), `2` trouble. Comparison is over the effective last-wins view — formatting, comments, and shadowed duplicates are invisible on purpose. `--format human|json|html` and `-o`/`--output PATH` control output; HTML reports are secrets-masked unless `--reveal` — see [HTML reports](multi-env.md#html-reports).

## envs — discover the .env family

```sh
dotenvctl envs
dotenvctl envs --dir ./deploy
dotenvctl envs --json | jq -r '.data.files[].file'
```

```
$ dotenvctl envs
ENV                 FILE          KEYS  DISABLED  INHERITED  PLACEHOLDERS
default             .env          7     0         0          0
stag                .env.staging  6     0         0          0
prod                .env.prod     4     1         0          1
example (contract)  .env.example  4     0         0          0
```

Detection is non-recursive and filename-based: `.env` is `default`; long forms collapse to canonical short names (`.env.production` and `.env.prod` mean the same environment); `.env.example`/`.sample`/`.template`/`.dist` are **contracts** — listed, excluded from matrix columns, usable via `--contract`; backup/editor droppings (`.bak`, `.tmp`, `.swp`, …) are ignored; any other `.env.something` is kept verbatim. Ordering is distance-from-prod. The PLACEHOLDERS column counts values matching `^__[A-Z0-9_]+__$` — your "still needs a real value" signal. Full rules table: [multi-env tools](multi-env.md#envs--discover-the-family).

## matrix — the keys × environments drift table

```sh
dotenvctl matrix                                   # all detected files as columns
dotenvctl matrix .env.staging .env.prod            # exactly these files
dotenvctl matrix --only-drift                      # hide rows present everywhere
dotenvctl matrix --values                          # show values — masked as ••••••
dotenvctl matrix --values --reveal                 # real values (treat output as a secret)
dotenvctl matrix --contract .env.example           # CI gate: exit 1 when an env misses a contract key
dotenvctl matrix --format html -o envs.html        # self-contained shareable report
```

```
$ dotenvctl matrix .env.staging .env.prod --only-drift
KEY                        stag  prod
DB_PASS                    ✓     !
EXTRA                      —     ✓
FEATURE_X                  ✓     #
GITHUB_SECRET_DEPLOY_PATH  ✓     —
GITHUB_SECRET_GHCR_PAT     ✓     —

✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
```

Cell legend: `✓` an active pair with a real value · `!` set but to a `__PLACEHOLDER__` (pattern configurable via `--placeholder REGEX`) · `∅` set to empty · `#` only a commented-out setting exists · `→` a name-only declaration (value comes from the environment) · `—` no entry at all. The distinctions matter: *disabled*, *missing*, and *placeholder* are different problems with different fixes, and parse-to-map tools collapse all three into "absent".

The contract gate, with gaps named and exit `1`:

```
$ dotenvctl matrix --contract .env.example; echo "exit=$?"
KEY         default  stag  prod
DB_HOST     ✓        ✓     ✓
DB_PASS     —        ✓     !
SENTRY_DSN  —        —     —
…
contract: stag is missing 1 key(s): [SENTRY_DSN]
contract: prod is missing 1 key(s): [SENTRY_DSN]
exit=1
```

Sharp edges (sparse-column drift, duplicate environment labels, leak-proof JSON) and the HTML report details live in [multi-env tools](multi-env.md#matrix--the-drift-table).

## github — GitHub Actions secrets

Verbs: `push [KEY...]` · `list` · `prune` · `env-create NAME`. Auth and encryption come from an authenticated [`gh`](https://cli.github.com); the target repo resolves from the cwd's git remote (override with `--repo owner/name`), and `--env NAME` targets a deployment environment. There is deliberately no `--token` flag — argv is visible in `ps` and shell history; use `GH_TOKEN` if you must override auth.

```
$ dotenvctl -f .env.staging github push --prefix GITHUB_SECRET_ --strip-prefix --dry-run
target : ubgo/dotenv (from cwd git remote)
account: khanakia (stored gh auth)
keys   : 2
GHCR_PAT: would-push
DEPLOY_PATH: would-push
```

Target and acting account print **before** anything writes; interactive runs prompt `proceed? [y/N]`, non-interactive runs refuse without `--yes`. Selection is the [shared grammar](#selecting-keys--the-shared-grammar); placeholder/empty values are skipped with a warning; values travel via stdin, never argv; output prints names and actions, never values. `prune` deletes remote names absent from the local keep-set — which is deliberately wider than what push pushes (guard-skipped keys are kept, `--keep NAME` spares out-of-band names, every spare is reported with its reason). Full guide with every flag: [GitHub plugin](plugins/github.md).

## vercel — Vercel environment variables

Verbs: `push [KEY...]` · `list` · `prune`. Auth and project linking come from the [`vercel`](https://vercel.com/docs/cli) CLI; `--target production|preview|development` picks the environment, `--sensitive` marks values write-only. Same shared selection grammar, same guards, same confirm gate as every plugin:

```sh
dotenvctl vercel push --target production --yes
dotenvctl vercel push DB_URL --sensitive --dry-run
dotenvctl vercel push --prefix VERCEL_ --strip-prefix --target preview
dotenvctl vercel prune --target production --yes
```

Full guide: [Vercel plugin](plugins/vercel.md).

## plugins — discover exec plugins

Any executable named `dotenvctl-<name>` on PATH becomes a subcommand, git-style — `dotenvctl acme push` runs `dotenvctl-acme push`. `plugins` lists what was found; built-ins always win, and a PATH binary shadowed by one is reported as `shadowed by built-in`:

```
$ dotenvctl plugins
no exec plugins found on PATH (binaries named dotenvctl-<name>)
```

How to write one (the env-var contract, calling the host back for parsing): [plugins overview](plugins.md#writing-your-own-plugin-exec-plugins).

## completion — shell completions

```sh
dotenvctl completion bash|zsh|fish|powershell    # print the script; see `dotenvctl completion --help` for install lines
```

Built in via cobra — e.g. `source <(dotenvctl completion zsh)` in `.zshrc`, or write it to your shell's completions directory.

---

Every verb on this page is also an exported Go function — the CLI is a thin translator over `envkit`. See the [Go API guide](go-api.md).
