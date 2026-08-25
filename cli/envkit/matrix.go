package envkit

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/discover"
)

// CellState classifies one key × environment cell. Closed set — the strings
// are frozen API for JSON consumers and for anyone switching on them.
type CellState string

const (
	// StatePresent: an active pair with a real value.
	StatePresent CellState = "present"
	// StateEmpty: an active pair whose value is "".
	StateEmpty CellState = "empty"
	// StatePlaceholder: an active pair whose value matches the placeholder
	// pattern — configured but still a stand-in.
	StatePlaceholder CellState = "placeholder"
	// StateDisabled: only a commented-out setting exists.
	StateDisabled CellState = "disabled"
	// StateInherited: only a name-only declaration exists.
	StateInherited CellState = "inherited"
	// StateMissing: no entry of any kind.
	StateMissing CellState = "missing"
)

// StateSymbols is the canonical glyph per state. Exported so every renderer —
// the CLI table, the HTML report, a caller's own UI — draws from one source
// and cannot drift.
var StateSymbols = map[CellState]string{
	StatePresent:     "✓",
	StateEmpty:       "∅",
	StatePlaceholder: "!",
	StateDisabled:    "#",
	StateInherited:   "→",
	StateMissing:     "—",
}

// MatrixCell is one environment's entry for a key.
type MatrixCell struct {
	Env   string    `json:"env"`
	State CellState `json:"state"`
	// Value is populated ONLY when MatrixOptions.Reveal is set, so a payload
	// serialized by a caller who did not opt in cannot leak a secret.
	Value string `json:"value,omitempty"`
	// raw holds the value regardless of Reveal, for renderers that mask at
	// display time. Unexported so encoding/json can never see it.
	raw string
}

// Raw returns the cell's underlying value regardless of Reveal — for callers
// that mask at display time rather than at construction. Treat the result as
// a secret.
func (c MatrixCell) Raw() string { return c.raw }

// MatrixRow is one key across every environment, cells in Envs order.
type MatrixRow struct {
	Key   string       `json:"key"`
	Cells []MatrixCell `json:"cells"`
}

// Matrix is the keys × environments drift table.
type Matrix struct {
	// Envs are the column labels in display order.
	Envs []string `json:"envs"`
	// Rows are the keys, alphabetically.
	Rows []MatrixRow `json:"rows"`
	// ContractMissing maps env → contract keys that env lacks an active pair
	// for; non-nil only when a contract was checked.
	ContractMissing map[string][]string `json:"contract_missing,omitempty"`
	// Revealed records whether Value fields are populated.
	Revealed bool `json:"revealed"`
	// Sources are the file paths behind each column, same order as Envs —
	// for report headers and error messages.
	Sources []string `json:"sources"`
}

// HasContractGaps reports whether any environment lacks a contract key — the
// CI-gate question, so callers do not re-derive it from the map.
func (m *Matrix) HasContractGaps() bool {
	for _, missing := range m.ContractMissing {
		if len(missing) > 0 {
			return true
		}
	}
	return false
}

// MatrixOptions configures BuildMatrix.
type MatrixOptions struct {
	// Files are explicit paths to compare. When empty, Dir is scanned and
	// contract files (.env.example and kin) are excluded from the columns.
	Files []string
	// Dir is the directory to discover when Files is empty ("" means ".").
	Dir string
	// ContractPath adds a contract file's keys to the row union and computes
	// ContractMissing. Empty means no contract check.
	ContractPath string
	// PlaceholderPattern overrides DefaultPlaceholderPattern.
	PlaceholderPattern string
	// Reveal populates cell Values. Off by default: a matrix is the artifact
	// most likely to be pasted into a ticket.
	Reveal bool
	// OnlyDrift filters out rows that are present in EVERY column. NOTE: a
	// missing cell counts as drift, so a sparse column keeps nearly every row
	// — see SparseColumn for the diagnosis callers should surface.
	OnlyDrift bool
}

// BuildMatrix computes the drift table across the selected environment files.
//
// Fewer than two columns is an error: a one-column matrix is a list, and
// silently degrading to one would answer a comparison question with a
// non-comparison.
func BuildMatrix(opts MatrixOptions) (*Matrix, error) {
	columns, err := matrixColumns(opts)
	if err != nil {
		return nil, err
	}
	columns = DisambiguateColumns(columns)
	if len(columns) < 2 {
		return nil, fmt.Errorf("envkit: matrix needs at least two env files, found %d", len(columns))
	}

	placeholderRe, err := placeholderMatcher(opts.PlaceholderPattern)
	if err != nil {
		return nil, err
	}

	files := make(map[string]*dotenv.File, len(columns))
	for _, col := range columns {
		f, err := openExisting(col.Path)
		if err != nil {
			return nil, err
		}
		files[col.Env] = f
	}

	m := buildMatrix(columns, files, placeholderRe, opts.Reveal)

	if opts.ContractPath != "" {
		contract, err := openExisting(opts.ContractPath)
		if err != nil {
			return nil, err
		}
		applyContract(m, columns, files, contract)
	}
	if opts.OnlyDrift {
		m.Rows = DriftRows(m.Rows)
	}
	return m, nil
}

// matrixColumns resolves explicit paths (labeled by the same classifier
// discovery uses) or falls back to directory discovery with contracts
// excluded.
func matrixColumns(opts MatrixOptions) ([]discover.EnvFile, error) {
	if len(opts.Files) == 0 {
		dir := opts.Dir
		if dir == "" {
			dir = "."
		}
		found, err := discover.Scan(dir)
		if err != nil {
			return nil, fmt.Errorf("envkit: %w", err)
		}
		columns := found[:0]
		for _, ef := range found {
			if !ef.Contract {
				columns = append(columns, ef)
			}
		}
		return columns, nil
	}

	columns := make([]discover.EnvFile, 0, len(opts.Files))
	for _, path := range opts.Files {
		columns = append(columns, discover.EnvFile{
			Path: path,
			File: filepath.Base(path),
			Env:  EnvNameForPath(path),
		})
	}
	return columns, nil
}

