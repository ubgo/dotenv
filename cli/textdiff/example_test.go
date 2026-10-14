package textdiff_test

import (
	"fmt"

	"github.com/ubgo/dotenv/cli/textdiff"
)

// snippet:start api_textdiff_Lines

// ExampleLines previews an edit as "-old"/"+new" lines for the one region
// that changed. Identical input returns nil, which callers read as "no-op".
// It trims the common prefix and suffix; it is not a general diff.
func ExampleLines() {
	before := "# db\nDB_HOST=localhost\nDB_PORT=5432\n"
	after := "# db\nDB_HOST=db.prod\nDB_PORT=5432\n"

	for _, l := range textdiff.Lines(before, after) {
		fmt.Println(l)
	}
	fmt.Println(textdiff.Lines(before, before) == nil)

	// Two separate edits come back as one block spanning both.
	fmt.Println(textdiff.Lines("A=1\nB=2\nC=3", "A=9\nB=2\nC=8"))
	// Output:
	// -DB_HOST=localhost
	// +DB_HOST=db.prod
	// true
	// [-A=1 -B=2 -C=3 +A=9 +B=2 +C=8]
}

// snippet:end api_textdiff_Lines
