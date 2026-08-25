package dotenvcmd

import (
	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv/cli/envkit"
	"github.com/ubgo/dotenv/cli/outfmt"
	"github.com/ubgo/dotenv/cli/report"
)

// newDiffCmd compares the EFFECTIVE view of two files: last-wins maps, the
// same thing a consumer of each file would see. Formatting, comments, and
// shadowed duplicates are invisible to diff on purpose — byte-level diffing is
// what diff(1) is for; this answers "does the configuration differ".
//
// Exit contract mirrors diff(1): 0 identical, 1 differences found, 2 trouble.
func newDiffCmd(a *app) *cobra.Command {
	var expand, reveal bool
	var format, output string

	c := &cobra.Command{
		Use:   "diff FILE_A FILE_B",
		Short: "Compare the effective configuration of two .env files",
		Example: "  dotenvctl diff .env.staging .env.prod\n" +
			"  dotenvctl diff .env .env.example --expand\n" +
			"  dotenvctl diff a.env b.env --json | jq .data.changed\n" +
			"  dotenvctl diff a.env b.env --format html -o diff.html   # values masked; add --reveal to embed them",
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			payload, err := envkit.Diff(args[0], args[1], envkit.DiffOptions{Expand: expand})
			if err != nil {
				return a.failf(outfmt.CodeIO, "%v", err)
			}

			humanFn := func() {
				for _, p := range payload.Added {
					a.printer.Humanf("+ %s=%s", p.Key, p.Value)
				}
				for _, p := range payload.Removed {
					a.printer.Humanf("- %s=%s", p.Key, p.Value)
				}
				for _, ch := range payload.Changed {
					a.printer.Humanf("~ %s: %s -> %s", ch.Key, ch.From, ch.To)
				}
			}
			htmlFn := func() ([]byte, error) {
				return report.DiffHTML(diffReport(payload, args[0], args[1], reveal))
			}
			if err := a.emitFormatted(format, output, payload, humanFn, htmlFn); err != nil {
				return err
			}

			if !payload.Empty() {
				// Differences are a NORMAL outcome reported above; the
				// non-zero exit is purely the diff(1) shell contract.
				return exitWithCode(ExitFailure)
			}
			return nil
		},
	}

	c.Flags().BoolVar(&expand, flagExpand, false, "compare expanded values instead of raw ones")
	c.Flags().BoolVar(&reveal, flagReveal, false, "embed real values in an html report — treat the file as a secret")
	addFormatFlags(c, &format, &output)
	return c
}

// diffReport adapts the payload for HTML rendering. When NOT revealed, values
// are stripped here — before the template ever sees them — so a masked report
// cannot contain secret bytes even if the template had a bug (spec §3).
func diffReport(payload *envkit.DiffResult, fileA, fileB string, reveal bool) report.Diff {
	d := report.Diff{FileA: fileA, FileB: fileB, Revealed: reveal}
	for _, p := range payload.Added {
		d.Added = append(d.Added, report.DiffPair{Key: p.Key, Value: valueIf(reveal, p.Value)})
	}
	for _, p := range payload.Removed {
		d.Removed = append(d.Removed, report.DiffPair{Key: p.Key, Value: valueIf(reveal, p.Value)})
	}
	for _, ch := range payload.Changed {
		d.Changed = append(d.Changed, report.DiffChange{Key: ch.Key, From: valueIf(reveal, ch.From), To: valueIf(reveal, ch.To)})
	}
	return d
}

// valueIf passes a value through only when revealing — the strip-at-source
// half of the masked-report guarantee.
func valueIf(reveal bool, v string) string {
	if reveal {
		return v
	}
	return ""
}
