package providerkit

import (
	"context"
	"slices"

	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv/cli/internal/outfmt"
)

// Shared flag names — one vocabulary across every kit-built verb, matching
// the core CLI's constants by value (the words are the API, not the consts).
const (
	flagPrefix              = "prefix"
	flagExcludePrefix       = "exclude-prefix"
	flagStripPrefix         = "strip-prefix"
	flagInclude             = "include"
	flagExclude             = "exclude"
	flagExpand              = "expand"
	flagDryRun              = "dry-run"
	flagYes                 = "yes"
	flagKeep                = "keep"
	flagIncludePlaceholders = "include-placeholders"
	flagIncludeEmpty        = "include-empty"
)

// GateInfo is what a plugin's confirm gate learns before writes happen.
type GateInfo struct {
	// Names are the post-rename names about to be written or deleted.
	Names []string
	// DryRun: the gate should print its banner but never prompt — nothing
	// will be written.
	DryRun bool
	// Yes mirrors --yes: skip the prompt, proceed.
	Yes bool
}

// Gate runs before any mutating backend call. The plugin prints its
// resolution banner (target, account) and calls Confirm to enforce the
// prompt/--yes contract. A nil gate means "no banner, still gated by
// Confirm's rules" — the kit installs that default so no mutating verb can
// ever run ungated.
type Gate func(ctx context.Context, deps Deps, info GateInfo) error

// Confirm enforces the standard proceed contract (PLUGINS_SPEC §4): dry-run
// never prompts; --yes proceeds; an interactive human is asked; anything
// else refuses with a usage error demanding --yes. Plugins call this at the
// end of their Gate after printing their banner.
func Confirm(deps Deps, info GateInfo) error {
	if info.DryRun || info.Yes {
		return nil
	}
	if deps.Printer.JSON || !deps.Interactive() {
		return Fail(deps.Printer, outfmt.CodeUsage, "refusing to write without --%s in non-interactive mode", flagYes)
	}
	if !deps.Ask("proceed? [y/N] ") {
		return Fail(deps.Printer, outfmt.CodeUsage, "aborted")
	}
	return nil
}

// VerbConfig customizes a kit verb without surrendering its shape.
type VerbConfig struct {
	// Gate runs after selection, before writes. Plugins put target
	// resolution + banner + Confirm here.
	Gate Gate
	// Meta contributes plugin fields (target, account) to the --json payload
	// so scripts see resolution context, not just actions.
	Meta func(ctx context.Context) (map[string]string, error)
}

// pushPayload is the kit's --json data shape for push/prune verbs.
type pushPayload struct {
	// Meta carries plugin-resolved context (e.g. target, account).
	Meta map[string]string `json:"meta,omitempty"`
	// Results is every name's outcome in processing order.
	Results []Result `json:"results"`
	DryRun  bool     `json:"dry_run"`
}

// listPayload is the kit's --json data shape for list.
type listPayload struct {
	Names []string `json:"names"`
}

// NewPushCmd builds the standard `push [KEY...]` verb over a SecretWriter.
// The kit owns selection, guards, gating, output, and exit codes; the plugin
// owns the writer and may add backend flags to the returned command.
func NewPushCmd(deps Deps, store SecretWriter, cfg VerbConfig) *cobra.Command {
	var sel SelectOpts
	var dryRun, yes bool

	c := &cobra.Command{
		Use:   "push [KEY...]",
		Short: "Push selected values to the backend as secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			sel.Keys = args
			src, err := deps.Source(sel)
			if err != nil {
				return err
			}

			pairs := src.Pairs()
			results := make([]Result, 0, len(pairs)+len(src.Skipped()))
			for _, s := range src.Skipped() {
				results = append(results, Result(s))
			}

			names := make([]string, 0, len(pairs))
			for _, p := range pairs {
				names = append(names, p.Key)
			}
			if err := runGate(cmd.Context(), deps, cfg, GateInfo{Names: names, DryRun: dryRun, Yes: yes}); err != nil {
				return err
			}

			for _, p := range pairs {
				if dryRun {
					results = append(results, Result{Name: p.Key, Action: ActionWouldPush})
					continue
				}
				if err := store.Set(cmd.Context(), p.Key, p.Value); err != nil {
					// Report what already succeeded, then fail: the partial
					// record is what makes an idempotent re-run reasonable.
					// The emit's own error (a broken pipe) is deliberately
					// dropped — the backend failure is the one the caller
					// must see, and it carries the exit code.
					_ = emitResults(deps, cfg, cmd.Context(), results, dryRun)
					return Fail(deps.Printer, outfmt.CodeExec, "push %s: %v", p.Key, err)
				}
				results = append(results, Result{Name: p.Key, Action: ActionPushed})
			}
			return emitResults(deps, cfg, cmd.Context(), results, dryRun)
		},
	}

	addSelectionFlags(c, &sel)
	c.Flags().BoolVar(&dryRun, flagDryRun, false, "print what would be pushed without writing")
	c.Flags().BoolVar(&yes, flagYes, false, "skip the confirmation prompt")
	return c
}

