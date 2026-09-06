// Coverage for Scan's ordering tie-break: two unknown environment names
// share a rank, so the filename decides — deterministically, or matrix
// columns would reorder between runs.
package discover

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScan_UnknownNamesSortByFilename(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{".env.zeta", ".env.alpha"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("K=1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].File != ".env.alpha" || out[1].File != ".env.zeta" {
		t.Errorf("order %+v — equal-rank names must sort by filename", out)
	}
}
