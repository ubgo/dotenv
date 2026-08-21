package dotenv

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// --- test plugins -----------------------------------------------------------

// multiCap implements two capabilities from one object, which is the reason the
// design uses capability interfaces rather than one option per hook.
type multiCap struct {
	name  string
	value string
	calls *[]string
}

func (m *multiCap) Name() string { return m.name }

func (m *multiCap) Lookup(name string) (string, bool, error) {
	*m.calls = append(*m.calls, m.name+".Lookup("+name+")")
	if m.value == "" {
		return "", false, nil
	}
	return m.value, true, nil
}

func (m *multiCap) RunCommand(cmd string) (string, error) {
	*m.calls = append(*m.calls, m.name+".RunCommand("+cmd+")")
	return "ran:" + cmd, nil
}

// noCap implements only Name — legal, but usually a signature typo, which is
// what Plugins exists to reveal.
type noCap struct{}

func (noCap) Name() string { return "inert" }

// failingLookup reports an error rather than a miss.
type failingLookup struct{ err error }

func (failingLookup) Name() string { return "failing" }

func (f failingLookup) Lookup(string) (string, bool, error) { return "", false, f.err }

// compile-time assertions — the rule plugin authors must follow, because an
// optional interface with a wrong signature fails silently.
var (
	_ Plugin        = (*multiCap)(nil)
	_ Lookuper      = (*multiCap)(nil)
	_ CommandRunner = (*multiCap)(nil)
	_ Plugin        = noCap{}
)

// --- capability detection ---------------------------------------------------

func TestPlugins_ReportsDetectedCapabilities(t *testing.T) {
	var calls []string
	f := Parse("A=1\n",
		WithPlugin(&multiCap{name: "vault", value: "s3cret", calls: &calls}),
		WithLookup(func(string) (string, bool) { return "", false }),
		WithPlugin(noCap{}),
	)

	got := f.Plugins()
	want := []PluginInfo{
		{Name: "vault", Capabilities: []Capability{CapLookup, CapCommandRunner}},
		{Name: "lookup", Capabilities: []Capability{CapLookup}},
		{Name: "inert", Capabilities: nil},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Plugins() = %+v\nwant %+v", got, want)
	}
}

// TestPluginInfo_String is what a doctor command prints, and what turns a
// silently-inert plugin into an obvious one.
func TestPluginInfo_String(t *testing.T) {
	tests := map[string]PluginInfo{
		"vault: Lookuper, CommandRunner": {Name: "vault", Capabilities: []Capability{CapLookup, CapCommandRunner}},
		"env: Lookuper":                  {Name: "env", Capabilities: []Capability{CapLookup}},
		"tracer: (none)":                 {Name: "tracer"},
	}
	for want, info := range tests {
		if got := info.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}
}

func TestWithPlugin_IgnoresNil(t *testing.T) {
	f := Parse("A=1\n", WithPlugin(nil), WithLookup(nil))

	if n := len(f.Plugins()); n != 0 {
		t.Errorf("Plugins() = %d entries, want 0", n)
	}
}

// --- Lookuper ---------------------------------------------------------------

