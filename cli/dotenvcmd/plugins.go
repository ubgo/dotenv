package dotenvcmd

import (
	"bufio"
	"os"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/term"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/internal/outfmt"
	"github.com/ubgo/dotenv/cli/providerkit"
)

// pluginDeps assembles the Deps every plugin receives — the ONLY channel
// between plugins and the host (PLUGINS_SPEC §2). Built per invocation, so
// plugin state can never leak across runs.
func (a *app) pluginDeps() providerkit.Deps {
	return providerkit.Deps{
		Printer:     a.printer,
		File:        a.file,
		Source:      a.pluginSource,
		Runner:      a.runner,
		Interactive: a.interactiveFn,
		Ask:         a.askFn,
	}
}

// pluginSource is the one value pipeline behind every plugin: parse once,
// expand, select, rename, guard — centrally, so --prefix and placeholder
// semantics cannot drift between plugins (PLUGINS_SPEC §2).
func (a *app) pluginSource(opts providerkit.SelectOpts) (providerkit.Source, error) {
	f, err := a.openExisting(a.file)
	if err != nil {
		return nil, err
	}

	values, err := a.sourceValues(f, opts.Expand)
	if err != nil {
		return nil, err
	}

	keys, err := a.sourceKeys(f, opts)
	if err != nil {
		return nil, err
	}

	placeholderRe := regexp.MustCompile(defaultPlaceholderPattern)
	src := &pluginSource{}
	for _, key := range keys {
		value := values[key]
		name := key
		if opts.StripPrefix && opts.Prefix != "" {
			name = strings.TrimPrefix(key, opts.Prefix)
		}
		switch {
		case !opts.IncludePlaceholders && placeholderRe.MatchString(value):
			src.skips = append(src.skips, providerkit.Skip{Name: name, Action: providerkit.ActionSkippedPlaceholder})
		case !opts.IncludeEmpty && value == "":
			src.skips = append(src.skips, providerkit.Skip{Name: name, Action: providerkit.ActionSkippedEmpty})
		default:
			src.pairs = append(src.pairs, dotenv.Pair{Key: name, Value: value})
		}
	}
	return src, nil
}

// sourceValues returns the effective map, expanded when asked; a ${VAR:?}
// required-error fails here — before any plugin side effect.
func (a *app) sourceValues(f *dotenv.File, expand bool) (map[string]string, error) {
	if !expand {
		return f.Map(), nil
	}
	m, err := f.ExpandedMap()
	if err != nil {
		return nil, a.failf(outfmt.CodeRequired, "%v", err)
	}
	return m, nil
}

// sourceKeys resolves the selection to key names in file order, applying the
// SelectOpts resolution order (SECRETS_PORT_SPEC §1):
//
//	explicit Keys (as given; prefix filters do not second-guess them)
//	→ else: all keys → Prefix keeps → ExcludePrefix drops
//	→ IncludeKeys force-add (bypassing prefix filters)
//	→ ExcludeKeys drop
//
// An explicitly NAMED key (Keys or IncludeKeys) that does not exist fails
// loudly — silently skipping a key the user typed would report success for
// work that never happened. Excluding an absent key is a no-op: "make sure X
// never pushes" is valid even when X is already gone.
func (a *app) sourceKeys(f *dotenv.File, opts providerkit.SelectOpts) ([]string, error) {
	var keys []string
	switch {
	case len(opts.Keys) > 0:
		for _, k := range opts.Keys {
			if !f.Has(k) {
				return nil, a.failf(outfmt.CodeNotFound, "key %q not found in %s", k, a.file)
			}
		}
		keys = opts.Keys
	default:
		for _, k := range f.Keys() {
			if opts.Prefix != "" && !strings.HasPrefix(k, opts.Prefix) {
				continue
			}
			if opts.ExcludePrefix != "" && strings.HasPrefix(k, opts.ExcludePrefix) {
				continue
			}
			keys = append(keys, k)
		}
	}

	for _, k := range opts.IncludeKeys {
		if !f.Has(k) {
			return nil, a.failf(outfmt.CodeNotFound, "--include key %q not found in %s", k, a.file)
		}
		if !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}

	if len(opts.ExcludeKeys) > 0 {
		kept := keys[:0]
		for _, k := range keys {
			if !slices.Contains(opts.ExcludeKeys, k) {
				kept = append(kept, k)
			}
		}
		keys = kept
	}
	return keys, nil
}

// pluginSource is the concrete Source handed to plugins.
type pluginSource struct {
	pairs []dotenv.Pair
	skips []providerkit.Skip
}

// Pairs implements providerkit.Source.
func (s *pluginSource) Pairs() []dotenv.Pair { return s.pairs }

// Skipped implements providerkit.Source.
func (s *pluginSource) Skipped() []providerkit.Skip { return s.skips }

// stdinIsTerminal reports whether a human can answer a prompt.
//
// A real isatty check (x/term), NOT a char-device check: /dev/null IS a
// character device, so the naive Stat test classifies CI's `< /dev/null`
// stdin as interactive — the confirm gate would then prompt into the void
// and die with "aborted" instead of the actionable "--yes required" message.
// Caught by exactly that misbehavior in testing.
func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// askOnTerminal prompts on stderr (stdout may be piped or an envelope) and
// accepts y/yes, case-insensitive. Anything else — including read failure —
// is a No, because the safe default for "write to a remote store?" is no.
func (a *app) askOnTerminal(prompt string) bool {
	if _, err := a.errOut.Write([]byte(prompt)); err != nil {
		return false
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}
