package dotenv_test

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ubgo/dotenv"
)

// This file holds the runnable examples behind docs/library/plugins.md and
// docs/library/typed-config.md. Every region between `snippet:start` and
// `snippet:end` is copied byte for byte into those pages, so `go test` both
// compiles and output-checks what the docs show.

// ---------------------------------------------------------------------------
// Function hooks: WithLookup, WithValueTransform, WithSaveGuard,
// WithCommandRunner.
// ---------------------------------------------------------------------------

// snippet:start plug_WithLookup

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

// snippet:end plug_WithLookup

// snippet:start plug_WithValueTransform

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

// snippet:end plug_WithValueTransform

// snippet:start plug_WithSaveGuard

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

// snippet:end plug_WithSaveGuard

// snippet:start plug_WithCommandRunner

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

// snippet:end plug_WithCommandRunner

// ---------------------------------------------------------------------------
// Typed plugins: one type, several capabilities.
// ---------------------------------------------------------------------------

// snippet:start plug_WithPlugin

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

// snippet:end plug_WithPlugin

// snippet:start plug_WithPlugin_lookupError

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

// snippet:end plug_WithPlugin_lookupError

// snippet:start plug_WithPlugin_guard

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

// snippet:end plug_WithPlugin_guard

// snippet:start plug_CommandRunner

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

// snippet:end plug_CommandRunner

// snippet:start plug_EntryObserver

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

// snippet:end plug_EntryObserver

// snippet:start plug_ExpandObserver

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

// snippet:end plug_ExpandObserver

// ---------------------------------------------------------------------------
// Introspection: Capability, PluginInfo, Plugins.
// ---------------------------------------------------------------------------

// snippet:start plug_Capability

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

// snippet:end plug_Capability

// snippet:start plug_PluginInfo

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

// snippet:end plug_PluginInfo

// ---------------------------------------------------------------------------
// Ordering and the firewall.
// ---------------------------------------------------------------------------

// snippet:start plug_ordering

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

// snippet:end plug_ordering

// snippet:start plug_firewall

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

// snippet:end plug_firewall

// ---------------------------------------------------------------------------
// Typed configuration with the standard library only.
// ---------------------------------------------------------------------------

// snippet:start plug_typedConfig

// plugConfig is the typed view an application wants of its .env.
type plugConfig struct {
	Port  int
	Debug bool
	Wait  time.Duration
	Hosts []string
	Token string
}

// Defaults applied when a key is absent. A key that is present but malformed
// is an error, never a silent fall back to these.
const (
	plugDefaultPort = 8080
	plugDefaultWait = 30 * time.Second
	plugHostsSep    = ";" // HOSTS is a ;-separated list
)

// plugBind converts expanded string values into plugConfig. It collects every
// problem and returns them together, so one run shows everything to fix.
func plugBind(m map[string]string) (plugConfig, error) {
	cfg := plugConfig{Port: plugDefaultPort, Wait: plugDefaultWait}
	var errs []error

	if v, ok := m["PORT"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("PORT: %q is not an integer", v))
		}
		cfg.Port = n
	}
	if v, ok := m["DEBUG"]; ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("DEBUG: %q is not a boolean", v))
		}
		cfg.Debug = b
	}
	if v, ok := m["WAIT"]; ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("WAIT: %q is not a duration", v))
		}
		cfg.Wait = d
	}
	if v := m["HOSTS"]; v != "" {
		cfg.Hosts = strings.Split(v, plugHostsSep)
	}
	cfg.Token = m["TOKEN"]
	if cfg.Token == "" {
		errs = append(errs, errors.New("TOKEN: required"))
	}
	return cfg, errors.Join(errs...)
}

// Example_typedConfig binds expanded strings to a typed struct with the standard
// library only, reporting every malformed value at once.
func Example_typedConfig() {
	// dotenv supplies expanded strings; the application decides their types.
	good := "DOMAIN=acme.io\n" +
		"PORT=8443\n" +
		"DEBUG=true\n" +
		"HOSTS=api.${DOMAIN};www.${DOMAIN}\n" +
		"TOKEN=t-123\n"

	m, err := dotenv.Parse(good).ExpandedMap()
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	cfg, err := plugBind(m)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%+v\n", cfg)

	// Every problem is reported at once, and none falls back to a default.
	bad := "PORT=eighty\nWAIT=soon\n"
	m, err = dotenv.Parse(bad).ExpandedMap()
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	_, err = plugBind(m)
	fmt.Println(err)
	// Output:
	// {Port:8443 Debug:true Wait:30s Hosts:[api.acme.io www.acme.io] Token:t-123}
	// PORT: "eighty" is not an integer
	// WAIT: "soon" is not a duration
	// TOKEN: required
}

// snippet:end plug_typedConfig
