package platform

import (
	"fmt"
	"testing"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

type statusRuntime struct {
	tasks map[string]*task.Task
	next  int
}

func newStatusRuntime() *statusRuntime {
	return &statusRuntime{tasks: map[string]*task.Task{}}
}

func (r *statusRuntime) Create(input string, meta map[string]any) (*task.Task, error) {
	r.next++
	id := fmt.Sprintf("task_status_%d", r.next)
	n := time.Now().UTC()
	created := &task.Task{ID: id, Input: input, Status: task.Queued, Metadata: meta, CreatedAt: n, UpdatedAt: n}
	r.tasks[id] = created
	copy := *created
	return &copy, nil
}

func (r *statusRuntime) Task(id string) (*task.Task, error) {
	found, ok := r.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task not found")
	}
	copy := *found
	return &copy, nil
}

func (r *statusRuntime) Tasks() ([]*task.Task, error) {
	out := make([]*task.Task, 0, len(r.tasks))
	for _, found := range r.tasks {
		copy := *found
		out = append(out, &copy)
	}
	return out, nil
}

func newStatusManager(t *testing.T, runtime TaskRuntime) *Manager {
	t.Helper()
	dir := t.TempDir()
	events, err := eventlog.New(dir + "/events")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := New(dir+"/platform", runtime, events)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	return manager
}

func TestStatusSnapshotDoesNotPromoteNodeWithoutAuditedHeartbeat(t *testing.T) {
	m := newStatusManager(t, newStatusRuntime())
	node, err := m.CreateNodeSafe(Node{Name: "registered-only"})
	if err != nil {
		t.Fatal(err)
	}
	if node.Online {
		t.Fatal("new safe node unexpectedly online before heartbeat")
	}

	status, err := m.StatusSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if status.Counts["nodes_online"] != 0 {
		t.Fatalf("status read promoted unaudited node: %#v", status.Counts)
	}
	nodes, err := m.NodesSafe()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, stored := range nodes {
		if stored.ID != node.ID {
			continue
		}
		found = true
		if stored.Online {
			t.Fatal("StatusSnapshot changed persisted node trust to online")
		}
	}
	if !found {
		t.Fatalf("registered node %s disappeared from safe listing", node.ID)
	}
}

func TestStatusSnapshotSurfacesRuntimeReconciliationAttention(t *testing.T) {
	runtime := newStatusRuntime()
	n := time.Now().UTC()
	runtime.tasks["task_attention"] = &task.Task{
		ID:        "task_attention",
		Input:     "ambiguous side effect",
		Output:    "possible result",
		Status:    task.Reconciliation,
		CreatedAt: n,
		UpdatedAt: n,
	}
	m := newStatusManager(t, runtime)

	status, err := m.StatusSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !status.AttentionRequired {
		t.Fatal("runtime reconciliation did not set attention_required")
	}
	if status.Counts["runtime_tasks_reconciliation"] != 1 {
		t.Fatalf("runtime reconciliation count=%d, want 1", status.Counts["runtime_tasks_reconciliation"])
	}
	if status.TaskStatuses[string(task.Reconciliation)] != 1 {
		t.Fatalf("task status map missing reconciliation: %#v", status.TaskStatuses)
	}
}
