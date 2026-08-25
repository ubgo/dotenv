// Package vercelplugin pushes env values to Vercel environment variables
// through the vercel CLI (docsi/cli/PLUGINS_SPEC.md §5).
//
// The vercel CLI supplies auth (VERCEL_TOKEN or its stored login) and project
// linking (.vercel/project.json); this plugin supplies selection, the confirm
// gate, and the safety rules. Every vercel invocation goes through the
// injected Runner, so tests assert exact argv and stdin with no network and
// no vercel installed.
package vercelplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv/cli/outfmt"
	"github.com/ubgo/dotenv/cli/providerkit"
)

// vercelBinary is the backend CLI this plugin drives.
const vercelBinary = "vercel"

// Scope flag names — vercel-specific dimensions on the kit's standard verbs.
const (
	flagTarget    = "target"
	flagSensitive = "sensitive"
)

// defaultTarget is where values land when --target is not given. Development
// mirrors the vercel CLI's own default and is the least dangerous place for
// an accidental push.
const defaultTarget = "development"

// projectLinkFile is where the vercel CLI records which project a directory
// is linked to. Read for the confirm banner only — a display concern, which
// is why this file access is allowed despite plugins otherwise never touching
// the filesystem (the env VALUES still come exclusively from Deps.Source).
const projectLinkFile = ".vercel/project.json"

// tokenEnvVar is vercel's auth override, labeled in the banner the same way
// the github plugin labels GH_TOKEN. Deliberately no --token flag — argv is
// visible in ps and shell history.
const tokenEnvVar = "VERCEL_TOKEN"

// Plugin is the vercel namespace. Stateless; per-invocation state lives on
// the store cobra binds flags into.
type Plugin struct{}

// New constructs the plugin.
func New() *Plugin { return &Plugin{} }

// Command assembles the vercel command tree: kit-standard push/list/prune
// with vercel scope flags bolted onto each, sharing one store.
func (p *Plugin) Command(deps providerkit.Deps) *cobra.Command {
	store := &vercelStore{deps: deps}

	root := &cobra.Command{
		Use:   "vercel",
		Short: "Sync values to Vercel environment variables (via the vercel CLI)",
		Long: "Pushes selected env values to Vercel through the vercel CLI, which supplies\n" +
			"auth and project linking. Auth override: set VERCEL_TOKEN — there is\n" +
			"deliberately no --token flag, because argv is visible in ps and shell\n" +
			"history. Every mutating verb prints the linked project and acting account\n" +
			"before writing, and prompts unless --yes.",
		Example: "  dotenvctl vercel push --target production --yes\n" +
			"  dotenvctl vercel push DB_URL --sensitive --dry-run\n" +
			"  dotenvctl vercel push --prefix VERCEL_ --strip-prefix --target preview\n" +
			"  dotenvctl vercel list\n" +
			"  dotenvctl vercel prune --target production --yes",
	}

	cfg := providerkit.VerbConfig{Gate: store.gate, Meta: store.meta}
	push := providerkit.NewPushCmd(deps, store, cfg)
	prune := providerkit.NewPruneCmd(deps, store, store, cfg)
	list := providerkit.NewListCmd(deps, store)

	for _, c := range []*cobra.Command{push, prune, list} {
		c.Flags().StringSliceVar(&store.targets, flagTarget, []string{defaultTarget}, "Vercel environment(s): production, preview, development (repeatable)")
	}
	// Sensitivity only applies when writing.
	push.Flags().BoolVar(&store.sensitive, flagSensitive, false, "store as a sensitive env var (encrypted, unreadable in the dashboard)")

	root.AddCommand(push, list, prune)
	return root
}

// vercelStore implements the kit capabilities over the vercel CLI. Flag
// fields are bound by Command; cobra fills them before any method runs.
type vercelStore struct {
	deps      providerkit.Deps
	targets   []string
	sensitive bool
}

// Set implements providerkit.SecretWriter: one `vercel env add NAME TARGET`
// per selected target, value on STDIN — never argv. --force makes it an
// upsert, matching the kit's push semantics (vercel env add without it errors
// on an existing name).
func (s *vercelStore) Set(ctx context.Context, name, value string) error {
	for _, target := range s.targets {
		args := []string{"env", "add", name, target, "--force"}
		if s.sensitive {
			args = append(args, "--sensitive")
		}
		if _, err := s.deps.Runner.Run(ctx, value, vercelBinary, args...); err != nil {
			return fmt.Errorf("target %s: %w", target, err)
		}
	}
	return nil
}

