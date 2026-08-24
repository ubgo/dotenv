package dotenvcmd

import (
	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/providerkit"
)

// keysPayload is the --json data shape for `keys`.
type keysPayload struct {
	Keys []string `json:"keys"`
}

// newKeysCmd prints active key names, one per line — the composition verb for
// xargs / grep pipelines, which is why human mode is bare names with no
// decoration at all. The shared selection flags filter (and optionally
// rename) the names through the same pipeline every other verb uses.
func newKeysCmd(a *app) *cobra.Command {
	var sel providerkit.SelectOpts

	c := &cobra.Command{
		Use:   "keys",
		Short: "Print active key names, one per line",
		Example: "  dotenvctl keys\n" +
			"  dotenvctl keys --prefix GITHUB_SECRET_ --strip-prefix\n" +
			"  dotenvctl keys --exclude-prefix GITHUB_SECRET_\n" +
			"  dotenvctl keys --json | jq -r '.data.keys[]'",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			f, err := a.open()
			if err != nil {
				return err
			}

			keys, err := a.collectKeyNames(f, sel)
			if err != nil {
				return err
			}
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

	providerkit.AddSelectionFlags(c, &sel)
	return c
}

// collectKeyNames returns the (possibly selected and renamed) active key
// names. Bare `keys` keeps its original path; any selection flag routes
// through the shared pipeline with guards lifted — names are names, a
// placeholder VALUE never hides one.
func (a *app) collectKeyNames(f *dotenv.File, sel providerkit.SelectOpts) ([]string, error) {
	if !providerkit.SelectionActive(sel) {
		return f.Keys(), nil
	}
	sel.IncludePlaceholders = true
	sel.IncludeEmpty = true
	src, err := a.pluginSource(sel)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(src.Pairs()))
	for _, p := range src.Pairs() {
		names = append(names, p.Key)
	}
	return names, nil
}
