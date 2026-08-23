// Command dotenvctl is the comment-preserving .env control CLI over the
// github.com/ubgo/dotenv library.
//
// main stays a one-liner on purpose: Execute takes args and writers and
// returns the exit code, so the ENTIRE CLI — flag parsing to output — runs
// in-process in tests with buffers instead of a spawned binary.
package main

import (
	"os"

	"github.com/ubgo/dotenv/cli/dotenvcmd"
)

func main() {
	os.Exit(dotenvcmd.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