// NewListCmd builds the standard `list` verb over a SecretLister — remote
// NAMES only; the kit never asks a backend for values.
func NewListCmd(deps Deps, store SecretLister) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List remote secret names",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			names, err := store.Names(cmd.Context())
			if err != nil {
				return Fail(deps.Printer, outfmt.CodeExec, "list: %v", err)
			}
			slices.Sort(names)
			if len(names) == 0 {
				// An empty store must say so — bare silence reads like a
				// broken command, and users scoping the wrong target (repo
				// secrets vs --environment secrets) need the nudge.
				deps.Printer.Human("no secrets found for this scope (repo/environment/org secrets are listed separately — try --environment or --org)")
			}
			for _, n := range names {
				deps.Printer.Human(n)
			}
			return deps.Printer.OK(listPayload{Names: names})
		},
	}
}

// NewPruneCmd builds the standard `prune` verb: delete remote names absent
// from the local KEEP-SET. Doubly gated — the plugin's Gate AND --yes are
// both required paths, because deletion is the one verb with no undo.
//
// The keep-set is deliberately WIDER than the pushable selection
// (SECRETS_PORT_SPEC §7.5): guard-skipped names count as kept (a local
// placeholder must never delete a real remote value), and --keep names cover
// secrets managed outside the selection entirely (bundles, file secrets,
// other tools). Every spared name is REPORTED with its reason — an invisible
// keep decision would be as surprising as an invisible delete.
func NewPruneCmd(deps Deps, lister SecretLister, deleter SecretDeleter, cfg VerbConfig) *cobra.Command {
	var sel SelectOpts
	var keepNames []string
	var dryRun, yes bool

	c := &cobra.Command{
		Use:   "prune",
		Short: "Delete remote secrets absent from the local selection (skipped keys and --keep names are spared)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			src, err := deps.Source(sel)
			if err != nil {
				return err
			}
			keep := make(map[string]Action, len(src.Pairs())+len(src.Skipped())+len(keepNames))
			for _, p := range src.Pairs() {
				keep[p.Key] = "" // in the selection: kept silently, not reported
			}
			for _, s := range src.Skipped() {
				keep[s.Name] = ActionKeptSkipped
			}
			for _, n := range keepNames {
				keep[n] = ActionKeptFlag
			}

			remote, err := lister.Names(cmd.Context())
			if err != nil {
				return Fail(deps.Printer, outfmt.CodeExec, "list: %v", err)
			}
			slices.Sort(remote)

			var stale []string
			var kept []Result
			for _, n := range remote {
				action, isKept := keep[n]
				switch {
				case !isKept:
					stale = append(stale, n)
				case action != "":
					// Spared for a non-obvious reason — report it.
					kept = append(kept, Result{Name: n, Action: action})
				}
			}
			if len(stale) == 0 {
				deps.Printer.Human("nothing to prune")
				return emitResults(deps, cfg, cmd.Context(), kept, dryRun)
			}

			if err := runGate(cmd.Context(), deps, cfg, GateInfo{Names: stale, DryRun: dryRun, Yes: yes}); err != nil {
				return err
			}

			results := make([]Result, 0, len(stale)+len(kept))
			results = append(results, kept...)
			for _, n := range stale {
				if dryRun {
					results = append(results, Result{Name: n, Action: ActionWouldDelete})
					continue
				}
				if err := deleter.Delete(cmd.Context(), n); err != nil {
					// Same deliberate drop as push: the partial record prints,
					// the backend failure carries the exit code.
					_ = emitResults(deps, cfg, cmd.Context(), results, dryRun)
					return Fail(deps.Printer, outfmt.CodeExec, "delete %s: %v", n, err)
				}
				results = append(results, Result{Name: n, Action: ActionDeleted})
			}
			return emitResults(deps, cfg, cmd.Context(), results, dryRun)
		},
	}

	addSelectionFlags(c, &sel)
	c.Flags().StringSliceVar(&keepNames, flagKeep, nil, "never consider these remote names stale — for secrets managed outside this selection (repeat or comma-separate)")
	c.Flags().BoolVar(&dryRun, flagDryRun, false, "print what would be deleted without deleting")
	c.Flags().BoolVar(&yes, flagYes, false, "confirm deletion (required non-interactively)")
	return c
}

