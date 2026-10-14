# Command reference

Every verb, with real output. Global flags on every command: `-f/--file PATH` (default `./.env`) and `--json` ([machine envelope](ds:block?id=envelope-44255rwu)). [Exit codes](ds:block?id=exitcodes-msf66qpb): `0` ok · `1` operation failed · `2` usage — exceptions noted per command. Mutating verbs all take `--dry-run`.

On this page: [Global flags and `--version`](#global-flags-and---version) · the verbs below · [Selecting keys](#selecting-keys--the-shared-grammar) · [Drive the CLI from Go](#drive-the-cli-from-go) · [Pitfalls](#pitfalls) · [Related](#related)

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

## Global flags and `--version`

Three flags work on every built-in verb, before or after the verb name (`dotenvctl -f .env.prod get X` and `dotenvctl get X -f .env.prod` are the same). For an exec plugin, put `-f` and `--json` before the plugin name: everything after it is passed to the plugin untouched.

| Flag | Meaning |
|---|---|
| `-f`, `--file PATH` | the `.env` file to read or edit; default `.env` in the current directory |
| `--json` | print exactly one `{"ok":…}` envelope on stdout instead of human output |
| `-v`, `--version` | print the version and exit |

```console
$ dotenvctl --version
dotenvctl version (devel)
```

The version is read from the build itself, so there is no constant to forget at release time. A `go install …@<tag>` binary prints that module version; a binary built from a checkout prints a pseudo-version such as `v0.1.3-0.20261014080958-31847d420e08+dirty`; `go run` prints `(devel)`. A program that embeds the CLI with `dotenvcmd.Execute` prints the version of the `github.com/ubgo/dotenv/cli` module it was built with, not its own, after the name set with `dotenvcmd.WithProgramName` (default `dotenvctl`). `--version` ignores `--json` and always prints the plain line, so scripts that gate on a version should parse that line.

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
- What the author wrote around the value survives: `export` prefixes, indentation, spacing on both sides of the delimiter (`A = 1` becomes `A = 2`), inline comments, and the `=` vs `:` delimiter style. The value's quoting is chosen afresh for the new value. In cli v0.1.2 and earlier, the spacing after the delimiter was lost (`A = 1` became `A =2`).
- A value is stored literally: `dotenvctl set 'P=a$b'` writes `P='a$b'`, so `get --expand` gives `a$b` back. To store a reference such as `${HOST}`, edit the file by hand. In cli v0.1.2 and earlier, a `$` was written bare and read back as a reference.
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

`keys` prints the active key names, one per line, with no decoration at all, so its output feeds `grep`, `xargs` and `while read` loops directly. Use it when you need names and not values, for example to check a file defines everything a script expects, or to loop over a subset with `get`. It takes the same five [selection flags](#selecting-keys--the-shared-grammar) as `list`, so a filter you preview with `keys` is the filter a plugin push will use.

```sh
dotenvctl keys
dotenvctl keys --prefix DB_ --strip-prefix       # filtered and renamed
dotenvctl keys --exclude-prefix GITHUB_SECRET_   # app-config names only
dotenvctl keys | grep '^DB_'
dotenvctl keys --json | jq -r '.data.keys[]'
```

**Try it.** With this `.env`:

```env
# local development
DB_HOST=localhost
DB_PORT=5432
API_URL=https://api.${DOMAIN}
DOMAIN=acme.dev
```

```console
$ dotenvctl keys
DB_HOST
DB_PORT
API_URL
DOMAIN
$ dotenvctl keys --prefix DB_ --strip-prefix
HOST
PORT
$ dotenvctl keys --exclude-prefix DB_ --json
{"ok":true,"data":{"keys":["API_URL","DOMAIN"]}}
$ dotenvctl keys --include NOPE; echo "exit=$?"
dotenvctl: envkit: --include key "NOPE" not found
exit=1
```

The same runs are pinned by [`ExampleExecute_keys`](../cli/dotenvcmd/example_test.go).

**Pitfalls**

- `keys` lists each name once, in first-seen order, while `list` (without selection flags) prints every line, duplicates included. A file with `A=1`, `B=2`, `A=3` gives `A`, `B` from `keys` and three rows from `list`.
- Commented-out settings (`# KEY=…`) and name-only lines (`KEY` alone) are not active, so `keys` leaves them out. `list --disabled` and `list --inherited` show them.
- A missing file is not an error: `dotenvctl -f .env.typo keys` prints nothing and exits `0` (`--json` gives `{"ok":true,"data":{"keys":[]}}`). Check the file exists first when an empty answer would be wrong.
- `keys` takes no arguments: `dotenvctl keys DB_HOST` is a usage error (exit `2`). Filter with `--prefix` or `--include` instead.
- Placeholder values never hide a name: `keys --prefix X` lists `X_TOKEN=__YOU__` even though `github push --prefix X` would skip it.

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

```

The `--json` payload carries the same three lists plus the inputs. For a staging file with `FEATURE_X=on` and a prod file that adds `SENTRY_DSN` (from [`ExampleExecute_json`](../cli/dotenvcmd/example_test.go)):

```console
$ dotenvctl diff .env.staging .env.prod --json
{"ok":true,"data":{"added":[{"key":"SENTRY_DSN","value":"https://key@sentry.example/1"}],"removed":[{"key":"FEATURE_X","value":"on"}],"changed":[{"key":"DB_HOST","from":"stag.db.internal","to":"prod.db.internal"},{"key":"DB_PASS","from":"stag-pass","to":"__YOU__"},{"key":"DOMAIN","from":"staging.acme.io","to":"acme.io"}],"expanded":false,"file_a":".env.staging","file_b":".env.prod"}}
```

Unlike `matrix --json`, the diff payload contains real values: treat it like the files themselves.

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

The contract gate, with gaps named and exit `1`, run on the file family from [multi-env tools](multi-env.md):

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

### Changing what counts as a placeholder: `--placeholder`

By default a value is a placeholder when the whole value matches `^__[A-Z0-9_]+__$` (`__YOU__`, `__FILL_ME__`). If your team marks stand-ins differently, pass your own regular expression. The flag **replaces** the default, so keep the default in an alternation when you want both:

```console
$ dotenvctl matrix .env.staging .env.prod --placeholder '^https://key@'
KEY         stag  prod
DB_HOST     ✓     ✓
DB_PASS     ✓     ✓
DOMAIN      ✓     ✓
FEATURE_X   ✓     #
SENTRY_DSN  —     !

✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
$ dotenvctl matrix .env.staging .env.prod --placeholder '^(__[A-Z0-9_]+__|https://key@.*)$' --only-drift
KEY         stag  prod
DB_PASS     ✓     !
FEATURE_X   ✓     #
SENTRY_DSN  —     !

✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
```

In the first run `DB_PASS=__YOU__` turned into `✓`, because the new pattern no longer matches it. Both runs are pinned by [`ExampleExecute_matrix_placeholder`](../cli/dotenvcmd/example_test.go). An invalid expression fails before anything is read:

```console
$ dotenvctl matrix --placeholder '('; echo "exit=$?"
dotenvctl: envkit: placeholder pattern "(": error parsing regexp: missing closing ): `(`
exit=1
```

`--placeholder` exists only on `matrix`. The `envs` PLACEHOLDERS column, the plugin push guards and the `DOTENVCTL_PLACEHOLDER` value handed to exec plugins always use the default pattern.

**Pitfalls**

- `matrix` needs at least two files: `dotenvctl matrix .env.prod` exits `1` with `envkit: matrix needs at least two env files, found 1`. To check one file for placeholders, read the `envs` PLACEHOLDERS column instead (see the [CI recipe](recipes.md#ci-gate-the-build-on-env-completeness)).
- `--values` masks every value-bearing cell as `••••••`, placeholders included, so the `!` marker disappears. Use the default symbol view when you are looking for placeholders.
- The legend line itself contains `!`, so `dotenvctl matrix | grep '!'` always matches. Match the PLACEHOLDERS count from `envs`, or the `"state":"placeholder"` cells in `matrix --json`.
- The contract gate counts presence only: a contract key set to `__YOU__` satisfies `--contract`, and a key that exists only commented out (`#`) does not.

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

The default target is `development`, `--sensitive` is a `push`-only flag, and the project shown in the banner comes from `.vercel/project.json` in the current directory. Full guide, with real output for every verb and the failure cases: [Vercel plugin](plugins/vercel.md).

## plugins — discover exec plugins

Any executable named `dotenvctl-<name>` on PATH becomes a subcommand, git-style — `dotenvctl acme push` runs `dotenvctl-acme push`. `plugins` lists what was found, so you can see exactly which third-party code a `dotenvctl <name>` would run. Built-ins always win, and a PATH binary shadowed by one is reported as `shadowed by built-in`.

```sh
dotenvctl plugins
dotenvctl plugins --json | jq -r '.data.plugins[].name'
```

**Try it.** With nothing installed:

```console
$ dotenvctl plugins
no exec plugins found on PATH (binaries named dotenvctl-<name>)
$ dotenvctl plugins --json
{"ok":true,"data":{"plugins":[]}}
```

With the [`dotenvctl-export` example plugin](plugins.md#a-complete-exec-plugin-dotenvctl-export) installed in `/usr/local/bin`, plus a stray `dotenvctl-github` next to it:

```console
$ dotenvctl plugins
NAME    PATH                             STATUS
export  /usr/local/bin/dotenvctl-export  active
github  /usr/local/bin/dotenvctl-github  shadowed by built-in
```

`dotenvctl github` still runs the built-in GitHub plugin; the PATH binary is never executed. The `--json` rows carry `name`, `path` and `shadowed` (a boolean).

**Pitfalls**

- Only files with an execute bit count (on Windows, any matching file). A script you forgot to `chmod +x` is silently left out of the list and is not dispatched either.
- When the same name appears in two PATH directories, the first one wins, as with any command lookup. The list shows only that first hit.
- `help` and `completion` are built-ins too, so `dotenvctl-help` and `dotenvctl-completion` are always shadowed.
- In cli v0.1.2 and earlier, `plugins --json` with at least one plugin installed printed the human table before the envelope, so `jq` failed on the first line.

How to write one (the env-var contract, calling the host back for parsing, a complete script): [plugins overview](plugins.md#writing-your-own-plugin-exec-plugins).

## completion — shell completions

`completion` prints a tab-completion script for your shell, so `dotenvctl <TAB>` offers verbs, and `dotenvctl list --<TAB>` offers flags with their help text. Load it once per session to try it, then install it permanently.

```sh
dotenvctl completion bash          # also: zsh, fish, powershell
dotenvctl completion zsh --help    # the exact install lines for that shell
```

| Shell | Try in this session | Install for every session |
|---|---|---|
| bash (needs the `bash-completion` package) | `source <(dotenvctl completion bash)` | `dotenvctl completion bash > /etc/bash_completion.d/dotenvctl` (Linux) or `dotenvctl completion bash > $(brew --prefix)/etc/bash_completion.d/dotenvctl` (macOS) |
| zsh | `source <(dotenvctl completion zsh)` | `dotenvctl completion zsh > "${fpath[1]}/_dotenvctl"`, then start a new shell |
| fish | `dotenvctl completion fish \| source` | `dotenvctl completion fish > ~/.config/fish/completions/dotenvctl.fish` |
| powershell | `dotenvctl completion powershell \| Out-String \| Invoke-Expression` | add the output of that command to your PowerShell profile |

```console
$ dotenvctl completion zsh | head -2
#compdef dotenvctl
compdef _dotenvctl dotenvctl
```

Every shell subcommand takes `--no-descriptions` to complete names only, without the help text.

**Pitfalls**

- zsh: if completion is not enabled at all, run `echo "autoload -U compinit; compinit" >> ~/.zshrc` once first, as `dotenvctl completion zsh --help` says.
- The script completes verbs and flags. It does not complete key names from your `.env`; use `dotenvctl keys` for those.
- `completion` is a built-in, so an exec plugin named `dotenvctl-completion` can never replace it.

## Drive the CLI from Go

The whole verb tree is importable: `dotenvcmd.Execute(args, stdout, stderr, opts...)` parses `args` (without the program name), runs the verb, and returns the exit code instead of calling `os.Exit`. Use it to embed `dotenvctl` in your own tool, or to script it from Go tests without building a binary. With no options it behaves exactly like the `dotenvctl` binary; when you embed it under your own name, pass `dotenvcmd.WithProgramName` (Step 3).

**Before you start**

```sh
go get github.com/ubgo/dotenv/cli
```

**Step 1 — run a verb in-process.** Pass any writers: the output lands there exactly as on a terminal, and `--json` works the same way.

<!-- snippet: cli/dotenvcmd/example_test.go#cli_Execute -->
```go
// ExampleExecute embeds the whole dotenvctl CLI in a Go program: Execute
// takes the argument list (without the program name) and two writers, and
// returns the exit code instead of calling os.Exit.
func ExampleExecute() {
	dir, err := os.MkdirTemp("", "dotenvctl-")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() {
		// Cleanup is best-effort; a failure goes to stderr so the example's
		// checked stdout output is unaffected.
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
		}
	}()

	// -f takes any path; the file holds one plain and one derived value.
	envFile := filepath.Join(dir, ".env")
	// 0600 because env files hold credentials.
	if err := os.WriteFile(envFile, []byte("HOST=localhost\nURL=http://${HOST}:8080\n"), 0o600); err != nil {
		fmt.Println(err)
		return
	}

	code := dotenvcmd.Execute([]string{"-f", envFile, "get", "URL", "--expand"}, os.Stdout, os.Stderr)
	fmt.Println("exit code:", code)

	// A missing key is an operation failure: exit code 1. Its message names
	// the temp path, so both writers are discarded here.
	code = dotenvcmd.Execute([]string{"-f", envFile, "get", "PORT"}, io.Discard, io.Discard)
	fmt.Println("exit code:", code)
	// Output:
	// http://localhost:8080
	// exit code: 0
	// exit code: 1
}
```

**Step 2 — wrap it for a script of calls.** The examples in the same file use this small helper, which sends both streams to stdout and prints the exit code after each call:

<!-- snippet: cli/dotenvcmd/example_test.go#cli_Run -->
```go
// cliRun runs one dotenvctl invocation with both streams on stdout, so the
// Output block shows errors too, then prints the exit code.
func cliRun(args ...string) {
	code := dotenvcmd.Execute(args, os.Stdout, os.Stdout)
	fmt.Printf("exit=%d\n", code)
}
```

**Step 3 — print your own program name.** Embedded, the CLI still calls itself `dotenvctl` in its failure lines, unless you pass `WithProgramName` with the words your users type:

<!-- snippet: cli/dotenvcmd/example_test.go#cli_ExecuteProgramName -->
```go
// ExampleExecute_programName mounts the CLI inside a host program under the
// words its users type, "myapp env": WithProgramName puts that name in the
// lines the CLI prints about itself, so a failure never names a dotenvctl
// binary the user does not have.
func ExampleExecute_programName() {
	// No such verb: a usage failure, exit code 2, with the host's name in the
	// message and in the hint. Stderr goes to stdout here so it is checked.
	code := dotenvcmd.Execute([]string{"nosuchverb"}, os.Stdout, os.Stdout,
		dotenvcmd.WithProgramName("myapp env"))
	fmt.Println("exit code:", code)
	// Output:
	// myapp env: unknown command "nosuchverb" for "myapp env"
	// Run 'myapp env --help' for usage.
	// exit code: 2
}
```

The name reaches the `<name>: ` failure prefix, the `Run '<name> --help'` hint, the help Usage lines and the `--version` line. The command examples inside `--help` still say `dotenvctl`, and exec plugins are still found as `dotenvctl-<name>` binaries. In cli v0.1.2 and earlier there was no option and the prefix was always `dotenvctl:`.

**Try it**

```console
$ cd cli && go test -run '^ExampleExecute$' -v ./dotenvcmd/
=== RUN   ExampleExecute
--- PASS: ExampleExecute (0.00s)
PASS
ok  	github.com/ubgo/dotenv/cli/dotenvcmd	0.006s
```

Drop the `$` from the pattern (`-run '^ExampleExecute'`) to run all twelve.

Every verb on this page has a runnable example in [`cli/dotenvcmd/example_test.go`](../cli/dotenvcmd/example_test.go), each over the same four-file family (`.env`, `.env.staging`, `.env.prod`, `.env.example`) in a temp directory:

| Example | Verbs |
|---|---|
| `ExampleExecute` | embedding, exit codes |
| `ExampleExecute_programName` | embedding under your own program name |
| `ExampleExecute_get` | `get`, `get --expand`, a disabled key |
| `ExampleExecute_set` | `set --dry-run`, `set`, `--after`, a missing anchor |
| `ExampleExecute_unset` | `unset`, `restore`, `unset --delete --dry-run` |
| `ExampleExecute_list` | `list`, `list --disabled`, `list --prefix --strip-prefix --json` |
| `ExampleExecute_keys` | `keys` and its selection flags |
| `ExampleExecute_envs` | `envs` |
| `ExampleExecute_matrix` | `matrix --only-drift`, `matrix --contract` |
| `ExampleExecute_matrix_placeholder` | `matrix --placeholder` |
| `ExampleExecute_diff` | `diff`, both exit codes |
| `ExampleExecute_json` | the `--json` envelope for `get`, `diff`, `envs` |

The verbs themselves are thin translators over the `envkit` package, which you can call directly for typed results instead of text. See the [Go API guide](go-api.md).

## Pitfalls

These apply to every verb.

- **Failure messages go to stderr in human mode.** When a verb fails (a missing key, a missing anchor), the `dotenvctl: …` line is written to stderr and stdout stays empty, so `VALUE=$(dotenvctl get NOPE)` leaves `VALUE` empty. An empty value is also a valid value, so check the exit code before using captured output: `if VALUE=$(dotenvctl get KEY); then …; fi`. In cli v0.1.2 and earlier, human-mode failures (but not usage errors) went to stdout.
- **Flags after `--` belong to the child.** `dotenvctl run -- cmd --json` passes `--json` to `cmd`.
- **A missing file reads as empty.** `get` reports `not found`, `keys` and `list` print nothing and exit `0`, and `set` creates the file. A typo in `-f` is not reported as a missing file.
- **`--json` is exactly one envelope per run**, on stdout, including usage errors (`{"ok":false,"error":{"code":"usage",…}}`, exit `2`) and a plugin `push`/`prune` that fails part-way (an `ok:false` envelope whose `data` holds the partial record, see [Vercel failure behavior](plugins/vercel.md#failure-behavior)). Two kinds of output are not envelopes. `--version`, `help` (and `--help` on any verb) and `completion` print plain text even with `--json`, because their output is the version line, the help text or a shell script. `run` and exec plugins own their output: `run` passes the child's stdout through, and an exec plugin prints whatever it prints. In cli v0.1.2 and earlier, usage errors printed plain text on stderr, and the two cases above printed more than one line.

## Related

- [Getting started](getting-started.md) — install and the 60-second tour
- [Multi-environment tools](multi-env.md) — `envs`, `matrix`, HTML reports in depth
- [Recipes](recipes.md) — complete workflows built from these verbs
- [Plugins](plugins.md), [GitHub plugin](plugins/github.md), [Vercel plugin](plugins/vercel.md)
- [Go API guide](go-api.md) — `envkit`, `providerkit` and the helpers behind the verbs
- [Feature index](reference/features.md) — every feature with its guide and example
