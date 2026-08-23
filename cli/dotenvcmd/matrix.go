package dotenvcmd

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/internal/discover"
	"github.com/ubgo/dotenv/cli/internal/outfmt"
	"github.com/ubgo/dotenv/cli/internal/report"
)

// CellState classifies one key × environment cell. Closed set (spec §2) —
// symbols and JSON strings are frozen API for both the human table and jq
// consumers.
type CellState string

const (
	// StatePresent: an active pair with a real value.
	StatePresent CellState = "present"
	// StateEmpty: an active pair whose value is "".
	StateEmpty CellState = "empty"
	// StatePlaceholder: an active pair whose value matches the placeholder
	// pattern — configured but still fake.
	StatePlaceholder CellState = "placeholder"
	// StateDisabled: only a commented-out setting exists.
	StateDisabled CellState = "disabled"
	// StateInherited: only a name-only declaration exists.
	StateInherited CellState = "inherited"
	// StateMissing: no entry of any kind.
	StateMissing CellState = "missing"
)

// stateSymbols is the single source for the human/HTML glyphs — table
// renderers read from here so the two surfaces cannot drift.
var stateSymbols = map[CellState]string{
	StatePresent:     "✓",
	StateEmpty:       "∅",
	StatePlaceholder: "!",
	StateDisabled:    "#",
	StateInherited:   "→",
	StateMissing:     "—",
}

// matrixCell is one env's entry for a key.
type matrixCell struct {
	Env   string    `json:"env"`
	State CellState `json:"state"`
	// Value is populated ONLY under --reveal (spec §2): the default JSON must
	// be incapable of leaking a secret.
	Value string `json:"value,omitempty"`
	// value holds the real value regardless of reveal, for renderers that
	// mask at display time. Unexported so json.Marshal can never see it.
	value string
}

// matrixRow is one key across every environment.
type matrixRow struct {
	Key   string       `json:"key"`
	Cells []matrixCell `json:"cells"`
}

// matrixPayload is the --json data shape for `matrix`.
type matrixPayload struct {
	Envs []string    `json:"envs"`
	Rows []matrixRow `json:"rows"`
	// ContractMissing maps env → contract keys that env lacks an active pair
	// for; present (possibly empty) only when --contract was given.
	ContractMissing map[string][]string `json:"contract_missing,omitempty"`
	Revealed        bool                `json:"revealed"`
}

// newMatrixCmd renders the keys × environments drift table (spec §2).
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
			placeholderRe, err := regexp.Compile(placeholderPat)
			if err != nil {
				return a.failf(outfmt.CodeUsage, "--placeholder: %v", err)
			}

			columns, err := a.matrixColumns(args, dir)
			if err != nil {
				return err
			}
			columns = disambiguateColumns(columns)
			if len(columns) < 2 {
				return a.failf(outfmt.CodeUsage, "matrix needs at least two env files (found %d) — a one-column matrix is `list`", len(columns))
			}

			files := make(map[string]*dotenv.File, len(columns))
			for _, col := range columns {
				f, err := a.openExisting(col.Path)
				if err != nil {
					return err
				}
				files[col.Env] = f
			}

			payload := buildMatrix(columns, files, placeholderRe, reveal)

			if contractPath != "" {
				contract, err := a.openExisting(contractPath)
				if err != nil {
					return err
				}
				applyContract(&payload, columns, files, contract)
			}
			var driftHint string
			if onlyDrift {
				rowsBefore := len(payload.Rows)
				payload.Rows = driftRows(payload.Rows)
				driftHint = ineffectiveDriftHint(payload, rowsBefore)
			}

			if err := a.emitFormatted(format, output, payload, func() {
				a.renderMatrixHuman(payload, showValues, reveal)
				if driftHint != "" {
					a.printer.Human(driftHint)
				}
			}, func() ([]byte, error) {
				return report.MatrixHTML(matrixReport(payload, columns, reveal))
			}); err != nil {
				return err
			}

			// Contract gaps fail AFTER full output, diff(1)-style: the report
			// is the answer, the exit code is the shell signal.
			for _, missing := range payload.ContractMissing {
				if len(missing) > 0 {
					return exitWithCode(ExitFailure)
				}
			}
			return nil
		},
	}

	c.Flags().StringVar(&dir, flagDir, ".", "directory to scan when no FILEs are given")
	c.Flags().BoolVar(&showValues, flagValues, false, "show values instead of state symbols (masked unless --reveal)")
	c.Flags().BoolVar(&reveal, flagReveal, false, "include real values in output — treat the result as a secret")
	c.Flags().BoolVar(&onlyDrift, flagOnlyDrift, false, "hide rows that are ✓ present in EVERY column (a missing cell counts as drift, so a sparse file keeps all rows visible — name FILEs to compare only those)")
	c.Flags().StringVar(&contractPath, flagContract, "", "contract file (.env.example); exit 1 when an env misses one of its keys")
	c.Flags().StringVar(&placeholderPat, flagPlaceholder, defaultPlaceholderPattern, "regexp marking placeholder values")
	addFormatFlags(c, &format, &output)
	return c
}

