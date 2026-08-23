package dotenvcmd

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/internal/outfmt"
)

// envAssignSep joins key and value in a child environment entry — the
// KEY=VALUE shape os/exec expects.
const envAssignSep = "="

// newRunCmd executes a command with the file's values injected into the CHILD
// process environment. Our own os.Environ is never modified — that is the
// library's core promise and the CLI keeps it: the merge happens in cmd.Env
// only.
//
// Precedence: file values WIN over inherited environment values. That is why
// someone runs `dotenvctl run` — to impose the file. Inherited (name-only)
// declarations need no handling: the child inherits the parent environment as
// its base, which is exactly what those declarations mean.
//
// Exit contract deviates from 0/1/2 deliberately: the child's exit code is
// passed through VERBATIM, so `dotenvctl run -- make test` behaves like `make
// test` to CI. Our own failures (bad file, required-var error, spawn failure)
// use ExitFailure before the child ever starts.
func newRunCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "run -- COMMAND [ARGS...]",
		Short: "Run a command with the file's values in its environment",
		Example: "  dotenvctl run -- npm start\n" +
			"  dotenvctl -f .env.test run -- go test ./...\n" +
			"  dotenvctl run -- sh -c 'echo $DATABASE_URL'",
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			f, err := a.open()
			if err != nil {
				return err
			}

			values, err := f.ExpandedMap()
			if _, ok := errors.AsType[*dotenv.RequiredError](err); ok {
				return a.failf(outfmt.CodeRequired, "%v", err)
			}
			if err != nil {
				return a.failf(outfmt.CodeIO, "expand %s: %v", a.file, err)
			}

			child := exec.Command(args[0], args[1:]...)
			child.Env = mergeEnv(os.Environ(), values)
			// Output goes through the app's injected writers — NOT os.Stdout —
			// so tests capture it and an embedder's redirection is honored.
			// Stdin stays the real terminal: Execute takes no reader, and an
			// interactive child (a REPL, a prompt) must reach the user.
			child.Stdin = os.Stdin
			child.Stdout = a.printer.Out
			child.Stderr = a.errOut

			if err := child.Run(); err != nil {
				if xerr, ok := errors.AsType[*exec.ExitError](err); ok {
					// The child ran and failed — its code IS our code.
					return exitWithCode(xerr.ExitCode())
				}
				return a.failf(outfmt.CodeExec, "run %s: %v", args[0], err)
			}
			return nil
		},
	}
}

// mergeEnv overlays the file's values onto the inherited environment,
// file-wins. Order matters to os/exec: for duplicate names the LAST entry
// wins, so file values are appended after the base — replacing in place would
// be equivalent but O(n·m) for no benefit.
func mergeEnv(base []string, values map[string]string) []string {
	// Track which names the file defines so the base copy drops them —
	// appending alone would work, but duplicate entries confuse tools that
	// read the raw environ block (e.g. `env | sort` in the child).
	replaced := make(map[string]bool, len(values))
	for k := range values {
		replaced[k] = true
	}

	out := make([]string, 0, len(base)+len(values))
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, envAssignSep)
		if ok && replaced[name] {
			continue
		}
		out = append(out, entry)
	}
	for k, v := range values {
		out = append(out, k+envAssignSep+v)
	}
	return out
}
