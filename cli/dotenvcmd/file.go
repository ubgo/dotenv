package dotenvcmd

import (
	"github.com/ubgo/dotenv"

	"github.com/ubgo/dotenv/cli/outfmt"
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
