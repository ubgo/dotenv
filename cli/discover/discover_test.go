package discover

import (
	"os"
	"path/filepath"
	"testing"
)

// scanFixture builds a directory holding names and returns Scan's result.
func scanFixture(t *testing.T, names ...string) []EnvFile {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("A=1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestScan_ClassificationAndOrder(t *testing.T) {
	t.Parallel()

	got := scanFixture(t,
		".env.production", ".env.example", ".env", ".env.local", ".env.bob",
		".env.staging", ".env.bak", "notes.md", "foo.env", ".env.development",
	)

	// Spec order: default, local, dev, …, stag, prod, unknown, contract.
	// Junk (.bak), non-candidates (notes.md), and *.env (foo.env) are absent.
	wantEnvs := []string{"default", "local", "dev", "stag", "prod", "bob", "example"}
	if len(got) != len(wantEnvs) {
		t.Fatalf("got %d files, want %d: %+v", len(got), len(wantEnvs), got)
	}
	for i, want := range wantEnvs {
		if got[i].Env != want {
			t.Errorf("position %d: env = %q, want %q", i, got[i].Env, want)
		}
	}

	if !got[6].Contract {
		t.Error(".env.example not tagged contract")
	}
	if got[3].File != ".env.staging" || got[3].Env != "stag" {
		t.Errorf("long form not canonicalized: %+v", got[3])
	}
}

func TestScan_EmptyAndMissingDir(t *testing.T) {
	t.Parallel()

	if got := scanFixture(t, "README.md"); len(got) != 0 {
		t.Errorf("no candidates should yield empty, got %+v", got)
	}
	if _, err := Scan(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Error("missing dir must error, not read as empty")
	}
}

func TestScan_PathsKeepDirSpelling(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Scan(dir + "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != dir+"/.env" {
		t.Errorf("path = %+v, want dir spelling preserved without doubled slash", got)
	}
}

func TestScan_DirectoriesAreNotCandidates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A DIRECTORY named like an env file (".env.d" conventions) must be
	// skipped — opening it as a file would error deep inside a verb.
	if err := os.Mkdir(filepath.Join(dir, ".env.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Env != EnvDefault {
		t.Errorf("got %+v, want only the real .env", got)
	}
}
