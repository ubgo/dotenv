package dotenv

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// --- Set --------------------------------------------------------------------

// TestSet_PreservesEverythingElse is the point of line-wise editing: a value
// change must not disturb one byte of the surrounding file.
func TestSet_PreservesEverythingElse(t *testing.T) {
	f := load(t, sample)
	f.Set("APP_NAME", "changed")

	got := f.Render()
	if !strings.Contains(got, "APP_NAME=changed") {
		t.Fatalf("value not updated:\n%s", got)
	}
	for _, survivor := range []string{
		"# Top comment", "# second line of it", "# A commented-out setting",
		"# OLD_KEY=old", "not a pair line", "  SPACED   =   padded value",
	} {
		if !strings.Contains(got, survivor) {
			t.Errorf("edit destroyed %q:\n%s", survivor, got)
		}
	}
}

func TestSet_PreservesInlineCommentAndExport(t *testing.T) {
	f := load(t, "export INLINE=old # keep me\n")
	f.Set("INLINE", "new")

	if got := f.Render(); got != "export INLINE=new # keep me\n" {
		t.Errorf("Render() = %q", got)
	}
}

// TestSet_UpdatesLastDuplicate targets the effective entry — updating an earlier
// one would change nothing a consumer sees.
func TestSet_UpdatesLastDuplicate(t *testing.T) {
	f := load(t, "K=first\nK=second\n")
	f.Set("K", "third")

	if got := f.Render(); got != "K=first\nK=third\n" {
		t.Errorf("Render() = %q, want the LAST occurrence updated", got)
	}
}

func TestSet_AppendsMissingKey(t *testing.T) {
	f := load(t, "A=1\n")

	if created := f.Set("B", "2"); !created {
		t.Error("Set of a new key returned created=false")
	}
	if got := f.Render(); got != "A=1\nB=2\n" {
		t.Errorf("Render() = %q", got)
	}
}

// TestSet_CommentedKeyAppendsActive covers the interaction with Unset: a
// commented-out entry is not active, so setting it must add a live one rather
// than silently resurrect the comment.
func TestSet_CommentedKeyAppendsActive(t *testing.T) {
	f := load(t, "# OLD=value\n")

	if created := f.Set("OLD", "new"); !created {
		t.Error("expected an append, since the commented entry is not active")
	}
	got := f.Render()
	if !strings.Contains(got, "# OLD=value") || !strings.Contains(got, "OLD=new") {
		t.Errorf("Render() = %q, want the comment kept and a live entry added", got)
	}
}

func TestSet_Quoting(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"bare when safe", "simple", "K=simple\n"},
		{"empty", "", "K=\n"},
		{"space forces quotes", "two words", "K=\"two words\"\n"},
		{"hash forces quotes", "a #b", "K=\"a #b\"\n"},
		{"tab forces quotes", "a\tb", "K=\"a\tb\"\n"},
		{"double quote is escaped", `say "hi"`, "K=\"say \\\"hi\\\"\"\n"},
		{"backslash is escaped", `back\slash`, "K=\"back\\\\slash\"\n"},
		{"single quote forces quotes", "it's", "K=\"it's\"\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := load(t, "K=old\n")
			f.Set("K", tt.value)

			if got := f.Render(); got != tt.want {
				t.Errorf("Render() = %q, want %q", got, tt.want)
			}
			// Whatever quoting was chosen must parse back to the same value.
			if back, _ := Parse(f.Render()).Get("K"); back != tt.value {
				t.Errorf("round trip: got %q, want %q", back, tt.value)
			}
		})
	}
}

func TestSet_MultilineValue(t *testing.T) {
	f := load(t, "K=old\n")
	f.Set("K", "one\ntwo\nthree")

	got := f.Render()
	if got != "K=\"one\ntwo\nthree\"\n" {
		t.Errorf("Render() = %q", got)
	}
	if back, _ := Parse(got).Get("K"); back != "one\ntwo\nthree" {
		t.Errorf("round trip = %q", back)
	}
}

func TestSet_CollapsesMultilineToSingle(t *testing.T) {
	f := load(t, "K=\"one\ntwo\"\n")
	f.Set("K", "single")

	if got := f.Render(); got != "K=single\n" {
		t.Errorf("Render() = %q, want the block replaced by one line", got)
	}
}

