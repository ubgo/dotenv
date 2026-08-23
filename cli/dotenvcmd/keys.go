package dotenvcmd

import (
	"github.com/spf13/cobra"
)

// keysPayload is the --json data shape for `keys`.
type keysPayload struct {
	Keys []string `json:"keys"`
}

// newKeysCmd prints active key names, one per line — the composition verb for
// xargs / grep pipelines, which is why human mode is bare names with no
// decoration at all.
func newKeysCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "keys",
		Short: "Print active key names, one per line",
		Example: "  dotenvctl keys\n" +
			"  dotenvctl keys | grep '^DB_'\n" +
			"  dotenvctl keys --json | jq -r '.data.keys[]'",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			f, err := a.open()
			if err != nil {
				return err
			}

			keys := f.Keys()
			for _, k := range keys {
				a.printer.Human(k)
			}
			// Normalize nil→empty so the JSON field is always an array, never
			// null — jq consumers should not need a null guard.
			if keys == nil {
				keys = []string{}
			}
			return a.printer.OK(keysPayload{Keys: keys})
		},
	}
}
