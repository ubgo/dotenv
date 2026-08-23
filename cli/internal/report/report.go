// Package report renders matrix and diff results as self-contained HTML
// files: inline CSS, zero external requests, zero JavaScript, light/dark via
// prefers-color-scheme (docsi/cli/MULTI_ENV_SPEC.md §3).
//
// The load-bearing security property: when the input says Revealed=false, the
// input structs carry NO secret values (the CLI never populates them), and the
// templates render the mask glyph for value cells — so a masked report
// contains no secret bytes and is safe to attach, mail, or commit. Reveal is
// the caller's explicit, banner-stamped opt-in.
package report

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"time"
)

// MaskGlyph replaces every masked value, fixed-width so even value LENGTHS
// leak nothing. Shared with the human renderers so all surfaces mask alike.
const MaskGlyph = "••••••"

// Matrix is the report-side input for a matrix page. Value fields must be
// populated ONLY when Revealed is true — see the package doc.
type Matrix struct {
	Envs    []string
	Rows    []MatrixRow
	Sources []string
	// Revealed switches the value columns from mask glyphs to real values and
	// stamps the "contains secrets" banner.
	Revealed bool
	// ContractMissing maps env → missing contract keys; nil when no contract
	// was checked.
	ContractMissing map[string][]string
}

// MatrixRow is one key across the environments, cells in Envs order.
type MatrixRow struct {
	Key   string
	Cells []MatrixCell
}

// MatrixCell is one key × env cell. State is the spec's closed-set string
// (used as a CSS class); Symbol its table glyph; Value the real value when
// revealed, empty otherwise.
type MatrixCell struct {
	State  string
	Symbol string
	Value  string
}

// Diff is the report-side input for a diff page; the three slices mirror the
// CLI's added/removed/changed payload, values subject to the same
// Revealed-only rule as Matrix.
type Diff struct {
	FileA, FileB string
	Added        []DiffPair
	Removed      []DiffPair
	Changed      []DiffChange
	Revealed     bool
}

// DiffPair is one added or removed key.
type DiffPair struct {
	Key   string
	Value string
}

// DiffChange is one key with different values on each side.
type DiffChange struct {
	Key      string
	From, To string
}

//go:embed report.tmpl.html
var reportTemplate string

// tmpl parses once at init-time-equivalent (package var); template.Must is
// appropriate because a broken embedded template is a build defect, not a
// runtime condition.
var tmpl = template.Must(template.New("report").Funcs(template.FuncMap{
	"mask": func(revealed bool, value string) string {
		if revealed {
			return value
		}
		return MaskGlyph
	},
}).Parse(reportTemplate))

// page is the template root: exactly one of MatrixData / DiffData is set,
// which is how one template file serves both report kinds.
type page struct {
	Title       string
	GeneratedAt string
	Revealed    bool
	MatrixData  *Matrix
	DiffData    *Diff
}

// MatrixHTML renders a matrix report page.
func MatrixHTML(m Matrix) ([]byte, error) {
	return render(page{
		Title:       "dotenvctl matrix",
		GeneratedAt: time.Now().Format(time.RFC3339),
		Revealed:    m.Revealed,
		MatrixData:  &m,
	})
}

// DiffHTML renders a diff report page.
func DiffHTML(d Diff) ([]byte, error) {
	return render(page{
		Title:       fmt.Sprintf("dotenvctl diff — %s vs %s", d.FileA, d.FileB),
		GeneratedAt: time.Now().Format(time.RFC3339),
		Revealed:    d.Revealed,
		DiffData:    &d,
	})
}

// render executes the template into a buffer so a template error yields no
// partial file.
func render(p page) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, p); err != nil {
		return nil, fmt.Errorf("report: render: %w", err)
	}
	return buf.Bytes(), nil
}
