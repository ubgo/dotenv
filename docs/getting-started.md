# Getting started

Install `dotenvctl`, edit your first `.env` file without disturbing a byte you did not touch, and find the next guide for what you want to do.

On this page: [Before you start](#before-you-start) · [Install](#install) · [The 60-second tour](#the-60-second-tour) · [Choosing the file: `-f`](#choosing-the-file--f) · [Variable expansion: `--expand`](#variable-expansion---expand) · [Scripting: `--json` and exit codes](#scripting---json-and-exit-codes) · [Pitfalls](#pitfalls) · [What next](#what-next)

`dotenvctl` is a command-line tool for `.env` files: read a value, set one, comment one out, compare environments, run a program with a file's values, and push values to secret stores. It is built on the `github.com/ubgo/dotenv` Go library, which you can also use directly from Go (see [What next](#what-next)).

## Before you start

- Go, to install with `go install`. The CLI module declares Go 1.26.4; with an older Go and the default `GOTOOLCHAIN=auto`, `go install` downloads that toolchain first. The library on its own needs only Go 1.22.
- A shell. Every example below runs in an empty directory, so you can follow along without touching a real project.

## Install

```sh
go install github.com/ubgo/dotenv/cli/cmd/dotenvctl@main
dotenvctl --version
```

`--version` prints the module version stamped into the binary at build time (`dotenvctl version …`). Make sure Go's install directory (`$(go env GOPATH)/bin` unless you set `GOBIN`) is on your PATH, or the shell will not find `dotenvctl`.

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

**Try it** in an empty directory. Mutating verbs print nothing on success; the file shows what happened:

```console
$ dotenvctl set DB_HOST=localhost DB_PORT=5432
$ dotenvctl get DB_HOST
localhost
$ dotenvctl keys
DB_HOST
DB_PORT
$ dotenvctl list
DB_HOST  localhost
DB_PORT  5432
$ dotenvctl unset DB_PORT
$ dotenvctl restore DB_PORT
$ cat .env
DB_HOST=localhost
DB_PORT=5432
$ dotenvctl set DEBUG=true --dry-run
+DEBUG=true
$ dotenvctl run -- sh -c 'echo "child sees $DB_HOST:$DB_PORT"'
child sees localhost:5432
```

The `--dry-run` line is the whole change the write would make, as a minimal `-old`/`+new` diff; nothing was written. The point of this tool over `sed`/hand-editing: your `.env` files keep their comments, section banners, blank lines, and quoting exactly as written. Only the line you touch changes — verify it yourself with `--dry-run` on any mutating command.

Every verb in this tour also has a runnable Go example in [`cli/dotenvcmd/example_test.go`](../cli/dotenvcmd/example_test.go) (`ExampleExecute_get`, `ExampleExecute_set`, `ExampleExecute_unset`, `ExampleExecute_list`, `ExampleExecute_keys`).

## Choosing the file: `-f`

Every command takes `-f`/`--file` (default `./.env`):

```sh
dotenvctl -f .env.production get DATABASE_URL
dotenvctl -f deploy/.env.staging set REGION=eu-west-1
```

Reading a missing file reports keys as not found; `set` bootstraps a missing file with `0600` permissions (env files hold credentials — restrictive by default).

```console
$ dotenvctl -f .env.missing get X; echo "exit=$?"
dotenvctl: key "X" not found in .env.missing
exit=1
```

## Variable expansion: `--expand`

Values may reference other keys — `URL=${HOST}:${PORT}` — with the full Docker Compose interpolation syntax (defaults `${VAR:-x}`, requireds `${VAR:?msg}`, and so on). Reads return the raw text by default; `--expand` resolves:

```console
$ cat .env.x
HOST=localhost
PORT=5432
URL=${HOST}:${PORT}
$ dotenvctl -f .env.x get URL
${HOST}:${PORT}
$ dotenvctl -f .env.x get URL --expand
localhost:5432
```

The raw form is what is on disk; the expanded form is what a consumer sees. A failing `${VAR:?message}` makes the command exit `1` with the message — that is the point of the `:?` form.

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

```console
$ dotenvctl get; echo "exit=$?"
dotenvctl: accepts 1 arg(s), received 0
Run 'dotenvctl --help' for usage.
exit=2
```

[Error `code` values](ds:block?id=errcodes-tjhb87tm) are stable API: `not_found` · `io` · `required` · `usage` · `exec`. The envelope for every verb is pinned by [`ExampleExecute_json`](../cli/dotenvcmd/example_test.go).

## Pitfalls

- **Check the exit code, not the output.** A failing verb prints `dotenvctl: …` on stderr and nothing on stdout, so `VALUE=$(dotenvctl get KEY)` leaves `VALUE` empty when the key is missing, and an empty value is also a valid value. Test the status: `if VALUE=$(dotenvctl get KEY); then …; fi`. Before cli v0.1.3, human-mode failures were printed on stdout and ended up inside `VALUE`.
- **A typo in `-f` is not an error for reads.** A missing file reads as empty: `get` says `not found`, `keys` and `list` print nothing and exit `0`.
- **Reads are raw unless you ask.** `get` and `list` print `${HOST}` literally; add `--expand` when you want what a program would see. `run` and the plugin pushes expand by default.
- **Commented-out and name-only lines are not active.** `# KEY=value` and a bare `KEY` are invisible to `get`, `keys` and plain `list`; `list --disabled` and `list --inherited` show them.

## What next

**Using the CLI**

- Selecting subsets of a file (`--prefix`, `--exclude`, multi-value syntax): [the shared selection grammar](commands.md#selecting-keys--the-shared-grammar)
- Full flag-by-flag detail, `keys`, `plugins`, `completion` and `--version`: [Command reference](commands.md)
- Comparing environments and generating reports: [Multi-environment tools](multi-env.md)
- Complete copy-paste workflows (CI gates, env cloning, secret sync): [Recipes](recipes.md)
- Syncing secrets outward: [Plugins](plugins.md), with the [GitHub](plugins/github.md) and [Vercel](plugins/vercel.md) guides
- Writing your own subcommand in any language: [exec plugins](plugins.md#writing-your-own-plugin-exec-plugins)

**Using the Go library** (`go get github.com/ubgo/dotenv`)

- Read a file into a map, or open it to inspect keys, duplicates and disabled lines: [Reading files](library/reading.md)
- Change values in place without touching anything else, and save: [Editing files](library/editing.md)
- Exactly how quotes, comments, `export` and multi-line values parse: [Syntax](library/syntax.md)
- `${VAR}`, defaults, required variables and how expansion resolves: [Expansion](library/expansion.md)
- Hook into reads, expansion and saves with plugins: [Library plugins](library/plugins.md)

**Embedding the CLI or building on its packages**

- Run any verb in-process with `dotenvcmd.Execute`: [Drive the CLI from Go](commands.md#drive-the-cli-from-go)
- `envkit`, `providerkit` and the helper packages: [Go API guide](go-api.md)
- Every feature, with its guide and example: [Feature index](reference/features.md)