// matrixColumns resolves explicit FILE args (env name = discovery
// classification of the basename) or falls back to directory discovery with
// contracts excluded (spec §2).
func (a *app) matrixColumns(args []string, dir string) ([]discover.EnvFile, error) {
	if len(args) == 0 {
		found, err := discover.Scan(dir)
		if err != nil {
			return nil, a.failf(outfmt.CodeIO, "%v", err)
		}
		columns := found[:0]
		for _, ef := range found {
			if !ef.Contract {
				columns = append(columns, ef)
			}
		}
		return columns, nil
	}

	columns := make([]discover.EnvFile, 0, len(args))
	for _, path := range args {
		columns = append(columns, discover.EnvFile{Path: path, File: filepath.Base(path), Env: envNameForPath(path)})
	}
	return columns, nil
}

// disambiguateColumns makes every column label unique. Both `.env.stag` and
// `.env.staging` classify as "stag" (long forms collapse on purpose), and two
// different directories' `.env` files both classify as "default" — without
// this, the env-keyed file map silently dropped one file and rendered the
// OTHER file's data under both columns. Collided labels fall back to the
// filename, then to the full path (unique by construction).
func disambiguateColumns(columns []discover.EnvFile) []discover.EnvFile {
	seen := func(label func(discover.EnvFile) string) map[string]int {
		counts := make(map[string]int, len(columns))
		for _, c := range columns {
			counts[label(c)]++
		}
		return counts
	}

	byEnv := seen(func(c discover.EnvFile) string { return c.Env })
	byFile := seen(func(c discover.EnvFile) string { return c.File })

	out := make([]discover.EnvFile, len(columns))
	copy(out, columns)
	for i, c := range out {
		if byEnv[c.Env] <= 1 {
			continue
		}
		if byFile[c.File] <= 1 {
			out[i].Env = c.File
			continue
		}
		out[i].Env = c.Path
	}
	return out
}

// envNameForPath derives a column label from an explicit path via the same
// classification discovery uses, falling back to the basename for files
// outside the .env naming family.
func envNameForPath(path string) string {
	base := filepath.Base(path)
	if ef, ok := discover.Classify(base); ok {
		return ef.Env
	}
	return base
}

// openExisting is open with the missing-file leniency removed: comparing
// against a file that does not exist would read as "everything missing",
// which is a lie about the filesystem (same rule as diff).
func (a *app) openExisting(path string) (*dotenv.File, error) {
	f, err := dotenv.Open(path)
	if err != nil {
		return nil, a.failf(outfmt.CodeIO, "%v", err)
	}
	if !f.Existed() {
		return nil, a.failf(outfmt.CodeIO, "%s does not exist", path)
	}
	return f, nil
}

// buildMatrix computes the full table. Pure given its inputs — table-tested
// without the CLI harness.
func buildMatrix(columns []discover.EnvFile, files map[string]*dotenv.File, placeholderRe *regexp.Regexp, reveal bool) matrixPayload {
	keySet := map[string]bool{}
	for _, col := range columns {
		for _, k := range files[col.Env].Keys() {
			keySet[k] = true
		}
		for _, d := range files[col.Env].Disabled() {
			keySet[d.Key] = true
		}
		for _, name := range files[col.Env].Inherited() {
			keySet[name] = true
		}
	}
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	envs := make([]string, 0, len(columns))
	for _, col := range columns {
		envs = append(envs, col.Env)
	}

	rows := make([]matrixRow, 0, len(keys))
	for _, k := range keys {
		row := matrixRow{Key: k, Cells: make([]matrixCell, 0, len(columns))}
		for _, col := range columns {
			row.Cells = append(row.Cells, cellFor(files[col.Env], col.Env, k, placeholderRe, reveal))
		}
		rows = append(rows, row)
	}
	return matrixPayload{Envs: envs, Rows: rows, Revealed: reveal}
}

// cellFor classifies one key in one file, spec §2 precedence.
func cellFor(f *dotenv.File, env, key string, placeholderRe *regexp.Regexp, reveal bool) matrixCell {
	cell := matrixCell{Env: env, State: StateMissing}

	if v, ok := f.Get(key); ok {
		cell.value = v
		switch {
		case placeholderRe.MatchString(v):
			cell.State = StatePlaceholder
		case v == "":
			cell.State = StateEmpty
		default:
			cell.State = StatePresent
		}
		if reveal {
			cell.Value = v
		}
		return cell
	}

	for _, d := range f.Disabled() {
		if d.Key == key {
			cell.State = StateDisabled
			return cell
		}
	}
	for _, name := range f.Inherited() {
		if name == key {
			cell.State = StateInherited
			return cell
		}
	}
	return cell
}

