package githubplugin_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/envkit"
	"github.com/ubgo/dotenv/cli/outfmt"
	"github.com/ubgo/dotenv/cli/plugins/githubplugin"
	"github.com/ubgo/dotenv/cli/providerkit"
)

// apiGhRunner is a fake gh: it answers the two lookups the confirm gate
// makes, serves a canned `gh secret list --json name`, and records every call
// with its stdin. No gh binary, no network, no GitHub account.
type apiGhRunner struct {
	calls []string
}

// Run implements providerkit.Runner.
func (r *apiGhRunner) Run(_ context.Context, stdin string, name string, args ...string) ([]byte, error) {
	joined := strings.Join(args, " ")
	r.calls = append(r.calls, fmt.Sprintf("%s %s  stdin=%q", name, joined, stdin))
	switch {
	case strings.HasPrefix(joined, "api user"):
		return []byte("octocat\n"), nil
	case strings.HasPrefix(joined, "repo view"):
		return []byte("acme/api\n"), nil
	case strings.HasPrefix(joined, "secret list"):
		return []byte(`[{"name":"DB_URL"},{"name":"OLD_TOKEN"}]`), nil
	}
	return nil, nil
}

// apiGhAuthLabel matches the account banner line. The plugin appends the auth
// source — " (via GH_TOKEN)" or " (stored gh auth)" — read from the real
// process environment, so the examples cut it off to stay deterministic on
// any machine.
const apiGhAuthLabel = "account: "

// apiGhRun runs args against root and prints its output with the auth-source
// suffix removed, followed by the exit code.
func apiGhRun(root *cobra.Command, out *bytes.Buffer, args ...string) {
	out.Reset()
	root.SetArgs(args)
	code := 0
	if err := root.Execute(); err != nil {
		code = 1
		if ce, ok := errors.AsType[*providerkit.CmdError](err); ok {
			code = ce.Code
		}
	}
	for line := range strings.SplitSeq(strings.TrimRight(out.String(), "\n"), "\n") {
		if strings.HasPrefix(line, apiGhAuthLabel) {
			line, _, _ = strings.Cut(line, " (")
		}
		fmt.Println(line)
	}
	fmt.Println("exit:", code)
}

// snippet:start api_githubplugin_New

// ExampleNew mounts the github plugin under your own cobra root. All gh calls
// go through Deps.Runner — here a fake — and the secret value reaches gh on
// stdin, never in argv. The account is looked up once per run.
func ExampleNew() {
	var out bytes.Buffer
	runner := &apiGhRunner{}
	src := &envkit.Selection{Pairs: []dotenv.Pair{{Key: "DB_URL", Value: "postgres://db/app"}}}

	deps := providerkit.Deps{
		Printer:     &outfmt.Printer{Out: &out, Err: &out},
		File:        ".env.prod",
		Source:      func(providerkit.SelectOpts) (providerkit.Source, error) { return src, nil },
		Runner:      runner, // providerkit.ExecRunner{} in production
		Interactive: func() bool { return false },
		Ask:         func(string) bool { return false },
	}

	root := &cobra.Command{Use: "mytool", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	root.AddCommand(githubplugin.New().Command(deps))

	apiGhRun(root, &out, "github", "push", "--repo", "acme/api", "--yes")
	for _, c := range runner.calls {
		fmt.Println(c)
	}
	// Output:
	// target : acme/api
	// account: octocat
	// keys   : 1
	// DB_URL: pushed
	// exit: 0
	// gh api user --jq .login  stdin=""
	// gh secret set DB_URL --repo acme/api  stdin="postgres://db/app"
}

// snippet:end api_githubplugin_New

// snippet:start api_githubplugin_Plugin_Command

// ExamplePlugin_Command shows the gate refusing a whole push when any
// selected name is not a legal GitHub secret name — nothing is written, and
// the exit code is 2 (usage).
func ExamplePlugin_Command() {
	var out bytes.Buffer
	runner := &apiGhRunner{}
	src := &envkit.Selection{Pairs: []dotenv.Pair{
		{Key: "DB_URL", Value: "postgres://db/app"},
		{Key: "app.name", Value: "api"},
	}}

	cmd := githubplugin.New().Command(providerkit.Deps{
		Printer:     &outfmt.Printer{Out: &out, Err: &out},
		File:        ".env.prod",
		Source:      func(providerkit.SelectOpts) (providerkit.Source, error) { return src, nil },
		Runner:      runner,
		Interactive: func() bool { return false },
		Ask:         func(string) bool { return false },
	})
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetErr(io.Discard)

	apiGhRun(cmd, &out, "push", "--repo", "acme/api", "--yes")
	fmt.Println("gh calls:", len(runner.calls))
	// Output:
	// dotenvctl: key(s) [app.name] are not valid GitHub secret names ([A-Za-z_][A-Za-z0-9_]*) — rename them or select others; refusing a partial push
	// exit: 2
	// gh calls: 0
}

// snippet:end api_githubplugin_Plugin_Command