func TestLookuper(t *testing.T) {
	t.Run("supplies a value the file does not define", func(t *testing.T) {
		var calls []string
		f := Parse("URL=https://${HOST}/api\n",
			WithPlugin(&multiCap{name: "vault", value: "example.com", calls: &calls}))

		got, _, err := f.GetExpanded("URL")
		if err != nil {
			t.Fatalf("GetExpanded: %v", err)
		}
		if got != "https://example.com/api" {
			t.Errorf("GetExpanded = %q", got)
		}
		if len(calls) != 1 || !strings.Contains(calls[0], "Lookup(HOST)") {
			t.Errorf("calls = %v", calls)
		}
	})

	// The file wins. An environment fallback or a secret store must never
	// silently override a value somebody wrote down.
	t.Run("the file's own keys win", func(t *testing.T) {
		var calls []string
		f := Parse("HOST=from-file\nURL=https://${HOST}/api\n",
			WithPlugin(&multiCap{name: "vault", value: "from-plugin", calls: &calls}))

		got, _, _ := f.GetExpanded("URL")
		if got != "https://from-file/api" {
			t.Errorf("GetExpanded = %q, want the file's value", got)
		}
		if len(calls) != 0 {
			t.Errorf("the plugin was consulted despite the file defining the key: %v", calls)
		}
	})

	t.Run("first to answer wins", func(t *testing.T) {
		var calls []string
		f := Parse("V=${K}\n",
			WithPlugin(&multiCap{name: "first", value: "", calls: &calls}), // a miss
			WithPlugin(&multiCap{name: "second", value: "found", calls: &calls}),
			WithPlugin(&multiCap{name: "third", value: "later", calls: &calls}),
		)

		got, _, _ := f.GetExpanded("V")
		if got != "found" {
			t.Errorf("GetExpanded = %q, want the second plugin's value", got)
		}
		// The third must not be consulted once the second answered.
		if len(calls) != 2 {
			t.Errorf("calls = %v, want the chain to stop at the first answer", calls)
		}
	})

	// An unreachable secret store must not look like an unset variable, or a
	// deploy proceeds with an empty password.
	t.Run("an error aborts rather than counting as a miss", func(t *testing.T) {
		boom := errors.New("connection refused")
		f := Parse("V=${K}\n", WithPlugin(failingLookup{err: boom}))

		_, _, err := f.GetExpanded("V")
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the cause preserved", err)
		}
		if !strings.Contains(err.Error(), `plugin "failing"`) {
			t.Errorf("err = %v, want it to name the plugin", err)
		}
		if !strings.Contains(err.Error(), "lookup K") {
			t.Errorf("err = %v, want it to name the operation", err)
		}
	})

	t.Run("satisfies the required-variable operator", func(t *testing.T) {
		var calls []string
		f := Parse("V=${K:?must be set}\n",
			WithPlugin(&multiCap{name: "vault", value: "supplied", calls: &calls}))

		got, _, err := f.GetExpanded("V")
		if err != nil {
			t.Fatalf("a supplied value should satisfy :? — got %v", err)
		}
		if got != "supplied" {
			t.Errorf("GetExpanded = %q", got)
		}
	})

	t.Run("a miss still falls through to the default", func(t *testing.T) {
		var calls []string
		f := Parse("V=${K:-fallback}\n",
			WithPlugin(&multiCap{name: "vault", value: "", calls: &calls}))

		if got, _, _ := f.GetExpanded("V"); got != "fallback" {
			t.Errorf("GetExpanded = %q, want the default", got)
		}
	})

	t.Run("a supplied value may itself contain references", func(t *testing.T) {
		var calls []string
		f := Parse("BASE=root\nV=${K}\n",
			WithPlugin(&multiCap{name: "vault", value: "${BASE}/x", calls: &calls}))

		if got, _, _ := f.GetExpanded("V"); got != "root/x" {
			t.Errorf("GetExpanded = %q, want the supplied value expanded", got)
		}
	})

	t.Run("os.LookupEnv as the documented one-liner", func(t *testing.T) {
		t.Setenv("DOTENV_PLUGIN_PROBE", "from-env")
		f := Parse("V=${DOTENV_PLUGIN_PROBE}\n", WithLookup(func(n string) (string, bool) {
			if n == "DOTENV_PLUGIN_PROBE" {
				return "from-env", true
			}
			return "", false
		}))

		if got, _, _ := f.GetExpanded("V"); got != "from-env" {
			t.Errorf("GetExpanded = %q", got)
		}
	})
}

// TestLookuper_CycleTerminates: a plugin that resolves back into the file must
// not be able to loop, since the supplied value is expanded like any other.
func TestLookuper_CycleTerminates(t *testing.T) {
	var calls []string
	// The plugin always answers with a reference to the key being resolved.
	f := Parse("V=${K}\n", WithPlugin(&multiCap{name: "loop", value: "${K}", calls: &calls}))

	done := make(chan struct{})
	go func() {
		_, _, _ = f.GetExpanded("V")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a self-referential plugin did not terminate")
	}
}

// --- CommandRunner as a plugin ----------------------------------------------

func TestCommandRunner_AsPlugin(t *testing.T) {
	t.Run("installing one enables substitution", func(t *testing.T) {
		var calls []string
		f := Parse(`V="$(hello)"`+"\n",
			WithPlugin(&multiCap{name: "runner", calls: &calls}))

		if got, _, _ := f.GetExpanded("V"); got != "ran:hello" {
			t.Errorf("GetExpanded = %q", got)
		}
	})

	// A replacement, not a chain: passing a command through two runners is
	// meaningless, so the last installed wins.
	t.Run("the last runner installed wins", func(t *testing.T) {
		var calls []string
		f := Parse(`V="$(x)"`+"\n",
			WithPlugin(&multiCap{name: "first", calls: &calls}),
			WithCommandRunner(func(cmd string) (string, error) { return "second:" + cmd, nil }),
		)

		if got, _, _ := f.GetExpanded("V"); got != "second:x" {
			t.Errorf("GetExpanded = %q, want the last runner", got)
		}
		if len(calls) != 0 {
			t.Errorf("the replaced runner was still called: %v", calls)
		}
	})

	t.Run("the function sugar still reports as a plugin", func(t *testing.T) {
		f := Parse("A=1\n", WithCommandRunner(func(string) (string, error) { return "", nil }))

		got := f.Plugins()
		if len(got) != 1 || got[0].Name != "command-runner" {
			t.Fatalf("Plugins() = %+v", got)
		}
		if !reflect.DeepEqual(got[0].Capabilities, []Capability{CapCommandRunner}) {
			t.Errorf("capabilities = %v", got[0].Capabilities)
		}
	})
}