// applyContract widens the row union to the contract's keys and records which
// envs lack an active pair for each contract key.
func applyContract(payload *matrixPayload, columns []discover.EnvFile, files map[string]*dotenv.File, contract *dotenv.File) {
	missing := make(map[string][]string, len(columns))
	for _, col := range columns {
		missing[col.Env] = []string{}
	}

	have := map[string]bool{}
	for _, row := range payload.Rows {
		have[row.Key] = true
	}

	for _, key := range contract.Keys() {
		if !have[key] {
			// The key exists nowhere; synthesize an all-missing row so the
			// gap is visible in the table, not just the exit code.
			row := matrixRow{Key: key}
			for _, col := range columns {
				row.Cells = append(row.Cells, matrixCell{Env: col.Env, State: StateMissing})
			}
			payload.Rows = append(payload.Rows, row)
		}
		for _, col := range columns {
			if !files[col.Env].Has(key) {
				missing[col.Env] = append(missing[col.Env], key)
			}
		}
	}

	sort.Slice(payload.Rows, func(i, j int) bool { return payload.Rows[i].Key < payload.Rows[j].Key })
	payload.ContractMissing = missing
}

// sparseColumnThreshold: when one column is missing from at least this share
// of rows, it alone marks nearly everything as drift and --only-drift stops
// filtering anything — the hint below names the culprit instead of leaving
// the user staring at an unchanged table.
const sparseColumnThreshold = 0.6

// ineffectiveDriftHint explains an --only-drift run that hid (almost) nothing
// because one sparse column drags every row into drift. Returns "" when the
// filter worked or no single column is to blame.
func ineffectiveDriftHint(payload matrixPayload, rowsBefore int) string {
	hidden := rowsBefore - len(payload.Rows)
	if rowsBefore == 0 || float64(hidden)/float64(rowsBefore) > 0.1 {
		return ""
	}

	missingPerEnv := map[string]int{}
	for _, row := range payload.Rows {
		for _, c := range row.Cells {
			if c.State == StateMissing {
				missingPerEnv[c.Env]++
			}
		}
	}
	for _, env := range payload.Envs {
		n := missingPerEnv[env]
		if float64(n) >= sparseColumnThreshold*float64(len(payload.Rows)) {
			return fmt.Sprintf("\nhint: --only-drift hid %d row(s) — %q is missing %d of %d keys, so almost every row counts as drift. Compare just the files you mean: dotenvctl matrix FILE_A FILE_B --only-drift", hidden, env, n, len(payload.Rows))
		}
	}
	return ""
}

// driftRows filters to rows with at least one non-present cell — uniform ✓
// rows carry no information when you are hunting drift.
func driftRows(rows []matrixRow) []matrixRow {
	out := rows[:0]
	for _, row := range rows {
		for _, c := range row.Cells {
			if c.State != StatePresent {
				out = append(out, row)
				break
			}
		}
	}
	return out
}

// renderMatrixHuman prints the aligned table. Cell content: symbols, or —
// with values — the (masked) value for value-bearing states.
func (a *app) renderMatrixHuman(payload matrixPayload, showValues, reveal bool) {
	if a.printer.JSON {
		return
	}
	if reveal {
		a.printer.Human("⚠ values revealed — treat this output as a secret")
	}

	w := tabwriter.NewWriter(a.printer.Out, 0, 0, tabPadding, ' ', 0)
	writeTabRow(w, append([]string{"KEY"}, payload.Envs...)...)
	for _, row := range payload.Rows {
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

	if payload.ContractMissing != nil {
		for _, env := range payload.Envs {
			if keys := payload.ContractMissing[env]; len(keys) > 0 {
				a.printer.Humanf("\ncontract: %s is missing %d key(s): %v", env, len(keys), keys)
			}
		}
	}
}

// cellText picks what one cell shows in the human table.
func cellText(c matrixCell, showValues, reveal bool) string {
	valueBearing := c.State == StatePresent || c.State == StatePlaceholder || c.State == StateEmpty
	if !showValues || !valueBearing {
		return stateSymbols[c.State]
	}
	if !reveal {
		return report.MaskGlyph
	}
	if c.value == "" {
		return stateSymbols[StateEmpty]
	}
	return c.value
}

// matrixReport adapts the payload to the report package's input, which keeps
// html templating decoupled from the CLI's json shapes.
func matrixReport(payload matrixPayload, columns []discover.EnvFile, reveal bool) report.Matrix {
	sources := make([]string, 0, len(columns))
	for _, col := range columns {
		sources = append(sources, col.Path)
	}

	rows := make([]report.MatrixRow, 0, len(payload.Rows))
	for _, r := range payload.Rows {
		row := report.MatrixRow{Key: r.Key}
		for _, c := range r.Cells {
			row.Cells = append(row.Cells, report.MatrixCell{
				State:  string(c.State),
				Symbol: stateSymbols[c.State],
				// Value flows to the template ONLY when revealed — masked
				// HTML must contain no secret bytes (spec §3).
				Value: c.Value,
			})
		}
		rows = append(rows, row)
	}
	return report.Matrix{Envs: payload.Envs, Rows: rows, Sources: sources, Revealed: reveal, ContractMissing: payload.ContractMissing}
}
