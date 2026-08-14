package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

// Implement the optional production idempotency capability on callbackRuntime
// for v1.4 workflow tests. A stable key maps to one deterministic fake Task.
func (f *callbackRuntime) CreateIdempotent(input string, meta map[string]any, key string) (*task.Task, error) {
	h := sha256.Sum256([]byte(key))
	id := "task_idem_" + hex.EncodeToString(h[:16])
	f.mu.Lock()
	defer f.mu.Unlock()
	if existing, ok := f.tasks[id]; ok {
		copy := *existing
		return &copy, nil
	}
	f.next++
	n := time.Now().UTC()
	created := &task.Task{
		ID:        id,
		Input:     input,
		Output:    "done:" + input,
		Status:    task.Completed,
		Metadata:  meta,
		CreatedAt: n,
		UpdatedAt: n,
	}
	f.tasks[id] = created
	copy := *created
	return &copy, nil
}

func waitWorkflowStatusV14(t *testing.T, m *Manager, runID, want string) *WorkflowRun {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := m.WorkflowRuns()
		if err != nil {
			t.Fatal(err)
		}
		for _, run := range runs {
			if run.ID == runID && run.Status == want {
				return run
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("workflow %s did not reach %s", runID, want)
	return nil
}

func TestWorkflowV14RunsWithStableStepTaskIdentity(t *testing.T) {
	dir := t.TempDir()
	runtime := newCallbackRuntime()
	m := newManagerWithRuntimeForV14Test(t, dir, runtime)
	workflow, err := m.CreateWorkflowSafe(Workflow{Name: "v14-one-step", Steps: []WorkflowStep{{Prompt: "one"}}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := m.RunWorkflowV14(workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	finished := waitWorkflowStatusV14(t, m, run.ID, "completed")
	if len(finished.TaskIDs) != 1 || len(finished.Outputs) != 1 {
		t.Fatalf("unexpected completed run: %#v", finished)
	}
	if runtime.next != 1 {
		t.Fatalf("workflow created %d runtime tasks, want 1", runtime.next)
	}
}

func TestWorkflowV14RecoveryReusesTaskCreatedBeforeRunLinkPersisted(t *testing.T) {
	dir := t.TempDir()
	runtime := newCallbackRuntime()
	m := newManagerWithRuntimeForV14Test(t, dir, runtime)
	workflow, err := m.CreateWorkflowSafe(Workflow{Name: "recover-one-step", Steps: []WorkflowStep{{Prompt: "one"}}})
	if err != nil {
		t.Fatal(err)
	}
	runID := "wfrun_recovery_test"
	run := &WorkflowRun{ID: runID, WorkflowID: workflow.ID, Status: workflowRunStatusV14, CreatedAt: now(), UpdatedAt: now()}
	m.mu.Lock()
	if err := m.save("workflow-runs", runID, run); err != nil {
		m.mu.Unlock()
		t.Fatal(err)
	}
	m.mu.Unlock()

	step := workflow.Steps[0]
	key := workflowStepIdempotencyKey(runID, 0, step.ID)
	preexisting, err := runtime.CreateIdempotent(step.Prompt, map[string]any{
		"source":            "workflow_v14",
		"workflow_id":       workflow.ID,
		"workflow_run_id":   runID,
		"workflow_step":     step.ID,
		"workflow_step_idx": 0,
		"agent_id":          "",
	}, key)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.next != 1 {
		t.Fatalf("setup created %d tasks, want 1", runtime.next)
	}

	// Simulate restart after Runtime task creation but before CurrentTaskID was
	// persisted. Recovery must call CreateIdempotent and get the same Task.
	m.RecoverWorkflowRunsV14()
	finished := waitWorkflowStatusV14(t, m, runID, "completed")
	if runtime.next != 1 {
		t.Fatalf("recovery created a duplicate runtime task: count=%d", runtime.next)
	}
	if len(finished.TaskIDs) != 1 || finished.TaskIDs[0] != preexisting.ID {
		t.Fatalf("recovery did not link existing task %s: %#v", preexisting.ID, finished.TaskIDs)
	}
}

func newManagerWithRuntimeForV14Test(t *testing.T, dir string, runtime TaskRuntime) *Manager {
	t.Helper()
	m := newManagerAtDirForV14Test(t, dir, runtime)
	return m
}

func newManagerAtDirForV14Test(t *testing.T, dir string, runtime TaskRuntime) *Manager {
	t.Helper()
	events, err := newEventLogForV14Test(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(dir+"/platform", runtime, events)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m
}

func newEventLogForV14Test(dir string) (*eventlog.Log, error) {
	return eventlog.New(fmt.Sprintf("%s/events", dir))
}
