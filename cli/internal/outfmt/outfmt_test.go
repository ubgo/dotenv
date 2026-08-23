package outfmt

import (
	"bytes"
	"testing"
)

func TestPrinter_JSONEnvelopes(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	p := &Printer{Out: &buf, JSON: true}

	type payload struct {
		Name string `json:"name"`
	}
	if err := p.OK(payload{Name: "x"}); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), `{"ok":true,"data":{"name":"x"}}`+"\n"; got != want {
		t.Errorf("OK envelope = %q, want %q", got, want)
	}

	buf.Reset()
	if err := p.Fail(CodeNotFound, "no such key"); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), `{"ok":false,"error":{"code":"not_found","message":"no such key"}}`+"\n"; got != want {
		t.Errorf("Fail envelope = %q, want %q", got, want)
	}

	// Human prints must be silent in JSON mode — one clean object per run is
	// the envelope contract.
	buf.Reset()
	p.Human("stray line")
	p.Humanf("stray %s", "fmt")
	if buf.Len() != 0 {
		t.Errorf("human output leaked into JSON mode: %q", buf.String())
	}
}

func TestPrinter_HumanMode(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	p := &Printer{Out: &buf, JSON: false}

	// OK is a no-op in human mode; the verb's Human calls carry the output.
	if err := p.OK(struct{}{}); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("OK printed in human mode: %q", buf.String())
	}

	p.Human("line")
	p.Humanf("n=%d", 7)
	if got, want := buf.String(), "line\nn=7\n"; got != want {
		t.Errorf("human output = %q, want %q", got, want)
	}

	// Failures must never be silent in either mode.
	buf.Reset()
	if err := p.Fail(CodeIO, "boom"); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "dotenvctl: boom\n"; got != want {
		t.Errorf("human failure = %q, want %q", got, want)
	}
}
