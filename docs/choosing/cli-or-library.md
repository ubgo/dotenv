# Choosing between the CLI, the library and embedding

The same engine is reachable four ways. Pick by where your code runs and how much of the tool you want.

**On this page:** [Decision table](#decision-table) · [If X, use Y](#if-x-use-y) · [Pitfalls](#pitfalls) · [Related](#related)

## Decision table

| Way in | What you get | Dependencies | Use when | Guide | Example |
|---|---|---|---|---|---|
| `dotenvctl` (the CLI) | every verb from a shell: `get`, `set`, `unset`, `restore`, `list`, `keys`, `run`, `diff`, `envs`, `matrix`, `github`, `vercel`, `plugins` | a binary on `PATH` | scripts, CI jobs, Makefiles, people at a terminal | [getting started](../getting-started.md), [command reference](../commands.md) | [`ExampleExecute`](../../cli/dotenvcmd/example_test.go) |
| `github.com/ubgo/dotenv` (the library) | parse, read, edit, expand and save one file, plus plugins | none: standard library only; Go 1.22 or later | a Go program that reads or writes `.env` files | [reading](../library/reading.md), [editing](../library/editing.md) | [`Example_quickstart`](../../example_readme_test.go) |
| `github.com/ubgo/dotenv/cli/envkit` | the CLI's operations as Go functions: selection, discovery, the drift matrix, diff, byte-preserving edits, run | the `cli` module (cobra, pflag, x/term in its module graph); its `go.mod` asks for a newer Go than the library's | multi-file features (matrix, diff, selection) inside your own Go program | [Go API, envkit](../go-api.md#envkit--the-operations-api) | [`ExampleBuildMatrix`](../../cli/envkit/example_test.go), [`ExampleSelectFile`](../../cli/envkit/example_test.go) |
| `dotenvcmd.Execute(args, stdout, stderr)` | the whole CLI in one function call, returning the exit code; never calls `os.Exit` | the `cli` module | shipping `dotenvctl` inside your own binary, or driving it in-process from tests | [Go API, embedding](../go-api.md#embedding-the-cli) | [`ExampleExecute`](../../cli/dotenvcmd/example_test.go), [`ExampleExecute_json`](../../cli/dotenvcmd/example_test.go) |

The [CLI ↔ Go equivalence table](../go-api.md#cli--go-equivalence) maps every verb to its function.

## If X, use Y

- **If you work from a shell or a CI step**, use `dotenvctl`. Add `--json` for a stable `{"ok":…}` envelope and rely on exit codes `0` / `1` / `2` ([getting started](../getting-started.md#scripting---json-and-exit-codes)).
- **If your Go program reads its own configuration**, use the library. It has no dependencies, so it adds nothing to your module graph.
- **If your Go program edits a user's `.env`** (a scaffolder, an installer, a dev tool), use the library: `Open`, `Set`, `Save`. Reach for `envkit.Set` only if you also want its dry-run diff and options ([`ExampleSet`](../../cli/envkit/example_test.go)).
- **If you need to compare several files** (drift, contracts, diffs) from Go, use `envkit`: `BuildMatrix`, `Diff`, `DriftRows`. The library works on one file at a time and never merges.
- **If you need to run a child process with a file's values**, use `dotenvctl run` from a shell or `envkit.Run` from Go ([`ExampleRun`](../../cli/envkit/example_test.go)). Neither changes the calling process's environment.
- **If you want all of `dotenvctl` under your own command name**, call `dotenvcmd.Execute`. To mount only the GitHub or Vercel verbs, use `githubplugin.New()` / `vercelplugin.New()` instead ([mounting the plugins](../go-api.md#mounting-the-github-and-vercel-plugins)).
- **If you test code that shells out to `dotenvctl`**, call `dotenvcmd.Execute` with buffers instead: no binary, no subprocess, the same output.

## Pitfalls

- **The library never touches `os.Environ`.** It hands you values; applying them to a process, with whatever precedence you want, is your decision. `dotenvctl run` and `envkit.Run` build a child's environment instead.
- **Embedded, `--version` reports the CLI module's version, not your program's.** `--version` and `DOTENVCTL_VERSION` (passed to exec plugins) show the version of `github.com/ubgo/dotenv/cli` your binary was built with ([Go API](../go-api.md)). In cli v0.1.2 and earlier they showed the host program's module version.
- **Embedded with `dotenvcmd.Execute`, failure messages say `dotenvctl:` unless you name your program.** Pass `dotenvcmd.WithProgramName("myapp env")` to put your name in failure lines, the `--help` hint, the help Usage lines and `--version`; code that prints through its own `outfmt.Printer` sets `Program` instead ([Go API](../go-api.md#embedding-the-cli)). The command examples inside `--help` still say `dotenvctl`. In cli v0.1.2 and earlier neither existed and the prefix was fixed.
- **Check the exit code, not captured output.** Human-mode failures go to stderr, so `v=$(dotenvctl get NOPE)` leaves `v` empty, and an empty value is also a valid value. In cli v0.1.2 and earlier some failures went to stdout and ended up in `v`.
- **`envkit` options can switch expansion off.** A non-nil `Select` in `RunOptions` or `ChildEnv` turns expansion off unless you also set `Expand` ([Go API, running a command](../go-api.md#running-a-command-with-the-files-values)).

## Related

- [Getting started](../getting-started.md) · [Command reference](../commands.md) · [Go API](../go-api.md)
- [Which sync backend](sync-backend.md) · [Options for your kind of app](app-setups.md)
- [All choosing guides](README.md) · [Feature index](../reference/features.md)
