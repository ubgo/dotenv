package providerkit_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/envkit"
	"github.com/ubgo/dotenv/cli/outfmt"
	"github.com/ubgo/dotenv/cli/providerkit"
)

// apiKitExitUsage is the host's exit code for a usage error (unknown flag,
// bad args) — the same 2 that providerkit.Fail returns for CodeUsage.
const apiKitExitUsage = 2

// apiKitFileMode is 0600 because env files hold credentials.
const apiKitFileMode = 0o600

// snippet:start api_providerkit_store

// apiKitStore is an in-memory secret store implementing all three capability
// interfaces — the shape a real backend adapter takes, minus the network.
type apiKitStore struct {
	values map[string]string
}

// Set implements providerkit.SecretWriter.
func (s *apiKitStore) Set(_ context.Context, name, value string) error {
	s.values[name] = value
	return nil
}

// Names implements providerkit.SecretLister: names only, never values.
func (s *apiKitStore) Names(context.Context) ([]string, error) {
	names := make([]string, 0, len(s.values))
	for n := range s.values {
		names = append(names, n)
	}
	slices.Sort(names)
	return names, nil
}

// Delete implements providerkit.SecretDeleter.
func (s *apiKitStore) Delete(_ context.Context, name string) error {
	delete(s.values, name)
	return nil
}

// snippet:end api_providerkit_store

// apiKitDeps wires Deps the way a host does, over a fixed selection and a
// non-interactive session (like CI), printing to stdout.
func apiKitDeps(src *envkit.Selection) providerkit.Deps {
	return providerkit.Deps{
		Printer:     &outfmt.Printer{Out: os.Stdout, Err: os.Stdout},
		File:        ".env.prod",
		Source:      func(providerkit.SelectOpts) (providerkit.Source, error) { return src, nil },
		Runner:      providerkit.ExecRunner{},
		Interactive: func() bool { return false },
		Ask:         func(string) bool { return false },
	}
}

// apiKitRun executes one kit command with args and reports the exit code a
// host would return: 0, or the CmdError's code. Cobra's own error printing
// is silenced because the kit already printed the failure.
func apiKitRun(c *cobra.Command, args ...string) int {
	c.SilenceErrors, c.SilenceUsage = true, true
	c.SetErr(io.Discard)
	c.SetArgs(args)
	if err := c.Execute(); err != nil {
		if ce, ok := errors.AsType[*providerkit.CmdError](err); ok {
			return ce.Code
		}
		return apiKitExitUsage // a cobra usage error: unknown flag, bad args
	}
	return 0
}

// snippet:start api_providerkit_NewPushCmd

// ExampleNewPushCmd generates a standard `push` verb over your store. The kit
// owns selection, the placeholder/empty guards, the confirm gate and the
// output; your store only writes. Non-interactive runs need --yes.
func ExampleNewPushCmd() {
	store := &apiKitStore{values: map[string]string{}}
	src := &envkit.Selection{
		Pairs:   []dotenv.Pair{{Key: "DB_URL", Value: "postgres://db/app"}},
		Skipped: []envkit.Skip{{Name: "API_KEY", Reason: envkit.SkipPlaceholder}},
	}
	push := func() *cobra.Command {
		return providerkit.NewPushCmd(apiKitDeps(src), store, providerkit.VerbConfig{})
	}

	fmt.Println("exit:", apiKitRun(push()))
	fmt.Println("exit:", apiKitRun(push(), "--dry-run"))
	fmt.Println("exit:", apiKitRun(push(), "--yes"))
	fmt.Println("stored:", store.values)
	// Output:
	// dotenvctl: refusing to write without --yes in non-interactive mode
	// exit: 2
	// API_KEY: skipped-placeholder
	// DB_URL: would-push
	// exit: 0
	// API_KEY: skipped-placeholder
	// DB_URL: pushed
	// exit: 0
	// stored: map[DB_URL:postgres://db/app]
}

// snippet:end api_providerkit_NewPushCmd

// snippet:start api_providerkit_VerbConfig

