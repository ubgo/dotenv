package vercelplugin

import (
	"strings"
	"testing"

	"github.com/ubgo/dotenv/cli/envkit"
)

// TestList_EmptyHintIsVercelShaped pins bug 14 for this plugin: an empty
// `vercel list` printed GitHub's "try --environment or --org" hint. It must
// name Vercel's own scope flag, --target, and nothing GitHub-specific.
func TestList_EmptyHintIsVercelShaped(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{responses: map[string]string{"whoami": "khanakia\n", "env ls": "Vercel CLI 33.0.0\n> No Environment Variables found\n"}}
	out, err := runVercel(t, runner, &envkit.Selection{}, "list")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Contains(out, "--environment") || strings.Contains(out, "--org") {
		t.Errorf("vercel hint names GitHub flags: %q", out)
	}
	if !strings.Contains(out, "--target") {
		t.Errorf("vercel hint does not name --target: %q", out)
	}
}
