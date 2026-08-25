package dotenvcmd

import (
	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv/cli/envkit"
)

// restorePayload is the --json data shape for `restore`.
type restorePayload struct {
	Keys []string `json:"keys"`
}

// newRestoreCmd re-activates commented-out settings — the inverse of the
// default `unset`. It removes exactly the comment marker that was recorded,
// so "#KEY=v" comes back without inventing a space that was never there.
func newRestoreCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "restore KEY [KEY...]",
		Short: "Re-enable a commented-out setting",
		Example: "  dotenvctl restore DB_USER\n" +
			"  dotenvctl list --disabled            # see what can be restored",
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if _, err := envkit.Restore(a.file, args, envkit.EditOptions{}); err != nil {
				return a.editError(err)
			}
			return a.printer.OK(restorePayload{Keys: args})
		},
	}
}
