# Plugins

How to plug decryption, secret stores, sandboxed commands, save-time checks and auditing into the `dotenv` library without the library knowing about any of them.

**On this page**

- [What a plugin is](#what-a-plugin-is)
- [Before you start](#before-you-start)
- [The six capabilities](#the-six-capabilities)
- [Step 1 — Fill missing references with WithLookup](#step-1--fill-missing-references-with-withlookup)
- [Step 2 — Decrypt values with WithValueTransform](#step-2--decrypt-values-with-withvaluetransform)
- [Step 3 — Refuse bad writes with WithSaveGuard](#step-3--refuse-bad-writes-with-withsaveguard)
- [Step 4 — Sandbox commands with WithCommandRunner](#step-4--sandbox-commands-with-withcommandrunner)
- [Step 5 — Write a plugin type with several capabilities](#step-5--write-a-plugin-type-with-several-capabilities)
- [Step 6 — Handle a secret store that is down](#step-6--handle-a-secret-store-that-is-down)
- [Step 7 — A SaveGuard as a type](#step-7--a-saveguard-as-a-type)
- [Step 8 — A CommandRunner as a type](#step-8--a-commandrunner-as-a-type)
- [Step 9 — Audit every entry with an EntryObserver](#step-9--audit-every-entry-with-an-entryobserver)
- [Step 10 — Trace references with an ExpandObserver](#step-10--trace-references-with-an-expandobserver)
- [Ordering: what happens with several plugins](#ordering-what-happens-with-several-plugins)
- [The firewall: plugins never change saved bytes](#the-firewall-plugins-never-change-saved-bytes)
- [Compile-time assertions](#compile-time-assertions)
- [Checking what was detected: Plugins, PluginInfo, Capability](#checking-what-was-detected-plugins-plugininfo-capability)
- [Try it](#try-it)
- [Pitfalls](#pitfalls)
- [Related](#related)

## What a plugin is

A plugin is any value with a `Name() string` method, installed with `dotenv.WithPlugin`. What it does depends on which of six small capability interfaces it also implements. A plugin implements only the ones it needs, and one plugin may implement several, so a secret store can share one client and one cache between its lookup and its decryption.

You want a plugin when a value should read differently from how it is written: a password fetched from a vault, a ciphertext decrypted on read, an environment fallback. You also want one to refuse a write (a plaintext secret, a missing key) or to watch the file (an audit of disabled settings, a list of unused variables).

For a one-function hook, you do not need a type at all. Four options wrap a plain function: `WithLookup`, `WithValueTransform`, `WithSaveGuard` and `WithCommandRunner`. Steps 1 to 4 use them; steps 5 to 10 write plugin types.

## Before you start

```sh
go get github.com/ubgo/dotenv
```

You need Go 1.22 or later. Every example on this page parses an in-memory string or writes to a temporary directory, so you can paste it into a `_test.go` file and run it.

## The six capabilities

The interfaces, as declared in `plugin.go`:

<!-- snippet: exempt — type signature excerpt of the library's own interfaces -->
```go
type Plugin interface {
	Name() string
}

type Lookuper interface {
	Lookup(name string) (value string, ok bool, err error)
}

type ValueTransformer interface {
	TransformValue(key, value string) (string, error)
}

type CommandRunner interface {
	RunCommand(cmd string) (string, error)
}

type SaveGuard interface {
	GuardSave(f *File) error
}

type EntryObserver interface {
	ObserveEntry(e *Entry)
}

type ExpandObserver interface {
	ObserveExpand(key, name, resolved string)
}
```

When each one runs, and how several of the same kind combine:

| Capability | Runs | Several installed | Can fail | Function option | Name it reports |
|---|---|---|---|---|---|
| `EntryObserver` | once per entry while parsing, in file order | all run, install order | no | none | your `Name()` |
| `Lookuper` | for a `${VAR}` the file does not define | **first to answer wins** | yes, aborts the read | `WithLookup` | `lookup` |
| `CommandRunner` | for a `$(cmd)` in a value that interpolates | **last installed wins** (a replacement) | no: a runner error expands to `""` | `WithCommandRunner` | `command-runner` |
| `ExpandObserver` | after every reference resolution | all run, install order | no | none | your `Name()` |
| `ValueTransformer` | after a value is expanded, on literal values too | **chained**, each sees the last output | yes, aborts the read | `WithValueTransform` | `value-transform` |
| `SaveGuard` | before `Save` or `SaveAs` writes anything | run in order; **first error aborts** | yes, the write is refused | `WithSaveGuard` | `save-guard` |

Expansion runs in `GetExpanded`, `ExpandedMap` and `Read`. `Get`, `Map`, `Pairs`, `Render` and `Save` never expand, so lookups, runners, transformers and expand observers never touch them.

## Step 1 — Fill missing references with WithLookup

`WithLookup` takes a `func(name string) (string, bool)` and calls it for a `${VAR}` that the file does not define. The file always wins: a fallback cannot override a value somebody wrote down. A `${VAR:-default}` still falls back to its default when the lookup misses. In an application, pass `os.LookupEnv` to get Docker Compose's environment fallback; the example uses a map so its output is the same on every machine.

<!-- snippet: example_plugin_test.go#plug_WithLookup -->
```go
// ExampleWithLookup shows how references the file does not define are filled from
// a fallback source, while the file's own keys and ${VAR:-default} still win.
func ExampleWithLookup() {
	// WithLookup fills references the file does not define. A map stands in for
	// os.LookupEnv here, so the output does not depend on the machine running it.
	fallback := map[string]string{
		"HOST":   "from-fallback",
		"REGION": "eu-west-1",
	}
	lookup := func(name string) (string, bool) {
		v, ok := fallback[name]
		return v, ok
	}

	src := "HOST=from-file\n" +
		"URL=https://${HOST}/${REGION}\n" +
		"ZONE=${AZ:-default-zone}\n"
	f := dotenv.Parse(src, dotenv.WithLookup(lookup))

	for _, key := range []string{"URL", "ZONE"} {
		v, _, err := f.GetExpanded(key)
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		fmt.Printf("%s=%s\n", key, v)
	}
	// Output:
	// URL=https://from-file/eu-west-1
	// ZONE=default-zone
}
```

Example: [`ExampleWithLookup`](../../example_plugin_test.go).

## Step 2 — Decrypt values with WithValueTransform

`WithValueTransform` takes a `func(key, value string) (string, error)` that rewrites each value after it is expanded. Return the value unchanged when it is not yours. The transformer also runs on single-quoted and backtick values: quoting controls interpolation, not transformation, and an encrypted value is often single-quoted to keep `$` signs in the ciphertext literal.

Its output is not expanded again, so a decrypted secret containing `${...}` stays literal. An error aborts the read: you never get the ciphertext back where you expected a secret. `Get` and `Render` still return the ciphertext, so that is what gets saved.

<!-- snippet: example_plugin_test.go#plug_WithValueTransform -->
```go
// ExampleWithValueTransform decodes values on read only: GetExpanded returns the
// plain text, Get and Render keep the stored ciphertext, and a bad value is an
// error rather than leaked ciphertext.
func ExampleWithValueTransform() {
	// A transformer decodes values after they are read. Base64 stands in for a
	// real cipher; the shape is the same for age, KMS or sops.
	const prefix = "encrypted:" // marks a value the transformer owns

	decrypt := func(key, value string) (string, error) {
		payload, ok := strings.CutPrefix(value, prefix)
		if !ok {
			return value, nil // not ours: pass it through untouched
		}
		plain, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return "", err
		}
		return string(plain), nil
	}

	src := "PLAIN=hello\n" +
		"SECRET='encrypted:aHVudGVyMg=='\n" +
		"BROKEN=encrypted:%%%\n"
	f := dotenv.Parse(src, dotenv.WithValueTransform(decrypt))

	plain, _, err := f.GetExpanded("SECRET")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	raw, _ := f.Get("SECRET")
	fmt.Println("GetExpanded:", plain)
	fmt.Println("Get:        ", raw)

	// A value that fails to decode is an error, never the ciphertext.
	_, _, err = f.GetExpanded("BROKEN")
	fmt.Println("BROKEN:", err)

	// The written bytes keep the ciphertext.
	fmt.Print(f.Render())
	// Output:
	// GetExpanded: hunter2
	// Get:         encrypted:aHVudGVyMg==
	// BROKEN: dotenv: plugin "value-transform": transform BROKEN: illegal base64 data at input byte 0
	// PLAIN=hello
	// SECRET='encrypted:aHVudGVyMg=='
	// BROKEN=encrypted:%%%
}
```

Example: [`ExampleWithValueTransform`](../../example_plugin_test.go).

## Step 3 — Refuse bad writes with WithSaveGuard

`WithSaveGuard` takes a `func(*File) error`. It runs before `Save` or `SaveAs` writes anything. The `*File` it receives is a copy made for that call, so the guard can read anything but cannot change what is written. Returning an error refuses the write: no temp file is created and the destination keeps its old content. The error wraps yours, so `errors.Is` and `errors.Unwrap` reach it; the full message also names the plugin and the path (`dotenv: plugin "save-guard": guard save <path>: <your error>`).

<!-- snippet: example_plugin_test.go#plug_WithSaveGuard -->
```go
// ExampleWithSaveGuard refuses to write a file that fails a check, and shows the
// file already on disk is left untouched by the refused save.
func ExampleWithSaveGuard() {
	// A guard refuses to write a file that fails a check. This one blocks a
	// password that is not encrypted.
	guard := dotenv.WithSaveGuard(func(f *dotenv.File) error {
		for _, p := range f.Pairs() {
			if strings.Contains(p.Key, "PASSWORD") && !strings.HasPrefix(p.Value, "encrypted:") {
				return fmt.Errorf("%s looks like a plaintext secret", p.Key)
			}
		}
		return nil
	})

	dir, err := os.MkdirTemp("", "dotenv-guard-")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer func() {
		// Cleanup is best-effort; a failure goes to stderr so the example's
		// checked stdout output is unaffected.
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
		}
	}()

	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("ORIGINAL=1\n"), 0o600); err != nil {
		fmt.Println("error:", err)
		return
	}

	bad := dotenv.Parse("DB_PASSWORD=hunter2\n", guard)
	err = bad.SaveAs(path)
	// The full message also names the path; unwrap to print only the reason.
	fmt.Println("refused:", errors.Unwrap(err))

	kept, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("on disk: %q\n", kept)

	good := dotenv.Parse("DB_PASSWORD=encrypted:abc\n", guard)
	fmt.Println("saved:", good.SaveAs(path) == nil)
	// Output:
	// refused: DB_PASSWORD looks like a plaintext secret
	// on disk: "ORIGINAL=1\n"
	// saved: true
}
```

Example: [`ExampleWithSaveGuard`](../../example_plugin_test.go).

## Step 4 — Sandbox commands with WithCommandRunner

`$(command)` substitution is off by default, because with it on, merely reading a file runs shell commands. `WithCommandRunner` takes a `func(cmd string) (string, error)`, installs it as the runner and turns substitution on. Use it for an allow-list, a sandbox, or a test double. Without any runner, `$(...)` stays as written.

A runner error does **not** fail the read: the substitution expands to an empty string, the way a shell treats a failed `$(...)` inside a value. Single-quoted values are literal and never reach the runner.

<!-- snippet: example_plugin_test.go#plug_WithCommandRunner -->
```go
// ExampleWithCommandRunner shows $(command) substitution is off by default and
// that a runner decides what each command returns, so an allow-list keeps
// reading a file from running anything else.
func ExampleWithCommandRunner() {
	// A runner replaces `sh -c` for $(command) substitution. This one answers
	// from an allow-list, so reading the file can never run anything else.
	allowed := map[string]string{
		"git rev-parse --short HEAD": "abc1234",
	}
	runner := func(cmd string) (string, error) {
		out, ok := allowed[cmd]
		if !ok {
			return "", fmt.Errorf("command %q is not allowed", cmd)
		}
		return out, nil
	}

	src := `REV="$(git rev-parse --short HEAD)"` + "\n" +
		`WHO="$(whoami)"` + "\n" +
		`LIT='$(whoami)'` + "\n"

	// Without a runner, substitution is off and the text stays as written.
	off := dotenv.Parse(src)
	rev, _, err := off.GetExpanded("REV")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("off: REV =", rev)

	on := dotenv.Parse(src, dotenv.WithCommandRunner(runner))
	for _, key := range []string{"REV", "WHO", "LIT"} {
		v, _, err := on.GetExpanded(key)
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		fmt.Printf("on:  %s = %q\n", key, v)
	}
	// Output:
	// off: REV = $(git rev-parse --short HEAD)
	// on:  REV = "abc1234"
	// on:  WHO = ""
	// on:  LIT = "$(whoami)"
}
```

Example: [`ExampleWithCommandRunner`](../../example_plugin_test.go). For the plain on/off switch (`WithCommandSubstitution`, which runs `sh -c`), see [expansion.md](expansion.md).

## Step 5 — Write a plugin type with several capabilities

When hooks share state, write a type. This vault plugin implements `Lookuper` (fill `${DB_PASSWORD}`) and `ValueTransformer` (swap `vault:api/key` for the secret) over one store. Install it with `WithPlugin`; capabilities are detected once, at install time, by type assertion.

<!-- snippet: example_plugin_test.go#plug_WithPlugin -->
```go
// plugVault is a secret-store plugin holding two capabilities that share one
// client (here, a map): it fills undefined ${VAR} references, and it
// dereferences values written as "vault:<path>".
type plugVault struct {
	secrets map[string]string
}

// plugVaultScheme marks a value that names a secret path rather than holding
// the secret itself.
const plugVaultScheme = "vault:"

func (v *plugVault) Name() string { return "vault" }

// Lookup answers a reference the file does not define.
func (v *plugVault) Lookup(name string) (string, bool, error) {
	s, ok := v.secrets[name]
	return s, ok, nil
}

// TransformValue swaps "vault:<path>" for the secret at that path.
func (v *plugVault) TransformValue(key, value string) (string, error) {
	path, ok := strings.CutPrefix(value, plugVaultScheme)
	if !ok {
		return value, nil
	}
	s, found := v.secrets[path]
	if !found {
		return "", fmt.Errorf("no secret at %q", path)
	}
	return s, nil
}

// Compile-time assertions. A method with a slightly wrong name or signature
// compiles and installs but never runs; these lines turn that into a build error.
var (
	_ dotenv.Plugin           = (*plugVault)(nil)
	_ dotenv.Lookuper         = (*plugVault)(nil)
	_ dotenv.ValueTransformer = (*plugVault)(nil)
)

// ExampleWithPlugin installs one typed plugin that holds two capabilities over one
// shared store, and lists what it was detected as.
func ExampleWithPlugin() {
	// One plugin, two capabilities, one shared store.
	vault := &plugVault{secrets: map[string]string{
		"DB_PASSWORD": "s3cret",
		"api/key":     "k-123",
	}}

	src := "DB_URL=postgres://app:${DB_PASSWORD}@db/app\n" +
		"API_KEY='vault:api/key'\n"
	f := dotenv.Parse(src, dotenv.WithPlugin(vault))

	m, err := f.ExpandedMap()
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("DB_URL =", m["DB_URL"])
	fmt.Println("API_KEY =", m["API_KEY"])

	for _, p := range f.Plugins() {
		fmt.Println(p)
	}
	// Output:
	// DB_URL = postgres://app:s3cret@db/app
	// API_KEY = k-123
	// vault: Lookuper, ValueTransformer
}
```

Example: [`ExampleWithPlugin`](../../example_plugin_test.go).

## Step 6 — Handle a secret store that is down

A `Lookuper` returns `ok=false` for "I do not have this" and the next one is asked. It returns an error for "I could not check". An error aborts expansion, even when the reference has a default: an unreachable store must not look like an unset variable, or a deploy proceeds with an empty password. The error names the plugin and the variable, and wraps your cause for `errors.Is`.

<!-- snippet: example_plugin_test.go#plug_WithPlugin_lookupError -->
```go
// plugDownStore is a secret store that cannot be reached.
type plugDownStore struct{}

// errPlugUnreachable is the cause the store reports; callers match it with
// errors.Is.
var errPlugUnreachable = errors.New("connection refused")

func (plugDownStore) Name() string { return "vault" }

func (plugDownStore) Lookup(string) (string, bool, error) {
	return "", false, errPlugUnreachable
}

var _ dotenv.Lookuper = plugDownStore{}

// ExampleWithPlugin_lookupError shows a Lookuper error aborts expansion and stays
// matchable with errors.Is, instead of being read as "not set".
func ExampleWithPlugin_lookupError() {
	// A Lookuper error aborts expansion. It is never treated as "not set", so a
	// default cannot quietly stand in for a password the store failed to return.
	f := dotenv.Parse("DSN=db://app:${DB_PASSWORD:-}@db\n", dotenv.WithPlugin(plugDownStore{}))

	_, _, err := f.GetExpanded("DSN")
	fmt.Println(err)
	fmt.Println("unreachable:", errors.Is(err, errPlugUnreachable))
	// Output:
	// dotenv: plugin "vault": lookup DB_PASSWORD: connection refused
	// unreachable: true
}
```

Example: [`ExampleWithPlugin_lookupError`](../../example_plugin_test.go).

## Step 7 — A SaveGuard as a type

A guard type can carry configuration, here the list of keys a file must define before it may be saved. `Get` returns the raw value, so a key set to an unexpanded `${REF}` counts as present.

<!-- snippet: example_plugin_test.go#plug_WithPlugin_guard -->
```go
// plugRequireKeys is a SaveGuard that refuses to write a file missing any of
// the listed keys, or holding one with an empty value.
type plugRequireKeys struct {
	keys []string
}

func (plugRequireKeys) Name() string { return "require-keys" }

// GuardSave only reads the file. It is handed a copy, so an edit here would
// change nothing that is saved; see the firewall in docs/library/plugins.md.
func (g plugRequireKeys) GuardSave(f *dotenv.File) error {
	var missing []string
	for _, k := range g.keys {
		if v, ok := f.Get(k); !ok || v == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing or empty: %s", strings.Join(missing, ", "))
	}
	return nil
}

var _ dotenv.SaveGuard = plugRequireKeys{}

// ExampleWithPlugin_guard uses a typed SaveGuard to refuse a file with a missing or
// empty required key, then saves once the key is filled.
func ExampleWithPlugin_guard() {
	// Guards run before anything is written, on Save and SaveAs alike.
	guard := dotenv.WithPlugin(plugRequireKeys{keys: []string{"DATABASE_URL", "SECRET_KEY"}})

	dir, err := os.MkdirTemp("", "dotenv-require-")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer func() {
		// Cleanup is best-effort; a failure goes to stderr so the example's
		// checked stdout output is unaffected.
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
		}
	}()
	path := filepath.Join(dir, ".env")

	f := dotenv.Parse("DATABASE_URL=postgres://db/app\nSECRET_KEY=\n", guard)
	err = f.SaveAs(path)
	fmt.Println("refused:", errors.Unwrap(err))

	_, statErr := os.Stat(path)
	fmt.Println("file written:", statErr == nil)

	f.Set("SECRET_KEY", "k-123")
	fmt.Println("saved:", f.SaveAs(path) == nil)
	// Output:
	// refused: missing or empty: SECRET_KEY
	// file written: false
	// saved: true
}
```

Example: [`ExampleWithPlugin_guard`](../../example_plugin_test.go).

## Step 8 — A CommandRunner as a type

Installing any `CommandRunner` enables substitution, exactly like `WithCommandRunner`. References inside the command are expanded first, so the runner receives `echo hello world`, not `echo hello ${NAME}`.

<!-- snippet: example_plugin_test.go#plug_CommandRunner -->
```go
// plugRecorder is a CommandRunner that records each command instead of running
// it: the shape of a test double, or of a runner that logs before delegating.
type plugRecorder struct {
	seen []string
}

func (r *plugRecorder) Name() string { return "recorder" }

func (r *plugRecorder) RunCommand(cmd string) (string, error) {
	r.seen = append(r.seen, cmd)
	return "out(" + cmd + ")", nil
}

var _ dotenv.CommandRunner = (*plugRecorder)(nil)

// ExampleCommandRunner records commands instead of running them, and shows that
// references inside a command are expanded before the runner sees it.
func ExampleCommandRunner() {
	// Installing any CommandRunner turns substitution on. References inside the
	// command are expanded BEFORE the runner sees it.
	rec := &plugRecorder{}
	f := dotenv.Parse("NAME=world\nGREETING=\"$(echo hello ${NAME})\"\n", dotenv.WithPlugin(rec))

	v, _, err := f.GetExpanded("GREETING")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(v)
	fmt.Printf("runner saw: %q\n", rec.seen)
	// Output:
	// out(echo hello world)
	// runner saw: ["echo hello world"]
}
```

Example: [`ExampleCommandRunner`](../../example_plugin_test.go).

## Step 9 — Audit every entry with an EntryObserver

An `EntryObserver` sees every entry as it is parsed: comments, blank lines, pairs, commented-out pairs (`KindDisabledPair`), name-only declarations (`KindInherited`) and unrecognised lines (`KindOther`). A commented-out setting is a finding for an auditor, so nothing is filtered. A multi-line value is one entry. Each call gets a copy of the entry, so the observer can read it or keep it but cannot change the parsed file. It cannot fail; to reject a file, use a `SaveGuard` or inspect the parsed `File`.

It fires from `Parse`, `ParseReader` and `Open`, not from `Set`, `Append` or other edits.

<!-- snippet: example_plugin_test.go#plug_EntryObserver -->
```go
// plugAuditor is an EntryObserver that records every entry the parser
// produces, including commented-out settings.
type plugAuditor struct {
	lines    []string
	disabled []string
}

func (a *plugAuditor) Name() string { return "auditor" }

// ObserveEntry runs once per entry, in file order, during parsing.
func (a *plugAuditor) ObserveEntry(e *dotenv.Entry) {
	a.lines = append(a.lines, fmt.Sprintf("%-13s %s", e.Kind, e.Key))
	if e.Kind == dotenv.KindDisabledPair {
		a.disabled = append(a.disabled, e.Key)
	}
}

var _ dotenv.EntryObserver = (*plugAuditor)(nil)

// ExampleEntryObserver audits every entry the parser produces, including blanks,
// comments and commented-out settings.
func ExampleEntryObserver() {
	// The observer sees every kind of entry, not just active pairs.
	src := "# Database\n" +
		"\n" +
		"DB_HOST=localhost\n" +
		"# DB_USER=admin\n" +
		"export HOME\n" +
		"not a pair\n" +
		"NOTE=\"line one\nline two\"\n"

	audit := &plugAuditor{}
	dotenv.Parse(src, dotenv.WithPlugin(audit))

	for _, l := range audit.lines {
		fmt.Println(strings.TrimSpace(l))
	}
	fmt.Println("disabled:", audit.disabled)
	// Output:
	// comment
	// blank
	// pair          DB_HOST
	// disabled-pair DB_USER
	// inherited     HOME
	// other
	// pair          NOTE
	// disabled: [DB_USER]
}
```

Example: [`ExampleEntryObserver`](../../example_plugin_test.go).

## Step 10 — Trace references with an ExpandObserver

An `ExpandObserver` is told about every reference resolution: `key` is the entry being expanded, `name` the variable it referenced, and `resolved` the result. Branches of `${VAR:-default}` and `${VAR:+alt}` are reported too. Literal (single-quoted) values report nothing, because they are never expanded. It only fires during expansion, so call `ExpandedMap` (or `GetExpanded` for each key) first.

<!-- snippet: example_plugin_test.go#plug_ExpandObserver -->
```go
// plugTracer is an ExpandObserver that records which variables the file's
// values actually reference.
type plugTracer struct {
	used map[string]bool
}

func (t *plugTracer) Name() string { return "tracer" }

// ObserveExpand runs for every reference resolution: key is the entry being
// expanded, name the variable it referenced, resolved what that became.
func (t *plugTracer) ObserveExpand(key, name, resolved string) {
	t.used[name] = true
}

var _ dotenv.ExpandObserver = (*plugTracer)(nil)

// ExampleExpandObserver traces which variables other values reference, the basis
// of a dead-key check.
func ExampleExpandObserver() {
	// Dead-key detection: expand everything, then list the variables nothing
	// referenced. (Top-level keys like URL are consumed by the app, not by other
	// values, so a real check compares against what the app reads too.)
	src := "HOST=db\nPORT=5432\nLEGACY_HOST=old-db\nURL=postgres://${HOST}:${PORT}/app\n"

	tracer := &plugTracer{used: map[string]bool{}}
	f := dotenv.Parse(src, dotenv.WithPlugin(tracer))
	if _, err := f.ExpandedMap(); err != nil {
		fmt.Println("error:", err)
		return
	}

	var referenced, unreferenced []string
	for _, k := range f.Keys() {
		if tracer.used[k] {
			referenced = append(referenced, k)
		} else {
			unreferenced = append(unreferenced, k)
		}
	}
	sort.Strings(referenced)
	sort.Strings(unreferenced)
	fmt.Println("referenced:  ", referenced)
	fmt.Println("unreferenced:", unreferenced)
	// Output:
	// referenced:   [HOST PORT]
	// unreferenced: [LEGACY_HOST URL]
}
```

Example: [`ExampleExpandObserver`](../../example_plugin_test.go).

## Ordering: what happens with several plugins

Each capability combines in its own way:

- **Lookuper** — the file's own keys first, then each `Lookuper` in install order. The first that returns `ok=true` wins and later ones are not asked. A lookup error stops the chain.
- **ValueTransformer** — chained in install order. Each sees the previous one's output, so one plugin can dereference a value and another decode it.
- **CommandRunner** — a replacement, not a chain. The last runner installed is the only one used.
- **SaveGuard** — run in install order. The first error refuses the write and later guards do not run.
- **EntryObserver** and **ExpandObserver** — all run, in install order.

Install order is the order of the options, and of the plugins within one `WithPlugin(a, b)` call. The function options count as plugins in that order.

<!-- snippet: example_plugin_test.go#plug_ordering -->
```go
// ExampleFile_Plugins_ordering shows how several plugins with the same capability
// combine: first answer wins for lookups, transforms chain, the last runner wins.
func ExampleFile_Plugins_ordering() {
	// Each capability combines differently when several plugins hold it.
	answer := func(v string) func(string) (string, bool) {
		return func(string) (string, bool) { return v, v != "" }
	}
	prefix := func(p string) func(string, string) (string, error) {
		return func(_, v string) (string, error) { return p + v, nil }
	}
	run := func(tag string) func(string) (string, error) {
		return func(cmd string) (string, error) { return tag + ":" + cmd, nil }
	}

	f := dotenv.Parse("OWN=file\nA=${OWN}\nB=${MISSING}\nC=v\nD=\"$(cmd)\"\n",
		// Lookuper: the file wins, then the first plugin that answers.
		dotenv.WithLookup(answer("")),       // a miss
		dotenv.WithLookup(answer("second")), // answers first
		dotenv.WithLookup(answer("third")),  // never asked
		// ValueTransformer: chained in install order.
		dotenv.WithValueTransform(prefix("1-")),
		dotenv.WithValueTransform(prefix("2-")),
		// CommandRunner: a replacement, the last installed wins.
		dotenv.WithCommandRunner(run("first")),
		dotenv.WithCommandRunner(run("last")),
	)

	for _, k := range []string{"A", "B", "C", "D"} {
		v, _, err := f.GetExpanded(k)
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		fmt.Printf("%s=%s\n", k, v)
	}
	// Output:
	// A=2-1-file
	// B=2-1-second
	// C=2-1-v
	// D=2-1-last:cmd
}
```

Example: [`ExampleFile_Plugins_ordering`](../../example_plugin_test.go).

## The firewall: plugins never change saved bytes

> A hook may change what a value **reads** as. It may never change what gets **written**.

Lookups, runners, transformers and expand observers all live in the expansion path, which [`Render`](ds:block?id=render-c8njbs4p) and `Save` never call, and they are handed only strings. So a file round-trips byte for byte whatever is installed, and a decrypted secret cannot be written back in plaintext. The two hooks that receive structured state get copies: a `SaveGuard` is handed a clone of the file made for that call, and an `EntryObserver` a copy of each entry. A guard that calls `Set`, or an observer that rewrites `e.Raw`, changes only its copy, during the call or later through a kept pointer, so a guard can refuse a write but never alter it.

<!-- snippet: example_plugin_test.go#plug_firewall -->
```go
// ExampleFile_Plugins_firewall proves hooks change only what a value reads as;
// Render still returns the original bytes.
func ExampleFile_Plugins_firewall() {
	// Hooks change what a value READS as, never what gets WRITTEN: Render
	// returns the original bytes whatever is installed.
	const src = "HOST=local\nURL=https://${HOST}/api\n"

	f := dotenv.Parse(src,
		dotenv.WithLookup(func(string) (string, bool) { return "IGNORED", true }),
		dotenv.WithValueTransform(func(_, v string) (string, error) { return "MANGLED-" + v, nil }),
	)

	v, _, err := f.GetExpanded("URL")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("reads as:", v)
	fmt.Println("renders unchanged:", f.Render() == src)
	// Output:
	// reads as: MANGLED-https://local/api
	// renders unchanged: true
}
```

Example: [`ExampleFile_Plugins_firewall`](../../example_plugin_test.go). The rule is also asserted by the plain test `TestPlugins_NeverAffectBytes` in [`plugin_test.go`](../../plugin_test.go), which renders and edits one file with every value-changing hook and with none, and by the fuzz target `FuzzPluginsNeverAffectBytes` in [`fuzz_test.go`](../../fuzz_test.go), which does the same for arbitrary input. Its subtest also installs a plugin that edits everything a `SaveGuard` and an `EntryObserver` are handed, and checks that the parsed file and the saved bytes are unchanged.

## Compile-time assertions

Capabilities are optional interfaces, so a method with the wrong name or signature compiles, installs, and silently never runs. `LookUp` instead of `Lookup`, or a `Lookup` without the `error` return, both do this. Add one assertion per capability next to every plugin type; it turns the silence into a build error. Step 5 shows the block for the vault plugin: `_ dotenv.Lookuper = (*plugVault)(nil)` fails to compile the moment `Lookup` stops matching the interface.

Use the pointer form `(*T)(nil)` when the methods have pointer receivers, and `T{}` when they have value receivers. Every plugin type in [`example_plugin_test.go`](../../example_plugin_test.go) has its assertion.

## Checking what was detected: Plugins, PluginInfo, Capability

[`f.Plugins()`](ds:block?id=plugins-upbxam5w) returns one `PluginInfo` per installed plugin, in install order, with the capabilities it was detected as having. Use it when a plugin seems to do nothing: a capability missing from the list means [the method does not match the interface](ds:block?id=hascapability-tcbcz8hy). The [`Capability` constants](ds:block?id=capabilities-kmsggtyr) are `CapLookup`, `CapValueTransformer`, `CapCommandRunner`, `CapSaveGuard`, `CapEntryObserver` and `CapExpandObserver`; their string values are the interface names, and `PluginInfo.Capabilities` lists them in that fixed order.

<!-- snippet: example_plugin_test.go#plug_Capability -->
```go
// plugTypo meant to be a Lookuper, but the method is spelled LookUp, so it
// matches no capability interface. It compiles and installs, and does nothing.
type plugTypo struct{}

func (plugTypo) Name() string { return "typo" }

func (plugTypo) LookUp(name string) (string, bool, error) { return "x", true, nil }

// plugSink holds all six capabilities on one object.
type plugSink struct{}

func (plugSink) Name() string                               { return "sink" }
func (plugSink) Lookup(string) (string, bool, error)        { return "", false, nil }
func (plugSink) TransformValue(_, v string) (string, error) { return v, nil }
func (plugSink) RunCommand(string) (string, error)          { return "", nil }
func (plugSink) GuardSave(*dotenv.File) error               { return nil }
func (plugSink) ObserveEntry(*dotenv.Entry)                 {}
func (plugSink) ObserveExpand(_, _, _ string)               {}

// ExampleCapability shows how a misspelled hook method is caught: the plugin is
// detected with no capabilities at all.
func ExampleCapability() {
	// Plugins reports what each plugin was DETECTED as, which is how a
	// misspelled method shows up: as a plugin with no capabilities.
	f := dotenv.Parse("V=${K}\n", dotenv.WithPlugin(plugTypo{}, plugSink{}))

	for _, info := range f.Plugins() {
		fmt.Println(info)
	}
	for _, info := range f.Plugins() {
		has := false
		for _, c := range info.Capabilities {
			if c == dotenv.CapLookup {
				has = true
			}
		}
		fmt.Printf("%s has %s: %v\n", info.Name, dotenv.CapLookup, has)
	}
	// Output:
	// typo: (none)
	// sink: Lookuper, ValueTransformer, CommandRunner, SaveGuard, EntryObserver, ExpandObserver
	// typo has Lookuper: false
	// sink has Lookuper: true
}
```

`PluginInfo.String` renders `name: Cap, Cap`, or `name: (none)`. The four function options install plugins named `lookup`, `value-transform`, `save-guard` and `command-runner`; those names appear in `Plugins()` and in error messages.

<!-- snippet: example_plugin_test.go#plug_PluginInfo -->
```go
// ExamplePluginInfo prints the one-line form of PluginInfo and the fixed names the
// function options install their plugins under.
func ExamplePluginInfo() {
	// PluginInfo is plain data: a name and the detected capabilities. String
	// renders the one-line form a doctor command prints.
	infos := []dotenv.PluginInfo{
		{Name: "vault", Capabilities: []dotenv.Capability{dotenv.CapLookup, dotenv.CapValueTransformer}},
		{Name: "tracer"},
	}
	for _, p := range infos {
		fmt.Println(p.String())
	}

	// The function options install plugins with fixed names.
	f := dotenv.Parse("A=1\n",
		dotenv.WithLookup(func(string) (string, bool) { return "", false }),
		dotenv.WithValueTransform(func(_, v string) (string, error) { return v, nil }),
		dotenv.WithSaveGuard(func(*dotenv.File) error { return nil }),
		dotenv.WithCommandRunner(func(string) (string, error) { return "", nil }),
	)
	for _, p := range f.Plugins() {
		fmt.Println(p)
	}
	// Output:
	// vault: Lookuper, ValueTransformer
	// tracer: (none)
	// lookup: Lookuper
	// value-transform: ValueTransformer
	// save-guard: SaveGuard
	// command-runner: CommandRunner
}
```

Examples: [`ExampleCapability`](../../example_plugin_test.go), [`ExamplePluginInfo`](../../example_plugin_test.go), and the short [`ExampleFile_Plugins`](../../plugin_test.go). `Clone` keeps the plugins of the file it copies.

## Try it

Run every example on this page from the repository root:

```sh
go test -run '^(ExampleWithLookup|ExampleWithValueTransform|ExampleWithSaveGuard|ExampleWithCommandRunner|ExampleWithPlugin|ExampleWithPlugin_lookupError|ExampleWithPlugin_guard|ExampleCommandRunner|ExampleEntryObserver|ExampleExpandObserver|ExampleFile_Plugins_ordering|ExampleFile_Plugins_firewall|ExampleCapability|ExamplePluginInfo)$' -v .
```

```text
=== RUN   ExampleWithLookup
--- PASS: ExampleWithLookup (0.00s)
=== RUN   ExampleWithValueTransform
--- PASS: ExampleWithValueTransform (0.00s)
=== RUN   ExampleWithSaveGuard
--- PASS: ExampleWithSaveGuard (0.00s)
=== RUN   ExampleWithCommandRunner
--- PASS: ExampleWithCommandRunner (0.00s)
=== RUN   ExampleWithPlugin
--- PASS: ExampleWithPlugin (0.00s)
=== RUN   ExampleWithPlugin_lookupError
--- PASS: ExampleWithPlugin_lookupError (0.00s)
=== RUN   ExampleWithPlugin_guard
--- PASS: ExampleWithPlugin_guard (0.00s)
=== RUN   ExampleCommandRunner
--- PASS: ExampleCommandRunner (0.00s)
=== RUN   ExampleEntryObserver
--- PASS: ExampleEntryObserver (0.00s)
=== RUN   ExampleExpandObserver
--- PASS: ExampleExpandObserver (0.00s)
=== RUN   ExampleCapability
--- PASS: ExampleCapability (0.00s)
=== RUN   ExamplePluginInfo
--- PASS: ExamplePluginInfo (0.00s)
=== RUN   ExampleFile_Plugins_ordering
--- PASS: ExampleFile_Plugins_ordering (0.00s)
=== RUN   ExampleFile_Plugins_firewall
--- PASS: ExampleFile_Plugins_firewall (0.00s)
PASS
ok  	github.com/ubgo/dotenv	0.008s
```

Each example passes only when its printed output matches its `// Output:` block, so the outputs shown in the snippets above are the real ones.

## Pitfalls

- **A self-referencing default asks the `Lookuper` only when no earlier line defines the key.** As in Docker Compose, `ZONE=${ZONE:-default-zone}` sees the definitions above it first, so an earlier `ZONE=` line in the file beats the `Lookuper`; with none, the `Lookuper` is asked, then the default applies ([expansion, step 4](expansion.md#step-4--nesting-chains-and-cycles)). In library v0.1.1 and earlier it expanded to `""` and the `Lookuper` was never asked.
- **A lookup error beats a default.** `${DB_PASSWORD:-}` still fails when the `Lookuper` returns an error. Return `ok=false`, not an error, for "not found".
- **A runner error is silent.** A `CommandRunner` that returns an error yields `""` with a nil error from `GetExpanded`. If a refused command must fail the read, make the runner return a sentinel value and check for it, or validate in a `SaveGuard` or after `ExpandedMap`.
- **The function options name every hook the same.** Two `WithLookup` calls both report as `lookup`, so an error does not tell you which one failed. Write a type with its own `Name()` when that matters.
- **Editing what a guard or observer is handed does nothing.** `GuardSave` receives a copy of the file and `ObserveEntry` a copy of the entry, so a `Set` or a changed `e.Value` there is silently lost. To change a value before it is written, call `Set` on the file yourself before `Save`. In library v0.1.1 and earlier, both hooks were handed the live file and entry, and editing them changed the saved bytes.
- **A typed-nil plugin panics.** `WithPlugin((*MyPlugin)(nil))` satisfies the interface and panics on first use. A plain `nil` is ignored, and so is a `nil` function passed to `WithLookup`, `WithValueTransform` or `WithSaveGuard`. `WithCommandRunner(nil)` installs nothing but still turns substitution on with the default `sh -c` runner.
- **Plugins do not reach `os.Environ` by themselves.** The library never reads the process environment. Pass `WithLookup(os.LookupEnv)` when you want that.

## Related

- [typed-config.md](typed-config.md) — turning the expanded strings into a typed struct.
- [expansion.md](expansion.md) — the `${…}` operators and `$(…)` substitution that lookups, runners and observers hook into.
- [reading.md](reading.md) and [entries.md](entries.md) — `Read`, `Open`, `Parse` and the entry kinds an `EntryObserver` sees.
- [saving.md](saving.md) — `Save` and `SaveAs`, where guards run.
- [Feature index](../reference/features.md#library-plugins-and-hooks) — every plugin feature with its guide and example.
- Examples: [`example_plugin_test.go`](../../example_plugin_test.go); tests: [`plugin_test.go`](../../plugin_test.go).
