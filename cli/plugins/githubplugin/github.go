// Package githubplugin pushes env values to GitHub Actions secrets through
// the gh CLI (docsi/cli/GITHUB_PLUGIN_SPEC.md).
//
// gh supplies auth (GH_TOKEN → GITHUB_TOKEN → stored login), libsodium
// sealed-box encryption, and API compatibility; this plugin supplies
// selection, the confirm gate, and the safety rules. Every gh invocation
// goes through the injected Runner, so tests assert exact argv and stdin
// with no network and no gh installed.
package githubplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv/cli/internal/outfmt"
	"github.com/ubgo/dotenv/cli/providerkit"
)

// ghBinary is the backend CLI this plugin drives.
const ghBinary = "gh"

// Scope flag names — github-specific dimensions on the kit's standard verbs.
const (
	flagRepo        = "repo"
	flagEnvironment = "environment"
	flagOrg         = "org"
)

// secretNameRe is GitHub's secret-name grammar. Env keys legal under the
// Compose charset (dots, hyphens, digit-first) are NOT legal here; the
// plugin fails the whole run before any write rather than silently renaming
// — tool-invented mangling is how secrets get orphaned (spec §2).
var secretNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Auth-source env vars, in gh's own precedence order. Read only to LABEL the
// confirm banner — the token itself is never touched; gh consumes it from
// the inherited environment.
var tokenEnvVars = []string{"GH_TOKEN", "GITHUB_TOKEN"}

// Plugin is the github namespace. Stateless; per-invocation state lives on
// the store cobra binds flags into.
type Plugin struct{}

// New constructs the plugin.
func New() *Plugin { return &Plugin{} }

// Command assembles the github command tree: kit-standard push/list/prune
// with github scope flags bolted onto each, sharing one store.
func (p *Plugin) Command(deps providerkit.Deps) *cobra.Command {
	store := &ghStore{deps: deps}

	root := &cobra.Command{
		Use:   "github",
		Short: "Sync values to GitHub Actions secrets (via the gh CLI)",
		Long: "Pushes selected env values to GitHub Actions secrets through gh, which\n" +
			"supplies auth and encryption. Auth override: set GH_TOKEN (or GITHUB_TOKEN)\n" +
			"— there is deliberately no --token flag, because argv is visible in ps and\n" +
			"shell history. Every mutating verb prints the resolved target repo and\n" +
			"acting account before writing, and prompts unless --yes.",
		Example: "  dotenvctl github push --prefix GITHUB_SECRET_ --strip-prefix\n" +
			"  dotenvctl github push DB_URL --repo owner/name --dry-run\n" +
			"  dotenvctl github push --environment production --yes\n" +
			"  dotenvctl github list\n" +
			"  dotenvctl github prune --yes",
	}

	cfg := providerkit.VerbConfig{Gate: store.gate, Meta: store.meta}
	push := providerkit.NewPushCmd(deps, store, cfg)
	prune := providerkit.NewPruneCmd(deps, store, store, cfg)
	list := providerkit.NewListCmd(deps, store)

	for _, c := range []*cobra.Command{push, prune, list} {
		c.Flags().StringVarP(&store.repo, flagRepo, "R", "", "target repository (owner/name); default: the cwd's git remote")
		c.Flags().StringVar(&store.environment, flagEnvironment, "", "target a deployment environment's secrets")
		c.Flags().StringVar(&store.org, flagOrg, "", "target organization secrets (mutually exclusive with --repo/--environment)")
	}

	root.AddCommand(push, list, prune)
	return root
}

// ghStore implements the kit capabilities over gh. Flag fields are bound by
// Command; cobra fills them before any method runs.
type ghStore struct {
	deps        providerkit.Deps
	repo        string
	environment string
	org         string
}

// scopeArgs translates the scope flags into gh's own flags — passed through
// verbatim so gh stays the authority on their semantics.
func (s *ghStore) scopeArgs() []string {
	var args []string
	if s.repo != "" {
		args = append(args, "--repo", s.repo)
	}
	if s.environment != "" {
		args = append(args, "--env", s.environment)
	}
	if s.org != "" {
		args = append(args, "--org", s.org)
	}
	return args
}

// validateScope enforces the org/repo exclusivity the two APIs require.
func (s *ghStore) validateScope() error {
	if s.org != "" && (s.repo != "" || s.environment != "") {
		return providerkit.Fail(s.deps.Printer, outfmt.CodeUsage, "--%s is mutually exclusive with --%s/--%s", flagOrg, flagRepo, flagEnvironment)
	}
	return nil
}

