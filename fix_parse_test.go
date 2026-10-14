package dotenv

import "testing"

// TestCommentColon_ProseVsDisabledPair pins where a commented line that uses
// the `:` delimiter stops being a disabled setting and becomes prose.
//
// The rule (see parseComment): inside a comment, a `:` assignment is a
// disabled pair only when its value is a single token — quoted, or unquoted
// with no space or tab in it — and does not start with `//`. Prose such as
// "Note: rotate monthly" has a sentence after the colon, and a URL has `//`
// right after its scheme; neither reads as a setting. The `=` form is not
// affected at all: `# TODO=fix this` stays the documented false positive.
func TestCommentColon_ProseVsDisabledPair(t *testing.T) {
	tests := []struct {
		name  string
		input string
		kind  Kind
		key   string
		value string
	}{
		// Prose that used to read as a disabled pair.
		{"prose after a colon", "# Note: rotate monthly\n", KindComment, "", ""},
		{"URL", "# http://example.com\n", KindComment, "", ""},
		{"https URL with a path", "# https://example.com/docs?a=1\n", KindComment, "", ""},
		{"uppercase prose", "# TODO: fix this later\n", KindComment, "", ""},
		{"prose with an inline comment", "# Note: rotate monthly # ops\n", KindComment, "", ""},
		{"prose without a space after the colon", "#Note:rotate monthly\n", KindComment, "", ""},
		{"tab inside the value", "# Note:\trotate\tmonthly\n", KindComment, "", ""},

		// Settings that must stay disabled pairs, so Restore can bring them back.
		{"colon with a space", "# KEY: value\n", KindDisabledPair, "KEY", "value"},
		{"colon without a space", "# KEY:value\n", KindDisabledPair, "KEY", "value"},
		{"space before the colon", "# KEY : value\n", KindDisabledPair, "KEY", "value"},
		{"double-quoted value with spaces", `# KEY: "a b"` + "\n", KindDisabledPair, "KEY", "a b"},
		{"single-quoted value with spaces", "# KEY: 'a b'\n", KindDisabledPair, "KEY", "a b"},
		{"backtick value with spaces", "# KEY: `a b`\n", KindDisabledPair, "KEY", "a b"},
		{"inline comment after a token", "# KEY: v # why\n", KindDisabledPair, "KEY", "v"},
		{"trailing spaces after a token", "# KEY: v   \n", KindDisabledPair, "KEY", "v"},
		{"empty value", "# KEY:\n", KindDisabledPair, "KEY", ""},
		{"export prefix", "# export KEY: v\n", KindDisabledPair, "KEY", "v"},
		{"URL as the value", "# DB_URL: postgres://db/app\n", KindDisabledPair, "DB_URL", "postgres://db/app"},
		{"lowercase dotted key", "# spring.datasource.url: jdbc:x\n", KindDisabledPair, "spring.datasource.url", "jdbc:x"},

		// The `=` form is unchanged, including its documented false positive.
		{"equals with a sentence", "# TODO=fix this\n", KindDisabledPair, "TODO", "fix this"},
		{"equals with a URL-like value", "# http=//example.com\n", KindDisabledPair, "http", "//example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Parse(tt.input)
			e := f.Entries()[0]
			if e.Kind != tt.kind {
				t.Fatalf("Parse(%q) Kind = %s, want %s", tt.input, e.Kind, tt.kind)
			}
			if e.Key != tt.key || e.Value != tt.value {
				t.Errorf("got %q=%q, want %q=%q", e.Key, e.Value, tt.key, tt.value)
			}
			if out := f.Render(); out != tt.input {
				t.Errorf("round trip changed the line: %q", out)
			}
			if tt.kind == KindComment && f.Restore(tt.key) {
				t.Errorf("Restore activated prose %q", tt.input)
			}
		})
	}
}

// TestCommentColon_UnsetSaveRestore is the property the rule must keep: a pair
// commented out by Unset, written, and read back is still a disabled pair that
// Restore turns back into the original line. Checked for both delimiters.
//
// In memory Unset→Restore never re-parses, so it always works; the reload is
// what exercises parseComment.
func TestCommentColon_UnsetSaveRestore(t *testing.T) {
	lines := []string{
		"KEY=value", "KEY=two words", "KEY = v", "export KEY=v # note", `KEY="a b"`, "KEY=",
		"KEY: value", "KEY:value", "KEY : value", "export KEY: v # note",
		`KEY: "a b"`, "KEY: 'a b'", "KEY:", "KEY: postgres://db/app",
	}
	for _, line := range lines {
		t.Run(line, func(t *testing.T) {
			src := line + "\n"
			f := Parse(src)
			want, _ := f.Get("KEY")
			if !f.Unset("KEY", false) {
				t.Fatalf("Unset(KEY) found nothing in %q", src)
			}

			reloaded := Parse(f.Render())
			e := reloaded.Entries()[0]
			if e.Kind != KindDisabledPair || e.Key != "KEY" || e.Value != want {
				t.Fatalf("after Unset and reload %q parses as %s %q=%q, want disabled-pair KEY=%q",
					f.Render(), e.Kind, e.Key, e.Value, want)
			}
			if !reloaded.Restore("KEY") {
				t.Fatalf("Restore(KEY) found nothing in %q", reloaded.Render())
			}
			if got := reloaded.Render(); got != src {
				t.Errorf("Unset → reload → Restore = %q, want %q", got, src)
			}
		})
	}
}

// TestCommentColon_TradeOff pins the one thing the rule gives up, so changing
// it is a visible decision: an unquoted colon value with a space in it is
// indistinguishable from prose ("Note: rotate monthly"), so once commented out
// and reloaded it is a plain comment. Quote such a value, or use `=`, to keep
// it restorable after a reload. Unset→Restore within one File is unaffected.
func TestCommentColon_TradeOff(t *testing.T) {
	f := Parse("KEY: two words\n")
	f.Unset("KEY", false)
	if !f.Restore("KEY") {
		t.Fatal("in-memory Unset → Restore must still work")
	}

	f.Unset("KEY", false)
	reloaded := Parse(f.Render())
	if k := reloaded.Entries()[0].Kind; k != KindComment {
		t.Errorf("reloaded %q is %s; the documented trade-off says comment", f.Render(), k)
	}
}
