# Command reference

Global flags on every command: `-f/--file PATH` (default `./.env`) and `--json` (machine envelope). Exit codes: `0` ok · `1` operation failed · `2` usage — exceptions noted per command.

## get — print one value

```sh
dotenvctl get DATABASE_URL
dotenvctl get DATABASE_URL --expand        # resolve ${references} against the file
dotenvctl -f .env.prod get PORT --json
```

Exits `1` when the key has no active entry. A commented-out setting (`# KEY=…`) and a name-only declaration (`KEY` alone) do not count — they are inactive, and `list --disabled` / `list --inherited` is how you see them.

## set — upsert values, byte-preserving

```sh
dotenvctl set DB_HOST=localhost DB_PORT=5432   # multiple KEY=VALUE args
dotenvctl set CONN="postgres://u:p@h/db?x=1"   # values may contain '=' — only the first splits
dotenvctl set DB_PASSWORD=secret --after DB_PORT     # place a NEW key after an anchor
dotenvctl set DB_DRIVER=postgres --before DB_HOST    # …or before one
dotenvctl set DEBUG=true --dry-run             # print the would-be line diff, write nothing
```

Rules worth knowing:

- An existing key updates in place and never moves; only a new key is placed by `--after`/`--before`. A missing anchor exits `1` without writing anything.
- Setting a key to its current value is a full no-op — same bytes, same mtime.
- Everything the author wrote around the value survives: `export` prefixes, spacing, inline comments, and the `=` vs `:` delimiter style.
- A missing file is created with `0600` permissions.

`--dry-run` output is a minimal `-old`/`+new` line diff:

```
-DEBUG=false
+DEBUG=true
```

## unset — deactivate a key

```sh
dotenvctl unset OLD_KEY              # default: comments it out — '# OLD_KEY=…', reversible
dotenvctl unset OLD_KEY --delete     # removes the line entirely
dotenvctl unset A B C --dry-run      # preview, write nothing
```

Comment-out is the default on purpose: it is reversible, diff-friendly, and documentation above the setting stays attached to something.

## restore — the inverse of unset

```sh
dotenvctl restore OLD_KEY
dotenvctl list --disabled            # see what is restorable
```

Removes exactly the comment marker that was added, so `#KEY=v` comes back without inventing a space. Unset followed by restore returns the file to its original bytes.

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

Without selection flags, `list` shows the file as a linear read would: file order, duplicate keys included. With any selection flag it switches to the effective (last-wins) view — selected, optionally renamed.

## keys — names only, pipe-friendly

```sh
dotenvctl keys
dotenvctl keys --exclude-prefix GITHUB_SECRET_   # app-config names only
dotenvctl keys | grep '^DB_'
dotenvctl keys --json | jq -r '.data.keys[]'
```

## run — execute a command with the file's values

```sh
dotenvctl run -- npm start
dotenvctl -f .env.test run -- go test ./...
dotenvctl run -- sh -c 'echo $DATABASE_URL'
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

Output: `+ KEY=…` only in the second file · `- KEY=…` only in the first · `~ KEY: a -> b` changed. Exit `0` identical, `1` different (like `diff(1)`), `2` trouble. Comparison is over the effective last-wins view — formatting, comments, and shadowed duplicates are invisible on purpose. `--format human|json|html` and `-o`/`--output PATH` control output; HTML reports are secrets-masked unless `--reveal` — see [Multi-environment tools](multi-env.md#html-reports).

## Commands documented elsewhere

This page covers the single-file verbs. The rest of the surface:

| Command | Guide |
|---|---|
| `envs` — discover the directory's `.env` family | [Multi-environment tools](multi-env.md#envs--discover-the-family) |
| `matrix` — keys × environments drift table, `--contract` CI gate, HTML reports | [Multi-environment tools](multi-env.md#matrix--the-drift-table) |
| `github push/list/prune/env-create` — GitHub Actions secrets | [GitHub plugin](plugins/github.md) |
| `vercel push/list/prune` — Vercel environment variables | [Vercel plugin](plugins/vercel.md) |
| `plugins` — list third-party exec plugins found on PATH | [Plugins overview](plugins.md#writing-your-own-plugin-exec-plugins) |
| *(every verb, as a Go function)* | [Go API](go-api.md) |
| `completion` — shell completions (bash, zsh, fish, powershell) | built in via cobra; `dotenvctl completion --help` |
