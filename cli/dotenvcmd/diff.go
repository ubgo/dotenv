package dotenvcmd

import (
	"errors"
	"sort"

	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/internal/outfmt"
	"github.com/ubgo/dotenv/cli/internal/report"
)

// diffChange is one key whose value differs between the two files.
type diffChange struct {
	Key  string `json:"key"`
	From string `json:"from"`
	To   string `json:"to"`
}

// diffPayload is the --json data shape for `diff`. Arrays are always present
// and key-sorted so output is deterministic and diffable itself.
type diffPayload struct {
	// Added: keys present in B but not A.
	Added []pairPayload `json:"added"`
	// Removed: keys present in A but not B.
	Removed []pairPayload `json:"removed"`
	// Changed: keys in both with different values.
	Changed  []diffChange `json:"changed"`
	Expanded bool         `json:"expanded"`
}

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
			mapA, err := a.effectiveMap(args[0], expand)
			if err != nil {
				return err
			}
			mapB, err := a.effectiveMap(args[1], expand)
			if err != nil {
				return err
			}

			payload := buildDiff(mapA, mapB, expand)

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

			if len(payload.Added)+len(payload.Removed)+len(payload.Changed) > 0 {
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
func diffReport(payload diffPayload, fileA, fileB string, reveal bool) report.Diff {
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

// effectiveMap loads one file and returns its last-wins view. Unlike the
// shared open, a MISSING file here is an error: diffing against a file that
// does not exist would silently read as "everything was removed", which is a
// lie about the filesystem rather than a comparison.
func (a *app) effectiveMap(path string, expand bool) (map[string]string, error) {
	f, err := a.openExisting(path)
	if err != nil {
		return nil, err
	}
	if !expand {
		return f.Map(), nil
	}
	m, err := f.ExpandedMap()
	if _, ok := errors.AsType[*dotenv.RequiredError](err); ok {
		return nil, a.failf(outfmt.CodeRequired, "%s: %v", path, err)
	}
	if err != nil {
		return nil, a.failf(outfmt.CodeIO, "expand %s: %v", path, err)
	}
	return m, nil
}

// buildDiff computes the three change sets, key-sorted for deterministic
// output. Pure function — no app state — so it is trivially table-testable.
func buildDiff(mapA, mapB map[string]string, expanded bool) diffPayload {
	payload := diffPayload{
		Added:    []pairPayload{},
		Removed:  []pairPayload{},
		Changed:  []diffChange{},
		Expanded: expanded,
	}

	for k, vb := range mapB {
		va, inA := mapA[k]
		switch {
		case !inA:
			payload.Added = append(payload.Added, pairPayload{Key: k, Value: vb})
		case va != vb:
			payload.Changed = append(payload.Changed, diffChange{Key: k, From: va, To: vb})
		}
	}
	for k, va := range mapA {
		if _, inB := mapB[k]; !inB {
			payload.Removed = append(payload.Removed, pairPayload{Key: k, Value: va})
		}
	}

	sort.Slice(payload.Added, func(i, j int) bool { return payload.Added[i].Key < payload.Added[j].Key })
	sort.Slice(payload.Removed, func(i, j int) bool { return payload.Removed[i].Key < payload.Removed[j].Key })
	sort.Slice(payload.Changed, func(i, j int) bool { return payload.Changed[i].Key < payload.Changed[j].Key })
	return payload
}
