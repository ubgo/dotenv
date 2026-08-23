// Package dotenvcmd wires the dotenvctl verb tree.
//
// Design rules (from docsi/cli/PLAN.md — keep them or the commands stop being
// embeddable):
//
//   - constructor-per-command (newXCmd(app)): no init() registration and no
//     package-level state, so a host application can mount the tree twice or
//     under another root without cross-talk;
//   - all output flows through the app's outfmt.Printer — a verb never prints
//     to os.Stdout directly, which is what makes tests and --json airtight;
//   - mutating verbs go Open → edit → Save and skip the Save when the render
//     is byte-identical, extending the library's no-op invariant to the CLI.
package dotenvcmd

import (
	"errors"
	"fmt"
	"io"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv/cli/internal/outfmt"
	"github.com/ubgo/dotenv/cli/plugins/githubplugin"
	"github.com/ubgo/dotenv/cli/providerkit"
)

// Exit codes — the CLI's contract with shells and CI. Closed set; scripts
// branch on these, so the values are frozen API.
const (
	// ExitOK: the operation succeeded (for diff: the files are identical).
	ExitOK = 0

	// ExitFailure: the operation ran and failed — missing key, required-var
	// error, diff found differences. Mirrors diff(1)'s "1 = differences".
	ExitFailure = 1

	// ExitUsage: the invocation itself was malformed (unknown flag, missing
	// operand). Mirrors the BSD EX_USAGE convention of 2-for-usage that
	// grep/diff follow.
	ExitUsage = 2
)

// Flag names shared across verbs — defined once so a rename cannot leave a
// stale literal behind in one verb's lookup.
const (
	flagFile        = "file"
	flagJSON        = "json"
	flagExpand      = "expand"
	flagDryRun      = "dry-run"
	flagDelete      = "delete"
	flagAfter       = "after"
	flagBefore      = "before"
	flagDisabled    = "disabled"
	flagInherited   = "inherited"
	flagValues      = "values"
	flagOnlyDrift   = "only-drift"
	flagContract    = "contract"
	flagPlaceholder = "placeholder"
)

// defaultEnvFile is where every verb looks when -f/--file is not given: the
// conventional ./.env of the working directory.
const defaultEnvFile = ".env"

// exitError carries an exit code through cobra's error return so Execute can
// map operation failures (1) apart from usage errors (2) without stringly
// inspection. Every RunE failure a verb REPORTS deliberately is one of these;
// anything else bubbling out of cobra is by definition a usage problem.
type exitError struct {
	code int
	// quiet suppresses Execute's fallback printing — set when the verb already
	// emitted its own failure envelope/message via the Printer.
	quiet bool
	err   error
}

// Error implements error.
func (e *exitError) Error() string {
	if e.err == nil {
		return fmt.Sprintf("exit %d", e.code)
	}
	return e.err.Error()
}

// failf builds the standard operation-failure path: it emits the failure
// through the printer (both modes) and returns the quiet ExitFailure error, so
// verbs fail in one line and cannot forget the envelope.
func (a *app) failf(code outfmt.ErrCode, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	if err := a.printer.Fail(code, msg); err != nil {
		return &exitError{code: ExitFailure, err: err}
	}
	return &exitError{code: ExitFailure, quiet: true, err: errors.New(msg)}
}

// exitWithCode wraps an already-reported failure with a specific exit code —
// used by `run`, whose contract is to pass the child's exit code through.
func exitWithCode(code int) error {
	return &exitError{code: code, quiet: true}
}

// app is the per-invocation state every verb constructor closes over: flag
// values bound at parse time plus the single output seam. One app per Execute
// call — never shared, never global.
type app struct {
	// file is the target .env path (-f/--file).
	file string

	// jsonOut mirrors --json; verbs read it only through printer.JSON.
	jsonOut bool

	printer *outfmt.Printer

	// errOut receives usage/cobra errors, kept separate from printer.Out so
	// --json stdout stays a single clean envelope.
	errOut io.Writer

	// runner / interactiveFn / askFn are the plugin-facing seams with real
	// defaults set in Execute; tests replace them to drive plugins end-to-end
	// without subprocesses or a terminal.
	runner        providerkit.Runner
	interactiveFn func() bool
	askFn         func(prompt string) bool
}

