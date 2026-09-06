// Coverage for matrix arms the behavioral suite does not reach: contract
// files that cannot be opened, filename-level column collisions, revealed
// empty/placeholder cells, and SparseColumn's "nothing to blame" answer.
package envkit_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ubgo/dotenv/cli/discover"
	"github.com/ubgo/dotenv/cli/envkit"
)

func writeEnv(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestBuildMatrix_ContractUnreadable pins the contract-load error arm: a
// ContractPath that cannot be parsed (here: a directory) fails the build —
// a CI gate that silently skipped its contract would pass everything.
func TestBuildMatrix_ContractUnreadable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := writeEnv(t, dir, ".env.a", "K=1\n")
	b := writeEnv(t, dir, ".env.b", "K=2\n")
	if _, err := envkit.BuildMatrix(envkit.MatrixOptions{Files: []string{a, b}, ContractPath: t.TempDir()}); err == nil {
		t.Error("nil error for an unreadable contract path")
	}
}

// TestDisambiguateColumns_FilenameCollision pins the last-resort arm: when
// even the bare filenames collide (two directories' `.env` compared
// explicitly), labels fall back to the full path — labels must stay unique
// or one file's data renders under the other's column.
func TestDisambiguateColumns_FilenameCollision(t *testing.T) {
	t.Parallel()
	cols := envkit.DisambiguateColumns([]discover.EnvFile{
		{Env: "default", File: ".env", Path: "a/.env"},
		{Env: "default", File: ".env", Path: "b/.env"},
	})
	if cols[0].Env != "a/.env" || cols[1].Env != "b/.env" {
		t.Errorf("labels %q/%q — want the full paths when filenames collide", cols[0].Env, cols[1].Env)
	}
}

// TestBuildMatrix_RevealedEmptyAndPlaceholder pins the revealed-cell raw
// capture for the non-present states: an empty value and a placeholder must
// keep their state classification even when values are populated.
func TestBuildMatrix_RevealedEmptyAndPlaceholder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeEnv(t, dir, ".env.a", "EMPTY=\nPH=__YOU__\n")
	writeEnv(t, dir, ".env.b", "EMPTY=x\nPH=real\n")
	m, err := envkit.BuildMatrix(envkit.MatrixOptions{Dir: dir, Reveal: true})
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]envkit.CellState{}
	for _, row := range m.Rows {
		states[row.Key] = row.Cells[0].State
	}
	if states["EMPTY"] != envkit.StateEmpty || states["PH"] != envkit.StatePlaceholder {
		t.Errorf("revealed states = %v — reveal must not reclassify empty/placeholder cells", states)
	}
}

// TestSparseColumn_NoSingleCulprit pins the end-of-loop answer: when the
// drift filter hid almost nothing AND no one column owns the missing cells,
// there is no sparse column to blame — ok must be false, not a guess.
func TestSparseColumn_NoSingleCulprit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Disjoint key sets: every row drifts (hidden = 0), and the missing
	// cells split evenly between the two columns.
	writeEnv(t, dir, ".env.a", "A1=1\nA2=1\nA3=1\n")
	writeEnv(t, dir, ".env.b", "B1=1\nB2=1\nB3=1\n")
	m, err := envkit.BuildMatrix(envkit.MatrixOptions{Dir: dir, OnlyDrift: true})
	if err != nil {
		t.Fatal(err)
	}
	if env, n, ok := envkit.SparseColumn(m, len(m.Rows)); ok {
		t.Errorf("blamed %q (%d missing) — no single column is sparse here", env, n)
	}
}