func TestSet_MultilinePreservesInlineComment(t *testing.T) {
	f := load(t, "K=old # note\n")
	f.Set("K", "a\nb")

	got := f.Render()
	if !strings.HasSuffix(strings.TrimRight(got, "\n"), `b" # note`) {
		t.Errorf("Render() = %q, want the inline comment after the closing quote", got)
	}
}

// --- Unset ------------------------------------------------------------------

func TestUnset_CommentsOutByDefault(t *testing.T) {
	f := load(t, "A=1\nB=2\n")

	if !f.Unset("A", false) {
		t.Fatal("Unset reported not found")
	}
	if got := f.Render(); got != "# A=1\nB=2\n" {
		t.Errorf("Render() = %q", got)
	}
	if f.Has("A") {
		t.Error("a commented-out key is still active")
	}
}

// TestUnset_CommentsOutEveryLineOfABlock matters because a half-commented
// multi-line value would leave a dangling quote and corrupt the file.
func TestUnset_CommentsOutEveryLineOfABlock(t *testing.T) {
	f := load(t, "K=\"one\ntwo\"\n")
	f.Unset("K", false)

	got := f.Render()
	for _, line := range strings.Split(strings.TrimRight(got, "\n"), "\n") {
		if !strings.HasPrefix(line, "# ") {
			t.Errorf("line %q was left uncommented:\n%s", line, got)
		}
	}
	if Parse(got).Has("K") {
		t.Error("the commented-out block still parses as active")
	}
}

func TestUnset_Delete(t *testing.T) {
	f := load(t, "A=1\nB=2\n")

	if !f.Unset("A", true) {
		t.Fatal("Unset reported not found")
	}
	if got := f.Render(); got != "B=2\n" {
		t.Errorf("Render() = %q", got)
	}
}

func TestUnset_MissingKey(t *testing.T) {
	f := load(t, "A=1\n")

	if f.Unset("NOPE", false) {
		t.Error("Unset of an absent key reported found")
	}
	if got := f.Render(); got != "A=1\n" {
		t.Errorf("Render() = %q, want the file untouched", got)
	}
}

// --- views ------------------------------------------------------------------

func TestViews(t *testing.T) {
	f := load(t, "A=1\n# note\nB=2\nA=3\n")

	t.Run("Pairs keeps order and duplicates", func(t *testing.T) {
		want := []Pair{{"A", "1"}, {"B", "2"}, {"A", "3"}}
		if got := f.Pairs(); !reflect.DeepEqual(got, want) {
			t.Errorf("Pairs() = %v, want %v", got, want)
		}
	})

	t.Run("Keys deduplicates and keeps order", func(t *testing.T) {
		if got := f.Keys(); !reflect.DeepEqual(got, []string{"A", "B"}) {
			t.Errorf("Keys() = %v", got)
		}
	})

	t.Run("Map is last-wins", func(t *testing.T) {
		want := map[string]string{"A": "3", "B": "2"}
		if got := f.Map(); !reflect.DeepEqual(got, want) {
			t.Errorf("Map() = %v, want %v", got, want)
		}
	})

	t.Run("Count reports duplicates", func(t *testing.T) {
		if got := f.Count("A"); got != 2 {
			t.Errorf("Count(A) = %d, want 2 — only the last is effective", got)
		}
		if got := f.Count("NOPE"); got != 0 {
			t.Errorf("Count(NOPE) = %d, want 0", got)
		}
	})

	t.Run("Has", func(t *testing.T) {
		if !f.Has("A") || f.Has("NOPE") {
			t.Error("Has disagrees with Get")
		}
	})
}

// --- Load and Save ----------------------------------------------------------

// TestOpen_MissingFileBootstraps lets a caller create a fresh .env without
// special-casing the absent file.
func TestOpen_MissingFileBootstraps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.env")

	f, err := Open(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.Existed() {
		t.Error("Existed() = true for a file that is not there")
	}
	if got := f.Render(); got != "" {
		t.Errorf("Render() = %q, want empty", got)
	}

	f.Set("NEW", "value")
	if err := f.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "NEW=value\n" {
		t.Errorf("saved %q", data)
	}
}

func TestOpen_ReadError(t *testing.T) {
	// A directory is readable by Stat but not by ReadFile.
	if _, err := Open(t.TempDir()); err == nil {
		t.Error("Load of a directory = nil error")
	}
}