// --- the firewall -----------------------------------------------------------

// TestPlugins_NeverAffectBytes is the rule the whole design rests on: a hook may
// change what a value READS as, never what gets WRITTEN.
//
// It holds structurally — every hook lives in the expansion path, which Render
// does not call — but asserting it means a future hook cannot quietly breach it.
func TestPlugins_NeverAffectBytes(t *testing.T) {
	const src = "# banner\nHOST=local\nURL=https://${HOST}/api\nCMD=\"$(whoami)\"\n# OLD=x\n\ntrailing junk\n"

	var calls []string
	var entries, expands []string
	plain := Parse(src)
	hooked := Parse(src,
		WithPlugin(&multiCap{name: "vault", value: "REPLACED", calls: &calls}),
		WithLookup(func(string) (string, bool) { return "ALSO-REPLACED", true }),
		// Every value-changing hook, including the phase 2 transformer that
		// rewrites whatever it is handed.
		WithValueTransform(func(_, v string) (string, error) { return "MANGLED-" + v, nil }),
		WithPlugin(observerPlugin{name: "obs", entries: &entries, expands: &expands}),
	)

	if plain.Render() != hooked.Render() {
		t.Errorf("plugins changed the rendered bytes:\n plain: %q\nhooked: %q", plain.Render(), hooked.Render())
	}
	if hooked.Render() != src {
		t.Errorf("round trip broken with plugins installed:\n%q", hooked.Render())
	}

	// Editing with plugins installed must also produce the same bytes as
	// editing without them.
	plain.Set("HOST", "changed")
	hooked.Set("HOST", "changed")
	if plain.Render() != hooked.Render() {
		t.Errorf("plugins changed the bytes after an edit:\n plain: %q\nhooked: %q", plain.Render(), hooked.Render())
	}

	// And the reading side genuinely did change, or this test proves nothing.
	got, _, _ := hooked.GetExpanded("CMD")
	if got != "MANGLED-ran:whoami" {
		t.Errorf("GetExpanded(CMD) = %q — the hooks were not actually active", got)
	}
	if len(entries) == 0 {
		t.Error("the entry observer never fired")
	}
}

// TestPlugins_CloneCarriesThem: a clone that lost its plugins would expand
// differently from its source, which is the same trap as a lost dialect.
func TestPlugins_CloneCarriesThem(t *testing.T) {
	var calls []string
	f := Parse("V=${K}\n", WithPlugin(&multiCap{name: "vault", value: "found", calls: &calls}))

	c := f.Clone()
	if got, _, _ := c.GetExpanded("V"); got != "found" {
		t.Errorf("clone GetExpanded = %q, want the plugin to still apply", got)
	}
	if len(c.Plugins()) != 1 {
		t.Errorf("clone Plugins() = %+v", c.Plugins())
	}
}

func TestPluginNameFor_Unknown(t *testing.T) {
	// Defensive: an index past the installed plugins, or an unknown capability,
	// must not panic.
	o := &options{}
	if got := o.pluginNameFor(CapLookup, 0); got != "unknown" {
		t.Errorf("pluginNameFor = %q, want %q", got, "unknown")
	}
	if got := o.pluginNameFor(Capability("nope"), 0); got != "unknown" {
		t.Errorf("pluginNameFor(unknown capability) = %q", got)
	}
}

func ExampleFile_Plugins() {
	f := Parse("A=1\n", WithLookup(func(string) (string, bool) { return "", false }))
	for _, p := range f.Plugins() {
		fmt.Println(p)
	}
	// Output: lookup: Lookuper
}

// TestLookuperName_SkipsNonLookupers covers the index walk when a plugin without
// the capability sits between two that have it — the filtered slice and the
// install order do not line up, and an error must still name the right plugin.
func TestLookuperName_SkipsNonLookupers(t *testing.T) {
	boom := errors.New("boom")
	var calls []string

	f := Parse("V=${K}\n",
		WithPlugin(noCap{}), // no capability
		WithPlugin(&multiCap{name: "first-miss", value: "", calls: &calls}), // Lookuper, misses
		WithPlugin(noCap{}),                  // no capability
		WithPlugin(failingLookup{err: boom}), // Lookuper, fails
	)

	_, _, err := f.GetExpanded("V")
	if err == nil {
		t.Fatal("expected the failing lookuper to abort expansion")
	}
	if !strings.Contains(err.Error(), `plugin "failing"`) {
		t.Errorf("err = %v, want it to name the plugin that actually failed", err)
	}
}

