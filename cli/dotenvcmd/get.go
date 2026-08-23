package dotenvcmd

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/internal/outfmt"
)

// getPayload is the --json data shape for `get`. Named struct rather than an
// inline map so the wire shape is greppable and stable.
type getPayload struct {
	Key string `json:"key"`
	// Value is raw by default; the resolved form when --expand was given.
	Value string `json:"value"`
	// Expanded records which of the two the caller received, so scripts never
	// have to re-derive it from their own flag handling.
	Expanded bool `json:"expanded"`
}

// newGetCmd prints one key's value.
//
// Exit contract: ExitFailure when the key has no ACTIVE entry — a disabled
// (commented-out) setting does not count, matching the library's Get.
func newGetCmd(a *app) *cobra.Command {
	var expand bool

	c := &cobra.Command{
		Use:   "get KEY",
		Short: "Print the value of KEY",
		Example: "  dotenvctl get DATABASE_URL\n" +
			"  dotenvctl get DATABASE_URL --expand   # resolve ${references}\n" +
			"  dotenvctl -f .env.prod get PORT --json",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			key := args[0]

			f, err := a.open()
			if err != nil {
				return err
			}

			value, found := f.Get(key)
			if expand {
				var expErr error
				value, found, expErr = f.GetExpanded(key)
				if _, ok := errors.AsType[*dotenv.RequiredError](expErr); ok {
					return a.failf(outfmt.CodeRequired, "%v", expErr)
				}
				if expErr != nil {
					return a.failf(outfmt.CodeIO, "expand %s: %v", key, expErr)
				}
			}
			if !found {
				return a.failf(outfmt.CodeNotFound, "key %q not found in %s", key, a.file)
			}

			a.printer.Human(value)
			if err := a.printer.OK(getPayload{Key: key, Value: value, Expanded: expand}); err != nil {
				return err
			}
			return nil
		},
	}

	c.Flags().BoolVar(&expand, flagExpand, false, "resolve ${VAR} references against the file")
	return c
}
