package report

import (
	"errors"
	"html/template"
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

// TestRender_TemplateFailureYieldsNoPartialFile covers the error arm with a
// template that fails at execution time.
func TestRender_TemplateFailureYieldsNoPartialFile(t *testing.T) {
	t.Parallel()
	// A func that exists at parse time but errors at exec time:
	failing := template.Must(template.New("x").Funcs(template.FuncMap{
		"boom": func() (string, error) { return "", errors.New("kaboom") },
	}).Parse(`{{boom}}`))
	out, err := render(failing, page{Title: "t"})
	if err == nil || out != nil {
		t.Errorf("out=%v err=%v — want error and no bytes", out, err)
	}
	if !strings.Contains(err.Error(), "report: render") {
		t.Errorf("error not wrapped: %v", err)
	}
}
