package dotenvcmd

import (
	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv/cli/internal/outfmt"
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
			f, err := a.open()
			if err != nil {
				return err
			}
			before := f.Render()

			for _, key := range args {
				if !f.Restore(key) {
					return a.failf(outfmt.CodeNotFound, "no disabled entry for %q in %s", key, a.file)
				}
			}

			if _, err := a.saveIfChanged(f, before); err != nil {
				return err
			}
			return a.printer.OK(restorePayload{Keys: args})
		},
	}
}
