package cognition

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/evolution"
	"github.com/kingaiwork/KINGAIBOT/internal/memory"
)

func TestLearningPersistsAndProposesOnlyAfterThreshold(t *testing.T) {
	root := t.TempDir()
	events, err := eventlog.New(filepath.Join(root, "events"))
	if err != nil {
		t.Fatal(err)
	}
	mem, err := memory.New(filepath.Join(root, "memory"), 100)
	if err != nil {
		t.Fatal(err)
	}
	store, err := evolution.New(filepath.Join(root, "evolution"))
	if err != nil {
		t.Fatal(err)
	}
	controller, err := evolution.NewController(store, events)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := New(filepath.Join(root, "cognition"), mem, controller, events, Config{Enabled: true, ReflectionInterval: time.Hour, MaxPrinciples: 16, AutoProposalFailureThreshold: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	if err := engine.ObserveSuccess("task_ok", "hello", "world", "local"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := engine.ObserveFailure("task_fail_"+itoa(i), "input", "provider-a", "provider timeout"); err != nil {
			t.Fatal(err)
		}
	}

	snap := engine.Snapshot()
	if snap.State.Episodes != 4 || snap.State.Successes != 1 || snap.State.Failures != 3 {
		t.Fatalf("unexpected cognitive stats: %+v", snap.State)
	}
	if snap.Disclaimer == "" {
		t.Fatal("expected consciousness disclaimer")
	}
	proposals, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals) != 1 {
		t.Fatalf("expected one controlled proposal, got %d", len(proposals))
	}
	if proposals[0].Status != "proposed" {
		t.Fatalf("proposal should remain review-only, got %q", proposals[0].Status)
	}

	engine.Close()
	reloaded, err := New(filepath.Join(root, "cognition"), mem, controller, events, Config{Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	if reloaded.Snapshot().State.Episodes != 4 {
		t.Fatal("cognitive state did not persist across restart")
	}
}