// TestWithCommandRunner_NilKeepsSubstitutionEnabled: the flag-only path, where a
// caller enables substitution without supplying a runner.
func TestWithCommandRunner_NilKeepsSubstitutionEnabled(t *testing.T) {
	f := Parse("V=\"a$(printf b)c\"\n", WithCommandRunner(nil))

	if got, _, _ := f.GetExpanded("V"); got != "abc" {
		t.Errorf("GetExpanded = %q, want the default shell runner", got)
	}
	if n := len(f.Plugins()); n != 0 {
		t.Errorf("Plugins() = %d, want none — no runner was supplied", n)
	}
}

// --- phase 2: ValueTransformer ----------------------------------------------

// upperTransform is a transformer that also records what it saw, so ordering and
// chaining are observable.
type upperTransform struct {
	name   string
	prefix string
	calls  *[]string
	err    error
}

func (u upperTransform) Name() string { return u.name }

func (u upperTransform) TransformValue(key, value string) (string, error) {
	*u.calls = append(*u.calls, fmt.Sprintf("%s(%s=%q)", u.name, key, value))
	if u.err != nil {
		return "", u.err
	}
	return u.prefix + value, nil
}

var _ ValueTransformer = upperTransform{}

func TestValueTransformer(t *testing.T) {
	t.Run("rewrites a value after it is read", func(t *testing.T) {
		var calls []string
		f := Parse("K=raw\n", WithPlugin(upperTransform{name: "t", prefix: "seen:", calls: &calls}))

		got, _, err := f.GetExpanded("K")
		if err != nil {
			t.Fatalf("GetExpanded: %v", err)
		}
		if got != "seen:raw" {
			t.Errorf("GetExpanded = %q", got)
		}
	})

	// Runs AFTER expansion, so a transformer receives a final value rather than
	// text still containing unresolved references.
	t.Run("runs after expansion", func(t *testing.T) {
		var calls []string
		f := Parse("BASE=root\nK=${BASE}/x\n",
			WithPlugin(upperTransform{name: "t", prefix: "", calls: &calls}))

		if _, _, err := f.GetExpanded("K"); err != nil {
			t.Fatal(err)
		}
		if len(calls) == 0 || !strings.Contains(calls[len(calls)-1], `K="root/x"`) {
			t.Errorf("calls = %v, want the transformer to see the EXPANDED value", calls)
		}
	})

	// The output is not expanded again: a decrypted secret containing ${...}
	// stays literal, which is what a secret full of dollar signs wants.
	t.Run("output is not re-expanded", func(t *testing.T) {
		var calls []string
		f := Parse("BASE=root\nK=v\n", WithPlugin(upperTransform{name: "t", prefix: "${BASE}/", calls: &calls}))

		got, _, _ := f.GetExpanded("K")
		if got != "${BASE}/v" {
			t.Errorf("GetExpanded = %q, want the transformer output left literal", got)
		}
	})

	t.Run("chained, each seeing the previous output", func(t *testing.T) {
		var calls []string
		f := Parse("K=v\n",
			WithPlugin(upperTransform{name: "first", prefix: "a-", calls: &calls}),
			WithPlugin(upperTransform{name: "second", prefix: "b-", calls: &calls}),
		)

		got, _, _ := f.GetExpanded("K")
		if got != "b-a-v" {
			t.Errorf("GetExpanded = %q, want the chain applied in install order", got)
		}
		if len(calls) != 2 || !strings.Contains(calls[1], `"a-v"`) {
			t.Errorf("calls = %v, want the second to see the first's output", calls)
		}
	})

	// Quoting controls interpolation, not transformation: an "encrypted:" value
	// is very often single-quoted precisely to keep the parser out of it.
	t.Run("applies to literal values too", func(t *testing.T) {
		var calls []string
		for _, src := range []string{"K='${NOPE}'\n", "K=`${NOPE}`\n"} {
			f := Parse(src, WithPlugin(upperTransform{name: "t", prefix: "seen:", calls: &calls}))
			got, _, _ := f.GetExpanded("K")
			if got != "seen:${NOPE}" {
				t.Errorf("%q: GetExpanded = %q, want the literal transformed but not expanded", src, got)
			}
		}
	})

	// A caller must never silently receive ciphertext where it expected a secret.
	t.Run("an error aborts the read", func(t *testing.T) {
		var calls []string
		boom := errors.New("no decryption key")
		f := Parse("K=v\n", WithPlugin(upperTransform{name: "age", calls: &calls, err: boom}))

		_, _, err := f.GetExpanded("K")
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the cause preserved", err)
		}
		if !strings.Contains(err.Error(), `plugin "age"`) || !strings.Contains(err.Error(), "transform K") {
			t.Errorf("err = %v, want the plugin and operation named", err)
		}
	})

	t.Run("ExpandedMap transforms every value", func(t *testing.T) {
		var calls []string
		m, err := Parse("A=1\nB=2\n", WithPlugin(upperTransform{name: "t", prefix: "x-", calls: &calls})).ExpandedMap()
		if err != nil {
			t.Fatal(err)
		}
		if m["A"] != "x-1" || m["B"] != "x-2" {
			t.Errorf("ExpandedMap = %v", m)
		}
	})

	// The decryption example from the proposal, end to end.
	t.Run("encrypted-prefix decryption", func(t *testing.T) {
		f := Parse("PLAIN=hello\nSECRET=encrypted:aGlkZGVu\n",
			WithValueTransform(func(key, v string) (string, error) {
				s, ok := strings.CutPrefix(v, "encrypted:")
				if !ok {
					return v, nil
				}
				return "decrypted(" + s + ")", nil
			}))

		if got, _, _ := f.GetExpanded("SECRET"); got != "decrypted(aGlkZGVu)" {
			t.Errorf("SECRET = %q", got)
		}
		if got, _, _ := f.GetExpanded("PLAIN"); got != "hello" {
			t.Errorf("PLAIN = %q, want untouched", got)
		}
		// Get stays raw, so the ciphertext is what gets written back.
		if raw, _ := f.Get("SECRET"); raw != "encrypted:aGlkZGVu" {
			t.Errorf("Get(SECRET) = %q, want the ciphertext", raw)
		}
	})
}

