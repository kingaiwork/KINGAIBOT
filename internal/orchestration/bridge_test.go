package orchestration

import (
	"encoding/json"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/authority"
	"github.com/kingaiwork/KINGAIBOT/internal/cluster"
	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/workgraph"
)

type bridgeHarness struct {
	bridge      *Bridge
	graphs      *workgraph.Store
	cluster     *cluster.Coordinator
	authorities *authority.Store
}

func newBridgeHarness(t *testing.T) *bridgeHarness {
	t.Helper()
	dir := t.TempDir()
	events, err := eventlog.New(dir + "/events")
	if err != nil {
		t.Fatal(err)
	}
	graphs, err := workgraph.NewStore(dir+"/workgraphs", events)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := cluster.New(dir+"/cluster", events)
	if err != nil {
		t.Fatal(err)
	}
	authorities, err := authority.NewStore(dir+"/authority", events)
	if err != nil {
		t.Fatal(err)
	}
	if err := coordinator.SetAuthorityChecker(authorities); err != nil {
		t.Fatal(err)
	}
	bridge, err := New(dir+"/orchestration", graphs, coordinator, authorities, events)
	if err != nil {
		t.Fatal(err)
	}
	// Tests drive synchronization explicitly to make state transitions fully
	// deterministic; Close stops only the background ticker, not bridge methods.
	bridge.Close()
	return &bridgeHarness{bridge: bridge, graphs: graphs, cluster: coordinator, authorities: authorities}
}

