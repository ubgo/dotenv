package dotenvcmd

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// execPluginPrefix names third-party plugin binaries: `dotenvctl-foo` on PATH
// serves `dotenvctl foo …`, git/kubectl style (PLUGINS_SPEC §6).
const execPluginPrefix = "dotenvctl-"

// Context env vars passed to exec plugins — facts only, NEVER secret values;
// a child that wants values calls the host back (the callback protocol).
// Closed set: plugin authors program against these names.
const (
	// EnvPluginFile is the absolute path of the -f file the user chose.
	EnvPluginFile = "DOTENVCTL_FILE"
	// EnvPluginJSON is "1" when the user passed --json, else "0".
	EnvPluginJSON = "DOTENVCTL_JSON"
	// EnvPluginPlaceholder is the active placeholder regexp.
	EnvPluginPlaceholder = "DOTENVCTL_PLACEHOLDER"
	// EnvPluginBin is the absolute path of the running dotenvctl — the
	// callback handle: `"$DOTENVCTL_BIN" -f "$DOTENVCTL_FILE" list --json`.
	EnvPluginBin = "DOTENVCTL_BIN"
	// EnvPluginVersion is the host version, so plugins can gate on features.
	EnvPluginVersion = "DOTENVCTL_VERSION"
)

// envJSONTrue / envJSONFalse are DOTENVCTL_JSON's two values — "1"/"0" rather
// than "true"/"false" so shell tests stay `[ "$DOTENVCTL_JSON" = 1 ]`.
const (
	envJSONTrue  = "1"
	envJSONFalse = "0"
)

// execDispatch runs BEFORE cobra: when the first non-flag token names no
// built-in command but a dotenvctl-<name> executable exists on PATH, the
// remaining argv is handed to it verbatim and its exit code becomes ours.
//
// Built-ins ALWAYS win — a PATH binary can never shadow `github` (the
// anti-typosquatting rule); `dotenvctl plugins` reports such shadowing
// instead. Returns handled=false to fall through to cobra (which owns the
// unknown-command usage error when no plugin exists either).
func (a *app) execDispatch(root *cobra.Command, args []string, stdout, stderr io.Writer) (code int, handled bool) {
	name, rest, file, jsonOut := scanForSubcommand(args)
	if name == "" || hasBuiltin(root, name) {
		return 0, false
	}
	path, err := exec.LookPath(execPluginPrefix + name)
	if err != nil {
		return 0, false
	}

	child := exec.Command(path, rest...)
	child.Stdin = os.Stdin
	child.Stdout = stdout
	child.Stderr = stderr
	child.Env = append(os.Environ(), a.pluginEnv(file, jsonOut)...)

	if err := child.Run(); err != nil {
		if xerr, ok := err.(*exec.ExitError); ok {
			// The plugin ran and failed — its code IS our code, same contract
			// as `run`.
			return xerr.ExitCode(), true
		}
		_, _ = stderr.Write([]byte("dotenvctl: exec plugin " + path + ": " + err.Error() + "\n"))
		return ExitFailure, true
	}
	return ExitOK, true
}

// pluginEnv builds the context block (spec §6). The file path is absolutized
// so the child is immune to its own chdir; the callback handle is our own
// executable path.
func (a *app) pluginEnv(file string, jsonOut bool) []string {
	absFile, err := filepath.Abs(file)
	if err != nil {
		absFile = file
	}
	self, err := os.Executable()
	if err != nil {
		// A host that cannot name itself still dispatches; the child's
		// callback simply won't resolve, which its own error reporting owns.
		self = ""
	}
	jsonFlag := envJSONFalse
	if jsonOut {
		jsonFlag = envJSONTrue
	}
	return []string{
		EnvPluginFile + "=" + absFile,
		EnvPluginJSON + "=" + jsonFlag,
		EnvPluginPlaceholder + "=" + defaultPlaceholderPattern,
		EnvPluginBin + "=" + self,
		EnvPluginVersion + "=" + versionString(debug.ReadBuildInfo),
	}
}

