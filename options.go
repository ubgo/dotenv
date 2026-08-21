package dotenv

// EscapeMode selects how backslash sequences inside double-quoted values are
// decoded.
//
// The two modes are genuinely incompatible, which is why this is a choice rather
// than a default someone has to work around: under Compose, `\n` in a value
// stays two characters; under Extended it becomes a newline. Whichever a parser
// picks, some .env in the wild is misread — so the caller picks, based on who
// wrote the file.
type EscapeMode int

const (
	// EscapeExtended interprets \n, \r, \t, \" and \\ — what most .env
	// tooling outside Docker does, and what most application developers
	// expect. This is the default.
	EscapeExtended EscapeMode = iota

	// EscapeCompose interprets only \" and \\. Every other backslash sequence
	// is kept literally, which is how Docker Compose reads a .env. Use it for
	// files compose consumes, and for values holding Windows paths or regexes
	// where a backslash means itself.
	EscapeCompose
)

// String implements fmt.Stringer.
func (m EscapeMode) String() string {
	switch m {
	case EscapeExtended:
		return "extended"
	case EscapeCompose:
		return "compose"
	default:
		return "unknown"
	}
}

// options is the resolved configuration for a parse.
type options struct {
	escapes    EscapeMode
	allowShell bool
	runCommand func(string) (string, error)

	// plugins is every installed plugin, in install order, kept for reporting.
	plugins []Plugin

	// The remaining slices are the other pre-filtered capability subsets, for
	// the same reason as lookupers below.
	transformers []ValueTransformer
	guards       []SaveGuard
	entryObs     []EntryObserver
	expandObs    []ExpandObserver

	// lookupers is the pre-filtered subset implementing Lookuper.
	//
	// Capabilities are resolved ONCE at construction rather than asserted per
	// call: expansion runs this loop for every reference in every value, and a
	// type assertion there would be paid on the hottest path in the package. A
	// plugin without the capability then costs nothing at all.
	lookupers []Lookuper
}

// addPlugin installs p and records which capabilities it was detected as having.
func (o *options) addPlugin(p Plugin) {
	o.plugins = append(o.plugins, p)

	if v, ok := p.(Lookuper); ok {
		o.lookupers = append(o.lookupers, v)
	}
	if v, ok := p.(ValueTransformer); ok {
		o.transformers = append(o.transformers, v)
	}
	if v, ok := p.(SaveGuard); ok {
		o.guards = append(o.guards, v)
	}
	if v, ok := p.(EntryObserver); ok {
		o.entryObs = append(o.entryObs, v)
	}
	if v, ok := p.(ExpandObserver); ok {
		o.expandObs = append(o.expandObs, v)
	}
	if v, ok := p.(CommandRunner); ok {
		// A replacement rather than a chain: the last runner installed wins,
		// and installing one enables substitution.
		o.allowShell = true
		o.runCommand = v.RunCommand
	}
}

// pluginNameFor returns the name of the i-th plugin holding capability cap.
//
// The capability slices are filtered, so their indices do not line up with the
// install order; this walks it to find the matching plugin, so an error can name
// the one that actually failed.
func (o *options) pluginNameFor(cap Capability, i int) string {
	seen := 0
	for _, p := range o.plugins {
		if !hasCapability(p, cap) {
			continue
		}
		if seen == i {
			return p.Name()
		}
		seen++
	}
	return "unknown"
}

// hasCapability reports whether p implements the interface named by cap.
func hasCapability(p Plugin, cap Capability) bool {
	switch cap {
	case CapLookup:
		_, ok := p.(Lookuper)
		return ok
	case CapValueTransformer:
		_, ok := p.(ValueTransformer)
		return ok
	case CapCommandRunner:
		_, ok := p.(CommandRunner)
		return ok
	case CapSaveGuard:
		_, ok := p.(SaveGuard)
		return ok
	case CapEntryObserver:
		_, ok := p.(EntryObserver)
		return ok
	case CapExpandObserver:
		_, ok := p.(ExpandObserver)
		return ok
	default:
		return false
	}
}