// Set implements providerkit.SecretWriter: gh secret set NAME --body - with
// the value on STDIN — never argv (spec §1: argv is visible in ps).
func (s *ghStore) Set(ctx context.Context, name, value string) error {
	if !secretNameRe.MatchString(name) {
		// Defense in depth: the gate already batch-validated; this guards
		// direct programmatic use of the store.
		return fmt.Errorf("invalid secret name %q", name)
	}
	args := append([]string{"secret", "set", name, "--body", "-"}, s.scopeArgs()...)
	_, err := s.deps.Runner.Run(ctx, value, ghBinary, args...)
	return err
}

// Names implements providerkit.SecretLister via gh's JSON output.
func (s *ghStore) Names(ctx context.Context) ([]string, error) {
	args := append([]string{"secret", "list", "--json", "name"}, s.scopeArgs()...)
	out, err := s.deps.Runner.Run(ctx, "", ghBinary, args...)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("parse gh secret list output: %w", err)
	}
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, r.Name)
	}
	return names, nil
}

// Delete implements providerkit.SecretDeleter.
func (s *ghStore) Delete(ctx context.Context, name string) error {
	args := append([]string{"secret", "delete", name}, s.scopeArgs()...)
	_, err := s.deps.Runner.Run(ctx, "", ghBinary, args...)
	return err
}

// gate is the plugin's confirm gate (spec §3): validate names, resolve and
// PRINT target + acting account, then apply the standard Confirm contract.
// Runs before any write, including dry-runs (banner only).
func (s *ghStore) gate(ctx context.Context, deps providerkit.Deps, info providerkit.GateInfo) error {
	if err := s.validateScope(); err != nil {
		return err
	}
	if bad := invalidNames(info.Names); len(bad) > 0 {
		return providerkit.Fail(deps.Printer, outfmt.CodeUsage,
			"key(s) %v are not valid GitHub secret names ([A-Za-z_][A-Za-z0-9_]*) — rename them or select others; refusing a partial push", bad)
	}

	target, err := s.resolveTarget(ctx)
	if err != nil {
		return providerkit.Fail(deps.Printer, outfmt.CodeExec, "resolve target: %v (is gh installed and authenticated? https://cli.github.com)", err)
	}
	account, err := s.resolveAccount(ctx)
	if err != nil {
		return providerkit.Fail(deps.Printer, outfmt.CodeExec, "resolve account: %v (try `gh auth login`)", err)
	}

	deps.Printer.Humanf("target : %s", target)
	deps.Printer.Humanf("account: %s", account)
	deps.Printer.Humanf("keys   : %d", len(info.Names))
	return providerkit.Confirm(deps, info)
}

// meta feeds the resolved context into the --json payload so scripts see
// where actions landed, not just that they happened.
func (s *ghStore) meta(ctx context.Context) (map[string]string, error) {
	target, err := s.resolveTarget(ctx)
	if err != nil {
		return nil, err
	}
	account, err := s.resolveAccount(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]string{"target": target, "account": account}, nil
}

// resolveTarget names where writes will land: the explicit flag, or the
// cwd's repo exactly as gh itself resolves it — the display half of the
// "never write to an inferred target silently" rule.
func (s *ghStore) resolveTarget(ctx context.Context) (string, error) {
	switch {
	case s.org != "":
		return "org:" + s.org, nil
	case s.repo != "" && s.environment != "":
		return s.repo + " (environment " + s.environment + ")", nil
	case s.repo != "":
		return s.repo, nil
	}
	out, err := s.deps.Runner.Run(ctx, "", ghBinary, "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	if err != nil {
		return "", err
	}
	target := strings.TrimSpace(string(out)) + " (from cwd git remote)"
	if s.environment != "" {
		target += " (environment " + s.environment + ")"
	}
	return target, nil
}

// resolveAccount names who is writing, labeled with the auth source so a
// forgotten exported token is visible before it acts (spec §4).
func (s *ghStore) resolveAccount(ctx context.Context) (string, error) {
	out, err := s.deps.Runner.Run(ctx, "", ghBinary, "api", "user", "--jq", ".login")
	if err != nil {
		return "", err
	}
	login := strings.TrimSpace(string(out))
	for _, v := range tokenEnvVars {
		if os.Getenv(v) != "" {
			return login + " (via " + v + ")", nil
		}
	}
	return login + " (stored gh auth)", nil
}

// invalidNames returns the selection's names that GitHub would reject.
func invalidNames(names []string) []string {
	var bad []string
	for _, n := range names {
		if !secretNameRe.MatchString(n) {
			bad = append(bad, n)
		}
	}
	return bad
}
