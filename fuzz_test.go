package dotenv

import (
	"strings"
	"testing"
)

// FuzzParseRender asserts the package's central invariant against arbitrary
// input: parsing and re-rendering an untouched file returns it byte for byte.
//
// Example-based tests only cover the shapes someone thought of. Any input at all
// is valid here, since an unrecognised line is meant to survive verbatim rather
// than be normalised.
func FuzzParseRender(f *testing.F) {
	for _, s := range []string{
		"", "\n", "A=1\n", "A=1", "# c\n\nA=1\n",
		"A=\"multi\nline\"\n", "A='lit'\n", "A=`tick`\n",
		"A=1\r\nB=2\r\n", "export A=1 # note\n", "not a pair\n",
		"A=${B}\n", "A=$$\n", "A=\"unterminated\n", "=bad\n",
		"A=a=b\n", "   \n\t\n", "A=héllo\n", "A=\\\n",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, in string) {
		if got := Parse(in).Render(); got != in {
			t.Errorf("round trip changed the input:\n got: %q\nwant: %q", got, in)
		}
	})
}

// FuzzParseIsIdempotent asserts re-parsing rendered output yields the same
// result, so a file cannot drift by being processed repeatedly.
func FuzzParseIsIdempotent(f *testing.F) {
	for _, s := range []string{"A=1\n", "A=\"x\ny\"\n", "# c\nA=1\n", "A=`t`\n"} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, in string) {
		once := Parse(in).Render()
		if twice := Parse(once).Render(); once != twice {
			t.Errorf("parsing is not idempotent:\n once: %q\ntwice: %q", once, twice)
		}
	})
}

// FuzzSetRoundTrip is the editor's guarantee: whatever value Set writes, the
// parser reads back identically, for ANY value.
//
// This is the property that caught the backtick-quoting bug — the renderer had
// not learned about a delimiter the parser had.
func FuzzSetRoundTrip(f *testing.F) {
	for _, s := range []string{
		"", "simple", "with space", "multi\nline", `"quoted"`, "'single'",
		"`backtick`", `back\slash`, "#hash", "$VAR", "a=b", "héllo", "\t",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, value string) {
		// A carriage return cannot survive: the format is line-based, so the
		// parser reads it as a terminator. Skipping documents the one genuine
		// limit rather than hiding it.
		if strings.ContainsRune(value, '\r') {
			t.Skip("carriage returns are line terminators, not value content")
		}

		file := Parse("K=placeholder\n")
		file.Set("K", value)
		rendered := file.Render()

		back, ok := Parse(rendered).Get("K")
		if !ok {
			t.Fatalf("Set(%q) produced output with no K:\n%q", value, rendered)
		}
		if back != value {
			t.Errorf("Set(%q) → Get = %q\nrendered: %q", value, back, rendered)
		}
	})
}

// FuzzSetIsIdempotent asserts a repeated edit produces no churn, which is what
// makes running a tool twice safe in version control.
func FuzzSetIsIdempotent(f *testing.F) {
	for _, s := range []string{"", "x", "with space", "multi\nline"} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, value string) {
		if strings.ContainsRune(value, '\r') {
			t.Skip("carriage returns are line terminators, not value content")
		}

		file := Parse("K=start\n# keep me\n")
		file.Set("K", value)
		once := file.Render()
		file.Set("K", value)

		if twice := file.Render(); twice != once {
			t.Errorf("Set(%q) twice differs:\n once: %q\ntwice: %q", value, once, twice)
		}
		if !strings.Contains(once, "# keep me") {
			t.Errorf("Set(%q) destroyed the comment:\n%q", value, once)
		}
	})
}

// FuzzExpandTerminates asserts expansion always halts and never panics, whatever
// tangle of references the input contains.
func FuzzExpandTerminates(f *testing.F) {
	for _, s := range []string{
		"A=${B}\nB=${A}\n", "A=${A}\n", "A=${B:-${C:-${A}}}\n",
		"A=$$\n", "A=${\n", "A=$(\n", "A=${:-}\n", "A=$\n",
		"A=${B:?}\n", "A=${B:+${C}}\n", "A=$$$$$\n",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, in string) {
		file := Parse(in)
		// A panic or a hang is the failure; the values are unchecked because
		// arbitrary input has no expected output.
		for _, k := range file.Keys() {
			_, _, _ = file.GetExpanded(k)
		}
		_, _ = file.ExpandedMap()
	})
}

