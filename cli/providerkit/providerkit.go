// Package providerkit is the seam between dotenvctl's core and its plugins
// (docsi/cli/PLUGINS_SPEC.md).
//
// The mandatory contract is one method — Plugin.Command(Deps) — a plugin is a
// cobra namespace, free to shape its own verbs. Everything else here is
// opt-in machinery that keeps plugins uniform where uniformity is honest:
// Deps carries the ONE value pipeline (plugins never parse env files), the
// capability interfaces plus New{Push,List,Prune}Cmd generate the standard
// secret-store verbs with the shared flags and safety rules, and Runner is
// the injectable exec seam that makes backend CLIs (gh, vercel) testable
// without a network.
package providerkit

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv/cli/envkit"
	"github.com/ubgo/dotenv/cli/outfmt"
)

// Plugin contributes one top-level namespace to the CLI. Constructor-per-
// command, no init(), no package state — a host may mount the tree under any
// root, twice if it wants.
type Plugin interface {
	Command(deps Deps) *cobra.Command
}

// Deps is everything a plugin may depend on from the host. Injected, never
// reached-for: plugins do not read os.Environ for configuration the CLI owns
// and do not open the env file themselves.
type Deps struct {
	// Printer is the single output seam — the {ok,data|error} envelope and
	// human printing behave identically across core and plugins because this
	// is the only way out.
	Printer *outfmt.Printer

	// File is the -f path the user chose. Display only — values come from
	// Source, never from re-parsing this path.
	File string

	// Source resolves the env file into a selection: parse once, expand,
	// filter, rename — centrally, so --prefix and placeholder-skipping mean
	// exactly one thing for every plugin. A ${VAR:?} required-error or IO
	// failure surfaces here, BEFORE any plugin side effect.
	Source func(SelectOpts) (Source, error)

	// Runner executes backend CLIs. Tests inject a fake to assert exact argv
	// and stdin; production uses ExecRunner.
	Runner Runner

	// Interactive reports whether a human is on stdin (a terminal), gating
	// whether Ask may prompt. Non-interactive mutating verbs require --yes.
	Interactive func() bool

	// Ask prompts the human and reports approval. Only called when
	// Interactive() is true.
	Ask func(prompt string) bool
}

// SelectOpts is the shared selection grammar — an ALIAS for the canonical
// type in envkit, not a copy. Selection semantics exist in exactly one place
// (envkit.Select); this alias just spares plugin authors an extra import.
type SelectOpts = envkit.SelectOptions

// Source is the resolved selection a verb consumes — the concrete result
// envkit produces. A struct rather than an interface on purpose: there is one
// implementation, it is pure data, and tests construct it directly instead of
// hand-rolling a fake.
type Source = *envkit.Selection

// Skip re-exports envkit's guard record so plugin code can name it.
type Skip = envkit.Skip

// SkipAction maps a selection guard's reason onto the push-action vocabulary.
// The two vocabularies stay separate deliberately: "why the selection held a
// key back" and "what a push did about it" are different questions that only
// happen to line up today.
func SkipAction(r envkit.SkipReason) Action {
	switch r {
	case envkit.SkipEmpty:
		return ActionSkippedEmpty
	default:
		return ActionSkippedPlaceholder
	}
}

// Action classifies one per-name outcome. Closed set — frozen API for humans
// and --json consumers alike (PLUGINS_SPEC §3).
type Action string

const (
	// ActionPushed: the value was written to the backend.
	ActionPushed Action = "pushed"
	// ActionWouldPush: dry-run — the value would be written.
	ActionWouldPush Action = "would-push"
	// ActionSkippedPlaceholder: guarded out — the value matches the
	// placeholder pattern and pushing a stand-in to production is a disaster.
	ActionSkippedPlaceholder Action = "skipped-placeholder"
	// ActionSkippedEmpty: guarded out — empty value.
	ActionSkippedEmpty Action = "skipped-empty"
	// ActionDeleted: the remote entry was removed (prune).
	ActionDeleted Action = "deleted"
	// ActionWouldDelete: dry-run prune.
	ActionWouldDelete Action = "would-delete"
	// ActionKeptSkipped: prune spared a remote name because the local key was
	// guard-skipped (placeholder/empty). Skipped ≠ stale — "not filled in
	// locally" must never mean "delete remotely" (SECRETS_PORT_SPEC §7.5).
	ActionKeptSkipped Action = "kept-skipped"
	// ActionKeptFlag: prune spared a remote name because --keep listed it —
	// the escape hatch for secrets managed outside the selection (bundles,
	// file secrets, other tools).
	ActionKeptFlag Action = "kept-keep-flag"
)

// Result is one name's outcome. Values never appear here — names and actions
// are the entire output surface.
type Result struct {
	Name   string `json:"name"`
	Action Action `json:"action"`
}

// Capability interfaces — a backend implements exactly what it truly
// supports; the kit generates verbs for those capabilities and no others.

// SecretWriter can upsert one named secret.
type SecretWriter interface {
	// Set writes value under name. The implementation MUST pass the value to
	// any child process via stdin, never argv (argv is world-readable in ps).
	Set(ctx context.Context, name, value string) error
}

// SecretLister can enumerate remote secret NAMES. Names only — most secret
// stores are write-only for values, and the kit never asks for them.
type SecretLister interface {
	Names(ctx context.Context) ([]string, error)
}

// SecretDeleter can remove one named secret.
type SecretDeleter interface {
	Delete(ctx context.Context, name string) error
}

// Runner executes a backend CLI. The seam exists so plugin tests assert the
// exact argv and stdin of every gh/vercel call without a network or the real
// binary installed.
type Runner interface {
	// Run executes name with args, feeding stdin when non-empty, returning
	// combined-stdout. A non-zero exit is an error carrying stderr context.
	Run(ctx context.Context, stdin string, name string, args ...string) ([]byte, error)
}

// ExecRunner is the production Runner: real subprocesses.
type ExecRunner struct{}

// Run implements Runner via os/exec. Stderr is captured into the error so a
// backend CLI's complaint ("HTTP 403") reaches the user instead of vanishing.
func (ExecRunner) Run(ctx context.Context, stdin string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != "" {
		cmd.Stdin = bytes.NewReader([]byte(stdin))
	}
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s %v: %w: %s", name, args, err, bytes.TrimSpace(errBuf.Bytes()))
	}
	return out.Bytes(), nil
}

// CmdError carries an exit code through cobra's error return, mirroring the
// core CLI's convention so plugin failures map to the same 0/1/2 contract.
// Constructed via Fail; the envelope/message has already been printed when
// Quiet is true.
type CmdError struct {
	Code  int
	Quiet bool
	Err   error
}

// Error implements error.
func (e *CmdError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit %d", e.Code)
	}
	return e.Err.Error()
}

// Exit codes duplicated from the core (frozen contract, PLAN.md): 1 =
// operation failed, 2 = usage. Redefined rather than imported to keep the
// dependency direction core→kit only.
const (
	exitFailure = 1
	exitUsage   = 2
)

// Fail emits the standard failure (envelope in JSON mode, dotenvctl: line in
// human mode) and returns the quiet operation-failure error — one line to
// fail correctly, impossible to forget the envelope.
func Fail(p *outfmt.Printer, code outfmt.ErrCode, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	if err := p.Fail(code, msg); err != nil {
		return &CmdError{Code: exitFailure, Err: err}
	}
	exit := exitFailure
	if code == outfmt.CodeUsage {
		exit = exitUsage
	}
	return &CmdError{Code: exit, Quiet: true, Err: fmt.Errorf("%s", msg)}
}