// AddSelectionFlags attaches the five pure-selection flags (which keys, which
// names) to any verb — store verbs AND core read verbs share this registrar so
// the flag names and help text exist exactly once. Exported because selection
// is a CORE capability: `list --prefix` and `github push --prefix` must be the
// same grammar or the pipeline's whole point is lost.
func AddSelectionFlags(c *cobra.Command, sel *SelectOpts) {
	c.Flags().StringVar(&sel.Prefix, flagPrefix, "", "select only keys with this prefix")
	c.Flags().StringVar(&sel.ExcludePrefix, flagExcludePrefix, "", "drop keys with this prefix (the inverse selector)")
	c.Flags().StringSliceVar(&sel.IncludeKeys, flagInclude, nil, "force-include a key even when prefix filters would drop it (repeatable)")
	c.Flags().StringSliceVar(&sel.ExcludeKeys, flagExclude, nil, "drop a specific key from the selection (repeatable)")
	c.Flags().BoolVar(&sel.StripPrefix, flagStripPrefix, false, "remove the matched prefix from the emitted names")
}

// SelectionActive reports whether any selection flag was used — read verbs
// switch from their legacy full view to the pipeline view only when the user
// actually selected something, keeping back-compat output for bare calls.
func SelectionActive(sel SelectOpts) bool {
	return sel.Prefix != "" || sel.ExcludePrefix != "" ||
		len(sel.IncludeKeys) > 0 || len(sel.ExcludeKeys) > 0 || sel.StripPrefix
}

// addSelectionFlags is the store-verb surface: pure selection plus the
// value-affecting flags whose DEFAULTS are store-shaped (expand on, guards
// on). Read verbs deliberately do not get these — when reading, you want to
// SEE placeholder values, not have them hidden.
func addSelectionFlags(c *cobra.Command, sel *SelectOpts) {
	AddSelectionFlags(c, sel)
	c.Flags().BoolVar(&sel.Expand, flagExpand, true, "resolve ${references} before pushing")
	c.Flags().BoolVar(&sel.IncludePlaceholders, flagIncludePlaceholders, false, "push __PLACEHOLDER__ values instead of skipping them")
	c.Flags().BoolVar(&sel.IncludeEmpty, flagIncludeEmpty, false, "push empty values instead of skipping them")
}

// runGate applies the plugin's gate, or the bare Confirm contract when the
// plugin supplied none — a mutating kit verb can never run ungated.
func runGate(ctx context.Context, deps Deps, cfg VerbConfig, info GateInfo) error {
	if cfg.Gate != nil {
		return cfg.Gate(ctx, deps, info)
	}
	return Confirm(deps, info)
}

// emitResults prints the per-name actions (human) and the JSON payload with
// plugin meta — the single tail shared by push and prune so the two can't
// drift in shape.
func emitResults(deps Deps, cfg VerbConfig, ctx context.Context, results []Result, dryRun bool) error {
	for _, r := range results {
		deps.Printer.Humanf("%s: %s", r.Name, r.Action)
	}
	payload := pushPayload{Results: results, DryRun: dryRun}
	if cfg.Meta != nil {
		// Meta failure downgrades to omission: the actions already happened
		// and MUST be reported; missing context beats a lost record.
		if m, err := cfg.Meta(ctx); err == nil {
			payload.Meta = m
		}
	}
	return deps.Printer.OK(payload)
}
