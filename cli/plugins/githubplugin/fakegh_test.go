package githubplugin

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/envkit"
	"github.com/ubgo/dotenv/cli/outfmt"
	"github.com/ubgo/dotenv/cli/providerkit"
)

// fakeGhScript stands in for the real gh binary. It answers the two gate
// lookups and, for `secret set`, dumps argv (NUL-separated) and stdin byte for
// byte so the test sees exactly what a real process would receive. This is the
// layer the in-process fakeRunner cannot see: the real ExecRunner and a real
// pipe. Issue #1 lived there — `--body -` is invisible to a fake that just
// records strings, because only gh itself knows `-` is not a stdin marker.
const fakeGhScript = `#!/bin/sh
case "$1 $2" in
  "repo view") echo "o/r"; exit 0 ;;
  "api user")  echo "me";  exit 0 ;;
  "secret set")
    printf '%s\0' "$@" > "$FAKE_GH_DIR/argv"
    cat > "$FAKE_GH_DIR/stdin"
    exit 0 ;;
esac
exit 1
`

// TestPush_RealProcess_ValueOnStdinNoBodyFlag pushes awkward values through the
// real ExecRunner into a fake gh on PATH and asserts the invariant that makes
// gh store the right thing: no --body flag at all (gh takes a non-empty --body
// literally, so `--body -` stored "-"), and the value arriving on stdin
// unmodified. The values cover every character class a shell, a dotenv parser
// or a flag parser would mangle.
func TestPush_RealProcess_ValueOnStdinNoBodyFlag(t *testing.T) {
	values := map[string]string{
		"dollar":   "ab$1cd${X}",
		"hash":     "pass #word",
		"spaces":   "  lead and trail  ",
		"quotes":   `it's "quoted" and ` + "`tick`",
		"dash":     "-",
		"flaglike": "--body",
		"trailing": "line1\nline2\n",
		"crlf":     "v\r\n",
	}
	for name, value := range values {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(fakeGhScript), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("FAKE_GH_DIR", dir)

			var out bytes.Buffer
			src := &envkit.Selection{Pairs: []dotenv.Pair{{Key: "K", Value: value}}}
			c := New().Command(providerkit.Deps{
				Printer:     &outfmt.Printer{Out: &out},
				File:        ".env.test",
				Source:      func(providerkit.SelectOpts) (providerkit.Source, error) { return src, nil },
				Runner:      providerkit.ExecRunner{},
				Interactive: func() bool { return false },
				Ask:         func(string) bool { return false },
			})
			c.SetArgs([]string{"push", "--yes"})
			if err := c.Execute(); err != nil {
				t.Fatalf("push: %v\n%s", err, out.String())
			}

			rawArgv, err := os.ReadFile(filepath.Join(dir, "argv"))
			if err != nil {
				t.Fatalf("fake gh never saw `secret set`: %v\n%s", err, out.String())
			}
			argv := strings.Split(strings.TrimSuffix(string(rawArgv), "\x00"), "\x00")
			if want := []string{"secret", "set", "K"}; strings.Join(argv, "|") != strings.Join(want, "|") {
				t.Errorf("argv = %q, want exactly %q (no --body: gh reads stdin only when it is absent)", argv, want)
			}
			for _, a := range argv {
				if strings.HasPrefix(a, "--body") || strings.HasPrefix(a, "-b") {
					t.Errorf("argv carries a body flag %q", a)
				}
			}
			stdin, err := os.ReadFile(filepath.Join(dir, "stdin"))
			if err != nil {
				t.Fatal(err)
			}
			if string(stdin) != value {
				t.Errorf("stdin = %q, want %q byte for byte", stdin, value)
			}
		})
	}
}