// Execute parses args, runs the selected verb, and returns the process exit
// code. It never calls os.Exit itself — main does — so tests drive the full
// CLI in-process with buffers for both streams.
func Execute(args []string, stdout, stderr io.Writer) int {
	a := &app{printer: &outfmt.Printer{Out: stdout}, errOut: stderr}
	return executeApp(a, args, stdout, stderr)
}

// executeApp is Execute with the app supplied — the test entry point that
// lets a suite pre-seed the plugin seams (fake runner, scripted prompts).
func executeApp(a *app, args []string, stdout, stderr io.Writer) int {
	if a.runner == nil {
		a.runner = providerkit.ExecRunner{}
	}
	if a.interactiveFn == nil {
		a.interactiveFn = stdinIsTerminal
	}
	if a.askFn == nil {
		a.askFn = a.askOnTerminal
	}

	root := newRootCmd(a)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	if err := root.Execute(); err != nil {
		if xe, ok := errors.AsType[*exitError](err); ok {
			if !xe.quiet && xe.err != nil {
				// Terminal stderr — a failed print of the failure message has
				// no further fallback; the exit code still tells the story.
				_, _ = fmt.Fprintf(stderr, "dotenvctl: %v\n", xe.err)
			}
			return xe.code
		}
		// Plugin/kit failures carry their own code via providerkit.CmdError —
		// the same 0/1/2 contract, produced on the other side of the seam.
		if pe, ok := errors.AsType[*providerkit.CmdError](err); ok {
			if !pe.Quiet && pe.Err != nil {
				_, _ = fmt.Fprintf(stderr, "dotenvctl: %v\n", pe.Err)
			}
			return pe.Code
		}
		// Cobra already printed the usage message for its own errors.
		return ExitUsage
	}
	return ExitOK
}

// newRootCmd assembles the verb tree. Persistent flags bind into the shared
// app so every verb sees the same -f/--json without redeclaring them.
func newRootCmd(a *app) *cobra.Command {
	root := &cobra.Command{
		Use:   "dotenvctl",
		Short: "Comment-preserving .env control — get, set, diff, run without disturbing a byte you didn't touch",
		Long: "dotenvctl edits .env files through the ubgo/dotenv library: every byte outside\n" +
			"the entry you touch survives verbatim — comments, blank lines, ordering, and\n" +
			"quoting style included. It never modifies its own process environment.",
		Version:       versionString(),
		SilenceUsage:  true,
		SilenceErrors: true,
		// PersistentPreRun rather than doing it in Execute: the flag values
		// exist only after cobra parses, and this is the first hook after.
		PersistentPreRun: func(*cobra.Command, []string) {
			a.printer.JSON = a.jsonOut
		},
	}

	root.PersistentFlags().StringVarP(&a.file, flagFile, "f", defaultEnvFile, "path to the .env file")
	root.PersistentFlags().BoolVar(&a.jsonOut, flagJSON, false, "emit a machine-readable {ok,data|error} envelope")

	root.AddCommand(
		newGetCmd(a),
		newSetCmd(a),
		newUnsetCmd(a),
		newRestoreCmd(a),
		newListCmd(a),
		newKeysCmd(a),
		newEnvsCmd(a),
		newMatrixCmd(a),
		newDiffCmd(a),
		newRunCmd(a),
	)

	// Plugins mount as namespaces via the one Deps channel (PLUGINS_SPEC §1).
	// Deps are built lazily-enough here: flag values (a.file, a.jsonOut) bind
	// before RunE fires, and Source parses only when a verb asks.
	deps := a.pluginDeps()
	root.AddCommand(githubplugin.New().Command(deps))
	return root
}

// versionString reads the module version stamped by `go install` — no
// hand-maintained constant to forget at release time. "devel" for source
// builds.
func versionString() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "devel"
}
