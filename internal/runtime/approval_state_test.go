package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/approval"
	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

func runtimeForApprovalTest(t *testing.T) (*Runtime, *task.Store, *approval.Store, *eventlog.Log, string) {
	t.Helper()
	dir := t.TempDir()
	ts, err := task.NewStore(filepath.Join(dir, "tasks"))
	if err != nil {
		t.Fatal(err)
	}
	as, err := approval.New(filepath.Join(dir, "approvals"))
	if err != nil {
		t.Fatal(err)
	}
	eventDir := filepath.Join(dir, "events")
	el, err := eventlog.New(eventDir)
	if err != nil {
		t.Fatal(err)
	}
	r := &Runtime{tasks: ts, approvals: as, events: el, queue: make(chan string, 2)}
	return r, ts, as, el, eventDir
}

func seedWaitingApproval(t *testing.T, ts *task.Store, as *approval.Store) {
	t.Helper()
	if err := as.Save(&approval.Approval{ID: "appr_test", TaskID: "task_test", Tool: "file_write", Capability: "file_write", Arguments: []byte(`{"path":"a.txt","content":"x"}`), Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	if err := ts.Save(&task.Task{ID: "task_test", Input: "write file", Status: task.WaitingApproval, PendingApproval: "appr_test"}); err != nil {
		t.Fatal(err)
	}
}

func TestDeniedApprovalTerminatesWaitingTask(t *testing.T) {
	r, ts, as, _, _ := runtimeForApprovalTest(t)
	seedWaitingApproval(t, ts, as)
	if err := r.DecideApproval("appr_test", "denied"); err != nil {
		t.Fatal(err)
	}
	got, err := ts.Get("task_test")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != task.Failed || got.PendingApproval != "" || got.Error != "approval denied" {
		t.Fatalf("denied task left in unsafe state: %#v", got)
	}
}

func TestApprovalRollsBackWhenAuditIsUnhealthy(t *testing.T) {
	r, ts, as, el, eventDir := runtimeForApprovalTest(t)
	seedWaitingApproval(t, ts, as)
	if err := el.Append(eventlog.Event{Type: "seed"}); err != nil {
		t.Fatal(err)
	}
	// Tamper the audit log and verify it so the in-memory log enters a broken
	// state. Any trust-changing decision must now fail closed.
	if err := os.WriteFile(filepath.Join(eventDir, "events.jsonl"), []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := el.Verify(); err == nil {
		t.Fatal("tampered audit log unexpectedly verified")
	}
	if err := r.DecideApproval("appr_test", "approved"); err == nil {
		t.Fatal("approval unexpectedly succeeded with unhealthy audit")
	}
	a, err := as.Get("appr_test")
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != "pending" {
		t.Fatalf("approval was not rolled back to pending: %#v", a)
	}
	got, err := ts.Get("task_test")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != task.WaitingApproval || got.PendingApproval != "appr_test" {
		t.Fatalf("task changed despite failed approval audit: %#v", got)
	}
}