// ExampleVerbConfig adds a Gate (print where the write lands, then apply the
// standard Confirm contract) and Meta (resolution context for the --json
// payload) to a kit verb.
func ExampleVerbConfig() {
	store := &apiKitStore{values: map[string]string{}}
	src := &envkit.Selection{Pairs: []dotenv.Pair{{Key: "TOKEN", Value: "t-1"}}}

	cfg := providerkit.VerbConfig{
		Gate: func(_ context.Context, deps providerkit.Deps, info providerkit.GateInfo) error {
			deps.Printer.Humanf("target : memory store")
			deps.Printer.Humanf("keys   : %s (dry run: %t)", strings.Join(info.Names, ","), info.DryRun)
			return providerkit.Confirm(deps, info)
		},
		Meta: func(context.Context) (map[string]string, error) {
			return map[string]string{"target": "memory"}, nil
		},
	}

	human := apiKitDeps(src)
	fmt.Println("exit:", apiKitRun(providerkit.NewPushCmd(human, store, cfg), "--dry-run"))

	machine := apiKitDeps(src)
	machine.Printer.JSON = true
	fmt.Println("exit:", apiKitRun(providerkit.NewPushCmd(machine, store, cfg), "--yes"))
	// Output:
	// target : memory store
	// keys   : TOKEN (dry run: true)
	// TOKEN: would-push
	// exit: 0
	// {"ok":true,"data":{"meta":{"target":"memory"},"results":[{"name":"TOKEN","action":"pushed"}],"dry_run":false}}
	// exit: 0
}

// snippet:end api_providerkit_VerbConfig

// snippet:start api_providerkit_NewListCmd

// ExampleNewListCmd lists remote NAMES, sorted. The kit never asks a backend
// for values.
func ExampleNewListCmd() {
	store := &apiKitStore{values: map[string]string{"B_KEY": "x", "A_KEY": "y"}}
	fmt.Println("exit:", apiKitRun(providerkit.NewListCmd(apiKitDeps(&envkit.Selection{}), store)))
	// Output:
	// A_KEY
	// B_KEY
	// exit: 0
}

// snippet:end api_providerkit_NewListCmd

// snippet:start api_providerkit_NewPruneCmd

// ExampleNewPruneCmd deletes remote names that are not in the local
// selection. A name the selection guarded out (a placeholder) and a name
// passed to --keep are spared, and the output says why.
func ExampleNewPruneCmd() {
	store := &apiKitStore{values: map[string]string{
		"DB_URL": "live", "API_KEY": "live", "LEGACY": "live", "BUNDLE": "live",
	}}
	src := &envkit.Selection{
		Pairs:   []dotenv.Pair{{Key: "DB_URL", Value: "postgres://db/app"}},
		Skipped: []envkit.Skip{{Name: "API_KEY", Reason: envkit.SkipPlaceholder}},
	}
	prune := providerkit.NewPruneCmd(apiKitDeps(src), store, store, providerkit.VerbConfig{})
	fmt.Println("exit:", apiKitRun(prune, "--keep", "BUNDLE", "--yes"))

	remaining, err := store.Names(context.Background())
	if err != nil {
		fmt.Println("names:", err)
		return
	}
	fmt.Println("remaining:", remaining)
	// Output:
	// API_KEY: kept-skipped
	// BUNDLE: kept-keep-flag
	// LEGACY: deleted
	// exit: 0
	// remaining: [API_KEY BUNDLE DB_URL]
}

// snippet:end api_providerkit_NewPruneCmd

// snippet:start api_providerkit_Confirm

// ExampleConfirm shows the proceed contract every mutating verb follows:
// dry-run and --yes proceed, an interactive human is asked, and anything else
// is refused with a usage error (exit code 2).
func ExampleConfirm() {
	deps := apiKitDeps(&envkit.Selection{})
	names := []string{"DB_URL"}

	fmt.Println(providerkit.Confirm(deps, providerkit.GateInfo{Names: names, DryRun: true}))
	fmt.Println(providerkit.Confirm(deps, providerkit.GateInfo{Names: names, Yes: true}))

	err := providerkit.Confirm(deps, providerkit.GateInfo{Names: names})
	if ce, ok := errors.AsType[*providerkit.CmdError](err); ok {
		fmt.Println("code:", ce.Code, "quiet:", ce.Quiet)
	}

	deps.Interactive = func() bool { return true }
	deps.Ask = func(prompt string) bool {
		fmt.Printf("asked %q\n", prompt)
		return true
	}
	fmt.Println(providerkit.Confirm(deps, providerkit.GateInfo{Names: names}))
	// Output:
	// <nil>
	// <nil>
	// dotenvctl: refusing to write without --yes in non-interactive mode
	// code: 2 quiet: true
	// asked "proceed? [y/N] "
	// <nil>
}