func (h *bridgeHarness) createExecutableGraph(t *testing.T, owner string) *workgraph.Graph {
	t.Helper()
	graph, err := h.graphs.Create("write a verified remote artifact", []workgraph.Node{{
		ID:     "write",
		Type:   workgraph.TypeExecute,
		Owner:  owner,
		Risk:   workgraph.RiskHigh,
		Replay: workgraph.ReplayManual,
		Inputs: map[string]any{"cluster": map[string]any{
			"kind":                  "file.write",
			"payload":               map[string]any{"path": "report.txt", "content": "ok"},
			"required_capabilities": []string{"task.execute"},
			"required_tool":         "file.write",
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	graph, err = h.graphs.Refresh(graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	if graph.Nodes["write"].State != workgraph.StateReady {
		t.Fatalf("expected ready node, got %s", graph.Nodes["write"].State)
	}
	return graph
}

func (h *bridgeHarness) createAuthority(t *testing.T, owner string) *authority.Grant {
	t.Helper()
	grant, err := h.authorities.CreateRoot(authority.Envelope{
		SubjectID:    owner,
		Capabilities: []string{"task.execute"},
		ToolScopes:   []string{"file.write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return grant
}

func TestDispatchCompletesGraphWithClusterEvidence(t *testing.T) {
	h := newBridgeHarness(t)
	owner := "agent_alpha"
	h.createAuthority(t, owner)
	graph := h.createExecutableGraph(t, owner)

	binding, err := h.bridge.Dispatch(graph.ID, "write")
	if err != nil {
		t.Fatal(err)
	}
	if binding.State != bindingActive {
		t.Fatalf("expected active binding, got %s", binding.State)
	}
	storedGraph, err := h.graphs.Get(graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedGraph.Nodes["write"].State != workgraph.StateRunning {
		t.Fatalf("expected running graph node, got %s", storedGraph.Nodes["write"].State)
	}
	job, err := h.cluster.Job(binding.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "queued" {
		t.Fatalf("expected activated queued job, got %s", job.Status)
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
	if _, err := h.cluster.CompleteAuthorized(worker, job.ID, lease.LeaseToken, json.RawMessage(`{"remote_id":"abc123"}`), ""); err != nil {
		t.Fatal(err)
	}
	if err := h.bridge.Sync(); err != nil {
		t.Fatal(err)
	}

	storedGraph, err = h.graphs.Get(graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	node := storedGraph.Nodes["write"]
	if node.State != workgraph.StateCompleted {
		t.Fatalf("expected completed graph node, got %s", node.State)
	}
	if len(node.Evidence) != 1 || node.Evidence[0].Kind != "cluster_job" || node.Evidence[0].Reference != job.ID || node.Evidence[0].SHA256 == "" {
		t.Fatalf("missing cluster completion evidence: %#v", node.Evidence)
	}
	binding, err = h.bridge.Binding(graph.ID, "write")
	if err != nil {
		t.Fatal(err)
	}
	if binding.State != bindingCompleted {
		t.Fatalf("expected completed binding, got %s", binding.State)
	}
}

func TestMidFlightAuthorityRevocationMovesGraphToReconciliation(t *testing.T) {
	h := newBridgeHarness(t)
	owner := "agent_beta"
	grant := h.createAuthority(t, owner)
	graph := h.createExecutableGraph(t, owner)
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
	lease, err := h.cluster.LeaseJobAuthorized(worker, 30)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.authorities.Revoke(grant.Envelope.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.cluster.CompleteAuthorized(worker, binding.JobID, lease.LeaseToken, json.RawMessage(`{"remote_id":"uncertain"}`), ""); err != nil {
		t.Fatal(err)
	}
	if err := h.bridge.Sync(); err != nil {
		t.Fatal(err)
	}
	storedGraph, err := h.graphs.Get(graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedGraph.Nodes["write"].State != workgraph.StateReconciling {
		t.Fatalf("expected graph reconciliation after authority loss, got %s", storedGraph.Nodes["write"].State)
	}

	// Human/admin reconciliation may accept already-observed external evidence
	// even though the original execution authority is now revoked.
	if _, err := h.cluster.ReconcileAuthorized(binding.JobID, "complete", "verified remote state", json.RawMessage(`{"remote_id":"uncertain","verified":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := h.bridge.Sync(); err != nil {
		t.Fatal(err)
	}
	storedGraph, err = h.graphs.Get(graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedGraph.Nodes["write"].State != workgraph.StateCompleted {
		t.Fatalf("expected graph completion after admin reconciliation, got %s", storedGraph.Nodes["write"].State)
	}
}

func TestDispatchWithoutTrustedOwnerAuthorityFailsBeforeGraphStart(t *testing.T) {
	h := newBridgeHarness(t)
	graph := h.createExecutableGraph(t, "agent_unprivileged")
	if _, err := h.bridge.Dispatch(graph.ID, "write"); err == nil {
		t.Fatal("unprivileged workgraph owner unexpectedly dispatched cluster job")
	}
	storedGraph, err := h.graphs.Get(graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedGraph.Nodes["write"].State != workgraph.StateReady {
		t.Fatalf("failed pre-authority dispatch changed graph state to %s", storedGraph.Nodes["write"].State)
	}
}

func TestRevokedAuthorityCannotRequeueReconciledJob(t *testing.T) {
	h := newBridgeHarness(t)
	owner := "agent_gamma"
	grant := h.createAuthority(t, owner)
	graph := h.createExecutableGraph(t, owner)
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
	lease, err := h.cluster.LeaseJobAuthorized(worker, 30)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.authorities.Revoke(grant.Envelope.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.cluster.CompleteAuthorized(worker, binding.JobID, lease.LeaseToken, json.RawMessage(`{"maybe":true}`), ""); err != nil {
		t.Fatal(err)
	}
	if err := h.bridge.Sync(); err != nil {
		t.Fatal(err)
	}
	if _, err := h.cluster.ReconcileAuthorized(binding.JobID, "requeue", "operator requested retry", nil); err == nil {
		t.Fatal("revoked execution authority unexpectedly allowed reconciliation requeue")
	}
	storedGraph, err := h.graphs.Get(graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedGraph.Nodes["write"].State != workgraph.StateReconciling {
		t.Fatalf("blocked requeue changed graph state to %s", storedGraph.Nodes["write"].State)
	}
}
