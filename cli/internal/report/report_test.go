package report

import (
	"strings"
	"testing"
)

func TestMatrixHTML_EscapesAndMasks(t *testing.T) {
	t.Parallel()

	m := Matrix{
		Envs:    []string{"stag", "prod"},
		Sources: []string{".env.staging", ".env.prod"},
		Rows: []MatrixRow{
			// A hostile key name: html/template must escape it, never execute it.
			{Key: `<script>alert(1)</script>`, Cells: []MatrixCell{
				{State: "present", Symbol: "✓"},
				{State: "missing", Symbol: "—"},
			}},
		},
	}

	page, err := MatrixHTML(m)
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)

	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Error("key name not escaped — XSS via .env content")
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Error("escaped key missing from page")
	}
	if !strings.Contains(html, "values masked") || strings.Contains(html, "CONTAINS SECRETS") {
		t.Error("masked page must say masked and carry no secrets banner")
	}
	// Self-containment: no external fetches of any kind.
	for _, marker := range []string{"http://", "https://", "<script", "src="} {
		if strings.Contains(html, marker) {
			t.Errorf("page not self-contained: found %q", marker)
		}
	}
}

func TestDiffHTML_RevealAndMask(t *testing.T) {
	t.Parallel()

	d := Diff{
		FileA:   "a.env",
		FileB:   "b.env",
		Changed: []DiffChange{{Key: "TOKEN", From: "old-secret", To: "new-secret"}},
	}

	// Masked: the template's mask function must replace even values a caller
	// mistakenly left populated — defense in depth behind the CLI's
	// strip-at-source.
	page, err := DiffHTML(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(page), "old-secret") {
		t.Error("masked diff page leaked a value")
	}
	if !strings.Contains(string(page), MaskGlyph) {
		t.Error("mask glyph missing")
	}

	d.Revealed = true
	page, err = DiffHTML(d)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "old-secret") || !strings.Contains(string(page), "CONTAINS SECRETS") {
		t.Error("revealed diff page missing values or banner")
	}
}
