package providerkit

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/ubgo/dotenv/cli/outfmt"
)

// TestExecRunner exercises the one production Runner against real (tiny)
// subprocesses — the seam every plugin's backend calls flow through.
func TestExecRunner(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("sh-based fixtures")
	}
	r := ExecRunner{}
	ctx := context.Background()

	t.Run("stdout captured", func(t *testing.T) {
		t.Parallel()
		out, err := r.Run(ctx, "", "sh", "-c", "printf hello")
		if err != nil || string(out) != "hello" {
			t.Errorf("out=%q err=%v", out, err)
		}
	})

	t.Run("stdin reaches the child", func(t *testing.T) {
		t.Parallel()
		out, err := r.Run(ctx, "secret-value", "sh", "-c", "cat")
		if err != nil || string(out) != "secret-value" {
			t.Errorf("out=%q err=%v", out, err)
		}
	})

	t.Run("failure carries stderr context", func(t *testing.T) {
		t.Parallel()
		_, err := r.Run(ctx, "", "sh", "-c", "echo 'HTTP 403' >&2; exit 1")
		if err == nil || !strings.Contains(err.Error(), "HTTP 403") {
			t.Errorf("err = %v, want the child's stderr surfaced", err)
		}
	})

	t.Run("missing binary errors", func(t *testing.T) {
		t.Parallel()
		if _, err := r.Run(ctx, "", "definitely-not-a-binary-xyz"); err == nil {
			t.Error("want error for a missing binary")
		}
	})
}

// TestFail pins the failure contract plugins rely on: envelope emitted, quiet
// CmdError returned, usage mapped to exit 2 and everything else to exit 1.
func TestFail(t *testing.T) {
	t.Parallel()

	t.Run("operation failure is exit 1, human line printed", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		err := Fail(&outfmt.Printer{Out: &buf}, outfmt.CodeExec, "push %s: boom", "KEY")
		var ce *CmdError
		if !errors.As(err, &ce) || ce.Code != exitFailure || !ce.Quiet {
			t.Fatalf("err = %#v, want quiet exit-1 CmdError", err)
		}
		if got := buf.String(); got != "dotenvctl: push KEY: boom\n" {
			t.Errorf("human output = %q", got)
		}
	})

	t.Run("usage failure is exit 2, json envelope shaped", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		err := Fail(&outfmt.Printer{Out: &buf, JSON: true}, outfmt.CodeUsage, "bad flag")
		var ce *CmdError
		if !errors.As(err, &ce) || ce.Code != exitUsage {
			t.Fatalf("err = %#v, want exit-2 CmdError", err)
		}
		if got := buf.String(); got != `{"ok":false,"error":{"code":"usage","message":"bad flag"}}`+"\n" {
			t.Errorf("envelope = %q", got)
		}
	})
}

// TestCmdErrorString pins both Error branches in this package's own coverage
// (cross-package tests don't count here).
func TestCmdErrorString(t *testing.T) {
	t.Parallel()
	if got := (&CmdError{Code: 2}).Error(); got != "exit 2" {
		t.Errorf("bare = %q", got)
	}
	if got := (&CmdError{Code: 1, Err: errors.New("boom")}).Error(); got != "boom" {
		t.Errorf("wrapped = %q", got)
	}
}
