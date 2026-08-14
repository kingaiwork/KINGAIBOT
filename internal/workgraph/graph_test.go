package workgraph

import (
	"testing"
	"time"
)

func TestDependencyOrderAndApprovalGate(t *testing.T) {
	now := time.Now().UTC()
	g, err := New("g1", "deploy safely", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Add(Node{ID: "inspect", Type: TypeRead}, now); err != nil {
		t.Fatal(err)
	}
	if err := g.Add(Node{ID: "deploy", Type: TypeExecute, DependsOn: []string{"inspect"}, RequiresApproval: true}, now); err != nil {
		t.Fatal(err)
	}

	ready := g.Refresh(now)
	if len(ready) != 1 || ready[0] != "inspect" {
		t.Fatalf("unexpected ready nodes: %#v", ready)
	}
	if err := g.Start("inspect", now); err != nil {
		t.Fatal(err)
	}
	if err := g.Complete("inspect", nil, nil, now); err != nil {
		t.Fatal(err)
	}
	if ready := g.Refresh(now); len(ready) != 0 {
		t.Fatalf("approval-gated node must not become executable: %#v", ready)
	}
	if g.Nodes["deploy"].State != StateApproval {
		t.Fatalf("expected approval gate, got %s", g.Nodes["deploy"].State)
	}
	if err := g.Approve("deploy", now); err != nil {
		t.Fatal(err)
	}
	if err := g.Start("deploy", now); err != nil {
		t.Fatal(err)
	}
}

func TestHighRiskCompletionRequiresEvidence(t *testing.T) {
	now := time.Now().UTC()
	g, _ := New("g2", "change production state", now)
	if err := g.Add(Node{ID: "change", Type: TypeExecute, Risk: RiskHigh}, now); err != nil {
		t.Fatal(err)
	}
	g.Refresh(now)
	if err := g.Start("change", now); err != nil {
		t.Fatal(err)
	}
	if err := g.Complete("change", nil, nil, now); err == nil {
		t.Fatal("expected evidence requirement")
	}
	if err := g.Complete("change", nil, []Evidence{{Kind: "receipt", Reference: "change-123"}}, now); err != nil {
		t.Fatal(err)
	}
}

func TestAmbiguousManualReplayMovesToReconciliation(t *testing.T) {
	now := time.Now().UTC()
	g, _ := New("g3", "send external change", now)
	if err := g.Add(Node{ID: "send", Type: TypeExecute, Replay: ReplayManual}, now); err != nil {
		t.Fatal(err)
	}
	g.Refresh(now)
	if err := g.Start("send", now); err != nil {
		t.Fatal(err)
	}
	if err := g.AmbiguousSideEffect("send", "connection lost after request", now); err != nil {
		t.Fatal(err)
	}
	if g.Nodes["send"].State != StateReconciling {
		t.Fatalf("expected reconciliation, got %s", g.Nodes["send"].State)
	}
}

func TestCycleRejected(t *testing.T) {
	now := time.Now().UTC()
	g, _ := New("g4", "cycle test", now)
	if err := g.Add(Node{ID: "a", Type: TypeThink}, now); err != nil {
		t.Fatal(err)
	}
	if err := g.Add(Node{ID: "b", Type: TypeThink, DependsOn: []string{"a"}}, now); err != nil {
		t.Fatal(err)
	}
	g.Nodes["a"].DependsOn = []string{"b"}
	if err := g.Validate(); err == nil {
		t.Fatal("expected cycle validation failure")
	}
}
