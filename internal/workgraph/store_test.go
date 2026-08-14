package workgraph

import (
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	events, err := eventlog.New(dir + "/events")
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(dir+"/workgraphs", events)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestStoreCreatePersistsOutOfOrderDependencies(t *testing.T) {
	store := newTestStore(t)
	graph, err := store.Create("ship verified result", []Node{
		{ID: "report", Type: TypeReport, DependsOn: []string{"verify"}},
		{ID: "verify", Type: TypeVerify, DependsOn: []string{"execute"}},
		{ID: "execute", Type: TypeExecute, Risk: RiskHigh, Replay: ReplayManual},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(graph.Nodes))
	}
	stored, err := store.Get(graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Nodes["execute"].RequiresEvidence != true {
		t.Fatal("high-risk node must require evidence")
	}
}

func TestStoreApprovalEvidenceLifecycle(t *testing.T) {
	store := newTestStore(t)
	graph, err := store.Create("controlled deployment", []Node{{
		ID:               "deploy",
		Type:             TypeExecute,
		Risk:             RiskCritical,
		Replay:           ReplayManual,
		RequiresApproval: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	graph, err = store.Refresh(graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	if graph.Nodes["deploy"].State != StateApproval {
		t.Fatalf("expected approval state, got %s", graph.Nodes["deploy"].State)
	}
	graph, err = store.Approve(graph.ID, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if graph.Nodes["deploy"].State != StateReady {
		t.Fatalf("expected ready state, got %s", graph.Nodes["deploy"].State)
	}
	graph, err = store.Start(graph.ID, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Complete(graph.ID, "deploy", nil, nil); err == nil {
		t.Fatal("critical node completed without evidence")
	}
	graph, err = store.Complete(graph.ID, "deploy", map[string]any{"release": "ok"}, []Evidence{{Kind: "deployment_receipt", Reference: "release-123"}})
	if err != nil {
		t.Fatal(err)
	}
	if graph.Nodes["deploy"].State != StateCompleted {
		t.Fatalf("expected completed state, got %s", graph.Nodes["deploy"].State)
	}
}

func TestStoreAmbiguousSideEffectRequiresReconciliation(t *testing.T) {
	store := newTestStore(t)
	graph, err := store.Create("external write", []Node{{
		ID:     "write",
		Type:   TypeExecute,
		Replay: ReplayManual,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Refresh(graph.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Start(graph.ID, "write"); err != nil {
		t.Fatal(err)
	}
	graph, err = store.Ambiguous(graph.ID, "write", "remote timeout after request transmission")
	if err != nil {
		t.Fatal(err)
	}
	if graph.Nodes["write"].State != StateReconciling {
		t.Fatalf("expected reconciliation, got %s", graph.Nodes["write"].State)
	}
}

func TestStoreRejectsDependencyCycle(t *testing.T) {
	store := newTestStore(t)
	_, err := store.Create("cycle", []Node{
		{ID: "a", Type: TypeThink, DependsOn: []string{"b"}},
		{ID: "b", Type: TypeThink, DependsOn: []string{"a"}},
	})
	if err == nil {
		t.Fatal("expected dependency cycle to fail")
	}
}