// scanForSubcommand finds the first non-flag token and the pre-subcommand
// global flag values the plugin env needs. Only the root's own globals can
// legally precede the subcommand, so the scan handles exactly those:
// -f/--file (value-taking, split or =-joined) and --json.
func scanForSubcommand(args []string) (name string, rest []string, file string, jsonOut bool) {
	file = defaultEnvFile
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-f" || arg == "--"+flagFile:
			if i+1 < len(args) {
				i++
				file = args[i]
			}
		case strings.HasPrefix(arg, "-f=") || strings.HasPrefix(arg, "--"+flagFile+"="):
			_, file, _ = strings.Cut(arg, "=")
		case arg == "--"+flagJSON:
			jsonOut = true
		case strings.HasPrefix(arg, "-"):
			// Some other flag; unknown ones fall to cobra's usage error later.
		default:
			return arg, args[i+1:], file, jsonOut
		}
	}
	return "", nil, file, jsonOut
}

// hasBuiltin reports whether name matches a registered command or alias —
// the always-wins set that PATH binaries can never shadow.
func hasBuiltin(root *cobra.Command, name string) bool {
	for _, c := range root.Commands() {
		if c.Name() == name || c.HasAlias(name) {
			return true
		}
	}
	// Cobra's implicit verbs are built-ins too.
	return slices.Contains(cobraImplicitCommands, name)
}

// cobraImplicitCommands are verbs cobra registers automatically without an
// AddCommand call — they must win over PATH plugins like any other built-in.
var cobraImplicitCommands = []string{"help", "completion"}

// discoveredPlugin is one PATH hit in plugins' --json output.
type discoveredPlugin struct {
	Name string `json:"name"`
	Path string `json:"path"`
	// Shadowed: the name collides with a built-in, which always wins — this
	// binary will never be dispatched to.
	Shadowed bool `json:"shadowed"`
}

// pluginsPayload is the --json data shape for `plugins`.
type pluginsPayload struct {
	Plugins []discoveredPlugin `json:"plugins"`
}

// newPluginsCmd lists every exec plugin discoverable on PATH, shadowing
// included — nothing executable is invisible (spec §6).
func newPluginsCmd(a *app, root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "plugins",
		Short: "List exec plugins (dotenvctl-* binaries) found on PATH",
		Example: "  dotenvctl plugins\n" +
			"  dotenvctl plugins --json | jq -r '.data.plugins[].name'",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			found := discoverExecPlugins(root)
			if len(found) == 0 {
				a.printer.Human("no exec plugins found on PATH (binaries named dotenvctl-<name>)")
				return a.printer.OK(pluginsPayload{Plugins: []discoveredPlugin{}})
			}

			w := tabwriter.NewWriter(a.printer.Out, 0, 0, tabPadding, ' ', 0)
			writeTabRow(w, "NAME", "PATH", "STATUS")
			for _, p := range found {
				status := "active"
				if p.Shadowed {
					status = "shadowed by built-in"
				}
				writeTabRow(w, p.Name, p.Path, status)
			}
			// Terminal output; same deliberate flush-error drop as the other
			// tables.
			_ = w.Flush()
			return a.printer.OK(pluginsPayload{Plugins: found})
		},
	}
}

// discoverExecPlugins scans each PATH directory for dotenvctl-* executables,
// first-hit-per-name wins (mirroring exec.LookPath's resolution order).
func discoverExecPlugins(root *cobra.Command) []discoveredPlugin {
	seen := map[string]bool{}
	var found []discoveredPlugin
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // unreadable PATH entries are normal; skip silently
		}
		for _, e := range entries {
			base := e.Name()
			if !strings.HasPrefix(base, execPluginPrefix) || e.IsDir() {
				continue
			}
			name := strings.TrimPrefix(base, execPluginPrefix)
			// Windows carries an extension; strip the executable one.
			name = strings.TrimSuffix(name, filepath.Ext(name))
			if name == "" || seen[name] {
				continue
			}
			full := filepath.Join(dir, base)
			if !isExecutable(full) {
				continue
			}
			seen[name] = true
			found = append(found, discoveredPlugin{Name: name, Path: full, Shadowed: hasBuiltin(root, name)})
		}
	}
	return found
}

// isExecutable reports whether the file has an execute bit (POSIX). On
// Windows PATHEXT decides instead; presence is treated as executable there.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if isWindows() {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

// isWindows is split out only so isExecutable reads cleanly.
func isWindows() bool { return os.PathSeparator == '\\' }