// --- phase 2: SaveGuard -----------------------------------------------------

type guardPlugin struct {
	name  string
	err   error
	calls *[]string
}

func (g guardPlugin) Name() string { return g.name }

func (g guardPlugin) GuardSave(f *File) error {
	*g.calls = append(*g.calls, g.name+".GuardSave")
	return g.err
}

var _ SaveGuard = guardPlugin{}

func TestSaveGuard(t *testing.T) {
	t.Run("a passing guard allows the write", func(t *testing.T) {
		var calls []string
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")

		f := Parse("A=1\n", WithPlugin(guardPlugin{name: "ok", calls: &calls}))
		f.Path = path
		if err := f.Save(); err != nil {
			t.Fatalf("Save: %v", err)
		}
		if len(calls) != 1 {
			t.Errorf("calls = %v, want the guard consulted once", calls)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("the file was not written: %v", err)
		}
	})

	// A rejected file must leave the destination exactly as it was — not
	// half-replaced, and with no temp file beside it.
	t.Run("a failing guard aborts and leaves the destination untouched", func(t *testing.T) {
		var calls []string
		dir := t.TempDir()
		path := write(t, dir, ".env", "ORIGINAL=1\n")
		boom := errors.New("plaintext secret")

		f := Parse("SECRET=hunter2\n", WithPlugin(guardPlugin{name: "scanner", err: boom, calls: &calls}))
		f.Path = path

		err := f.Save()
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the cause preserved", err)
		}
		if !strings.Contains(err.Error(), `plugin "scanner"`) {
			t.Errorf("err = %v, want the plugin named", err)
		}

		got, _ := os.ReadFile(path)
		if string(got) != "ORIGINAL=1\n" {
			t.Errorf("destination = %q, want it untouched", got)
		}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), tempSuffix) {
				t.Errorf("a temp file was left behind: %s", e.Name())
			}
		}
	})

	t.Run("every guard runs and the first error wins", func(t *testing.T) {
		var calls []string
		first := errors.New("first")
		f := Parse("A=1\n",
			WithPlugin(guardPlugin{name: "a", calls: &calls}),
			WithPlugin(guardPlugin{name: "b", err: first, calls: &calls}),
			WithPlugin(guardPlugin{name: "c", err: errors.New("second"), calls: &calls}),
		)
		f.Path = filepath.Join(t.TempDir(), ".env")

		err := f.Save()
		if !errors.Is(err, first) {
			t.Fatalf("err = %v, want the FIRST failure", err)
		}
		// The third must not run once the second refused.
		if len(calls) != 2 {
			t.Errorf("calls = %v, want the chain to stop at the first refusal", calls)
		}
	})

	t.Run("guards apply to SaveAs too", func(t *testing.T) {
		var calls []string
		boom := errors.New("nope")
		f := Parse("A=1\n", WithPlugin(guardPlugin{name: "g", err: boom, calls: &calls}))

		to := filepath.Join(t.TempDir(), ".env.prod")
		if err := f.SaveAs(to); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the guard to apply", err)
		}
		if _, err := os.Stat(to); err == nil {
			t.Error("SaveAs wrote the file despite a refusing guard")
		}
	})

	// The secret-scanner example from the proposal.
	t.Run("plaintext secret scanner", func(t *testing.T) {
		guard := WithSaveGuard(func(f *File) error {
			for _, p := range f.Pairs() {
				if strings.Contains(p.Key, "PASSWORD") && !strings.HasPrefix(p.Value, "encrypted:") {
					return fmt.Errorf("%s looks like a plaintext secret", p.Key)
				}
			}
			return nil
		})

		dir := t.TempDir()
		bad := Parse("DB_PASSWORD=hunter2\n", guard)
		if err := bad.SaveAs(filepath.Join(dir, ".env")); err == nil {
			t.Error("the scanner allowed a plaintext password")
		}

		good := Parse("DB_PASSWORD=encrypted:abc\n", guard)
		if err := good.SaveAs(filepath.Join(dir, ".env.ok")); err != nil {
			t.Errorf("the scanner rejected an encrypted password: %v", err)
		}
	})
}

