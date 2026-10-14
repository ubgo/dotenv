package dotenvcmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestMatrixContract_NoBlankLines pins bug 18: each `contract:` line was
// preceded by a blank line, so two failing environments printed as two
// separated paragraphs. The contract lines now follow the legend directly,
// one per line.
func TestMatrixContract_NoBlankLines(t *testing.T) {
	t.Parallel()
	dir := multiEnvDir(t, map[string]string{
		".env.dev":     "A=1\n",
		".env.prod":    "A=1\n",
		".env.example": "A=x\nB=x\n",
	})
	code, out, _ := runCLI(t, "matrix", "--dir", dir, "--contract", filepath.Join(dir, ".env.example"))
	if code != ExitFailure {
		t.Fatalf("exit = %d, want 1", code)
	}
	want := "— missing\n" +
		"contract: dev is missing 1 key(s): [B]\n" +
		"contract: prod is missing 1 key(s): [B]\n"
	if !strings.HasSuffix(out, want) {
		t.Errorf("output tail wrong; want suffix %q, got:\n%s", want, out)
	}
}
