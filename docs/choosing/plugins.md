# Choosing a plugin hook

Plugins let code outside this package take part in reading, expanding and saving a file. Pick the hook by the moment you need to act, then pick a function option or a plugin type.

**On this page:** [Hook by need](#hook-by-need) · [Function option or plugin type](#function-option-or-plugin-type) · [If X, use Y](#if-x-use-y) · [Pitfalls](#pitfalls) · [Related](#related)

## Hook by need

| You need to… | Capability | Runs | Function option | Guide | Example |
|---|---|---|---|---|---|
| fill a `${VAR}` the file does not define (environment, secret store) | `Lookuper` — `Lookup(name) (value, ok, err)` | during expansion, only for names the file does not define; first plugin that finds it wins | `WithLookup(fn)` (no error return) | [plugins, step 1](../library/plugins.md#step-1--fill-missing-references-with-withlookup) | [`ExampleWithLookup`](../../example_plugin_test.go) |
| decrypt or rewrite a value as it is read | `ValueTransformer` — `TransformValue(key, value) (string, error)` | after a value is expanded; every transformer runs, each on the previous one's output | `WithValueTransform(fn)` | [plugins, step 2](../library/plugins.md#step-2--decrypt-values-with-withvaluetransform) | [`ExampleWithValueTransform`](../../example_plugin_test.go) |
| refuse a save (plaintext secrets, missing keys) | `SaveGuard` — `GuardSave(f) error` | before `Save` / `SaveAs` writes; the first error aborts, nothing is written | `WithSaveGuard(fn)` | [plugins, step 3](../library/plugins.md#step-3--refuse-bad-writes-with-withsaveguard) | [`ExampleWithSaveGuard`](../../example_plugin_test.go) |
| run `$(command)` through your own code | `CommandRunner` — `RunCommand(cmd) (string, error)` | for each `$(…)`; the last runner installed replaces the others | `WithCommandRunner(fn)` | [plugins, step 4](../library/plugins.md#step-4--sandbox-commands-with-withcommandrunner) | [`ExampleCommandRunner`](../../example_plugin_test.go) |
| audit every line as it is parsed (lint, inventory) | `EntryObserver` — `ObserveEntry(e)` | once per entry during parsing, including comments and disabled settings | none; use `WithPlugin` | [plugins, step 9](../library/plugins.md#step-9--audit-every-entry-with-an-entryobserver) | [`ExampleEntryObserver`](../../example_plugin_test.go) |
| trace which names are referenced (dead-key detection) | `ExpandObserver` — `ObserveExpand(key, name, resolved)` | on every reference resolution | none; use `WithPlugin` | [plugins, step 10](../library/plugins.md#step-10--trace-references-with-an-expandobserver) | [`ExampleExpandObserver`](../../example_plugin_test.go) |

No hook can change [the bytes `Render`, `Save` and `SaveAs` write](ds:block?id=render-c8njbs4p): value-changing hooks only run during expansion, which the write path never calls ([the firewall](../library/plugins.md#the-firewall-plugins-never-change-saved-bytes), [`ExampleFile_Plugins_firewall`](../../example_plugin_test.go)).

## Function option or plugin type

| Use | When | Example |
|---|---|---|
| A function option (`WithLookup`, `WithValueTransform`, `WithSaveGuard`, `WithCommandRunner`) | one hook, a few lines, nothing shared | the four examples above |
| A plugin type passed to `WithPlugin(p)` | several capabilities that share a client or a cache (a secret store that both looks up and decrypts), an observer, or a lookup that must be able to fail | [`ExampleWithPlugin`](../../example_plugin_test.go) |

A plugin type is any value with a `Name() string` method plus the capability methods it implements. Capabilities are detected once, when the file is created; check what was detected with `f.Plugins()` ([`ExamplePluginInfo`](../../example_plugin_test.go), [`ExampleCapability`](../../example_plugin_test.go)).

## If X, use Y

- **If you want Docker Compose's environment fallback**, use `WithLookup(os.LookupEnv)`. The file always wins; the environment only fills names the file does not define. It does not override values the file sets.
- **If your secret store can be unreachable**, implement `Lookuper` on a type and return an error. A lookup error aborts the expansion instead of counting as a miss, so a deploy never proceeds with an empty password ([`ExampleWithPlugin_lookupError`](../../example_plugin_test.go)). `WithLookup` cannot report errors.
- **If values are stored encrypted**, use a `ValueTransformer`. `Get` still returns the ciphertext, so that is what is saved; only `GetExpanded` sees plaintext.
- **If a bad file must never reach disk**, use a `SaveGuard`. A refusal leaves the destination as it was, with no temp file beside it ([`ExampleWithPlugin_guard`](../../example_plugin_test.go)).
- **If you only want to look at the file once**, you may not need a hook: walking `f.Entries()` after parsing does the same as an `EntryObserver` without a plugin.
- **If several plugins are installed**, read the [ordering rules](../library/plugins.md#ordering-what-happens-with-several-plugins) ([`ExampleFile_Plugins_ordering`](../../example_plugin_test.go)).

## Pitfalls

- **A method with the wrong signature is silently ignored.** `LookUp` instead of `Lookup`, or a missing `error` result, compiles and never runs. Add compile-time assertions such as `var _ dotenv.Lookuper = (*Vault)(nil)` ([compile-time assertions](../library/plugins.md#compile-time-assertions)).
- **A `CommandRunner` error does not fail the read.** The `$(…)` expands to `""`, as a shell does.
- **A `SaveGuard` receives the live `*File`.** Calling `f.Set` inside a guard changes what is saved. Treat the file as read-only.
- **An `EntryObserver` receives the live `*Entry`.** Changing its fields changes the rendered line. Treat it as read-only.
- **A self-referencing default does not ask the lookup.** `ZONE=${ZONE:-default-zone}` expands to `""`; see [Which expansion setup](expansion.md#pitfalls).

## Related

- [Plugins](../library/plugins.md) · [Variable expansion](../library/expansion.md) · [Typed configuration](../library/typed-config.md)
- [Which expansion setup](expansion.md) · [Options for your kind of app](app-setups.md)
- [All choosing guides](README.md) · [Feature index](../reference/features.md)
