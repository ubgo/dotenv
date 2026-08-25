package dotenvcmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv/cli/envkit"
	"github.com/ubgo/dotenv/cli/outfmt"
	"github.com/ubgo/dotenv/cli/report"
)

// CellState and the state constants are aliases for envkit's canonical
// vocabulary — kept exported here so existing importers of dotenvcmd keep
// compiling, but there is exactly ONE definition (envkit's).
type CellState = envkit.CellState

const (
	StatePresent     = envkit.StatePresent
	StateEmpty       = envkit.StateEmpty
	StatePlaceholder = envkit.StatePlaceholder
	StateDisabled    = envkit.StateDisabled
	StateInherited   = envkit.StateInherited
	StateMissing     = envkit.StateMissing
)

// newMatrixCmd renders the keys × environments drift table. The command owns
// flags and formatting only — the table itself comes from envkit.BuildMatrix,
// which any Go program can call directly.
func newMatrixCmd(a *app) *cobra.Command {
	var (
		dir, contractPath, placeholderPat, format, output string
		showValues, reveal, onlyDrift                     bool
	)

	c := &cobra.Command{
		Use:   "matrix [FILE...]",
		Short: "Compare keys across every environment file",
		Long: "Matrix renders every key across the directory's env files (or the FILEs you\n" +
			"name) so drift is visible at a glance. Cell legend:\n\n" +
			"  ✓  present     an active pair with a real value\n" +
			"  !  placeholder set, but to a __STAND_IN__ value — still needs a real one\n" +
			"  ∅  empty       set to an empty string (KEY=)\n" +
			"  #  disabled    only a commented-out setting exists (# KEY=value)\n" +
			"  →  inherited   name-only declaration; the value comes from the environment\n" +
			"  —  missing     no entry of any kind\n\n" +
			"Values are never shown unless you ask (--values) and never real unless you\n" +
			"insist (--reveal); --contract turns the matrix into a CI gate (exit 1 on gaps).\n\n" +
			"--only-drift keeps a row when ANY cell is not ✓ — missing counts as drift.\n" +
			"So if one discovered file has only a few keys (a bare .env next to full\n" +
			".env.stag/.env.prod), its — cells mark nearly every row as drift and the\n" +
			"filter appears to do nothing. Name the files you actually want to compare:\n\n" +
			"  dotenvctl matrix .env.staging .env.prod --only-drift",
		Example: "  dotenvctl matrix\n" +
			"  dotenvctl matrix --only-drift\n" +
			"  dotenvctl matrix .env.staging .env.prod --values\n" +
			"  dotenvctl matrix --contract .env.example   # exit 1 on gaps\n" +
			"  dotenvctl matrix --format html -o envs.html",
		RunE: func(_ *cobra.Command, args []string) error {
			opts := envkit.MatrixOptions{
				Files:              args,
				Dir:                dir,
				ContractPath:       contractPath,
				PlaceholderPattern: placeholderPat,
				Reveal:             reveal,
			}

			// Built WITHOUT the drift filter so the pre-filter row count is
			// known — the sparse-column hint needs both numbers.
			m, err := envkit.BuildMatrix(opts)
			if err != nil {
				return a.failf(outfmt.CodeIO, "%v", err)
			}
			rowsBefore := len(m.Rows)
			var driftHint string
			if onlyDrift {
				m.Rows = envkit.DriftRows(m.Rows)
				if env, missing, ok := envkit.SparseColumn(m, rowsBefore); ok {
					driftHint = sparseColumnHint(rowsBefore-len(m.Rows), env, missing, len(m.Rows))
				}
			}

			if err := a.emitFormatted(format, output, m, func() {
				a.renderMatrixHuman(m, showValues, reveal)
				if driftHint != "" {
					a.printer.Human(driftHint)
				}
			}, func() ([]byte, error) {
				return report.MatrixHTML(matrixReport(m, reveal))
			}); err != nil {
				return err
			}

			// Contract gaps fail AFTER full output, diff(1)-style: the report
			// is the answer, the exit code is the shell signal.
			if m.HasContractGaps() {
				return exitWithCode(ExitFailure)
			}
			return nil
		},
	}

	c.Flags().StringVar(&dir, flagDir, ".", "directory to scan when no FILEs are given")
	c.Flags().BoolVar(&showValues, flagValues, false, "show values instead of state symbols (masked unless --reveal)")
	c.Flags().BoolVar(&reveal, flagReveal, false, "include real values in output — treat the result as a secret")
	c.Flags().BoolVar(&onlyDrift, flagOnlyDrift, false, "hide rows that are ✓ present in EVERY column (a missing cell counts as drift, so a sparse file keeps all rows visible — name FILEs to compare only those)")
	c.Flags().StringVar(&contractPath, flagContract, "", "contract file (.env.example); exit 1 when an env misses one of its keys")
	c.Flags().StringVar(&placeholderPat, flagPlaceholder, envkit.DefaultPlaceholderPattern, "regexp marking placeholder values")
	addFormatFlags(c, &format, &output)
	return c
}

