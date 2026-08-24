package providerkit

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ubgo/dotenv"
	"github.com/ubgo/dotenv/cli/internal/outfmt"
)

// fakeStore records calls and can fail on demand — the whole capability
// surface in one test double.
type fakeStore struct {
	sets    []string // "NAME=VALUE"
	deletes []string
	names   []string
	failOn  string // Set/Delete on this name errors
}

func (f *fakeStore) Set(_ context.Context, name, value string) error {
	if name == f.failOn {
		return errors.New("backend boom")
	}
	f.sets = append(f.sets, name+"="+value)
	return nil
}

func (f *fakeStore) Names(context.Context) ([]string, error) { return f.names, nil }

func (f *fakeStore) Delete(_ context.Context, name string) error {
	if name == f.failOn {
		return errors.New("backend boom")
	}
	f.deletes = append(f.deletes, name)
	return nil
}

// fakeSource is a canned selection.
type fakeSource struct {
	pairs []dotenv.Pair
	skips []Skip
}

func (s *fakeSource) Pairs() []dotenv.Pair { return s.pairs }
func (s *fakeSource) Skipped() []Skip      { return s.skips }

// testDeps wires a Deps over buffers with scripted interactivity.
func testDeps(src *fakeSource, interactive, answer bool) (Deps, *bytes.Buffer) {
	var out bytes.Buffer
	return Deps{
		Printer:     &outfmt.Printer{Out: &out},
		File:        ".env.test",
		Source:      func(SelectOpts) (Source, error) { return src, nil },
		Runner:      ExecRunner{},
		Interactive: func() bool { return interactive },
		Ask:         func(string) bool { return answer },
	}, &out
}

