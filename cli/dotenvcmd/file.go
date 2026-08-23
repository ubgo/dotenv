package dotenvcmd

import (
	"github.com/ubgo/dotenv"

	"github.com/ubgo/dotenv/cli/internal/outfmt"
	"github.com/ubgo/dotenv/cli/internal/textdiff"
)

// open loads the target .env through the library. A missing file is NOT an
// error here — the library returns an empty File for it — because the right
// response differs per verb: `get` on an absent file reports the key missing,
// `set` bootstraps the file at 0600. Verbs that must distinguish ask Existed.
func (a *app) open() (*dotenv.File, error) {
	f, err := dotenv.Open(a.file)
	if err != nil {
		return nil, a.failf(outfmt.CodeIO, "%v", err)
	}
	return f, nil
}

// saveIfChanged writes the file only when the edit actually changed its
// rendering — extending the library's "Set to the current value is a byte
// no-op" invariant to the filesystem: an unchanged file keeps its mtime, so
// watchers and build tools see no phantom event.
//
// before must be the Render() captured BEFORE the edit. Returns whether a
// write happened.
func (a *app) saveIfChanged(f *dotenv.File, before string) (bool, error) {
	if f.Render() == before {
		return false, nil
	}
	if err := f.Save(); err != nil {
		return false, a.failf(outfmt.CodeIO, "%v", err)
	}
	return true, nil
}

// previewOrSave is the shared tail of every mutating verb: under --dry-run it
// prints the minimal line diff and writes NOTHING; otherwise it saves only
// when the render changed. Returns whether the file changed (or would).
//
// One implementation on purpose — if set and unset previewed differently, the
// dry-run output would stop being trustworthy as a "what will happen" answer.
func (a *app) previewOrSave(f *dotenv.File, before string, dryRun bool) (bool, error) {
	if !dryRun {
		return a.saveIfChanged(f, before)
	}
	lines := textdiff.Lines(before, f.Render())
	for _, l := range lines {
		a.printer.Human(l)
	}
	return lines != nil, nil
}
