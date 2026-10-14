package report_test

import (
	"bytes"
	"fmt"

	"github.com/ubgo/dotenv/cli/envkit"
	"github.com/ubgo/dotenv/cli/report"
)

// apiReportSecret is a value that must never appear in a masked page.
const apiReportSecret = "postgres://prod-password@db"

// snippet:start api_report_MatrixHTML

// ExampleMatrixHTML renders a drift matrix as one self-contained HTML page.
// Fill Value only when Revealed is true; a masked page shows each cell's
// state symbol, never a value. The page embeds a generation timestamp, so
// two renders are not byte-identical.
func ExampleMatrixHTML() {
	m := report.Matrix{
		Envs:    []string{"stag", "prod"},
		Sources: []string{".env.stag", ".env.prod"},
		Rows: []report.MatrixRow{{
			Key: "DB_URL",
			Cells: []report.MatrixCell{
				{State: string(envkit.StatePresent), Symbol: envkit.StateSymbols[envkit.StatePresent]},
				{State: string(envkit.StateMissing), Symbol: envkit.StateSymbols[envkit.StateMissing]},
			},
		}},
	}
	page, err := report.MatrixHTML(m)
	if err != nil {
		fmt.Println("render:", err)
		return
	}
	fmt.Println(bytes.HasPrefix(page, []byte("<!doctype html>")))
	fmt.Println(bytes.Contains(page, []byte("<script")), bytes.Contains(page, []byte("CONTAINS SECRETS")))
	// Output:
	// true
	// false false
}

// snippet:end api_report_MatrixHTML

// snippet:start api_report_DiffHTML

// ExampleDiffHTML renders a diff page. With Revealed false every value cell
// is report.MaskGlyph, even if a value was passed in; with Revealed true the
// values appear and the page carries a "contains secrets" banner.
func ExampleDiffHTML() {
	d := report.Diff{
		FileA:   ".env.stag",
		FileB:   ".env.prod",
		Changed: []report.DiffChange{{Key: "DB_URL", From: "postgres://stag", To: apiReportSecret}},
	}
	for _, reveal := range []bool{false, true} {
		d.Revealed = reveal
		page, err := report.DiffHTML(d)
		if err != nil {
			fmt.Println("render:", err)
			return
		}
		fmt.Printf("revealed=%t secret=%t glyph=%t banner=%t\n", reveal,
			bytes.Contains(page, []byte(apiReportSecret)),
			bytes.Contains(page, []byte(report.MaskGlyph)),
			bytes.Contains(page, []byte("CONTAINS SECRETS")))
	}
	// Output:
	// revealed=false secret=false glyph=true banner=false
	// revealed=true secret=true glyph=false banner=true
}

// snippet:end api_report_DiffHTML
