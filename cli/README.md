# dotenvctl

The command-line companion to [`github.com/ubgo/dotenv`](../README.md) — a comment-preserving `.env` control tool. Every byte outside the entry you touch survives verbatim: comments, blank lines, ordering, quoting style, even the author's `=` vs `:` delimiter. It never modifies its own process environment.

```sh
go install github.com/ubgo/dotenv/cli/cmd/dotenvctl@latest
```

## At a glance

```sh
dotenvctl get DATABASE_URL --expand            # read, with ${references} resolved
dotenvctl set DB_HOST=db.prod --after DB_PORT  # comment-preserving upsert, anchored placement
dotenvctl unset OLD_KEY && dotenvctl restore OLD_KEY   # reversible disable — byte-exact round trip
dotenvctl run -- npm start                     # child gets the file's values; your env untouched

dotenvctl list --prefix GITHUB_SECRET_ --strip-prefix --json   # read one audience of a shared file
dotenvctl envs                                 # discover .env / .env.staging / .env.prod / …
dotenvctl matrix --only-drift                  # keys × environments drift table
dotenvctl diff .env.staging .env.prod          # effective-config diff, exit 1 on difference
dotenvctl matrix --format html -o envs.html    # shareable report — secrets masked by default

dotenvctl github push --prefix GITHUB_SECRET_ --strip-prefix   # sync to GitHub Actions secrets
```

Full documentation: [docs/](../docs/README.md) — [getting started](../docs/getting-started.md) · [command reference](../docs/commands.md) · [multi-env tools](../docs/multi-env.md) · [plugins](../docs/plugins.md) · [GitHub plugin](../docs/plugins/github.md).

## Why this over the usual suspects

- **Edits don't destroy files.** Parse-to-map tools rewrite your `.env` and lose every comment and blank line. Here, `set` on a commented, hand-formatted file changes exactly one line — pinned by fuzz tests in the underlying library.
- **Safe by default with secrets.** Values are masked in shareable output (matrix `--values`, JSON, HTML reports) unless you pass `--reveal`; plugin pushes print names and actions, never values; placeholder values (`__YOU__`) are never pushed silently; secret values reach backend CLIs via stdin, never argv.
- **Script-grade interface.** `--json` envelope on every command, stable error codes, `0/1/2` exit convention, `run` passing the child's exit code through verbatim.
- **A unique binary name.** `dotenvctl` collides with nothing on npm, PyPI, Homebrew, crates, or RubyGems — no PATH roulette with the Node/Python/Ruby dotenv tools.

## Development

This directory is its own Go module, so cobra and CLI dependencies never enter the library's dependency graph — the library stays stdlib-only.

```sh
task cli:build      # → cli/bin/dotenvctl        (from the repo root)
task cli:test       # race-enabled tests
task cli:ci         # fmt-check + vet + race tests
task ci             # the full gate: library AND cli
```

Layout: `cmd/dotenvctl` (main), `dotenvcmd` (verb tree — exported, embeddable), `providerkit` (plugin seam), `plugins/githubplugin`, `internal/{discover,outfmt,report,textdiff}`.