// pluginInfo reports each plugin with the capabilities detected for it.
func (o *options) pluginInfo() []PluginInfo {
	// A fixed order so diagnostics read the same on every run.
	all := []Capability{
		CapLookup, CapValueTransformer, CapCommandRunner,
		CapSaveGuard, CapEntryObserver, CapExpandObserver,
	}

	out := make([]PluginInfo, 0, len(o.plugins))
	for _, p := range o.plugins {
		info := PluginInfo{Name: p.Name()}
		for _, c := range all {
			if hasCapability(p, c) {
				info.Capabilities = append(info.Capabilities, c)
			}
		}
		out = append(out, info)
	}
	return out
}

// Option customises parsing and expansion.
type Option func(*options)

// WithEscapes selects the escape-sequence dialect. Defaults to EscapeExtended.
func WithEscapes(m EscapeMode) Option {
	return func(o *options) { o.escapes = m }
}

// WithCommandSubstitution enables $(command) expansion in GetExpanded.
//
// OFF by default, deliberately. With it on, merely READING a configuration file
// executes arbitrary shell — so a .env fetched from a repository, a container
// image, or a teammate becomes remote code execution. That is a decision the
// calling application must make knowingly about files it trusts, not something
// a parser should do silently.
//
// When enabled, commands run through `sh -c` and a failing command expands to
// the empty string, matching how shells treat a failed substitution in a value.
func WithCommandSubstitution(enabled bool) Option {
	return func(o *options) { o.allowShell = enabled }
}

// WithCommandRunner overrides how $(command) is executed.
//
// Exists so tests can exercise substitution without spawning a shell, and so an
// application can supply a sandboxed or allow-listed runner instead of raw
// `sh -c`. Implies WithCommandSubstitution(true).
//
// Sugar for WithPlugin over a CommandRunner — a one-function hook should not
// require declaring a type.
func WithCommandRunner(run func(string) (string, error)) Option {
	return func(o *options) {
		if run == nil {
			o.allowShell = true
			return
		}
		o.addPlugin(funcRunner{name: "command-runner", fn: run})
	}
}

// WithValueTransform rewrites values after they are read.
//
// Sugar for WithPlugin over a ValueTransformer:
//
//	dotenv.Parse(src, dotenv.WithValueTransform(func(key, v string) (string, error) {
//	    s, ok := strings.CutPrefix(v, "encrypted:")
//	    if !ok { return v, nil }
//	    return decrypt(s)
//	}))
func WithValueTransform(fn func(key, value string) (string, error)) Option {
	return func(o *options) {
		if fn == nil {
			return
		}
		o.addPlugin(funcTransformer{name: "value-transform", fn: fn})
	}
}

// WithSaveGuard refuses a write when the file fails a check.
//
// Sugar for WithPlugin over a SaveGuard. The guard may reject, never rewrite.
func WithSaveGuard(fn func(*File) error) Option {
	return func(o *options) {
		if fn == nil {
			return
		}
		o.addPlugin(funcGuard{name: "save-guard", fn: fn})
	}
}

// WithPlugin installs a plugin.
//
// The plugin's capabilities are detected once, here, by type assertion. A
// plugin implementing none is legal — it may exist only to be listed — but is
// usually a signature typo, which is what Plugins exists to make visible.
func WithPlugin(plugins ...Plugin) Option {
	return func(o *options) {
		for _, p := range plugins {
			if p == nil {
				continue
			}
			o.addPlugin(p)
		}
	}
}

// WithLookup supplies values for references the file does not define.
//
// Sugar for WithPlugin over a Lookuper:
//
//	dotenv.Parse(src, dotenv.WithLookup(os.LookupEnv))
//
// That single line is how a caller opts into Compose's environment fallback
// without this package ever touching os.Environ itself.
func WithLookup(fn func(name string) (string, bool)) Option {
	return func(o *options) {
		if fn == nil {
			return
		}
		o.addPlugin(funcLookuper{name: "lookup", fn: fn})
	}
}

// newOptions applies opts over the defaults.
//
// runCommand starts as shellRun and is only ever replaced by a CommandRunner's
// method value, so it cannot end up nil — no post-loop guard is needed.
//
// A caller CAN defeat that by installing a typed-nil plugin —
// WithPlugin((*MyPlugin)(nil)) satisfies the interface and panics on call. Only
// reflection distinguishes that from a real value, which is not worth carrying
// on every install for a mistake the compiler already makes obvious at the call
// site.
func newOptions(opts []Option) options {
	o := options{escapes: EscapeExtended, runCommand: shellRun}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}
