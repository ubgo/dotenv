package outfmt_test

import (
	"fmt"
	"os"

	"github.com/ubgo/dotenv/cli/outfmt"
)

// apiOutfmtPayload is a verb's data shape. Pass a named struct to OK, never a
// bare map, so the JSON shape is visible where it is produced.
type apiOutfmtPayload struct {
	Keys []string `json:"keys"`
}

// snippet:start api_outfmt_Printer

// ExamplePrinter prints the same verb in both modes. Human mode prints lines
// and OK is silent; JSON mode prints exactly one envelope per call and Human
// is silent, so --json output stays parseable.
func ExamplePrinter() {
	for _, jsonMode := range []bool{false, true} {
		p := &outfmt.Printer{Out: os.Stdout, JSON: jsonMode}
		p.Human("DB_URL")
		p.Humanf("%d keys", 1)
		if err := p.OK(apiOutfmtPayload{Keys: []string{"DB_URL"}}); err != nil {
			fmt.Println("write:", err)
		}
	}
	// Output:
	// DB_URL
	// 1 keys
	// {"ok":true,"data":{"keys":["DB_URL"]}}
}

// snippet:end api_outfmt_Printer

// snippet:start api_outfmt_Printer_Fail

// ExamplePrinter_Fail reports a failure. Human mode prints "dotenvctl: <msg>"
// to Out (not stderr); JSON mode prints the error envelope. Fail only prints:
// the exit code is yours to return.
func ExamplePrinter_Fail() {
	human := &outfmt.Printer{Out: os.Stdout}
	machine := &outfmt.Printer{Out: os.Stdout, JSON: true}
	for _, p := range []*outfmt.Printer{human, machine} {
		if err := p.Fail(outfmt.CodeNotFound, `key "DB_URL" not found`); err != nil {
			fmt.Println("write:", err)
		}
	}
	// Output:
	// dotenvctl: key "DB_URL" not found
	// {"ok":false,"error":{"code":"not_found","message":"key \"DB_URL\" not found"}}
}

// snippet:end api_outfmt_Printer_Fail

// snippet:start api_outfmt_ErrCode

// ExampleErrCode lists the closed set of error codes scripts branch on.
func ExampleErrCode() {
	for _, c := range []outfmt.ErrCode{outfmt.CodeNotFound, outfmt.CodeIO, outfmt.CodeRequired, outfmt.CodeUsage, outfmt.CodeExec, outfmt.CodeDiff} {
		fmt.Println(c)
	}
	// Output:
	// not_found
	// io
	// required
	// usage
	// exec
	// diff
}

// snippet:end api_outfmt_ErrCode