func TestOpen_RealFileEndToEnd(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, ".env", sample)

	f, err := Open(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !f.Existed() {
		t.Error("Existed() = false for a file that is there")
	}
	if err := f.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != sample {
		t.Errorf("load → save changed the file:\n%s", data)
	}
}

// TestSave_PreservesMode guards against a credentials file silently widening
// its permissions on every write.
func TestSave_PreservesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions")
	}
	dir := t.TempDir()
	path := write(t, dir, ".env", "A=1\n")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}

	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Mode(); got != 0o640 {
		t.Errorf("Mode() = %o, want 640", got)
	}
	f.Set("A", "2")
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("mode after save = %o, want 640", info.Mode().Perm())
	}
}

// TestSave_NewFileIsPrivate: a file we create must not be group- or
// world-readable, because it is about to hold secrets.
func TestSave_NewFileIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions")
	}
	path := filepath.Join(t.TempDir(), ".env")

	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Set("SECRET", "value")
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != defaultFileMode {
		t.Errorf("mode = %o, want %o", info.Mode().Perm(), defaultFileMode)
	}
}

func TestSave_Errors(t *testing.T) {
	t.Run("no path", func(t *testing.T) {
		if err := Parse("A=1\n").Save(); err == nil {
			t.Error("Save with no Path = nil error")
		}
	})

	t.Run("unwritable directory", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("root bypasses directory permissions")
		}
		dir := filepath.Join(t.TempDir(), "locked")
		if err := os.Mkdir(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

		f, _ := Open(filepath.Join(dir, ".env"))
		f.Set("A", "1")
		if err := f.Save(); err == nil {
			t.Error("Save into an unwritable directory = nil error")
		}
	})

	t.Run("chmod failure", func(t *testing.T) {
		prev := chmod
		chmod = func(string, os.FileMode) error { return errors.New("chmod failed") }
		t.Cleanup(func() { chmod = prev })

		f, _ := Open(filepath.Join(t.TempDir(), ".env"))
		f.Set("A", "1")
		if err := f.Save(); err == nil || !strings.Contains(err.Error(), "mode") {
			t.Errorf("Save = %v, want a chmod error", err)
		}
	})

	t.Run("rename failure", func(t *testing.T) {
		prev := rename
		rename = func(string, string) error { return errors.New("rename failed") }
		t.Cleanup(func() { rename = prev })

		f, _ := Open(filepath.Join(t.TempDir(), ".env"))
		f.Set("A", "1")
		if err := f.Save(); err == nil || !strings.Contains(err.Error(), "rename") {
			t.Errorf("Save = %v, want a rename error", err)
		}
	})
}

// TestSave_LeavesNoTempFile: a failed save must not litter the directory beside
// the real file, where a later glob or backup would pick it up.
func TestSave_LeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	prev := rename
	rename = func(string, string) error { return errors.New("rename failed") }
	t.Cleanup(func() { rename = prev })

	f, _ := Open(path)
	f.Set("A", "1")
	_ = f.Save()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), tempSuffix) {
			t.Errorf("temp file %q was left behind", e.Name())
		}
	}
}

// --- positional insertion ---------------------------------------------------

