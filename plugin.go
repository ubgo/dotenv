package dotenv

import "fmt"

// Plugin is anything that can be installed with WithPlugin.
//
// A plugin declares its abilities by implementing the capability interfaces
// below — Lookuper, CommandRunner, and the rest — and implements only the ones
// it needs. One plugin may hold several: a secret store naturally wants Lookuper
// for undefined references and, later, a value transformer for its own URI
// scheme, sharing one client and one cache between them.
//
// Name is required rather than derived so a failure can say which plugin caused
// it: "dotenv: plugin \"vault\": lookup DB_PASSWORD: connection refused".
type Plugin interface {
	Name() string
}

// Lookuper supplies a value for a reference the file itself does not define.
//
// It runs AFTER the file's own keys, so the file always wins — adding an
// environment fallback cannot silently override a value somebody wrote down.
// Compose behaves the same way.
//
// Returning ok=false means "I do not have this", and the next Lookuper is tried;
// the first one to answer wins. An error aborts expansion rather than being
// treated as a miss, because a secret store that is down must not look like a
// variable that is unset.
//
// A supplied value is expanded like any other, so it may itself contain
// references, and the cycle guard covers it.
type Lookuper interface {
	Lookup(name string) (value string, ok bool, err error)
}

// CommandRunner executes a $(command) substitution.
//
// Unlike Lookuper this is a REPLACEMENT, not a chain: passing a command through
// two runners is meaningless, so the last one installed wins. Installing any
// runner enables command substitution, which is otherwise off.
type CommandRunner interface {
	RunCommand(cmd string) (string, error)
}

// ValueTransformer rewrites a value after it has been read.
//
// Runs AFTER expansion, and its output is NOT expanded again: a decrypted secret
// containing ${...} stays literal, which is what a secret full of dollar signs
// wants. Transformers are CHAINED — each sees the previous one's output — so a
// value can be dereferenced by one plugin and decoded by another.
//
// It applies to literal values too, single-quoted and backtick alike. Quoting
// controls interpolation, not transformation: an "encrypted:" value is very
// often single-quoted precisely to keep the parser out of it, and a transformer
// that skipped those would be useless.
//
// An error aborts the read rather than yielding the untransformed value, because
// a caller must never silently receive ciphertext where it expected a secret.
type ValueTransformer interface {
	TransformValue(key, value string) (string, error)
}

// SaveGuard inspects a file before it is written and may refuse.
//
// It may VETO ONLY. Rewriting content on the way to disk would breach the rule
// the package rests on — a hook may change what a value reads as, never what
// gets written — and would invalidate the round-trip guarantee everything else
// depends on. Encrypt-on-write is a real want and is deliberately not this.
//
// Every guard runs and the first error aborts the write, leaving the destination
// untouched.
type SaveGuard interface {
	GuardSave(f *File) error
}

// EntryObserver sees each entry as it is parsed.
//
// Infallible by design, which is what lets Parse keep its no-error signature. A
// plugin that needs to reject a file does it at SaveGuard time, or the caller
// inspects the parsed File.
//
// Observers see EVERY entry, including KindDisabledPair and KindOther. A
// commented-out setting is a finding for an auditor, not noise, and filtering it
// out would hide the most interesting case.
type EntryObserver interface {
	ObserveEntry(e *Entry)
}

// ExpandObserver sees every reference resolution.
//
// key is the entry whose value is being expanded, name is the variable
// referenced, and resolved is what it became. Enough to answer "which variables
// does this file actually use", which is what dead-key detection needs.
//
// Infallible, for the same reason as EntryObserver.
type ExpandObserver interface {
	ObserveExpand(key, name, resolved string)
}

// Capability names one ability a plugin was detected as having.
//
// Reported by Plugins so a missing capability is visible. Optional interfaces
// fail silently — a method with a slightly wrong signature compiles, installs,
// and never runs — so being able to see what was actually detected is the
// difference between a one-line fix and an hour of debugging.
type Capability string

const (
	CapLookup           Capability = "Lookuper"
	CapValueTransformer Capability = "ValueTransformer"
	CapCommandRunner    Capability = "CommandRunner"
	CapSaveGuard        Capability = "SaveGuard"
	CapEntryObserver    Capability = "EntryObserver"
	CapExpandObserver   Capability = "ExpandObserver"
)

// PluginInfo is one installed plugin and the capabilities it was recognised as
// having.
type PluginInfo struct {
	Name         string
	Capabilities []Capability
}

// String renders as "vault: Lookuper, CommandRunner", or "tracer: (none)".
func (p PluginInfo) String() string {
	if len(p.Capabilities) == 0 {
		return p.Name + ": (none)"
	}
	out := p.Name + ": "
	for i, c := range p.Capabilities {
		if i > 0 {
			out += ", "
		}
		out += string(c)
	}
	return out
}

// Plugins reports every installed plugin and what it was detected as.
//
// Use it when a plugin appears to do nothing: a capability missing from this
// list means the method exists under a different name or signature than the
// interface requires.
func (f *File) Plugins() []PluginInfo { return f.opts.pluginInfo() }

// funcLookuper adapts a plain function to Lookuper, so a small hook does not
// require declaring a type.
type funcLookuper struct {
	name string
	fn   func(string) (string, bool)
}

func (f funcLookuper) Name() string { return f.name }

func (f funcLookuper) Lookup(name string) (string, bool, error) {
	v, ok := f.fn(name)
	return v, ok, nil
}

// funcTransformer adapts a plain function to ValueTransformer.
type funcTransformer struct {
	name string
	fn   func(key, value string) (string, error)
}

func (f funcTransformer) Name() string { return f.name }

func (f funcTransformer) TransformValue(key, value string) (string, error) {
	return f.fn(key, value)
}

// funcGuard adapts a plain function to SaveGuard.
type funcGuard struct {
	name string
	fn   func(*File) error
}

func (f funcGuard) Name() string { return f.name }

func (f funcGuard) GuardSave(file *File) error { return f.fn(file) }

// funcRunner adapts a plain function to CommandRunner.
type funcRunner struct {
	name string
	fn   func(string) (string, error)
}

func (f funcRunner) Name() string { return f.name }

func (f funcRunner) RunCommand(cmd string) (string, error) { return f.fn(cmd) }

// pluginError wraps a plugin failure so the message names the plugin.
func pluginError(name, op string, err error) error {
	return fmt.Errorf("dotenv: plugin %q: %s: %w", name, op, err)
}
