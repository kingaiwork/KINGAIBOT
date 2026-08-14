package workgraph

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
)

func breakWorkGraphAudit(t *testing.T, eventsDir string) {
	t.Helper()
	path := filepath.Join(eventsDir, "events.jsonl")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func newAuditOrderStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	eventsDir := filepath.Join(dir, "events")
	events, err := eventlog.New(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(filepath.Join(dir, "workgraphs"), events)
	if err != nil {
		t.Fatal(err)
	}
	return store, eventsDir
}

func TestApproveAuditFailureDoesNotExpandExecutionState(t *testing.T) {
	store, eventsDir := newAuditOrderStore(t)
	graph, err := store.Create("approval ordering", []Node{{
		ID:               "execute",
		Type:             TypeExecute,
		RequiresApproval: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	graph, err = store.Refresh(graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	if graph.Nodes["execute"].State != StateApproval {
		t.Fatalf("expected approval state, got %s", graph.Nodes["execute"].State)
	}
	breakWorkGraphAudit(t, eventsDir)
	if _, err := store.Approve(graph.ID, "execute"); err == nil {
		t.Fatal("approve unexpectedly succeeded with broken audit log")
	}
	stored, err := store.Get(graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Nodes["execute"].State != StateApproval {
		t.Fatalf("failed approval audit expanded state to %s", stored.Nodes["execute"].State)
	}
}

func TestFailAuditFailureKeepsFailClosedState(t *testing.T) {
	store, eventsDir := newAuditOrderStore(t)
	graph, err := store.Create("fail closed ordering", []Node{{ID: "execute", Type: TypeExecute}})
	if err != nil {
		t.Fatal(err)
	}
	graph, err = store.Refresh(graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	graph, err = store.Start(graph.ID, "execute")
	if err != nil {
		t.Fatal(err)
	}
	if graph.Nodes["execute"].State != StateRunning {
		t.Fatalf("expected running state, got %s", graph.Nodes["execute"].State)
	}
	breakWorkGraphAudit(t, eventsDir)
	if _, err := store.Fail(graph.ID, "execute", "remote execution denied"); err == nil {
		t.Fatal("expected audit failure from fail-closed transition")
	}
	stored, err := store.Get(graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Nodes["execute"].State != StateFailed {
		t.Fatalf("audit failure rolled fail-closed state back to %s", stored.Nodes["execute"].State)
	}
}

func TestCreateAuditFailureNeverPersistsGraph(t *testing.T) {
	store, eventsDir := newAuditOrderStore(t)
	breakWorkGraphAudit(t, eventsDir)
	if _, err := store.Create("must not persist", []Node{{ID: "n1", Type: TypeRead}}); err == nil {
		t.Fatal("create unexpectedly succeeded with broken audit log")
	}
	graphs, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(graphs) != 0 {
		t.Fatalf("audit-failed graph became visible: %d graphs", len(graphs))
	}
}
