package textdiff

import (
	"reflect"
	"testing"
)

func TestLines(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		before, after string
		want          []string
	}{
		{"identical is nil (the no-op contract)", "A=1\n", "A=1\n", nil},
		{"one line changed", "A=1\nB=2\nC=3\n", "A=1\nB=9\nC=3\n", []string{"-B=2", "+B=9"}},
		{"append", "A=1\n", "A=1\nB=2\n", []string{"+B=2"}},
		{"remove", "A=1\nB=2\n", "A=1\n", []string{"-B=2"}},
		{"empty to content", "", "A=1\n", []string{"+A=1"}},
		// The prefix/suffix overlap trap: naive trimming would claim the "x"
		// twice and slice out of range.
		{"repeated line grows", "x", "x\nx", []string{"+x"}},
		{"change at first line", "A=1\nB=2\n", "A=9\nB=2\n", []string{"-A=1", "+A=9"}},
		{"change at last line keeps trailing terminator stable", "A=1\nB=2", "A=1\nB=9", []string{"-B=2", "+B=9"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Lines(tt.before, tt.after); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Lines(%q, %q) = %v, want %v", tt.before, tt.after, got, tt.want)
			}
		})
	}
}