// snippet:end api_providerkit_Confirm

// snippet:start api_providerkit_Fail

// ExampleFail prints a failure in the active mode and returns a *CmdError
// carrying the exit code: CodeUsage maps to 2, every other code to 1. Quiet
// is true because the message is already printed.
func ExampleFail() {
	human := &outfmt.Printer{Out: os.Stdout, Err: os.Stdout} // Err: os.Stdout only so the Example shows the line
	err := providerkit.Fail(human, outfmt.CodeExec, "push %s: %s", "DB_URL", "HTTP 403")
	if ce, ok := errors.AsType[*providerkit.CmdError](err); ok {
		fmt.Println("code:", ce.Code, "quiet:", ce.Quiet, "error:", ce)
	}

	machine := &outfmt.Printer{Out: os.Stdout, JSON: true}
	err = providerkit.Fail(machine, outfmt.CodeUsage, "--org is mutually exclusive with --repo")
	if ce, ok := errors.AsType[*providerkit.CmdError](err); ok {
		fmt.Println("code:", ce.Code)
	}
	// Output:
	// dotenvctl: push DB_URL: HTTP 403
	// code: 1 quiet: true error: push DB_URL: HTTP 403
	// {"ok":false,"error":{"code":"usage","message":"--org is mutually exclusive with --repo"}}
	// code: 2
}

// snippet:end api_providerkit_Fail

// snippet:start api_providerkit_CmdError

// ExampleCmdError is the host side: map whatever a plugin command returned to
// a process exit code, and print the message only when the plugin has not
// already printed it (Quiet false).
func ExampleCmdError() {
	exitCode := func(err error) int {
		if err == nil {
			return 0
		}
		if ce, ok := errors.AsType[*providerkit.CmdError](err); ok {
			if !ce.Quiet && ce.Err != nil {
				fmt.Println("dotenvctl:", ce.Err)
			}
			return ce.Code
		}
		fmt.Println("dotenvctl:", err)
		return apiKitExitUsage // anything else out of cobra is a usage error
	}

	fmt.Println(exitCode(nil))
	fmt.Println(exitCode(&providerkit.CmdError{Code: 1, Err: errors.New("backend unreachable")}))
	fmt.Println(exitCode(&providerkit.CmdError{Code: 1, Quiet: true, Err: errors.New("already printed")}))
	fmt.Println(exitCode(errors.New(`unknown flag: --tagret`)))
	// Output:
	// 0
	// dotenvctl: backend unreachable
	// 1
	// 1
	// dotenvctl: unknown flag: --tagret
	// 2
}

// snippet:end api_providerkit_CmdError

// snippet:start api_providerkit_AddSelectionFlags

// ExampleAddSelectionFlags gives your own command the shared selection flags
// (--prefix, --exclude-prefix, --include, --exclude, --strip-prefix), and
// SelectionActive tells you whether the user actually selected anything.
func ExampleAddSelectionFlags() {
	run := func(args ...string) {
		var sel providerkit.SelectOpts
		c := &cobra.Command{
			Use: "show",
			RunE: func(*cobra.Command, []string) error {
				fmt.Printf("active=%t prefix=%q strip=%t exclude=%v\n",
					providerkit.SelectionActive(sel), sel.Prefix, sel.StripPrefix, sel.ExcludeKeys)
				return nil
			},
		}
		providerkit.AddSelectionFlags(c, &sel)
		c.SetArgs(args)
		if err := c.Execute(); err != nil {
			fmt.Println("error:", err)
		}
	}

	run()
	run("--prefix", "APP_", "--strip-prefix", "--exclude", "APP_DEBUG")
	// Output:
	// active=false prefix="" strip=false exclude=[]
	// active=true prefix="APP_" strip=true exclude=[APP_DEBUG]
}

// snippet:end api_providerkit_AddSelectionFlags

// snippet:start api_providerkit_SkipAction

// ExampleSkipAction maps why the selection held a key back onto what a push
// reports for it.
func ExampleSkipAction() {
	fmt.Println(providerkit.SkipAction(envkit.SkipPlaceholder))
	fmt.Println(providerkit.SkipAction(envkit.SkipEmpty))
	// Output:
	// skipped-placeholder
	// skipped-empty
}

// snippet:end api_providerkit_SkipAction

// snippet:start api_providerkit_pluginType

