package dotenvcmd

import (
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv/cli/envkit"
	"github.com/ubgo/dotenv/cli/outfmt"
)

// flagDir selects the discovery directory on multi-file verbs; shared const
// for the same reason as the other flag names.
const flagDir = "dir"

// defaultPlaceholderPattern marks values that are stand-ins rather than
// configuration — the `__YOU__` / `__CONFIRM__` convention. Spec §2; verbs
// that care expose --placeholder to override it.
const defaultPlaceholderPattern = `^__[A-Z0-9_]+__$`

// envFilePayload is one row of envs --json — an alias for envkit's canonical
// inventory type.
type envFilePayload = envkit.Inventory

// envsPayload is the --json data shape for `envs`.
type envsPayload struct {
	Files []envFilePayload `json:"files"`
}

// newEnvsCmd lists the directory's env-file family — the discovery verb the
// other multi-file verbs build on (spec §1).
func newEnvsCmd(a *app) *cobra.Command {
	var dir string

	c := &cobra.Command{
		Use:   "envs",
		Short: "Discover the directory's .env file family",
		Example: "  dotenvctl envs\n" +
			"  dotenvctl envs --dir ./deploy\n" +
			"  dotenvctl envs --json | jq -r '.data.files[].file'",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			rows, err := envkit.Inventories(envkit.InventoryOptions{Dir: dir})
			if err != nil {
				return a.failf(outfmt.CodeIO, "%v", err)
			}

			a.renderEnvsHuman(rows)
			return a.printer.OK(envsPayload{Files: rows})
		},
	}

	c.Flags().StringVar(&dir, flagDir, ".", "directory to scan for .env files")
	return c
}

// renderEnvsHuman prints the discovery table; empty discovery says so rather
// than printing a bare header over nothing.
func (a *app) renderEnvsHuman(rows []envFilePayload) {
	if a.printer.JSON {
		return
	}
	if len(rows) == 0 {
		a.printer.Human("no env files found")
		return
	}

	w := tabwriter.NewWriter(a.printer.Out, 0, 0, tabPadding, ' ', 0)
	writeTabRow(w, "ENV", "FILE", "KEYS", "DISABLED", "INHERITED", "PLACEHOLDERS")
	for _, r := range rows {
		env := r.Env
		if r.Contract {
			env += " (contract)"
		}
		writeTabRow(w, env, r.File, strconv.Itoa(r.Keys), strconv.Itoa(r.Disabled), strconv.Itoa(r.Inherited), strconv.Itoa(r.Placeholders))
	}
	// Terminal output; same deliberate flush-error drop as list's table.
	_ = w.Flush()
}
