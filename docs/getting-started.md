# Getting started

## Install

```sh
go install github.com/ubgo/dotenv/cli/cmd/dotenvctl@latest
dotenvctl --version
```

## The 60-second tour

```sh
cd your-project                      # commands default to ./.env

dotenvctl set DB_HOST=localhost DB_PORT=5432   # creates .env (0600) if missing
dotenvctl get DB_HOST                          # → localhost
dotenvctl keys                                 # names, one per line
dotenvctl list                                 # aligned KEY VALUE table

dotenvctl unset DB_PORT                        # comments it out: '# DB_PORT=5432' — reversible
dotenvctl restore DB_PORT                      # …and back, byte-identical

dotenvctl run -- npm start                     # child process gets the file's values
```

The point of this tool over `sed`/hand-editing: your `.env` files keep their comments, section banners, blank lines, and quoting exactly as written. Only the line you touch changes — verify it yourself with `--dry-run` on any mutating command.

## Choosing the file: `-f`

Every command takes `-f`/`--file` (default `./.env`):

```sh
dotenvctl -f .env.production get DATABASE_URL
dotenvctl -f deploy/.env.staging set REGION=eu-west-1
```

Reading a missing file reports keys as not found; `set` bootstraps a missing file with `0600` permissions (env files hold credentials — restrictive by default).

## Variable expansion: `--expand`

Values may reference other keys — `URL=${HOST}:${PORT}` — with the full Docker Compose interpolation syntax (defaults `${VAR:-x}`, requireds `${VAR:?msg}`, and so on). Reads return the raw text by default; `--expand` resolves:

```sh
dotenvctl get URL             # ${HOST}:${PORT}   — what is on disk
dotenvctl get URL --expand    # localhost:5432    — what a consumer sees
```

A failing `${VAR:?message}` makes the command exit `1` with the message — that is the point of the `:?` form.

## Scripting: `--json` and exit codes

Every command supports `--json`: exactly one envelope object on stdout.

```sh
dotenvctl get DB_HOST --json
# {"ok":true,"data":{"key":"DB_HOST","value":"localhost","expanded":false}}

dotenvctl get NOPE --json
# {"ok":false,"error":{"code":"not_found","message":"key \"NOPE\" not found in .env"}}
# exit code: 1
```

Exit codes: `0` success · `1` the operation ran and failed (missing key, required-var error, diff found differences) · `2` the invocation was malformed. `run` is the exception by design: it passes the child's exit code through untouched, so `dotenvctl run -- make test` behaves exactly like `make test` to CI.

Error `code` values are stable API: `not_found` · `io` · `required` · `usage` · `exec`.

## Next

- Selecting subsets of a file (`--prefix`, `--exclude`, multi-value syntax): [the shared selection grammar](commands.md#selecting-keys--the-shared-grammar)
- Full flag-by-flag detail: [Command reference](commands.md)
- Comparing environments and generating reports: [Multi-environment tools](multi-env.md)
- Syncing secrets outward: [Plugins](plugins.md)
