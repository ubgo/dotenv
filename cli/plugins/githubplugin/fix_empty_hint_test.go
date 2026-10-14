package githubplugin

import (
	"strings"
	"testing"

	"github.com/ubgo/dotenv/cli/envkit"
)

// TestList_EmptyHintIsGitHubShaped pins that the GitHub plugin keeps its
// scope nudge after the hint moved out of the kit (bug 14): repository,
// environment and organization secrets are listed separately.
func TestList_EmptyHintIsGitHubShaped(t *testing.T) {
	t.Parallel()
	resp := ghResponses()
	resp["secret list"] = `[]`
	out, err := runGithub(t, &fakeRunner{responses: resp}, &envkit.Selection{}, "list")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "--environment") || !strings.Contains(out, "--org") {
		t.Errorf("github hint lost its scope nudge: %q", out)
	}
}
