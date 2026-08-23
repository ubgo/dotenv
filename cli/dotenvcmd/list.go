package dotenvcmd

import (
	"errors"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/internal/outfmt"
)

// pairPayload is one key/value in list's --json output.
type pairPayload struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// listPayload is the --json data shape for `list`. All three sections are
// always present (empty arrays, never null) so the schema is identical no
// matter which display flags were given — flags shape the HUMAN view only.
type listPayload struct {
	// Pairs are the active entries in file order; duplicates included, exactly
	// as a linear read of the file would see them (expanded mode collapses to
	// effective last-wins values, so there each key appears once).
	Pairs []pairPayload `json:"pairs"`
	// Disabled are commented-out settings ("# KEY=value") — inactive but
	// visible, so tooling can say "disabled" instead of the misleading
	// "missing".
	Disabled []pairPayload `json:"disabled"`
	// Inherited are Compose name-only declarations. Names, not values: the
	// library never reads the process environment, and neither does list.
	Inherited []string `json:"inherited"`
	Expanded  bool     `json:"expanded"`
}

// tabPadding is the tabwriter cell gap for the human table — two spaces reads
// as a table without drifting into ASCII-art territory.
const tabPadding = 2

// newListCmd shows the file's effective contents.
func newListCmd(a *app) *cobra.Command {
	var expand, showDisabled, showInherited bool

	c := &cobra.Command{
		Use:   "list",
		Short: "List active pairs (add --disabled / --inherited for the full picture)",
		Example: "  dotenvctl list\n" +
			"  dotenvctl list --expand\n" +
			"  dotenvctl list --disabled --inherited --json",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			f, err := a.open()
			if err != nil {
				return err
			}

			pairs, err := a.collectPairs(f, expand)
			if err != nil {
				return err
			}

			disabled := make([]pairPayload, 0, len(f.Disabled()))
			for _, d := range f.Disabled() {
				disabled = append(disabled, pairPayload{Key: d.Key, Value: d.Value})
			}
			inherited := f.Inherited()
			if inherited == nil {
				inherited = []string{}
			}

			a.renderListHuman(pairs, disabled, inherited, showDisabled, showInherited)
			return a.printer.OK(listPayload{Pairs: pairs, Disabled: disabled, Inherited: inherited, Expanded: expand})
		},
	}

	c.Flags().BoolVar(&expand, flagExpand, false, "resolve ${VAR} references against the file")
	c.Flags().BoolVar(&showDisabled, flagDisabled, false, "also show commented-out settings")
	c.Flags().BoolVar(&showInherited, flagInherited, false, "also show inherited (name-only) declarations")
	return c
}

// collectPairs returns the rows to display: raw Pairs in file order, or — with
// expand — the effective last-wins values in key order, because expansion is
// defined over the effective view, not over shadowed duplicates.
func (a *app) collectPairs(f *dotenv.File, expand bool) ([]pairPayload, error) {
	if !expand {
		raw := f.Pairs()
		out := make([]pairPayload, 0, len(raw))
		for _, p := range raw {
			out = append(out, pairPayload{Key: p.Key, Value: p.Value})
		}
		return out, nil
	}

	keys := f.Keys()
	out := make([]pairPayload, 0, len(keys))
	for _, k := range keys {
		v, _, err := f.GetExpanded(k)
		if _, ok := errors.AsType[*dotenv.RequiredError](err); ok {
			return nil, a.failf(outfmt.CodeRequired, "%v", err)
		}
		if err != nil {
			return nil, a.failf(outfmt.CodeIO, "expand %s: %v", k, err)
		}
		out = append(out, pairPayload{Key: k, Value: v})
	}
	return out, nil
}

// renderListHuman prints the aligned table plus the optional sections. Human
// mode only; the printer swallows it under --json.
func (a *app) renderListHuman(pairs, disabled []pairPayload, inherited []string, showDisabled, showInherited bool) {
	if a.printer.JSON {
		return
	}

	w := tabwriter.NewWriter(a.printer.Out, 0, 0, tabPadding, ' ', 0)
	for _, p := range pairs {
		writeTabRow(w, p.Key, p.Value)
	}
	// Flush error deliberately dropped: tabwriter only propagates the
	// underlying writer's error, and human output targets a terminal buffer
	// whose failure has no recovery path mid-print.
	_ = w.Flush()

	if showDisabled && len(disabled) > 0 {
		a.printer.Human("\n# disabled")
		for _, d := range disabled {
			a.printer.Humanf("# %s=%s", d.Key, d.Value)
		}
	}
	if showInherited && len(inherited) > 0 {
		a.printer.Human("\n# inherited (values come from the environment, not this file)")
		for _, name := range inherited {
			a.printer.Human(name)
		}
	}
}

// writeTabRow emits one tabwriter row from its cells; isolated so the
// write-error drop is in exactly one commented place rather than scattered per
// loop iteration.
func writeTabRow(w *tabwriter.Writer, cells ...string) {
	// Same rationale as the Flush above — terminal output, no recovery path.
	_, _ = w.Write([]byte(strings.Join(cells, "\t") + "\n"))
}
