package outfmt

import (
	"bytes"
	"testing"
)

// TestPrinter_FailWith pins the one-envelope contract for a part-way failure:
// JSON mode prints a single ok:false envelope that carries data, human mode
// prints only the failure line on Err (the caller prints data itself).
func TestPrinter_FailWith(t *testing.T) {
	t.Parallel()
	type partial struct {
		Done []string `json:"done"`
	}
	tests := []struct {
		name    string
		json    bool
		data    any
		wantOut string
		wantErr string
	}{
		{"json carries data", true, partial{Done: []string{"A"}}, `{"ok":false,"data":{"done":["A"]},"error":{"code":"exec","message":"push B: boom"}}` + "\n", ""},
		{"json nil data is plain failure", true, nil, `{"ok":false,"error":{"code":"exec","message":"push B: boom"}}` + "\n", ""},
		{"human prints the line only", false, partial{Done: []string{"A"}}, "", "dotenvctl: push B: boom\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out, errBuf bytes.Buffer
			p := &Printer{Out: &out, Err: &errBuf, JSON: tt.json}
			if err := p.FailWith(CodeExec, "push B: boom", tt.data); err != nil {
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