// sparseColumnHint phrases envkit's sparse-column diagnosis for a human —
// the message a user needs when --only-drift appears to do nothing.
func sparseColumnHint(hidden int, env string, missing, rows int) string {
	return fmt.Sprintf("\nhint: --only-drift hid %d row(s) — %q is missing %d of %d keys, so almost every row counts as drift. Compare just the files you mean: dotenvctl matrix FILE_A FILE_B --only-drift",
		hidden, env, missing, rows)
}

// renderMatrixHuman prints the aligned table. Cell content: symbols, or —
// with values — the (masked) value for value-bearing states.
func (a *app) renderMatrixHuman(m *envkit.Matrix, showValues, reveal bool) {
	if a.printer.JSON {
		return
	}
	if reveal {
		a.printer.Human("⚠ values revealed — treat this output as a secret")
	}

	w := tabwriter.NewWriter(a.printer.Out, 0, 0, tabPadding, ' ', 0)
	writeTabRow(w, append([]string{"KEY"}, m.Envs...)...)
	for _, row := range m.Rows {
		cells := make([]string, 0, len(row.Cells)+1)
		cells = append(cells, row.Key)
		for _, c := range row.Cells {
			cells = append(cells, cellText(c, showValues, reveal))
		}
		writeTabRow(w, cells...)
	}
	// Terminal output; same deliberate flush-error drop as the other tables.
	_ = w.Flush()

	// One-line legend, matching the HTML report's footer — symbols without a
	// key are a quiz, and `--help` is a page away, not a glance away.
	a.printer.Human("\n✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing")

	if m.ContractMissing != nil {
		for _, env := range m.Envs {
			if keys := m.ContractMissing[env]; len(keys) > 0 {
				a.printer.Humanf("\ncontract: %s is missing %d key(s): %v", env, len(keys), keys)
			}
		}
	}
}

// cellText picks what one cell shows in the human table.
func cellText(c envkit.MatrixCell, showValues, reveal bool) string {
	valueBearing := c.State == StatePresent || c.State == StatePlaceholder || c.State == StateEmpty
	if !showValues || !valueBearing {
		return envkit.StateSymbols[c.State]
	}
	if !reveal {
		return report.MaskGlyph
	}
	if c.Raw() == "" {
		return envkit.StateSymbols[StateEmpty]
	}
	return c.Raw()
}

// matrixReport adapts envkit's matrix to the report package's input, keeping
// HTML templating decoupled from the operations types.
func matrixReport(m *envkit.Matrix, reveal bool) report.Matrix {
	rows := make([]report.MatrixRow, 0, len(m.Rows))
	for _, r := range m.Rows {
		row := report.MatrixRow{Key: r.Key}
		for _, c := range r.Cells {
			row.Cells = append(row.Cells, report.MatrixCell{
				State:  string(c.State),
				Symbol: envkit.StateSymbols[c.State],
				// Value flows to the template ONLY when revealed — masked
				// HTML must contain no secret bytes (spec §3).
				Value: c.Value,
			})
		}
		rows = append(rows, row)
	}
	return report.Matrix{
		Envs:            m.Envs,
		Rows:            rows,
		Sources:         m.Sources,
		Revealed:        reveal,
		ContractMissing: m.ContractMissing,
	}
}
