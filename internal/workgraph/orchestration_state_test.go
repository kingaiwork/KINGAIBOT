package workgraph

import (
	"testing"
	"time"
)

func TestAbortUnleasedStartReturnsNodeToReady(t *testing.T) {
	now := time.Now().UTC()
	g, err := New("wg_abort", "safe dispatch handoff", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Add(Node{ID: "execute", Type: TypeExecute}, now); err != nil {
		t.Fatal(err)
	}
	g.Refresh(now)
	if err := g.Start("execute", now); err != nil {
		t.Fatal(err)
	}
	if err := g.AbortUnleasedStart("execute", "held job never activated", now); err != nil {
		t.Fatal(err)
	}
	if g.Nodes["execute"].State != StateReady {
		t.Fatalf("expected ready after proven unleased abort, got %s", g.Nodes["execute"].State)
	}
}

func TestRemoteReconciliationCanResumeThenComplete(t *testing.T) {
	now := time.Now().UTC()
	g, err := New("wg_reconcile", "remote execution", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Add(Node{ID: "execute", Type: TypeExecute, Risk: RiskHigh}, now); err != nil {
		t.Fatal(err)
	}
	g.Refresh(now)
	if err := g.Start("execute", now); err != nil {
		t.Fatal(err)
	}
	if err := g.RequireReconciliation("execute", "remote result uncertain", now); err != nil {
		t.Fatal(err)
	}
	if g.Nodes["execute"].State != StateReconciling {
		t.Fatalf("expected reconciling, got %s", g.Nodes["execute"].State)
	}
	if err := g.ResumeReconciliation("execute", now); err != nil {
		t.Fatal(err)
	}
	if g.Nodes["execute"].State != StateRunning {
		t.Fatalf("expected running after remote requeue, got %s", g.Nodes["execute"].State)
	}
	if err := g.Complete("execute", map[string]any{"ok": true}, []Evidence{{Kind: "cluster_job", Reference: "job_123"}}, now); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteFailureTerminatesRunningOrReconcilingNode(t *testing.T) {
	now := time.Now().UTC()
	for _, reconcileFirst := range []bool{false, true} {
		g, err := New("wg_fail", "remote failure", now)
		if err != nil {
			t.Fatal(err)
		}
		if err := g.Add(Node{ID: "execute", Type: TypeExecute}, now); err != nil {
			t.Fatal(err)
		}
		g.Refresh(now)
		if err := g.Start("execute", now); err != nil {
			t.Fatal(err)
		}
		if reconcileFirst {
			if err := g.RequireReconciliation("execute", "uncertain", now); err != nil {
				t.Fatal(err)
			}
		}
		if err := g.Fail("execute", "worker failed", now); err != nil {
			t.Fatal(err)
		}
		if g.Nodes["execute"].State != StateFailed {
			t.Fatalf("expected failed state, got %s", g.Nodes["execute"].State)
		}
	}
}
