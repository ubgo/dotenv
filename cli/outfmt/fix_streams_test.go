package outfmt

import (
	"bytes"
	"testing"
)

// TestPrinter_FailStreamsAndProgram pins bugs 11 and 16 at the Printer seam:
// a human-mode failure goes to Err (never Out, which carries values scripts
// capture), it is prefixed with Program, and under --json the envelope stays
// on Out because it is the output.
func TestPrinter_FailStreamsAndProgram(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		json    bool
		program string
		wantOut string
		wantErr string
	}{
		{"human default program", false, "", "", "dotenvctl: boom\n"},
		{"human custom program", false, "myhost env", "", "myhost env: boom\n"},
		{"json ignores program", true, "myhost", `{"ok":false,"error":{"code":"io","message":"boom"}}` + "\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out, errBuf bytes.Buffer
			p := &Printer{Out: &out, Err: &errBuf, JSON: tt.json, Program: tt.program}
			if err := p.Fail(CodeIO, "boom"); err != nil {
				t.Fatal(err)
			}
			if out.String() != tt.wantOut {
				t.Errorf("Out = %q, want %q", out.String(), tt.wantOut)
			}
			if errBuf.String() != tt.wantErr {
				t.Errorf("Err = %q, want %q", errBuf.String(), tt.wantErr)
			}
		})
	}
}
