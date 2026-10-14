package vercelplugin_test

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
	"github.com/ubgo/dotenv/cli/plugins/vercelplugin"
	"github.com/ubgo/dotenv/cli/providerkit"
)

// apiVercelRunner is a fake vercel CLI: it answers `vercel whoami`, serves a
// canned `vercel env ls` table, and records every call with its stdin. No
// vercel binary, no network, no Vercel account.
type apiVercelRunner struct {
	calls []string
}

// apiVercelEnvLs mimics the vercel CLI's human `env ls` table, banner lines
// included — the plugin parses the first column of data rows.
const apiVercelEnvLs = "Vercel CLI 33.0.0\n> Environment Variables found for acme/api\n\n name    value      environments  created\n DB_URL  Encrypted  Production    2d ago\n OLD     Encrypted  Production    9d ago\n"

// Run implements providerkit.Runner.
func (r *apiVercelRunner) Run(_ context.Context, stdin string, name string, args ...string) ([]byte, error) {
	joined := strings.Join(args, " ")
	r.calls = append(r.calls, fmt.Sprintf("%s %s  stdin=%q", name, joined, stdin))
	switch {
	case joined == "whoami":
		return []byte("acme-bot\n"), nil
	case strings.HasPrefix(joined, "env ls"):
		return []byte(apiVercelEnvLs), nil
	}
	return nil, nil
}

// apiVercelAuthLabel matches the account banner line. The plugin appends the
// auth source — " (via VERCEL_TOKEN)" or " (stored vercel auth)" — read from
// the real process environment, so the examples cut it off to stay
// deterministic on any machine.
const apiVercelAuthLabel = "account: "

// apiVercelRun runs args against root and prints its output with the
// auth-source suffix removed, followed by the exit code.
func apiVercelRun(root *cobra.Command, out *bytes.Buffer, args ...string) {
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
		if strings.HasPrefix(line, apiVercelAuthLabel) {
			line, _, _ = strings.Cut(line, " (")
		}
		fmt.Println(line)
	}
	fmt.Println("exit:", code)
}

// apiVercelDeps wires a non-interactive host over a fixed selection.
func apiVercelDeps(out *bytes.Buffer, runner providerkit.Runner, src *envkit.Selection) providerkit.Deps {
	return providerkit.Deps{
		Printer:     &outfmt.Printer{Out: out, Err: out},
		File:        ".env.prod",
		Source:      func(providerkit.SelectOpts) (providerkit.Source, error) { return src, nil },
		Runner:      runner, // providerkit.ExecRunner{} in production
		Interactive: func() bool { return false },
		Ask:         func(string) bool { return false },
	}
}

// snippet:start api_vercelplugin_New

// ExampleNew mounts the vercel plugin under your own cobra root and pushes one
// value to two Vercel environments. Each target is one `vercel env add`, the
// value on stdin. The project line reads .vercel/project.json in the working
// directory; there is none here.
func ExampleNew() {
	var out bytes.Buffer
	runner := &apiVercelRunner{}
	src := &envkit.Selection{Pairs: []dotenv.Pair{{Key: "DB_URL", Value: "postgres://db/app"}}}

	root := &cobra.Command{Use: "mytool", SilenceErrors: true, SilenceUsage: true}
	root.SetErr(io.Discard)
	root.AddCommand(vercelplugin.New().Command(apiVercelDeps(&out, runner, src)))

	apiVercelRun(root, &out, "vercel", "push", "--target", "production", "--target", "preview", "--yes")
	for _, c := range runner.calls {
		if strings.HasPrefix(c, "vercel env add") {
			fmt.Println(c)
		}
	}
	// Output:
	// project: unlinked directory (vercel CLI will resolve or prompt)
	// account: acme-bot
	// targets: production, preview
	// keys   : 1
	// DB_URL: pushed
	// exit: 0
	// vercel env add DB_URL production --force  stdin="postgres://db/app"
	// vercel env add DB_URL preview --force  stdin="postgres://db/app"
}

// snippet:end api_vercelplugin_New

// snippet:start api_vercelplugin_Plugin_Command

// ExamplePlugin_Command previews a prune: OLD exists in Vercel but not in the
// local selection, so a dry run reports it as would-delete and removes
// nothing.
func ExamplePlugin_Command() {
	var out bytes.Buffer
	runner := &apiVercelRunner{}
	src := &envkit.Selection{Pairs: []dotenv.Pair{{Key: "DB_URL", Value: "postgres://db/app"}}}

	cmd := vercelplugin.New().Command(apiVercelDeps(&out, runner, src))
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetErr(io.Discard)

	apiVercelRun(cmd, &out, "prune", "--target", "production", "--dry-run")
	// Output:
	// project: unlinked directory (vercel CLI will resolve or prompt)
	// account: acme-bot
	// targets: production
	// keys   : 1
	// OLD: would-delete
	// exit: 0
}

// snippet:end api_vercelplugin_Plugin_Command
