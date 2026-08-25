// Package discover finds and classifies the env-file family of a directory —
// .env, .env.local, .env.prod, .env.example — so multi-file verbs (envs,
// matrix) agree on what exists and in which order.
//
// Pure and filesystem-read-only: Scan lists one directory, classifies by
// filename alone, and never opens the files. Content-level facts (key counts,
// drift) belong to the verbs, which parse through the dotenv library.
//
// The full classification and ordering contract lives in
// docsi/cli/MULTI_ENV_SPEC.md §1 — change that spec first, then this code.
package discover

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// envFilePrefix is the family marker: a candidate is exactly this name, or
// this name plus a dot-suffix. `*.env` files (foo.env) are deliberately NOT
// candidates — that convention belongs to other tools and matching it would
// drag build artifacts into the matrix.
const envFilePrefix = ".env"

// EnvDefault is the environment name of the bare `.env` file — it has no
// suffix to name it, and "default" is what every dotenv loader calls it.
const EnvDefault = "default"

// Rank buckets, spec §1: distance-from-prod ordering. Unknown suffixes sit
// between prod and contracts; within a bucket ties break alphabetically, so
// the output order is fully deterministic — a contract of the verbs' output.
const (
	rankDefault = iota
	rankLocal
	rankDev
	rankTest
	rankCI
	rankStag
	rankProd
	rankUnknown
	rankContract
)

// canonicalEnvs maps every KNOWN suffix (lowercased) to its canonical short
// name and rank. Long forms collapse to short (`production`→`prod`) so a repo
// mixing conventions still gets one column per environment concept.
var canonicalEnvs = map[string]struct {
	name string
	rank int
}{
	"local":       {"local", rankLocal},
	"dev":         {"dev", rankDev},
	"development": {"dev", rankDev},
	"test":        {"test", rankTest},
	"testing":     {"test", rankTest},
	"ci":          {"ci", rankCI},
	"stag":        {"stag", rankStag},
	"staging":     {"stag", rankStag},
	"prod":        {"prod", rankProd},
	"production":  {"prod", rankProd},
}

// contractSuffixes are template files that document required keys rather than
// configure an environment. Excluded from matrix columns; offered to
// --contract.
var contractSuffixes = map[string]bool{
	"example":  true,
	"sample":   true,
	"template": true,
	"dist":     true,
}

// junkSuffixes are editor/backup droppings that would otherwise classify as
// unknown environments and pollute every matrix.
var junkSuffixes = map[string]bool{
	"bak":  true,
	"tmp":  true,
	"old":  true,
	"orig": true,
	"swp":  true,
	"swo":  true,
	"lock": true,
}

// EnvFile is one discovered file.
type EnvFile struct {
	// Path is the file's path as given to the verbs (dir-joined, not
	// absolutized — output stays stable regardless of where the tool ran).
	Path string
	// File is the bare filename, e.g. ".env.staging".
	File string
	// Env is the canonical environment name: "default", "stag", "prod", or an
	// unknown suffix verbatim. For contracts it is the suffix ("example").
	Env string
	// Contract marks template files (.env.example and kin).
	Contract bool

	// rank drives the deterministic sort; unexported because ordering is
	// Scan's job, not the caller's.
	rank int
}

// Scan discovers the env-file family of one directory, non-recursively,
// returning files in the spec's display order. An empty result is not an
// error — the caller decides what "nothing found" means for its verb.
func Scan(dir string) ([]EnvFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("discover: read %s: %w", dir, err)
	}

	var out []EnvFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		f, ok := Classify(e.Name())
		if !ok {
			continue
		}
		// filepath.Join would also clean "./" away; keeping dir verbatim makes
		// the printed paths match what the user typed for --dir.
		f.Path = joinDir(dir, f.File)
		out = append(out, f)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].rank != out[j].rank {
			return out[i].rank < out[j].rank
		}
		return out[i].File < out[j].File
	})
	return out, nil
}

// Classify maps one filename into the taxonomy, reporting ok=false for
// non-candidates and junk. Exported so verbs taking explicit FILE args can
// label columns with the same vocabulary discovery uses.
func Classify(name string) (EnvFile, bool) {
	if name == envFilePrefix {
		return EnvFile{File: name, Env: EnvDefault, rank: rankDefault}, true
	}
	if !strings.HasPrefix(name, envFilePrefix+".") {
		return EnvFile{}, false
	}

	suffix := strings.ToLower(strings.TrimPrefix(name, envFilePrefix+"."))
	switch {
	case junkSuffixes[suffix]:
		return EnvFile{}, false
	case contractSuffixes[suffix]:
		return EnvFile{File: name, Env: suffix, Contract: true, rank: rankContract}, true
	}
	if known, ok := canonicalEnvs[suffix]; ok {
		return EnvFile{File: name, Env: known.name, rank: known.rank}, true
	}
	return EnvFile{File: name, Env: suffix, rank: rankUnknown}, true
}

// joinDir joins without cleaning, so "--dir ." prints paths the way the user
// wrote them ("./.env" stays recognizable as cwd-relative).
func joinDir(dir, file string) string {
	if strings.HasSuffix(dir, "/") {
		return dir + file
	}
	return dir + "/" + file
}