// apiKitPlugin is a complete third-party plugin: one namespace with the
// three kit verbs over its own store.
type apiKitPlugin struct {
	store *apiKitStore
}

// Command implements providerkit.Plugin.
func (p *apiKitPlugin) Command(deps providerkit.Deps) *cobra.Command {
	root := &cobra.Command{Use: "memstore", Short: "Sync values to an in-memory store"}
	root.AddCommand(
		providerkit.NewPushCmd(deps, p.store, providerkit.VerbConfig{}),
		providerkit.NewListCmd(deps, p.store),
		providerkit.NewPruneCmd(deps, p.store, p.store, providerkit.VerbConfig{}),
	)
	return root
}

// snippet:end api_providerkit_pluginType

// snippet:start api_providerkit_Plugin

// ExamplePlugin mounts a plugin under your own cobra root. Deps.Source reads
// the env file through envkit, so --prefix and the guards mean the same thing
// here as in dotenvctl.
func ExamplePlugin() {
	dir, err := os.MkdirTemp("", "providerkit-example-")
	if err != nil {
		fmt.Println("setup:", err)
		return
	}
	defer func() {
		// Cleanup is best-effort; a failure goes to stderr so the example's
		// checked stdout output is unaffected.
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
		}
	}()
	envPath := filepath.Join(dir, ".env.prod")
	if err := os.WriteFile(envPath, []byte("APP_DB=postgres://db\nAPP_KEY=__TODO__\nOTHER=x\n"), apiKitFileMode); err != nil {
		fmt.Println("setup:", err)
		return
	}

	deps := apiKitDeps(nil)
	deps.Source = func(o providerkit.SelectOpts) (providerkit.Source, error) {
		return envkit.SelectFile(envPath, o)
	}

	var plugin providerkit.Plugin = &apiKitPlugin{store: &apiKitStore{values: map[string]string{}}}
	root := &cobra.Command{Use: "mytool"}
	root.AddCommand(plugin.Command(deps))

	fmt.Println("exit:", apiKitRun(root, "memstore", "push", "--prefix", "APP_", "--strip-prefix", "--yes"))
	fmt.Println("exit:", apiKitRun(root, "memstore", "list"))
	// Output:
	// KEY: skipped-placeholder
	// DB: pushed
	// exit: 0
	// DB
	// exit: 0
}

// snippet:end api_providerkit_Plugin

// apiKitRunner is a fake providerkit.Runner: it records each call and never
// starts a process, which is how plugin code is tested without the backend
// CLI installed or a network.
type apiKitRunner struct {
	calls []string
}

// Run implements providerkit.Runner.
func (r *apiKitRunner) Run(_ context.Context, stdin string, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, fmt.Sprintf("%s %s <stdin %q>", name, strings.Join(args, " "), stdin))
	return nil, nil
}

// snippet:start api_providerkit_Runner

// ExampleRunner swaps the exec seam for a recording fake. A store passes the
// secret on stdin, never in argv, because argv is visible in ps.
func ExampleRunner() {
	runner := &apiKitRunner{}
	var r providerkit.Runner = runner

	if _, err := r.Run(context.Background(), "s3cr3t", "mycli", "secret", "set", "DB_URL"); err != nil {
		fmt.Println("run:", err)
		return
	}
	fmt.Println(runner.calls[0])
	// Output:
	// mycli secret set DB_URL <stdin "s3cr3t">
}

// snippet:end api_providerkit_Runner

// snippet:start api_providerkit_SecretWriter

// ExampleSecretWriter drives a store through the three capability
// interfaces directly — what the kit's verbs do for you. Implement only the
// interfaces your backend truly supports; the kit builds verbs for those.
func ExampleSecretWriter() {
	store := &apiKitStore{values: map[string]string{}}
	ctx := context.Background()

	var w providerkit.SecretWriter = store
	var l providerkit.SecretLister = store
	var d providerkit.SecretDeleter = store

	for _, name := range []string{"B_KEY", "A_KEY"} {
		if err := w.Set(ctx, name, "value"); err != nil {
			fmt.Println("set:", err)
			return
		}
	}
	if err := d.Delete(ctx, "B_KEY"); err != nil {
		fmt.Println("delete:", err)
		return
	}
	names, err := l.Names(ctx)
	if err != nil {
		fmt.Println("names:", err)
		return
	}
	fmt.Println(names)
	// Output:
	// [A_KEY]
}

// snippet:end api_providerkit_SecretWriter
