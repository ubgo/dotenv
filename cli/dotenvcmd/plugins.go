package dotenvcmd

import (
	"bufio"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/ubgo/dotenv/cli/envkit"
	"github.com/ubgo/dotenv/cli/outfmt"
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

// pluginSource resolves the selection for plugins by delegating to envkit —
// the CLI adds nothing here but its file path and error mapping, which is the
// point: the selection semantics a plugin sees are byte-identical to the ones
// a Go caller of envkit.SelectFile sees.
func (a *app) pluginSource(opts providerkit.SelectOpts) (providerkit.Source, error) {
	sel, err := envkit.SelectFile(a.file, opts)
	if err != nil {
		return nil, a.selectionError(err)
	}
	return sel, nil
}

// selectionError maps an envkit selection failure onto the CLI's error codes:
// a named-but-absent key is "not found", an unsatisfied ${VAR:?} is
// "required", everything else is IO.
func (a *app) selectionError(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not found"):
		return a.failf(outfmt.CodeNotFound, "%v", err)
	case strings.Contains(msg, "expand"):
		return a.failf(outfmt.CodeRequired, "%v", err)
	default:
		return a.failf(outfmt.CodeIO, "%v", err)
	}
}

// stdinOrDefault resolves the answer source, defaulting to the real stdin so
// a direct (non-Execute) construction still behaves.
func (a *app) stdinOrDefault() io.Reader {
	if a.stdin == nil {
		return os.Stdin
	}
	return a.stdin
}

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
	line, err := bufio.NewReader(a.stdinOrDefault()).ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}