func TestSetAfterBefore(t *testing.T) {
	const src = "# db\nDB_HOST=h\nDB_PORT=p\n\n# auth\nAUTH_KEY=k\n"

	t.Run("after places the new key next to its sibling", func(t *testing.T) {
		f := Parse(src)
		created, err := f.SetAfter("DB_HOST", "DB_PASSWORD", "secret")

		if err != nil || !created {
			t.Fatalf("SetAfter = (%v, %v)", created, err)
		}
		if got := f.Render(); got != "# db\nDB_HOST=h\nDB_PASSWORD=secret\nDB_PORT=p\n\n# auth\nAUTH_KEY=k\n" {
			t.Errorf("Render() =\n%s", got)
		}
	})

	t.Run("before places it above the anchor", func(t *testing.T) {
		f := Parse(src)
		if _, err := f.SetBefore("DB_HOST", "DB_DRIVER", "postgres"); err != nil {
			t.Fatalf("SetBefore: %v", err)
		}
		// It must land after the comment, not between the comment and nothing.
		if got := f.Render(); got != "# db\nDB_DRIVER=postgres\nDB_HOST=h\nDB_PORT=p\n\n# auth\nAUTH_KEY=k\n" {
			t.Errorf("Render() =\n%s", got)
		}
	})

	t.Run("an existing key is updated in place, never moved", func(t *testing.T) {
		f := Parse(src)
		created, err := f.SetAfter("AUTH_KEY", "DB_HOST", "changed")

		if err != nil {
			t.Fatalf("SetAfter: %v", err)
		}
		if created {
			t.Error("created = true for a key that already exists")
		}
		if got := f.Render(); got != "# db\nDB_HOST=changed\nDB_PORT=p\n\n# auth\nAUTH_KEY=k\n" {
			t.Errorf("the entry was relocated:\n%s", got)
		}
	})

	t.Run("a missing anchor errors and changes nothing", func(t *testing.T) {
		f := Parse(src)
		created, err := f.SetAfter("NOPE", "NEW", "v")

		if !errors.Is(err, ErrAnchorNotFound) {
			t.Fatalf("err = %v, want ErrAnchorNotFound", err)
		}
		if created {
			t.Error("created = true despite the error")
		}
		if got := f.Render(); got != src {
			t.Errorf("the file was modified despite the error:\n%s", got)
		}
	})

	t.Run("a commented-out key is not a valid anchor", func(t *testing.T) {
		f := Parse("# OLD=x\nA=1\n")
		if _, err := f.SetAfter("OLD", "NEW", "v"); !errors.Is(err, ErrAnchorNotFound) {
			t.Errorf("err = %v, want ErrAnchorNotFound — a commented entry is not active", err)
		}
	})

	t.Run("after a multi-line value clears the whole block", func(t *testing.T) {
		f := Parse("KEY=\"one\ntwo\nthree\"\nNEXT=n\n")
		if _, err := f.SetAfter("KEY", "NEW", "v"); err != nil {
			t.Fatalf("SetAfter: %v", err)
		}

		want := "KEY=\"one\ntwo\nthree\"\nNEW=v\nNEXT=n\n"
		if got := f.Render(); got != want {
			t.Errorf("insertion landed inside the block:\n got:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("before a multi-line value goes above its first line", func(t *testing.T) {
		f := Parse("KEY=\"one\ntwo\"\n")
		if _, err := f.SetBefore("KEY", "NEW", "v"); err != nil {
			t.Fatalf("SetBefore: %v", err)
		}
		if got := f.Render(); got != "NEW=v\nKEY=\"one\ntwo\"\n" {
			t.Errorf("Render() =\n%s", got)
		}
	})

	t.Run("anchoring on the last duplicate", func(t *testing.T) {
		f := Parse("A=1\nB=b\nA=2\n")
		if _, err := f.SetAfter("A", "NEW", "v"); err != nil {
			t.Fatalf("SetAfter: %v", err)
		}
		if got := f.Render(); got != "A=1\nB=b\nA=2\nNEW=v\n" {
			t.Errorf("Render() = %q, want the insertion after the EFFECTIVE entry", got)
		}
	})

	t.Run("inserting at the very start and end", func(t *testing.T) {
		f := Parse("A=1\n")
		if _, err := f.SetBefore("A", "FIRST", "f"); err != nil {
			t.Fatal(err)
		}
		if _, err := f.SetAfter("A", "LAST", "l"); err != nil {
			t.Fatal(err)
		}
		if got := f.Render(); got != "FIRST=f\nA=1\nLAST=l\n" {
			t.Errorf("Render() = %q", got)
		}
	})

	t.Run("values are quoted and round-trip", func(t *testing.T) {
		f := Parse("A=1\n")
		if _, err := f.SetAfter("A", "NEW", "needs quoting # here"); err != nil {
			t.Fatal(err)
		}
		if v, _ := Parse(f.Render()).Get("NEW"); v != "needs quoting # here" {
			t.Errorf("round trip = %q", v)
		}
	})

	t.Run("CRLF files get CRLF on the inserted line", func(t *testing.T) {
		f := Parse("A=1\r\n")
		if _, err := f.SetAfter("A", "NEW", "v"); err != nil {
			t.Fatal(err)
		}
		if got := f.Render(); got != "A=1\r\nNEW=v\r\n" {
			t.Errorf("Render() = %q", got)
		}
	})
}
