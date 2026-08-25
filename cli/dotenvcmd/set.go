package dotenvcmd

import (
	"cmp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/envkit"
	"github.com/ubgo/dotenv/cli/outfmt"
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

			res, err := envkit.Set(a.file, assignments, envkit.EditOptions{
				DryRun:       dryRun,
				Anchor:       cmp.Or(after, before),
				AnchorBefore: before != "",
			})
			if err != nil {
				return a.editError(err)
			}

			created := map[string]bool{}
			for _, k := range res.Created {
				created[k] = true
			}
			results := make([]setResult, 0, len(assignments))
			for _, kv := range assignments {
				results = append(results, setResult{Key: kv.Key, Created: created[kv.Key]})
			}

			a.printEditResult(res)
			if !res.Changed {
				a.printer.Human("no changes (values already current)")
			}
			return a.printer.OK(setPayload{Results: results, Changed: res.Changed, DryRun: dryRun})
		},
	}

	c.Flags().StringVar(&after, flagAfter, "", "place a NEW key immediately after this anchor key")
	c.Flags().StringVar(&before, flagBefore, "", "place a NEW key immediately before this anchor key")
	c.Flags().BoolVar(&dryRun, flagDryRun, false, "print the would-be diff without writing")
	return c
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
