# Choosing guides

Short decision guides for the questions people ask before they write code: which call, which option, which tool, which backend.

Each guide has a decision table, "if X, use Y" rules, the pitfalls that catch people, and for every option a link to its full guide and a runnable `Example`. Every claim was checked against the code and its tests.

| Guide | Answers |
|---|---|
| [How to read a file](reading.md) | `Read` vs `Open` vs `Parse` vs `ParseReader`; which accessor for which question |
| [How to edit a file](editing.md) | the `Set` family vs `Append` / `Insert*`; `Unset` vs delete; `Clone` vs rebuilding; `Save` vs `SaveAs` |
| [Which expansion setup](expansion.md) | `Get` vs `GetExpanded` vs `ExpandedMap`; which `${…}` operator; escape dialect; command substitution on or off |
| [Which plugin hook](plugins.md) | which option or capability for a need: env fallback, secret stores, decryption, save rules, auditing, sandboxed commands |
| [CLI, library or embedding](cli-or-library.md) | `dotenvctl` vs the `dotenv` package vs `envkit` vs `dotenvcmd.Execute` |
| [Which sync backend](sync-backend.md) | GitHub vs Vercel vs your own exec plugin vs a `providerkit` plugin |
| [Options for your kind of app](app-setups.md) | a 12-factor service, a CLI tool that edits files, a CI pipeline, a monorepo, Docker Compose compatibility |

## The fastest answers

| If you want to… | Use | Guide | Example |
|---|---|---|---|
| Load config values and never write the file | `dotenv.Read(path)` | [reading, step 1](../library/reading.md#step-1--read-the-values-in-one-call) | [`ExampleRead`](../../example_read_test.go) |
| Change a value and save without losing comments | `Open` → `Set` → `Save` | [editing, step 1](../library/editing.md#step-1--change-or-add-a-value-with-set) | [`ExampleFile_Set`](../../example_edit_test.go) |
| Generate a `.env.example` with sections | `NewComment` / `NewPair` / `NewBlank` + `Append` | [editing, step 7](../library/editing.md#step-7--generate-content-with-the-constructors) | [`ExampleFile_Append`](../../example_edit_test.go) |
| Derive `.env.prod` from `.env.staging` | `Clone` → `Set` → `SaveAs` | [editing, step 9](../library/editing.md#step-9--derive-another-environment-with-clone) | [`ExampleFile_Clone`](../../example_edit_test.go) |
| Bind values to a typed struct | `Read` + a binder | [typed configuration](../library/typed-config.md) | [`Example_typedConfig`](../../example_plugin_test.go) |
| Fail CI when an environment misses a key | `dotenvctl matrix --contract .env.example` | [recipes](../recipes.md#ci-gate-the-build-on-env-completeness) | [`ExampleBuildMatrix`](../../cli/envkit/example_test.go) |
| Push part of a file to GitHub Actions secrets | `dotenvctl github push --prefix …` | [GitHub plugin](../plugins/github.md) | [`ExamplePlugin_Command`](../../cli/plugins/githubplugin/example_test.go) |

## Related

- [Documentation home and learning path](../README.md)
- [Feature index](../reference/features.md): every feature with its guide and example
