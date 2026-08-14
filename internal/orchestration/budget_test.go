package orchestration

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/authority"
	"github.com/kingaiwork/KINGAIBOT/internal/workgraph"
)

func TestWorkGraphDispatchSharesAuthorityConcurrencyBudget(t *testing.T) {
	h := newBridgeHarness(t)
	owner := "agent:budgeted"
	grant, err := h.authorities.CreateRoot(authority.Envelope{
		SubjectID:         owner,
		Capabilities:      []string{"task.execute"},
		ToolScopes:        []string{"file.write"},
		MaxConcurrentWork: 1,
		MaxCostUnits:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = grant
	graph, err := h.graphs.Create("two independent remote writes", []workgraph.Node{
		{
			ID:     "first",
			Type:   workgraph.TypeExecute,
			Owner:  owner,
			Risk:   workgraph.RiskHigh,
			Replay: workgraph.ReplayManual,
			Inputs: map[string]any{"cluster": map[string]any{
				"kind":                  "file.write",
				"payload":               map[string]any{"path": "first.txt", "content": "one"},
				"required_capabilities": []string{"task.execute"},
				"required_tool":         "file.write",
				"cost_units":            2,
			}},
		},
		{
			ID:     "second",
			Type:   workgraph.TypeExecute,
			Owner:  owner,
			Risk:   workgraph.RiskHigh,
			Replay: workgraph.ReplayManual,
			Inputs: map[string]any{"cluster": map[string]any{
				"kind":                  "file.write",
				"payload":               map[string]any{"path": "second.txt", "content": "two"},
				"required_capabilities": []string{"task.execute"},
				"required_tool":         "file.write",
				"cost_units":            2,
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	graph, err = h.graphs.Refresh(graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	if graph.Nodes["first"].State != workgraph.StateReady || graph.Nodes["second"].State != workgraph.StateReady {
		t.Fatal("both independent nodes must be ready")
	}
	first, err := h.bridge.Dispatch(graph.ID, "first")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.bridge.Dispatch(graph.ID, "second"); err == nil {
		t.Fatal("second WorkGraph node unexpectedly bypassed authority concurrency budget")
	}
	stored, err := h.graphs.Get(graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Nodes["second"].State != workgraph.StateReady {
		t.Fatalf("budget-denied node must remain ready, got %s", stored.Nodes["second"].State)
	}

	issued, err := h.cluster.RegisterWorker("writer", []string{"task.execute"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := h.cluster.AuthenticateWorker(issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := h.cluster.LeaseJobAuthorized(worker, 30)
	if err != nil {
		t.Fatal(err)
	}
	if lease.Job.ID != first.JobID {
		t.Fatal("unexpected leased orchestration job")
	}
	if _, err := h.cluster.CompleteAuthorized(worker, lease.Job.ID, lease.LeaseToken, json.RawMessage(`{"ok":true}`), ""); err != nil {
		t.Fatal(err)
	}
	if err := h.bridge.Sync(); err != nil {
		t.Fatal(err)
	}
	if _, err := h.bridge.Dispatch(graph.ID, "second"); err != nil {
		t.Fatalf("second node must dispatch after first releases capacity: %v", err)
	}
}

func TestWorkGraphCostExhaustionFailsBeforeWorkerDelivery(t *testing.T) {
	h := newBridgeHarness(t)
	owner := "agent:cost-limited"
	if _, err := h.authorities.CreateRoot(authority.Envelope{
		SubjectID:         owner,
		Capabilities:      []string{"task.execute"},
		ToolScopes:        []string{"file.write"},
		MaxConcurrentWork: 1,
		MaxCostUnits:      3,
	}); err != nil {
		t.Fatal(err)
	}
	graph, err := h.graphs.Create("cost bounded remote write", []workgraph.Node{{
		ID:     "write",
		Type:   workgraph.TypeExecute,
		Owner:  owner,
		Risk:   workgraph.RiskHigh,
		Replay: workgraph.ReplayManual,
		Inputs: map[string]any{"cluster": map[string]any{
			"kind":                  "file.write",
			"payload":               map[string]any{"path": "expensive.txt", "content": "blocked"},
			"required_capabilities": []string{"task.execute"},
			"required_tool":         "file.write",
			"cost_units":            4,
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	graph, err = h.graphs.Refresh(graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := h.bridge.Dispatch(graph.ID, "write")
	if err != nil {
		t.Fatal(err)
	}
	issued, err := h.cluster.RegisterWorker("writer", []string{"task.execute"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := h.cluster.AuthenticateWorker(issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.cluster.LeaseJobAuthorized(worker, 30); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("worker must never receive over-budget orchestration lease; got %v", err)
	}
	job, err := h.cluster.Job(binding.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "failed" {
		t.Fatalf("over-budget remote job must fail closed, got %s", job.Status)
	}
	if err := h.bridge.Sync(); err != nil {
		t.Fatal(err)
	}
	stored, err := h.graphs.Get(graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Nodes["write"].State != workgraph.StateFailed {
		t.Fatalf("WorkGraph must reflect budget-blocked terminal job, got %s", stored.Nodes["write"].State)
	}
}
