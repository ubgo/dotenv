package dotenvcmd

import (
	"errors"

	"github.com/ubgo/dotenv/cli/envkit"
	"github.com/ubgo/dotenv/cli/outfmt"
)

// editError maps an envkit edit failure onto the CLI's error codes. The
// sentinel errors are the contract — matching on them rather than on message
// text is why envkit exports them.
func (a *app) editError(err error) error {
	switch {
	case errors.Is(err, envkit.ErrAnchorNotFound):
		return a.failf(outfmt.CodeNotFound, "%v", err)
	case errors.Is(err, envkit.ErrKeyNotFound):
		return a.failf(outfmt.CodeNotFound, "%v", err)
	default:
		return a.failf(outfmt.CodeIO, "%v", err)
	}
}

// printEditResult shows a dry-run's would-be diff. A real run prints nothing
// here — the file itself is the output, and echoing a diff after writing it
// would imply something still needed deciding.
func (a *app) printEditResult(res *envkit.EditResult) {
	if res.Written {
		return
	}
	for _, line := range res.Diff {
		a.printer.Human(line)
	}
}
