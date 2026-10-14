package dotenvcmd

import (
	"runtime/debug"
	"testing"
)

// TestVersionString_EmbeddedReportsCLIModule pins bug 15: a host binary that
// embeds dotenvcmd must report dotenvctl's own module version for --version
// and DOTENVCTL_VERSION, read from the build's dependency list, not the
// host's version.
func TestVersionString_EmbeddedReportsCLIModule(t *testing.T) {
	t.Parallel()
	host := debug.Module{Path: "example.com/host", Version: "v9.9.9"}
	tests := []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{"standalone dotenvctl", &debug.BuildInfo{Main: debug.Module{Path: cliModulePath, Version: "v0.1.3"}}, "v0.1.3"},
		{"embedded: version from deps", &debug.BuildInfo{Main: host, Deps: []*debug.Module{
			{Path: "github.com/ubgo/dotenv", Version: "v0.1.2"},
			{Path: cliModulePath, Version: "v0.1.3"},
		}}, "v0.1.3"},
		{"embedded: replaced with a versioned module", &debug.BuildInfo{Main: host, Deps: []*debug.Module{
			{Path: cliModulePath, Version: "v0.1.3", Replace: &debug.Module{Path: "example.com/fork", Version: "v0.1.3-fork"}},
		}}, "v0.1.3-fork"},
		{"embedded: replaced with a local directory", &debug.BuildInfo{Main: host, Deps: []*debug.Module{
			{Path: cliModulePath, Version: "v0.1.3", Replace: &debug.Module{Path: "../cli"}},
		}}, "devel"},
		{"embedded: module not in deps", &debug.BuildInfo{Main: host}, "devel"},
		{"standalone source build", &debug.BuildInfo{Main: debug.Module{Path: cliModulePath}}, "devel"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := versionString(func() (*debug.BuildInfo, bool) { return tt.info, true })
			if got != tt.want {
				t.Errorf("versionString = %q, want %q", got, tt.want)
			}
		})
	}
}
