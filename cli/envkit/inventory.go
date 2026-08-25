package envkit

import (
	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/discover"
)

// EnvFile is one discovered env file — re-exported so a caller can work
// through envkit alone without also importing the discover package.
type EnvFile = discover.EnvFile

// Discover finds and classifies the env-file family of a directory:
// `.env`, `.env.local`, `.env.staging`, `.env.prod`, `.env.example`, …
//
// Non-recursive and filename-based. Long environment names collapse to their
// canonical short form (`.env.production` and `.env.prod` are both "prod"),
// template files are tagged as contracts, and editor droppings (.bak/.tmp/…)
// are ignored. Order is distance-from-prod, deterministic — callers can rely
// on it for display.
func Discover(dir string) ([]EnvFile, error) {
	if dir == "" {
		dir = "."
	}
	return discover.Scan(dir)
}

// Inventory is the per-file summary behind the `envs` view.
type Inventory struct {
	// Env is the canonical environment name ("default", "stag", "prod", …).
	Env string `json:"env"`
	// File is the bare filename; Path is where it was found.
	File string `json:"file"`
	Path string `json:"path"`
	// Contract marks template files (.env.example and kin) — listed, but not
	// an environment.
	Contract bool `json:"contract"`
	// Keys counts ACTIVE pairs, deduplicated.
	Keys int `json:"keys"`
	// Disabled counts commented-out settings.
	Disabled int `json:"disabled"`
	// Inherited counts name-only declarations (values come from the process
	// environment — this package never resolves them).
	Inherited int `json:"inherited"`
	// Placeholders counts active values matching the placeholder pattern —
	// the "still needs a real value" signal.
	Placeholders int `json:"placeholders"`
}

// InventoryOptions configures Inventories.
type InventoryOptions struct {
	// Dir is the directory to scan ("" means ".").
	Dir string
	// PlaceholderPattern overrides DefaultPlaceholderPattern.
	PlaceholderPattern string
}

// Inventories discovers the directory's env files and summarizes each — the
// "what exists here and how complete is it" answer, in one call.
//
// An empty directory is not an error: it yields an empty slice, because
// "nothing found" is a legitimate answer a caller renders differently from a
// failure.
func Inventories(opts InventoryOptions) ([]Inventory, error) {
	placeholderRe, err := placeholderMatcher(opts.PlaceholderPattern)
	if err != nil {
		return nil, err
	}
	found, err := Discover(opts.Dir)
	if err != nil {
		return nil, err
	}

	out := make([]Inventory, 0, len(found))
	for _, ef := range found {
		f, err := dotenv.Open(ef.Path)
		if err != nil {
			return nil, err
		}
		inv := Inventory{
			Env:       ef.Env,
			File:      ef.File,
			Path:      ef.Path,
			Contract:  ef.Contract,
			Keys:      len(f.Keys()),
			Disabled:  len(f.Disabled()),
			Inherited: len(f.Inherited()),
		}
		for _, v := range f.Map() {
			if placeholderRe.MatchString(v) {
				inv.Placeholders++
			}
		}
		out = append(out, inv)
	}
	return out, nil
}
