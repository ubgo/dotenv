package dotenvcmd

import (
	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv/cli/internal/outfmt"
)

// unsetPayload is the --json data shape for `unset`.
type unsetPayload struct {
	Keys []string `json:"keys"`
	// Deleted is true for --delete (entry removed outright); false for the
	// default comment-out, which `restore` can reverse.
	Deleted bool `json:"deleted"`
	DryRun  bool `json:"dry_run"`
}

// newUnsetCmd deactivates keys. The DEFAULT is comment-out, not removal —
// reversible, diff-friendly, and the documentation above a setting stays
// attached to something. Removal is the explicit --delete opt-in.
func newUnsetCmd(a *app) *cobra.Command {
	var del, dryRun bool

	c := &cobra.Command{
		Use:   "unset KEY [KEY...]",
		Short: "Comment a setting out (or --delete it outright)",
		Example: "  dotenvctl unset OLD_KEY              # '# OLD_KEY=…' — reversible via restore\n" +
			"  dotenvctl unset OLD_KEY --delete     # remove the line entirely\n" +
			"  dotenvctl unset OLD_KEY --dry-run    # preview the diff, write nothing",
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			f, err := a.open()
			if err != nil {
				return err
			}
			before := f.Render()

			for _, key := range args {
				if !f.Unset(key, del) {
					return a.failf(outfmt.CodeNotFound, "key %q not found in %s", key, a.file)
				}
			}

			if _, err := a.previewOrSave(f, before, dryRun); err != nil {
				return err
			}
			return a.printer.OK(unsetPayload{Keys: args, Deleted: del, DryRun: dryRun})
		},
	}

	c.Flags().BoolVar(&del, flagDelete, false, "remove the entry instead of commenting it out")
	c.Flags().BoolVar(&dryRun, flagDryRun, false, "print the would-be diff without writing")
	return c
}
