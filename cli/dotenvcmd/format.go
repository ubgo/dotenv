package dotenvcmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv/cli/internal/outfmt"
)

// Output-format vocabulary (spec §3). Closed set; --format validates against
// it so a typo is a usage error, not silent human output.
const (
	formatHuman = "human"
	formatJSON  = "json"
	formatHTML  = "html"
)

// Flag names for the format group, shared like the root flag constants.
const (
	flagFormat = "format"
	flagOutput = "output"
	flagReveal = "reveal"
)

// reportFileMode: reports may contain secrets under --reveal, so they get the
// same owner-only default as .env files themselves.
const reportFileMode = 0o600

// addFormatFlags attaches the --format / -o pair to a verb that supports HTML
// output. One helper so every such verb documents them identically.
func addFormatFlags(c *cobra.Command, format, output *string) {
	c.Flags().StringVar(format, flagFormat, "", "output format: human, json, or html (default human; --json implies json)")
	c.Flags().StringVarP(output, flagOutput, "o", "", "write output to this file instead of stdout")
}

// emitFormatted routes a verb's result to the chosen format and destination.
//
// Precedence: explicit --format wins; else the global --json selects json;
// else human. -o redirects ANY format to a file (0600 — reports can carry
// secrets). humanFn prints the human form through the printer; htmlFn renders
// the page; json flows through the standard envelope so scripted consumers
// see the same shape whether they used --json or --format json.
func (a *app) emitFormatted(format, output string, jsonPayload any, humanFn func(), htmlFn func() ([]byte, error)) error {
	if format == "" {
		if a.printer.JSON {
			format = formatJSON
		} else {
			format = formatHuman
		}
	}

	if output != "" {
		f, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, reportFileMode)
		if err != nil {
			return a.failf(outfmt.CodeIO, "%v", err)
		}
		// Redirect the whole printer for this emission, then restore — the
		// failure envelope of a later error must still reach the terminal.
		prev := a.printer.Out
		a.printer.Out = f
		defer func() {
			a.printer.Out = prev
			// Close error surfaces via the file's write path in practice; a
			// close-time flush failure on a report file has no recovery here.
			_ = f.Close()
		}()
	}

	switch format {
	case formatHuman:
		a.printer.JSON = false
		humanFn()
		return nil
	case formatJSON:
		a.printer.JSON = true
		return a.printer.OK(jsonPayload)
	case formatHTML:
		page, err := htmlFn()
		if err != nil {
			return a.failf(outfmt.CodeIO, "%v", err)
		}
		if _, err := a.printer.Out.Write(page); err != nil {
			return a.failf(outfmt.CodeIO, "write report: %v", err)
		}
		return nil
	default:
		return a.failf(outfmt.CodeUsage, "--%s must be %s, %s, or %s", flagFormat, formatHuman, formatJSON, formatHTML)
	}
}