// --- phase 3: observers -----------------------------------------------------

type observerPlugin struct {
	name    string
	entries *[]string
	expands *[]string
}

func (o observerPlugin) Name() string { return o.name }

func (o observerPlugin) ObserveEntry(e *Entry) {
	*o.entries = append(*o.entries, fmt.Sprintf("%s:%s", e.Kind, e.Key))
}

func (o observerPlugin) ObserveExpand(key, name, resolved string) {
	*o.expands = append(*o.expands, fmt.Sprintf("%s->%s=%q", key, name, resolved))
}

var (
	_ EntryObserver  = observerPlugin{}
	_ ExpandObserver = observerPlugin{}
)

func TestEntryObserver(t *testing.T) {
	// Observers see EVERY entry, including disabled pairs and unrecognised
	// lines. A commented-out setting is a finding for an auditor, not noise.
	t.Run("sees every entry kind", func(t *testing.T) {
		var entries, expands []string
		Parse("# prose\n\nA=1\n# B=disabled\nnot a pair\n",
			WithPlugin(observerPlugin{name: "audit", entries: &entries, expands: &expands}))

		want := []string{"comment:", "blank:", "pair:A", "disabled-pair:B", "other:"}
		if !reflect.DeepEqual(entries, want) {
			t.Errorf("entries = %v\nwant %v", entries, want)
		}
	})

	t.Run("a multi-line value is one entry", func(t *testing.T) {
		var entries, expands []string
		Parse("K=\"one\ntwo\nthree\"\nNEXT=n\n",
			WithPlugin(observerPlugin{name: "audit", entries: &entries, expands: &expands}))

		if !reflect.DeepEqual(entries, []string{"pair:K", "pair:NEXT"}) {
			t.Errorf("entries = %v, want the block reported once", entries)
		}
	})

	t.Run("ParseReader and Open report too", func(t *testing.T) {
		var entries, expands []string
		obs := WithPlugin(observerPlugin{name: "audit", entries: &entries, expands: &expands})

		if _, err := ParseReader(strings.NewReader("A=1\n"), obs); err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Errorf("ParseReader: entries = %v", entries)
		}

		entries = nil
		if _, err := Open(write(t, t.TempDir(), ".env", "B=2\n"), obs); err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Errorf("Open: entries = %v", entries)
		}
	})
}

