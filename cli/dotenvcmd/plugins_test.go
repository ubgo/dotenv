package dotenvcmd

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/ubgo/dotenv/cli/envkit"
	"github.com/ubgo/dotenv/cli/outfmt"
	"github.com/ubgo/dotenv/cli/providerkit"
)

// sourceFixture builds an app over a written env file and returns its
// pluginSource output for the given selection.
func sourceFixture(t *testing.T, content string, opts providerkit.SelectOpts) (providerkit.Source, error) {
	t.Helper()
	var out bytes.Buffer
	a := &app{file: writeFixture(t, content), printer: &outfmt.Printer{Out: &out}, errOut: &out}
	return a.pluginSource(opts)
}

func TestPluginSource_SelectionSemantics(t *testing.T) {
	t.Parallel()
	const content = "GITHUB_SECRET_TOKEN=tok\nGITHUB_SECRET_ACTOR=__YOU__\nGITHUB_SECRET_EMPTY=\nOTHER=x\nREF=${OTHER}-suffix\n"

	t.Run("prefix + strip + guards", func(t *testing.T) {
		t.Parallel()
		src, err := sourceFixture(t, content, providerkit.SelectOpts{Prefix: "GITHUB_SECRET_", StripPrefix: true, Expand: true})
		if err != nil {
			t.Fatal(err)
		}
		pairs := src.Pairs
		if len(pairs) != 1 || pairs[0].Key != "TOKEN" || pairs[0].Value != "tok" {
			t.Errorf("pairs = %+v, want only stripped TOKEN", pairs)
		}
		skipped := map[string]envkit.SkipReason{}
		for _, s := range src.Skipped {
			skipped[s.Name] = s.Reason
		}
		if skipped["ACTOR"] != envkit.SkipPlaceholder || skipped["EMPTY"] != envkit.SkipEmpty {
			t.Errorf("skips = %v, want placeholder+empty guards with STRIPPED names", skipped)
		}
	})

	t.Run("expansion resolves references", func(t *testing.T) {
		t.Parallel()
		src, err := sourceFixture(t, content, providerkit.SelectOpts{Keys: []string{"REF"}, Expand: true})
		if err != nil {
			t.Fatal(err)
		}
		if p := src.Pairs; len(p) != 1 || p[0].Value != "x-suffix" {
			t.Errorf("expanded = %+v", p)
		}
	})

	t.Run("explicit missing key fails loudly", func(t *testing.T) {
		t.Parallel()
		if _, err := sourceFixture(t, content, providerkit.SelectOpts{Keys: []string{"NOPE"}}); err == nil {
			t.Error("missing explicit key must fail, not silently skip")
		}
	})

	t.Run("required-var error fails before any plugin work", func(t *testing.T) {
		t.Parallel()
		if _, err := sourceFixture(t, "A=${MISSING:?set me}\n", providerkit.SelectOpts{Expand: true}); err == nil {
			t.Error("want required-error failure from Source")
		}
	})

	t.Run("include flags lift the guards", func(t *testing.T) {
		t.Parallel()
		src, err := sourceFixture(t, content, providerkit.SelectOpts{Prefix: "GITHUB_SECRET_", IncludePlaceholders: true, IncludeEmpty: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(src.Pairs) != 3 || len(src.Skipped) != 0 {
			t.Errorf("guards not lifted: pairs=%d skips=%d", len(src.Pairs), len(src.Skipped))
		}
	})
}

// scriptedRunner is the end-to-end fake for the full-CLI path.
type scriptedRunner struct {
	calls []string
}

func (r *scriptedRunner) Run(_ context.Context, stdin string, _ string, args ...string) ([]byte, error) {
	joined := strings.Join(args, " ")
	r.calls = append(r.calls, joined+" <stdin:"+stdin+">")
	switch {
	case strings.HasPrefix(joined, "repo view"):
		return []byte("owner/repo\n"), nil
	case strings.HasPrefix(joined, "api user"):
		return []byte("tester\n"), nil
	case strings.HasPrefix(joined, "secret list"):
		return []byte(`[]`), nil
	}
	return []byte(""), nil
}

// TestGithubEndToEnd drives `dotenvctl github push` through the REAL root
// command — flag binding, deps assembly, source pipeline, kit verb, plugin
// store — with only the process boundary faked.
func TestGithubEndToEnd(t *testing.T) {
	t.Parallel()
	path := writeFixture(t, "GITHUB_SECRET_PAT=ghtoken\nGITHUB_SECRET_WHO=__YOU__\nUNRELATED=1\n")
	runner := &scriptedRunner{}
	var out, errBuf bytes.Buffer

	a := &app{
		printer:       &outfmt.Printer{Out: &out},
		errOut:        &errBuf,
		runner:        runner,
		interactiveFn: func() bool { return false },
	}
	code := executeApp(a, []string{"-f", path, "github", "push", "--prefix", "GITHUB_SECRET_", "--strip-prefix", "--yes"}, &out, &errBuf)
	if code != ExitOK {
		t.Fatalf("exit = %d\nout: %s\nerr: %s", code, out.String(), errBuf.String())
	}

	joined := strings.Join(runner.calls, "\n")
	if !strings.Contains(joined, "secret set PAT --body - <stdin:ghtoken>") {
		t.Errorf("push call wrong:\n%s", joined)
	}
	if strings.Contains(joined, "UNRELATED") || strings.Contains(joined, "WHO") {
		t.Errorf("selection leaked unselected/guarded keys:\n%s", joined)
	}
	for _, want := range []string{"target : owner/repo", "WHO: skipped-placeholder", "PAT: pushed"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

// TestSourceKeys_ResolutionOrder pins the SECRETS_PORT_SPEC §1 precedence:
// explicit keys > include/exclude > prefix filters, with loud missing-key
// failures for anything the user NAMED and a no-op for excluding absentees.
func TestSourceKeys_ResolutionOrder(t *testing.T) {
	t.Parallel()
	const content = "GITHUB_SECRET_A=1\nGITHUB_SECRET_B=2\nAPP_X=3\nAPP_Y=4\nACTOR=5\n"

	tests := []struct {
		name    string
		opts    providerkit.SelectOpts
		want    []string
		wantErr bool
	}{
		{"exclude-prefix inverts selection", providerkit.SelectOpts{ExcludePrefix: "GITHUB_SECRET_"}, []string{"APP_X", "APP_Y", "ACTOR"}, false},
		{"prefix then exclude-prefix compose", providerkit.SelectOpts{Prefix: "APP_", ExcludePrefix: "APP_Y"}, []string{"APP_X"}, false},
		{"include bypasses prefix filter", providerkit.SelectOpts{Prefix: "GITHUB_SECRET_", IncludeKeys: []string{"ACTOR"}}, []string{"GITHUB_SECRET_A", "GITHUB_SECRET_B", "ACTOR"}, false},
		{"include deduplicates", providerkit.SelectOpts{Prefix: "GITHUB_SECRET_", IncludeKeys: []string{"GITHUB_SECRET_A"}}, []string{"GITHUB_SECRET_A", "GITHUB_SECRET_B"}, false},
		{"exclude drops from prefixed selection", providerkit.SelectOpts{Prefix: "GITHUB_SECRET_", ExcludeKeys: []string{"GITHUB_SECRET_B"}}, []string{"GITHUB_SECRET_A"}, false},
		{"exclude wins over include", providerkit.SelectOpts{IncludeKeys: []string{"ACTOR"}, ExcludeKeys: []string{"ACTOR"}, Prefix: "APP_"}, []string{"APP_X", "APP_Y"}, false},
		{"excluding an absent key is a no-op", providerkit.SelectOpts{ExcludeKeys: []string{"NOPE"}}, []string{"GITHUB_SECRET_A", "GITHUB_SECRET_B", "APP_X", "APP_Y", "ACTOR"}, false},
		{"explicit keys ignore prefix filters", providerkit.SelectOpts{Keys: []string{"APP_X"}, Prefix: "GITHUB_SECRET_"}, []string{"APP_X"}, false},
		{"explicit keys still honor exclude", providerkit.SelectOpts{Keys: []string{"APP_X", "APP_Y"}, ExcludeKeys: []string{"APP_Y"}}, []string{"APP_X"}, false},
		{"missing include fails loudly", providerkit.SelectOpts{IncludeKeys: []string{"NOPE"}}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			src, err := sourceFixture(t, content, tt.opts)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			var got []string
			for _, p := range src.Pairs {
				got = append(got, p.Key)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("keys = %v, want %v", got, tt.want)
			}
		})
	}
}
