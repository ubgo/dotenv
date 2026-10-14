package providerkit

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ubgo/dotenv"
)

// partialEnvelope is the --json shape of a push or prune that failed part-way:
// one ok:false envelope that carries both the error and the partial record.
type partialEnvelope struct {
	OK   bool `json:"ok"`
	Data struct {
		Results []Result `json:"results"`
	} `json:"data"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// TestJSON_PartialFailureIsOneEnvelope pins bug 13: a push or prune that
// failed part-way under --json printed two envelopes (ok:true with the
// partial results, then ok:false). It must print one ok:false envelope whose
// data still holds what already happened, so an idempotent re-run can be
// reasoned about from a single parse.
func TestJSON_PartialFailureIsOneEnvelope(t *testing.T) {
	t.Parallel()
	pairs := []dotenv.Pair{{Key: "A", Value: "1"}, {Key: "BAD", Value: "x"}, {Key: "C", Value: "3"}}

	t.Run("push", func(t *testing.T) {
		t.Parallel()
		deps, out := testDeps(selection(pairs), false, false)
		deps.Printer.JSON = true
		c := NewPushCmd(deps, &fakeStore{failOn: "BAD"}, VerbConfig{})
		c.SetArgs([]string{"--yes"})
		if err := c.Execute(); err == nil {
			t.Fatal("want failure")
		}
		env := decodeOne(t, out.String())
		if env.OK || env.Error.Code != "exec" || !strings.Contains(env.Error.Message, "push BAD") {
			t.Errorf("envelope = %s, want ok:false code exec naming BAD", out.String())
		}
		if len(env.Data.Results) != 1 || env.Data.Results[0] != (Result{Name: "A", Action: ActionPushed}) {
			t.Errorf("partial results = %+v, want only A pushed", env.Data.Results)
		}
	})

	t.Run("prune", func(t *testing.T) {
		t.Parallel()
		deps, out := testDeps(selection(nil), false, false)
		deps.Printer.JSON = true
		store := &fakeStore{names: []string{"A", "BAD", "C"}, failOn: "BAD"}
		c := NewPruneCmd(deps, store, store, VerbConfig{})
		c.SetArgs([]string{"--yes"})
		if err := c.Execute(); err == nil {
			t.Fatal("want failure")
		}
		env := decodeOne(t, out.String())
		if env.OK || env.Error.Code != "exec" || !strings.Contains(env.Error.Message, "delete BAD") {
			t.Errorf("envelope = %s, want ok:false code exec naming BAD", out.String())
		}
		if len(env.Data.Results) != 1 || env.Data.Results[0] != (Result{Name: "A", Action: ActionDeleted}) {
			t.Errorf("partial results = %+v, want only A deleted", env.Data.Results)
		}
	})
}

// TestHuman_PartialFailureSplitsStreams pins the human half: the partial
// record is output (stdout), the failure line is an error (stderr).
func TestHuman_PartialFailureSplitsStreams(t *testing.T) {
	t.Parallel()
	pairs := []dotenv.Pair{{Key: "A", Value: "1"}, {Key: "BAD", Value: "x"}}
	deps, out := testDeps(selection(pairs), false, false)
	var errBuf strings.Builder
	deps.Printer.Err = &errBuf
	c := NewPushCmd(deps, &fakeStore{failOn: "BAD"}, VerbConfig{})
	c.SetArgs([]string{"--yes"})
	if err := c.Execute(); err == nil {
		t.Fatal("want failure")
	}
	if out.String() != "A: pushed\n" {
		t.Errorf("stdout = %q, want the partial record only", out.String())
	}
	if !strings.HasPrefix(errBuf.String(), "dotenvctl: push BAD") {
		t.Errorf("stderr = %q, want the failure line", errBuf.String())
	}
}

// decodeOne fails unless s is exactly one JSON line and decodes it.
func decodeOne(t *testing.T, s string) partialEnvelope {
	t.Helper()
	if strings.Count(s, "\n") != 1 || !strings.HasSuffix(s, "\n") {
		t.Fatalf("want exactly one envelope line, got:\n%s", s)
	}
	var env partialEnvelope
	if err := json.Unmarshal([]byte(s), &env); err != nil {
		t.Fatalf("not one JSON object (%v):\n%s", err, s)
	}
	return env
}