func TestExpandObserver(t *testing.T) {
	t.Run("reports which variables a value used", func(t *testing.T) {
		var entries, expands []string
		f := Parse("BASE=root\nSUB=x\nK=${BASE}/${SUB}\n",
			WithPlugin(observerPlugin{name: "trace", entries: &entries, expands: &expands}))

		if _, _, err := f.GetExpanded("K"); err != nil {
			t.Fatal(err)
		}
		want := []string{`K->BASE="root"`, `K->SUB="x"`}
		if !reflect.DeepEqual(expands, want) {
			t.Errorf("expands = %v\nwant %v", expands, want)
		}
	})

	// Dead-key detection: which declared variables nothing references.
	t.Run("dead-key detection", func(t *testing.T) {
		var entries, expands []string
		used := map[string]bool{}
		f := Parse("USED=1\nDEAD=2\nK=${USED}\n",
			WithPlugin(observerPlugin{name: "trace", entries: &entries, expands: &expands}))

		if _, err := f.ExpandedMap(); err != nil {
			t.Fatal(err)
		}
		for _, e := range expands {
			used[strings.Split(strings.Split(e, "->")[1], "=")[0]] = true
		}
		if !used["USED"] {
			t.Error("USED was not reported as referenced")
		}
		if used["DEAD"] {
			t.Error("DEAD was reported as referenced")
		}
	})

	t.Run("reports the alternate and default branches", func(t *testing.T) {
		var entries, expands []string
		f := Parse("SET=x\nA=${SET:+alt}\nB=${NOPE:-fb}\nC=${NOPE:+never}\n",
			WithPlugin(observerPlugin{name: "trace", entries: &entries, expands: &expands}))

		if _, err := f.ExpandedMap(); err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(expands, " ")
		for _, want := range []string{`A->SET="alt"`, `B->NOPE="fb"`, `C->NOPE=""`} {
			if !strings.Contains(joined, want) {
				t.Errorf("expands = %v, missing %q", expands, want)
			}
		}
	})

	t.Run("a literal value reports nothing", func(t *testing.T) {
		var entries, expands []string
		f := Parse("A=x\nK='${A}'\n",
			WithPlugin(observerPlugin{name: "trace", entries: &entries, expands: &expands}))

		if _, _, err := f.GetExpanded("K"); err != nil {
			t.Fatal(err)
		}
		if len(expands) != 0 {
			t.Errorf("expands = %v, want none — a literal is never expanded", expands)
		}
	})
}

// TestOptionSugar_IgnoresNil: every function-option wrapper must treat nil as
// "not installed" rather than registering a plugin that panics on first use.
func TestOptionSugar_IgnoresNil(t *testing.T) {
	f := Parse("A=1\n",
		WithLookup(nil),
		WithValueTransform(nil),
		WithSaveGuard(nil),
	)

	if n := len(f.Plugins()); n != 0 {
		t.Errorf("Plugins() = %+v, want none installed", f.Plugins())
	}
	// The file must still read and write normally.
	if v, _, err := f.GetExpanded("A"); err != nil || v != "1" {
		t.Errorf("GetExpanded = %q, %v", v, err)
	}
}

// TestFuncPlugins_AreNamed covers the adapter names, which is what an error
// message shows when a function-option hook fails.
func TestFuncPlugins_AreNamed(t *testing.T) {
	f := Parse("A=1\n",
		WithLookup(func(string) (string, bool) { return "", false }),
		WithValueTransform(func(_, v string) (string, error) { return v, nil }),
		WithSaveGuard(func(*File) error { return nil }),
		WithCommandRunner(func(string) (string, error) { return "", nil }),
	)

	want := []PluginInfo{
		{Name: "lookup", Capabilities: []Capability{CapLookup}},
		{Name: "value-transform", Capabilities: []Capability{CapValueTransformer}},
		{Name: "save-guard", Capabilities: []Capability{CapSaveGuard}},
		{Name: "command-runner", Capabilities: []Capability{CapCommandRunner}},
	}
	if got := f.Plugins(); !reflect.DeepEqual(got, want) {
		t.Errorf("Plugins() = %+v\nwant %+v", got, want)
	}
}

// TestHasCapability covers every branch, including the unknown one that guards
// against a capability constant added without a matching case.
func TestHasCapability(t *testing.T) {
	var calls []string
	all := &multiCap{name: "all", calls: &calls}

	for _, c := range []Capability{CapLookup, CapCommandRunner} {
		if !hasCapability(all, c) {
			t.Errorf("hasCapability(multiCap, %s) = false", c)
		}
	}
	for _, c := range []Capability{CapValueTransformer, CapSaveGuard, CapEntryObserver, CapExpandObserver} {
		if hasCapability(all, c) {
			t.Errorf("hasCapability(multiCap, %s) = true, want false", c)
		}
	}

	var entries, expands []string
	obs := observerPlugin{name: "obs", entries: &entries, expands: &expands}
	for _, c := range []Capability{CapEntryObserver, CapExpandObserver} {
		if !hasCapability(obs, c) {
			t.Errorf("hasCapability(observerPlugin, %s) = false", c)
		}
	}

	if hasCapability(noCap{}, Capability("invented")) {
		t.Error("hasCapability with an unknown capability = true")
	}
}