func TestPush_ActionsAndOrder(t *testing.T) {
	t.Parallel()
	src := &fakeSource{
		pairs: []dotenv.Pair{{Key: "A", Value: "1"}, {Key: "B", Value: "2"}},
		skips: []Skip{{Name: "PH", Action: ActionSkippedPlaceholder}},
	}
	store := &fakeStore{}
	deps, out := testDeps(src, true, true)

	c := NewPushCmd(deps, store, VerbConfig{})
	c.SetArgs([]string{"--yes"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}

	if len(store.sets) != 2 || store.sets[0] != "A=1" || store.sets[1] != "B=2" {
		t.Errorf("sets = %v", store.sets)
	}
	for _, want := range []string{"PH: skipped-placeholder", "A: pushed", "B: pushed"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestPush_DryRunWritesNothing(t *testing.T) {
	t.Parallel()
	src := &fakeSource{pairs: []dotenv.Pair{{Key: "A", Value: "1"}}}
	store := &fakeStore{}
	deps, out := testDeps(src, true, false) // answer=false: prompt would refuse

	c := NewPushCmd(deps, store, VerbConfig{})
	c.SetArgs([]string{"--dry-run"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(store.sets) != 0 {
		t.Errorf("dry-run wrote: %v", store.sets)
	}
	if !strings.Contains(out.String(), "A: would-push") {
		t.Errorf("missing would-push:\n%s", out.String())
	}
}

func TestPush_ConfirmMatrix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		interactive, yes  bool
		answer, wantWrite bool
	}{
		{"interactive approved", true, false, true, true},
		{"interactive declined", true, false, false, false},
		{"non-interactive without yes refuses", false, false, false, false},
		{"non-interactive with yes proceeds", false, true, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			src := &fakeSource{pairs: []dotenv.Pair{{Key: "A", Value: "1"}}}
			store := &fakeStore{}
			deps, _ := testDeps(src, tt.interactive, tt.answer)

			c := NewPushCmd(deps, store, VerbConfig{})
			if tt.yes {
				c.SetArgs([]string{"--yes"})
			} else {
				c.SetArgs([]string{})
			}
			err := c.Execute()
			if tt.wantWrite != (len(store.sets) == 1) {
				t.Errorf("wrote = %v, want %v (err %v)", store.sets, tt.wantWrite, err)
			}
			if !tt.wantWrite && err == nil {
				t.Error("refusal must surface as an error")
			}
		})
	}
}

func TestPush_MidFailureReportsPartial(t *testing.T) {
	t.Parallel()
	src := &fakeSource{pairs: []dotenv.Pair{{Key: "A", Value: "1"}, {Key: "BAD", Value: "x"}, {Key: "C", Value: "3"}}}
	store := &fakeStore{failOn: "BAD"}
	deps, out := testDeps(src, false, false)

	c := NewPushCmd(deps, store, VerbConfig{})
	c.SetArgs([]string{"--yes"})
	err := c.Execute()
	if err == nil {
		t.Fatal("want failure")
	}
	var ce *CmdError
	if !errors.As(err, &ce) || ce.Code != exitFailure {
		t.Errorf("err = %v, want CmdError exit 1", err)
	}
	// The partial record is the idempotence contract: A pushed, C never tried.
	if !strings.Contains(out.String(), "A: pushed") || strings.Contains(out.String(), "C: pushed") {
		t.Errorf("partial record wrong:\n%s", out.String())
	}
}

func TestPrune_StaleComputationAndGating(t *testing.T) {
	t.Parallel()
	src := &fakeSource{pairs: []dotenv.Pair{{Key: "KEEP", Value: "1"}}}
	store := &fakeStore{names: []string{"KEEP", "STALE_B", "STALE_A"}}
	deps, out := testDeps(src, false, false)

	c := NewPruneCmd(deps, store, store, VerbConfig{})
	c.SetArgs([]string{"--yes"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	// Sorted, KEEP survives.
	if len(store.deletes) != 2 || store.deletes[0] != "STALE_A" || store.deletes[1] != "STALE_B" {
		t.Errorf("deletes = %v", store.deletes)
	}
	if !strings.Contains(out.String(), "STALE_A: deleted") {
		t.Errorf("output missing deletion:\n%s", out.String())
	}

	// Without --yes non-interactively: refuse, delete nothing.
	store2 := &fakeStore{names: []string{"KEEP", "STALE"}}
	deps2, _ := testDeps(src, false, false)
	c2 := NewPruneCmd(deps2, store2, store2, VerbConfig{})
	c2.SetArgs([]string{})
	if err := c2.Execute(); err == nil {
		t.Error("ungated prune must refuse")
	}
	if len(store2.deletes) != 0 {
		t.Errorf("refused prune deleted: %v", store2.deletes)
	}
}

func TestList_NamesOnly(t *testing.T) {
	t.Parallel()
	store := &fakeStore{names: []string{"B", "A"}}
	deps, out := testDeps(&fakeSource{}, true, true)

	c := NewListCmd(deps, store)
	c.SetArgs([]string{})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.String() != "A\nB\n" {
		t.Errorf("list = %q, want sorted names", out.String())
	}
}

func TestGate_PluginBannerRuns(t *testing.T) {
	t.Parallel()
	src := &fakeSource{pairs: []dotenv.Pair{{Key: "A", Value: "1"}}}
	store := &fakeStore{}
	deps, out := testDeps(src, true, true)

	gateRan := false
	cfg := VerbConfig{Gate: func(_ context.Context, d Deps, info GateInfo) error {
		gateRan = true
		d.Printer.Humanf("target : fake/repo (%d keys)", len(info.Names))
		return Confirm(d, info)
	}}

	c := NewPushCmd(deps, store, cfg)
	c.SetArgs([]string{"--yes"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if !gateRan || !strings.Contains(out.String(), "target : fake/repo (1 keys)") {
		t.Errorf("gate banner missing:\n%s", out.String())
	}
}

func TestPrune_DryRunEmptyAndFailure(t *testing.T) {
	t.Parallel()

	t.Run("dry-run reports would-delete, deletes nothing", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{pairs: []dotenv.Pair{{Key: "KEEP", Value: "1"}}}
		store := &fakeStore{names: []string{"KEEP", "STALE"}}
		deps, out := testDeps(src, false, false)
		c := NewPruneCmd(deps, store, store, VerbConfig{})
		c.SetArgs([]string{"--dry-run"})
		if err := c.Execute(); err != nil {
			t.Fatal(err)
		}
		if len(store.deletes) != 0 || !strings.Contains(out.String(), "STALE: would-delete") {
			t.Errorf("deletes=%v out=%q", store.deletes, out.String())
		}
	})

	t.Run("nothing to prune says so and needs no gate", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{pairs: []dotenv.Pair{{Key: "A", Value: "1"}}}
		store := &fakeStore{names: []string{"A"}}
		deps, out := testDeps(src, false, false)
		c := NewPruneCmd(deps, store, store, VerbConfig{})
		c.SetArgs([]string{}) // no --yes — must still succeed: nothing to delete
		if err := c.Execute(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "nothing to prune") {
			t.Errorf("out = %q", out.String())
		}
	})

	t.Run("mid-delete failure reports partial and exits 1", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{}
		store := &fakeStore{names: []string{"A_OK", "B_BAD", "C_NEVER"}, failOn: "B_BAD"}
		deps, out := testDeps(src, false, false)
		c := NewPruneCmd(deps, store, store, VerbConfig{})
		c.SetArgs([]string{"--yes"})
		if err := c.Execute(); err == nil {
			t.Fatal("want failure")
		}
		if len(store.deletes) != 1 || store.deletes[0] != "A_OK" {
			t.Errorf("deletes = %v — must stop at the failure", store.deletes)
		}
		if !strings.Contains(out.String(), "A_OK: deleted") || strings.Contains(out.String(), "C_NEVER: deleted") {
			t.Errorf("partial record wrong:\n%s", out.String())
		}
	})
}

// failingLister makes Names error for the list/prune error arms.
type failingLister struct{ fakeStore }

func (failingLister) Names(context.Context) ([]string, error) { return nil, errors.New("api down") }

func TestListAndPrune_ListerFailure(t *testing.T) {
	t.Parallel()
	deps, out := testDeps(&fakeSource{}, false, false)
	c := NewListCmd(deps, &failingLister{})
	c.SetArgs([]string{})
	if err := c.Execute(); err == nil {
		t.Error("list must surface the lister failure")
	}
	if !strings.Contains(out.String(), "api down") {
		t.Errorf("cause lost: %q", out.String())
	}

	deps2, _ := testDeps(&fakeSource{}, false, false)
	l := &failingLister{}
	c2 := NewPruneCmd(deps2, l, &l.fakeStore, VerbConfig{})
	c2.SetArgs([]string{"--yes"})
	if err := c2.Execute(); err == nil {
		t.Error("prune must surface the lister failure")
	}
}

func TestPush_SourceFailureAborts(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	deps := Deps{
		Printer:     &outfmt.Printer{Out: &out},
		Source:      func(SelectOpts) (Source, error) { return nil, errors.New("required var missing") },
		Interactive: func() bool { return false },
		Ask:         func(string) bool { return false },
	}
	store := &fakeStore{}
	c := NewPushCmd(deps, store, VerbConfig{})
	c.SetArgs([]string{"--yes"})
	if err := c.Execute(); err == nil {
		t.Error("source failure must abort")
	}
	if len(store.sets) != 0 {
		t.Errorf("pushed despite source failure: %v", store.sets)
	}
}

func TestEmitResults_MetaErrorDowngradesToOmission(t *testing.T) {
	t.Parallel()
	src := &fakeSource{pairs: []dotenv.Pair{{Key: "A", Value: "1"}}}
	store := &fakeStore{}
	deps, out := testDeps(src, false, false)
	deps.Printer.JSON = true

	cfg := VerbConfig{Meta: func(context.Context) (map[string]string, error) {
		return nil, errors.New("resolution broke after the pushes")
	}}
	c := NewPushCmd(deps, store, cfg)
	c.SetArgs([]string{"--yes"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	// The actions happened and MUST be reported; only meta is absent.
	if !strings.Contains(out.String(), `"results":[{"name":"A","action":"pushed"}]`) {
		t.Errorf("results lost: %q", out.String())
	}
	if strings.Contains(out.String(), `"meta"`) {
		t.Errorf("failed meta must be omitted, not partial: %q", out.String())
	}
}

func TestFail_BrokenPrinterStillCarriesExitCode(t *testing.T) {
	t.Parallel()
	err := Fail(&outfmt.Printer{Out: brokenWriter{}}, outfmt.CodeExec, "boom")
	var ce *CmdError
	if !errors.As(err, &ce) || ce.Code != exitFailure {
		t.Fatalf("err = %#v", err)
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestList_EmptyStoreSaysSo(t *testing.T) {
	t.Parallel()
	deps, out := testDeps(&fakeSource{}, true, true)
	c := NewListCmd(deps, &fakeStore{})
	c.SetArgs([]string{})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no secrets found") {
		t.Errorf("empty list was silent: %q", out.String())
	}
}

// TestPrune_KeepSet pins SECRETS_PORT_SPEC §7.5: guard-skipped names and
// --keep names are spared and REPORTED; only genuinely stale names delete.
func TestPrune_KeepSet(t *testing.T) {
	t.Parallel()

	t.Run("skipped names are kept, not stale", func(t *testing.T) {
		t.Parallel()
		// Local: ACTIVE_KEY pushable, PLACEHOLDER_KEY guard-skipped.
		src := &fakeSource{
			pairs: []dotenv.Pair{{Key: "ACTIVE_KEY", Value: "v"}},
			skips: []Skip{{Name: "PLACEHOLDER_KEY", Action: ActionSkippedPlaceholder}},
		}
		// Remote holds both plus one genuinely stale name.
		store := &fakeStore{names: []string{"ACTIVE_KEY", "PLACEHOLDER_KEY", "TRULY_STALE"}}
		deps, out := testDeps(src, false, false)

		c := NewPruneCmd(deps, store, store, VerbConfig{})
		c.SetArgs([]string{"--yes"})
		if err := c.Execute(); err != nil {
			t.Fatal(err)
		}
		if len(store.deletes) != 1 || store.deletes[0] != "TRULY_STALE" {
			t.Fatalf("deletes = %v — a local placeholder must never delete a real remote value", store.deletes)
		}
		// The spare decision is visible, with its reason.
		if !strings.Contains(out.String(), "PLACEHOLDER_KEY: kept-skipped") {
			t.Errorf("kept-skipped not reported:\n%s", out.String())
		}
		// In-selection names are kept silently — no noise row.
		if strings.Contains(out.String(), "ACTIVE_KEY: kept") {
			t.Errorf("in-selection keep should be silent:\n%s", out.String())
		}
	})

	t.Run("--keep spares out-of-band names, both spellings", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{pairs: []dotenv.Pair{{Key: "A", Value: "1"}}}
		store := &fakeStore{names: []string{"A", "ENV_B64", "VPS_DEPLOY_KEY_B64", "STALE"}}
		deps, out := testDeps(src, false, false)

		c := NewPruneCmd(deps, store, store, VerbConfig{})
		c.SetArgs([]string{"--yes", "--keep", "ENV_B64,VPS_DEPLOY_KEY_B64"})
		if err := c.Execute(); err != nil {
			t.Fatal(err)
		}
		if len(store.deletes) != 1 || store.deletes[0] != "STALE" {
			t.Fatalf("deletes = %v — --keep names must survive", store.deletes)
		}
		for _, want := range []string{"ENV_B64: kept-keep-flag", "VPS_DEPLOY_KEY_B64: kept-keep-flag"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("kept reason missing %q:\n%s", want, out.String())
			}
		}
	})

	t.Run("nothing stale still reports the keeps", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{skips: []Skip{{Name: "PH", Action: ActionSkippedPlaceholder}}}
		store := &fakeStore{names: []string{"PH"}}
		deps, out := testDeps(src, false, false)

		c := NewPruneCmd(deps, store, store, VerbConfig{})
		c.SetArgs([]string{}) // no --yes needed: nothing deletes
		if err := c.Execute(); err != nil {
			t.Fatal(err)
		}
		if len(store.deletes) != 0 || !strings.Contains(out.String(), "nothing to prune") || !strings.Contains(out.String(), "PH: kept-skipped") {
			t.Errorf("out:\n%s deletes=%v", out.String(), store.deletes)
		}
	})
}
