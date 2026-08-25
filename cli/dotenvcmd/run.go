package dotenvcmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv/cli/envkit"
	"github.com/ubgo/dotenv/cli/outfmt"
)

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
		RunE: func(cmd *cobra.Command, args []string) error {
			code, err := envkit.Run(cmd.Context(), a.file, args, envkit.RunOptions{
				Base:   os.Environ(),
				Stdout: a.printer.Out,
				Stderr: a.errOut,
			})
			if err != nil {
				return a.failf(outfmt.CodeExec, "%v", err)
			}
			if code != ExitOK {
				// The child ran and failed — its code IS our code.
				return exitWithCode(code)
			}
			return nil
		},
	}
}
