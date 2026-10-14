# Choosing options for your kind of app

Starting points for five common setups: which calls, which options, and which commands to wire in. Each one links to the guide and a runnable example for every piece.

**On this page:** [At a glance](#at-a-glance) · [A 12-factor service](#a-12-factor-service) · [A CLI tool that edits files](#a-cli-tool-that-edits-files) · [A CI pipeline](#a-ci-pipeline) · [A monorepo or shared file](#a-monorepo-or-shared-file) · [Docker Compose compatibility](#docker-compose-compatibility) · [Related](#related)

## At a glance

| App | Read with | Options | Also use |
|---|---|---|---|
| 12-factor service | `Read` + a binder | `WithLookup(os.LookupEnv)` if references may point at the environment | `${VAR:?msg}` for required values |
| CLI tool that edits files | `Open` → `Set` / `Unset` → `Save` | `WithSaveGuard` for rules the file must keep | `SetAfter` to keep keys grouped |
| CI pipeline | `dotenvctl` | `--json`, exit codes | `matrix --contract`, `diff`, `run` |
| Monorepo or shared file | `dotenvctl` selection flags or `envkit.SelectFile` | `--prefix`, `--strip-prefix` | `github push` / `vercel push` |
| Docker Compose compatibility | `Open` or `Read` | `WithEscapes(EscapeCompose)` | `Inherited()` for name-only lines |

## A 12-factor service

Configuration lives in the environment; a `.env` file fills it in for local development.

1. Read the file with `dotenv.Read(path)` ([`ExampleRead`](../../example_read_test.go)). A missing file is an error; if the file is optional in production, use `Open` and check `Existed()` ([`ExampleFile_Existed`](../../example_read_test.go)).
2. Apply your own precedence. In 12-factor, the process environment wins over the file. This package never reads `os.Environ`, so do it in your code: for each key, prefer `os.LookupEnv`, fall back to the file's value. `WithLookup(os.LookupEnv)` is a different thing: it only fills `${VAR}` references the file does not define.
3. Bind to a typed struct with the standard library, go-envconfig, caarlos0/env or cfgkit ([typed configuration](../library/typed-config.md), [`Example_typedConfig`](../../example_plugin_test.go)). cfgkit does the layering of step 2 for you.
4. Mark values that must exist as `${VAR:?message}` in the file, so a missing one fails at startup with a `*RequiredError` ([`ExampleRequiredError`](../../example_expand_test.go)).

**Pitfall:** go-envconfig stops at the first field it cannot decode; caarlos0/env and cfgkit report every error at once.

## A CLI tool that edits files

A scaffolder, an installer or a dev tool that changes a user's `.env`.

1. `Open` the file; a missing one opens empty and `Save` creates it at `0600` ([`ExampleOpen_missing`](../../example_read_test.go)).
2. Edit with the upsert family: `Set`, `SetAfter`, `SetBefore` ([how to edit](editing.md)). Re-setting an unchanged value is a no-op, so running your tool twice changes nothing ([`ExampleFile_Set_noop`](../../example_edit_test.go)).
3. Turn settings off with `Unset(key, false)` so the user can `Restore` them ([`ExampleFile_Restore`](../../example_edit_test.go)).
4. Add a `WithSaveGuard` for invariants the file must keep, such as "no plaintext passwords" ([`ExampleWithSaveGuard`](../../example_plugin_test.go)).
5. `Save`. It is atomic and keeps the file's mode ([saving](../library/saving.md)).

**Pitfalls:** `Set` stores values literally, so it cannot write a reference such as `${HOST}`; `Append` a parsed line for that ([`ExampleFile_Append_verbatim`](../../example_edit_test.go)). It re-picks the value's quoting ([`ExampleFile_Set_quoting`](../../example_edit_test.go)).

## A CI pipeline

1. Gate on completeness: `dotenvctl matrix --contract .env.example` exits `1` when an environment misses a key the contract lists ([recipe](../recipes.md#ci-gate-the-build-on-env-completeness), [`ExampleBuildMatrix`](../../cli/envkit/example_test.go)).
2. Review drift: `dotenvctl diff .env.staging .env.prod` exits `1` on any difference, like `diff(1)` ([`ExampleExecute_diff`](../../cli/dotenvcmd/example_test.go)).
3. Run tests against a specific file: `dotenvctl -f .env.test run -- go test ./...` passes the child's exit code through ([recipe](../recipes.md#run-testsservices-against-a-specific-env-file)).
4. Parse output with `--json`: one `{"ok":…}` envelope per call ([`ExampleExecute_json`](../../cli/dotenvcmd/example_test.go)).
5. Sync secrets with `github push` or `vercel push` and `--yes` ([which sync backend](sync-backend.md)).

**Pitfalls:** check exit codes rather than captured text, since a failing verb prints nothing on stdout and an empty value is valid too; with `--json`, a partly failed push prints one `ok:false` envelope whose `data` lists what was written.

## A monorepo or shared file

One file often serves several audiences (app config for the container, deploy credentials for CI), split by a naming convention.

1. Name each audience with a prefix: `GITHUB_SECRET_DEPLOY_KEY`, `APP_DB_URL`.
2. Preview one audience: `dotenvctl list --prefix GITHUB_SECRET_ --strip-prefix` ([selection grammar](../commands.md#selecting-keys--the-shared-grammar), [`ExampleExecute_list`](../../cli/dotenvcmd/example_test.go)).
3. Push exactly that selection: `dotenvctl github push --prefix GITHUB_SECRET_ --strip-prefix` ([recipe](../recipes.md#one-file-several-audiences-the-monorepo-split)).
4. From Go, the same selection is `envkit.SelectFile` with `SelectOptions{Prefix: …, StripPrefix: true}` ([`ExampleSelectFile`](../../cli/envkit/example_test.go)).

**Pitfall:** `--strip-prefix` combined with `--include` can give two keys the same name after stripping. Preview with `list` first.

## Docker Compose compatibility

The parser already follows Compose's grammar: the full `${…}` operator set, `$$`, the key charset, the `:` delimiter and name-only lines ([syntax](../library/syntax.md)). For files Compose also reads:

1. Use `WithEscapes(EscapeCompose)`, so `\n` inside double quotes stays two characters, as in Compose ([`ExampleEscapeCompose`](../../example_syntax_test.go)).
2. Handle name-only lines (`HOME`, `AWS_SECRET_ACCESS_KEY`) yourself: `f.Inherited()` lists them; Compose fills them from the environment, this package does not ([`ExampleFile_Inherited`](../../example_edit_test.go)).
3. Add `WithLookup(os.LookupEnv)` if references in the file should fall back to the environment, as they do in Compose ([`ExampleWithLookup`](../../example_plugin_test.go)).

**Pitfall:** a line Compose would reject (`K!=v`, `HAS KEY=v`) is kept verbatim here as `KindOther` rather than failing the whole file ([`ExampleParse_syntaxKeys`](../../example_syntax_test.go)).

## Related

- [How to read a file](reading.md) · [How to edit a file](editing.md) · [Which expansion setup](expansion.md) · [Which plugin hook](plugins.md) · [CLI, library or embedding](cli-or-library.md) · [Which sync backend](sync-backend.md)
- [Recipes](../recipes.md) · [Typed configuration](../library/typed-config.md)
- [All choosing guides](README.md) · [Feature index](../reference/features.md)