// Names implements providerkit.SecretLister as the union across the selected
// targets, parsed from `vercel env ls TARGET` table output.
//
// The vercel CLI has no stable machine format for env ls, so this parses the
// human table with a documented heuristic (first column of data rows). The
// direct-API mode recorded as deferred in the spec is the clean upgrade path;
// until then a format change surfaces loudly as an empty/odd listing, never
// as silent misdata for push (which does not consult Names at all).
func (s *vercelStore) Names(ctx context.Context) ([]string, error) {
	seen := map[string]bool{}
	var names []string
	for _, target := range s.targets {
		out, err := s.deps.Runner.Run(ctx, "", vercelBinary, "env", "ls", target)
		if err != nil {
			return nil, err
		}
		for _, name := range parseEnvLs(string(out)) {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names, nil
}

// Delete implements providerkit.SecretDeleter across the selected targets.
func (s *vercelStore) Delete(ctx context.Context, name string) error {
	for _, target := range s.targets {
		if _, err := s.deps.Runner.Run(ctx, "", vercelBinary, "env", "rm", name, target, "--yes"); err != nil {
			return fmt.Errorf("target %s: %w", target, err)
		}
	}
	return nil
}

// gate prints the resolved project, account, and targets, then applies the
// standard Confirm contract — before any write, including dry-runs (banner
// only).
func (s *vercelStore) gate(ctx context.Context, deps providerkit.Deps, info providerkit.GateInfo) error {
	account, err := s.resolveAccount(ctx)
	if err != nil {
		return providerkit.Fail(deps.Printer, outfmt.CodeExec, "resolve account: %v (is the vercel CLI installed and logged in? https://vercel.com/docs/cli)", err)
	}

	deps.Printer.Humanf("project: %s", s.resolveProject())
	deps.Printer.Humanf("account: %s", account)
	deps.Printer.Humanf("targets: %s", strings.Join(s.targets, ", "))
	deps.Printer.Humanf("keys   : %d", len(info.Names))
	return providerkit.Confirm(deps, info)
}

// meta feeds resolved context into the --json payload.
func (s *vercelStore) meta(ctx context.Context) (map[string]string, error) {
	account, err := s.resolveAccount(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"project": s.resolveProject(),
		"account": account,
		"targets": strings.Join(s.targets, ","),
	}, nil
}

// resolveProject names the linked project from .vercel/project.json — display
// only. An unlinked directory is reported as such rather than guessed at; the
// vercel CLI itself decides what an unlinked invocation does.
func (s *vercelStore) resolveProject() string {
	raw, err := os.ReadFile(projectLinkFile)
	if err != nil {
		return "unlinked directory (vercel CLI will resolve or prompt)"
	}
	var link struct {
		ProjectID string `json:"projectId"`
		OrgID     string `json:"orgId"`
	}
	if err := json.Unmarshal(raw, &link); err != nil || link.ProjectID == "" {
		return "unlinked directory (vercel CLI will resolve or prompt)"
	}
	return link.ProjectID + " (from " + projectLinkFile + ")"
}

// resolveAccount names who is writing via `vercel whoami`, labeled with the
// auth source so an exported token is visible before it acts.
func (s *vercelStore) resolveAccount(ctx context.Context) (string, error) {
	out, err := s.deps.Runner.Run(ctx, "", vercelBinary, "whoami")
	if err != nil {
		return "", err
	}
	login := strings.TrimSpace(string(out))
	if os.Getenv(tokenEnvVar) != "" {
		return login + " (via " + tokenEnvVar + ")", nil
	}
	return login + " (stored vercel auth)", nil
}

// parseEnvLs extracts variable names from `vercel env ls` table output: the
// first whitespace-separated field of each data row. Header/banner rows are
// recognized by their known first tokens rather than by position, so leading
// CLI chatter ("Vercel CLI 33.0.0", "> Environment Variables …") is immune to
// count drift.
func parseEnvLs(out string) []string {
	var names []string
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		first := fields[0]
		switch strings.ToLower(first) {
		case "name", "vercel", ">", "error", "warn":
			continue
		}
		names = append(names, first)
	}
	return names
}