// EnvNameForPath labels an explicit path with the same vocabulary discovery
// uses, falling back to the basename for files outside the .env family.
func EnvNameForPath(path string) string {
	base := filepath.Base(path)
	if ef, ok := discover.Classify(base); ok {
		return ef.Env
	}
	return base
}

// DisambiguateColumns makes every column label unique.
//
// Both `.env.stag` and `.env.staging` classify as "stag" (long forms collapse
// on purpose), and two directories' `.env` files both classify as "default".
// Without this, an env-keyed lookup silently drops one file and renders the
// OTHER file's data under both columns — misdata, the worst failure mode.
// Collided labels fall back to the filename, then to the full path.
func DisambiguateColumns(columns []discover.EnvFile) []discover.EnvFile {
	count := func(label func(discover.EnvFile) string) map[string]int {
		counts := make(map[string]int, len(columns))
		for _, c := range columns {
			counts[label(c)]++
		}
		return counts
	}

	byEnv := count(func(c discover.EnvFile) string { return c.Env })
	byFile := count(func(c discover.EnvFile) string { return c.File })

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

// buildMatrix computes the table from already-opened files. Pure given its
// inputs.
func buildMatrix(columns []discover.EnvFile, files map[string]*dotenv.File, placeholderRe *regexp.Regexp, reveal bool) *Matrix {
	keySet := map[string]bool{}
	for _, col := range columns {
		f := files[col.Env]
		for _, k := range f.Keys() {
			keySet[k] = true
		}
		for _, d := range f.Disabled() {
			keySet[d.Key] = true
		}
		for _, name := range f.Inherited() {
			keySet[name] = true
		}
	}
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	envs := make([]string, 0, len(columns))
	sources := make([]string, 0, len(columns))
	for _, col := range columns {
		envs = append(envs, col.Env)
		sources = append(sources, col.Path)
	}

	rows := make([]MatrixRow, 0, len(keys))
	for _, k := range keys {
		row := MatrixRow{Key: k, Cells: make([]MatrixCell, 0, len(columns))}
		for _, col := range columns {
			row.Cells = append(row.Cells, cellFor(files[col.Env], col.Env, k, placeholderRe, reveal))
		}
		rows = append(rows, row)
	}
	return &Matrix{Envs: envs, Rows: rows, Revealed: reveal, Sources: sources}
}

// cellFor classifies one key in one file. Precedence: an ACTIVE pair decides
// the state (present/empty/placeholder); otherwise a disabled entry, then an
// inherited declaration, then missing.
func cellFor(f *dotenv.File, env, key string, placeholderRe *regexp.Regexp, reveal bool) MatrixCell {
	cell := MatrixCell{Env: env, State: StateMissing}

	if v, ok := f.Get(key); ok {
		cell.raw = v
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
// envs lack an active pair for each. Missing keys get a synthesized
// all-missing row so the gap is visible in the table, not only in a count.
func applyContract(m *Matrix, columns []discover.EnvFile, files map[string]*dotenv.File, contract *dotenv.File) {
	missing := make(map[string][]string, len(columns))
	for _, col := range columns {
		missing[col.Env] = []string{}
	}

	have := map[string]bool{}
	for _, row := range m.Rows {
		have[row.Key] = true
	}

	for _, key := range contract.Keys() {
		if !have[key] {
			row := MatrixRow{Key: key}
			for _, col := range columns {
				row.Cells = append(row.Cells, MatrixCell{Env: col.Env, State: StateMissing})
			}
			m.Rows = append(m.Rows, row)
		}
		for _, col := range columns {
			if !files[col.Env].Has(key) {
				missing[col.Env] = append(missing[col.Env], key)
			}
		}
	}

	sort.Slice(m.Rows, func(i, j int) bool { return m.Rows[i].Key < m.Rows[j].Key })
	m.ContractMissing = missing
}

// DriftRows filters to rows with at least one non-present cell — uniform ✓
// rows carry no information when you are hunting drift.
func DriftRows(rows []MatrixRow) []MatrixRow {
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

// SparseColumnThreshold: when one column is missing from at least this share
// of rows, that column alone marks nearly everything as drift and OnlyDrift
// appears to do nothing.
const SparseColumnThreshold = 0.6

// SparseColumn names the column responsible for an ineffective OnlyDrift
// filter, with how many keys it is missing — so callers can explain "the
// filter hid nothing because .env has 2 of 79 keys" instead of leaving a
// user staring at an unchanged table. Returns ok=false when the filter
// worked (hid >10% of rows) or no single column is to blame.
func SparseColumn(m *Matrix, rowsBefore int) (env string, missing int, ok bool) {
	hidden := rowsBefore - len(m.Rows)
	if rowsBefore == 0 || float64(hidden)/float64(rowsBefore) > 0.1 {
		return "", 0, false
	}

	missingPerEnv := map[string]int{}
	for _, row := range m.Rows {
		for _, c := range row.Cells {
			if c.State == StateMissing {
				missingPerEnv[c.Env]++
			}
		}
	}
	for _, e := range m.Envs {
		n := missingPerEnv[e]
		if float64(n) >= SparseColumnThreshold*float64(len(m.Rows)) {
			return e, n, true
		}
	}
	return "", 0, false
}