// FuzzAppendRoundTrip extends the editor's guarantee to literal placement:
// whatever Append writes, the parser reads back identically.
func FuzzAppendRoundTrip(f *testing.F) {
	for _, s := range []string{"", "x", "with space", "multi\nline", "#hash", "`tick`"} {
		f.Add("A=1\n", s)
	}
	f.Add("", "v")
	f.Add("A=1\r\n", "v")

	f.Fuzz(func(t *testing.T, src, value string) {
		if strings.ContainsRune(value, '\r') {
			t.Skip("carriage returns are line terminators, not value content")
		}
		// A source ending inside an unterminated quote swallows whatever is
		// appended, because the open quote keeps consuming lines. The file was
		// already malformed; nothing appended to it can be found again. Probe
		// for that rather than guessing from the text — see
		// TestAppend_UnterminatedQuoteSwallowsWhatFollows.
		probe := Parse(src)
		probe.Append(NewPair("VOLT_PROBE", "1"))
		if !Parse(probe.Render()).Has("VOLT_PROBE") {
			t.Skip("source ends inside an unterminated quote")
		}

		file := Parse(src)
		file.Append(NewComment("note"), NewPair("APPENDED", value), NewBlank())
		rendered := file.Render()

		back, ok := Parse(rendered).Get("APPENDED")
		if !ok {
			t.Fatalf("Append produced output with no APPENDED:\n%q", rendered)
		}
		if back != value {
			t.Errorf("Append(%q) → Get = %q\nrendered: %q", value, back, rendered)
		}
		// Everything that was already there must survive untouched.
		if !strings.HasPrefix(rendered, Parse(src).Render()) {
			t.Errorf("Append disturbed the existing content:\n%q", rendered)
		}
	})
}

// FuzzDisableEnableIsReversible asserts Unset followed by Restore returns the
// file to its exact original bytes — the property that makes commenting a key
// out a safe, undoable operation rather than a lossy one.
func FuzzDisableEnableIsReversible(f *testing.F) {
	for _, s := range []string{
		"A=1\n", "  A  =  1  \n", "export A=1 # note\n", "A=\"one\ntwo\"\n",
		"A=1\r\n", "A=\n", "A='lit'\n", "A=`tick`\n", "#A=1\n", "   #  A=1\n",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, src string) {
		file := Parse(src)
		if !file.Has("A") {
			t.Skip("nothing to disable")
		}
		// Restore targets the LAST disabled entry for the key, so a disabled
		// entry sitting after the one Unset just created is what comes back.
		// The round trip is exact only when there is no such entry — see
		// TestRestore_TargetsTheLastDisabledEntry.
		for _, d := range file.Disabled() {
			if d.Key == "A" {
				t.Skip("an existing disabled A would be restored instead")
			}
		}
		before := file.Render()

		if !file.Unset("A", false) {
			t.Fatal("Unset reported not found for an active key")
		}
		if !file.Restore("A") {
			t.Fatalf("Restore could not reverse Unset:\n%q", file.Render())
		}
		if after := file.Render(); after != before {
			t.Errorf("disable → enable was lossy:\n before: %q\n  after: %q", before, after)
		}
	})
}

// FuzzPluginsNeverAffectBytes is the firewall as a property: for ANY input, a
// file parsed with every value-changing hook installed must render to the same
// bytes as one parsed with none.
//
// The example-based version covers one crafted file. This covers the ones nobody
// thought of, which is where a future hook would breach the rule unnoticed.
func FuzzPluginsNeverAffectBytes(f *testing.F) {
	for _, s := range []string{
		"A=1\n", "A=${B}\n", "A=\"$(cmd)\"\n", "# A=disabled\n",
		"A='literal'\n", "A=`tick`\n", "A=\"multi\nline\"\n", "", "\n",
		"A=1\r\nB=${A}\r\n", "not a pair\n", "A=$$\n",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, src string) {
		plain := Parse(src)
		hooked := Parse(src,
			WithLookup(func(string) (string, bool) { return "SUPPLIED", true }),
			WithValueTransform(func(_, v string) (string, error) { return "MANGLED-" + v, nil }),
			WithCommandRunner(func(string) (string, error) { return "RAN", nil }),
		)

		if plain.Render() != hooked.Render() {
			t.Errorf("plugins changed the bytes:\n plain: %q\nhooked: %q", plain.Render(), hooked.Render())
		}

		// Reading is allowed to differ, and must terminate rather than hang.
		for _, k := range hooked.Keys() {
			_, _, _ = hooked.GetExpanded(k)
		}

		// An edit with hooks installed must also match one without.
		plain.Set("PROBE", "v")
		hooked.Set("PROBE", "v")
		if plain.Render() != hooked.Render() {
			t.Errorf("plugins changed the bytes after an edit:\n plain: %q\nhooked: %q", plain.Render(), hooked.Render())
		}
	})
}
