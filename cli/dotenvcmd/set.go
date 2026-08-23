package dotenvcmd

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/internal/outfmt"
)

// setResult is one key's outcome in set's --json output.
type setResult struct {
	Key string `json:"key"`
	// Created is true when the key did not exist and was appended/inserted;
	// false for an in-place update.
	Created bool `json:"created"`
}

// setPayload is the --json data shape for `set`.
type setPayload struct {
	Results []setResult `json:"results"`
	// Changed is false when every assignment matched the current value — the
	// byte-no-op case: nothing was written, the file's mtime is untouched.
	Changed bool `json:"changed"`
	// DryRun mirrors the flag so scripts can tell a preview from an applied
	// run without tracking their own arguments.
	DryRun bool `json:"dry_run"`
}

// newSetCmd upserts KEY=VALUE assignments — the library's showcase verb:
// everything around the touched entries (comments, ordering, quoting, the
// author's = vs : delimiter) survives byte-for-byte.
func newSetCmd(a *app) *cobra.Command {
	var after, before string
	var dryRun bool

	c := &cobra.Command{
		Use:   "set KEY=VALUE [KEY=VALUE...]",
		Short: "Set values, preserving every byte you didn't touch",
		Example: "  dotenvctl set DB_HOST=localhost DB_PORT=5432\n" +
			"  dotenvctl set DB_PASSWORD=secret --after DB_PORT\n" +
			"  dotenvctl set DEBUG=true --dry-run   # preview the diff, write nothing",
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if after != "" && before != "" {
				return a.failf(outfmt.CodeUsage, "--%s and --%s are mutually exclusive", flagAfter, flagBefore)
			}

			assignments, err := a.parseAssignments(args)
			if err != nil {
				return err
			}

			f, err := a.open()
			if err != nil {
				return err
			}
			beforeRender := f.Render()

			results := make([]setResult, 0, len(assignments))
			for _, kv := range assignments {
				created, err := a.applySet(f, kv, after, before)
				if err != nil {
					return err
				}
				results = append(results, setResult{Key: kv.Key, Created: created})
			}

			changed, err := a.previewOrSave(f, beforeRender, dryRun)
			if err != nil {
				return err
			}
			if !changed {
				a.printer.Human("no changes (values already current)")
			}
			return a.printer.OK(setPayload{Results: results, Changed: changed, DryRun: dryRun})
		},
	}

	c.Flags().StringVar(&after, flagAfter, "", "place a NEW key immediately after this anchor key")
	c.Flags().StringVar(&before, flagBefore, "", "place a NEW key immediately before this anchor key")
	c.Flags().BoolVar(&dryRun, flagDryRun, false, "print the would-be diff without writing")
	return c
}

// applySet routes one assignment through plain Set or the anchored variants.
// A missing anchor is an operation failure, not a silent append — the whole
// point of anchoring is WHERE the key lands.
func (a *app) applySet(f *dotenv.File, kv dotenv.Pair, after, before string) (bool, error) {
	switch {
	case after != "":
		created, err := f.SetAfter(after, kv.Key, kv.Value)
		if errors.Is(err, dotenv.ErrAnchorNotFound) {
			return false, a.failf(outfmt.CodeNotFound, "anchor key %q not found in %s", after, a.file)
		}
		return created, nil
	case before != "":
		created, err := f.SetBefore(before, kv.Key, kv.Value)
		if errors.Is(err, dotenv.ErrAnchorNotFound) {
			return false, a.failf(outfmt.CodeNotFound, "anchor key %q not found in %s", before, a.file)
		}
		return created, nil
	default:
		return f.Set(kv.Key, kv.Value), nil
	}
}

// parseAssignments splits each arg on its FIRST '=' — values legitimately
// contain '=' (connection strings, base64), so only the first is structural.
func (a *app) parseAssignments(args []string) ([]dotenv.Pair, error) {
	out := make([]dotenv.Pair, 0, len(args))
	for _, arg := range args {
		key, value, ok := strings.Cut(arg, "=")
		if !ok || key == "" {
			return nil, a.failf(outfmt.CodeUsage, "argument %q is not KEY=VALUE", arg)
		}
		out = append(out, dotenv.Pair{Key: key, Value: value})
	}
	return out, nil
}