// TestAllCapabilitiesOnOnePlugin is the design's whole justification: one object
// holding every hook, sharing one name and one set of state.
func TestAllCapabilitiesOnOnePlugin(t *testing.T) {
	p := &kitchenSink{}
	f := Parse("BASE=root\nK=${BASE}/${MISSING}\n", WithPlugin(p))

	want := []Capability{
		CapLookup, CapValueTransformer, CapCommandRunner,
		CapSaveGuard, CapEntryObserver, CapExpandObserver,
	}
	got := f.Plugins()
	if len(got) != 1 || !reflect.DeepEqual(got[0].Capabilities, want) {
		t.Fatalf("Plugins() = %+v\nwant one plugin with %v", got, want)
	}

	v, _, err := f.GetExpanded("K")
	if err != nil {
		t.Fatalf("GetExpanded: %v", err)
	}
	// Lookup supplied MISSING, then the transformer wrapped the final value.
	if v != "t(root/supplied)" {
		t.Errorf("GetExpanded = %q", v)
	}
	if p.entries == 0 || p.expands == 0 {
		t.Errorf("observers did not fire: entries=%d expands=%d", p.entries, p.expands)
	}

	if err := f.SaveAs(filepath.Join(t.TempDir(), ".env")); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	if p.guards != 1 {
		t.Errorf("guards = %d, want 1", p.guards)
	}
}

// kitchenSink implements every capability, to prove they compose on one object.
type kitchenSink struct {
	entries, expands, guards int
}

func (*kitchenSink) Name() string { return "sink" }

func (*kitchenSink) Lookup(name string) (string, bool, error) {
	if name == "MISSING" {
		return "supplied", true, nil
	}
	return "", false, nil
}
func (*kitchenSink) TransformValue(_, v string) (string, error) { return "t(" + v + ")", nil }
func (*kitchenSink) RunCommand(cmd string) (string, error)      { return cmd, nil }
func (k *kitchenSink) GuardSave(*File) error                    { k.guards++; return nil }
func (k *kitchenSink) ObserveEntry(*Entry)                      { k.entries++ }
func (k *kitchenSink) ObserveExpand(_, _, _ string)             { k.expands++ }

// --- Read -------------------------------------------------------------------

func TestRead(t *testing.T) {
	const src = "# banner\nDOMAIN=acme.io\nAPI=https://api.${DOMAIN}\n\n# OLD=disabled\nnot a pair\nDUP=first\nDUP=second\n"

	t.Run("returns expanded values only", func(t *testing.T) {
		m, err := Read(write(t, t.TempDir(), ".env", src))
		if err != nil {
			t.Fatalf("Read: %v", err)
		}

		want := map[string]string{
			"DOMAIN": "acme.io",
			"API":    "https://api.acme.io", // expanded, not the formula
			"DUP":    "second",              // last wins
		}
		if !reflect.DeepEqual(m, want) {
			t.Errorf("Read() = %v\nwant %v", m, want)
		}
	})

	// A missing file is an error here, unlike Open, which bootstraps one so a
	// caller can create it. Read is for consuming a file that should exist.
	t.Run("a missing file is an error", func(t *testing.T) {
		_, err := Read(filepath.Join(t.TempDir(), "absent.env"))
		if err == nil {
			t.Fatal("Read of a missing file = nil error")
		}
		if !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("err = %v, want it to say the file is missing", err)
		}
	})

	t.Run("an unreadable file is an error", func(t *testing.T) {
		if _, err := Read(t.TempDir()); err == nil {
			t.Error("Read of a directory = nil error")
		}
	})

	t.Run("a required-variable failure propagates", func(t *testing.T) {
		path := write(t, t.TempDir(), ".env", "K=${NOPE:?set me}\n")

		_, err := Read(path)
		var re *RequiredError
		if !errors.As(err, &re) {
			t.Fatalf("err = %v, want a *RequiredError", err)
		}
	})

	t.Run("options apply", func(t *testing.T) {
		path := write(t, t.TempDir(), ".env", "K=${FROM_PLUGIN}\n")

		m, err := Read(path, WithLookup(func(n string) (string, bool) {
			return "supplied", n == "FROM_PLUGIN"
		}))
		if err != nil {
			t.Fatal(err)
		}
		if m["K"] != "supplied" {
			t.Errorf("Read()[K] = %q, want the plugin's value", m["K"])
		}
	})

	t.Run("does not touch the environment", func(t *testing.T) {
		before := len(os.Environ())
		if _, err := Read(write(t, t.TempDir(), ".env", src)); err != nil {
			t.Fatal(err)
		}
		if after := len(os.Environ()); after != before {
			t.Errorf("environment changed size: %d -> %d", before, after)
		}
	})
}
