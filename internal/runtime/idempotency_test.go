package runtime

import (
	"errors"
	"strings"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

func TestRecoverIgnoresPendingAuditTask(t *testing.T) {
	r := bareRuntimeForCreateTest(t, 4)
	pending := &task.Task{ID: "task_pending_test", Input: "must not run", Status: task.PendingAudit}
	if err := r.tasks.Save(pending); err != nil {
		t.Fatal(err)
	}
	if err := r.Recover(); err != nil {
		t.Fatal(err)
	}
	if got := len(r.queue); got != 0 {
		t.Fatalf("pending-audit task was enqueued during recovery: queue=%d", got)
	}
	stored, err := r.Task(pending.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != task.PendingAudit {
		t.Fatalf("pending-audit task changed status during recovery: %s", stored.Status)
	}
}

func TestRecoverMovesInterruptedRunningTaskToReconciliationWithoutReplay(t *testing.T) {
	r := bareRuntimeForCreateTest(t, 4)
	interrupted := &task.Task{ID: "task_running_restart", Input: "external side effect may have happened", Status: task.Running, Attempts: 1}
	if err := r.tasks.Save(interrupted); err != nil {
		t.Fatal(err)
	}
	if err := r.Recover(); err != nil {
		t.Fatal(err)
	}
	if got := len(r.queue); got != 0 {
		t.Fatalf("interrupted running task was blindly replayed: queue=%d", got)
	}
	stored, err := r.Task(interrupted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != task.Reconciliation {
		t.Fatalf("interrupted running task status=%s, want reconciliation", stored.Status)
	}
	if !strings.Contains(stored.Error, "external side effects may be ambiguous") {
		t.Fatalf("reconciliation reason missing ambiguity evidence: %q", stored.Error)
	}
}

func TestRecoverMovesInterruptedCompletingTaskToReconciliationWithoutReplay(t *testing.T) {
	r := bareRuntimeForCreateTest(t, 4)
	interrupted := &task.Task{
		ID:       "task_completing_restart",
		Input:    "already produced output",
		Output:   "durable output",
		Provider: "test-provider",
		Status:   task.Completing,
		Attempts: 1,
	}
	if err := r.tasks.Save(interrupted); err != nil {
		t.Fatal(err)
	}
	if err := r.Recover(); err != nil {
		t.Fatal(err)
	}
	if got := len(r.queue); got != 0 {
		t.Fatalf("interrupted completing task was replayed: queue=%d", got)
	}
	stored, err := r.Task(interrupted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != task.Reconciliation {
		t.Fatalf("interrupted completing task status=%s, want reconciliation", stored.Status)
	}
	if stored.Output != interrupted.Output || stored.Provider != interrupted.Provider {
		t.Fatalf("completion evidence was lost during reconciliation: %#v", stored)
	}
	if !strings.Contains(stored.Error, "completion evidence is ambiguous") {
		t.Fatalf("completion reconciliation reason missing: %q", stored.Error)
	}
}

func TestRecoverRequeuesQueuedTask(t *testing.T) {
	r := bareRuntimeForCreateTest(t, 4)
	queued := &task.Task{ID: "task_queued_restart", Input: "not yet claimed", Status: task.Queued, Error: "stale", PendingApproval: "stale"}
	if err := r.tasks.Save(queued); err != nil {
		t.Fatal(err)
	}
	if err := r.Recover(); err != nil {
		t.Fatal(err)
	}
	if got := len(r.queue); got != 1 {
		t.Fatalf("queued task recovery queue=%d, want 1", got)
	}
	select {
	case id := <-r.queue:
		if id != queued.ID {
			t.Fatalf("recovered wrong task id=%s want=%s", id, queued.ID)
		}
	default:
		t.Fatal("queued task was not re-enqueued")
	}
	stored, err := r.Task(queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != task.Queued || stored.Error != "" || stored.PendingApproval != "" {
		t.Fatalf("queued recovery did not normalize transient fields: %#v", stored)
	}
}

func TestCreateIdempotentReturnsExistingTaskWithoutDuplicateQueue(t *testing.T) {
	r := bareRuntimeForCreateTest(t, 4)
	first, err := r.CreateIdempotent("one durable action", map[string]any{"source": "workflow"}, "wf_run_1:step_1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.CreateIdempotent("one durable action", map[string]any{"source": "workflow"}, "wf_run_1:step_1")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("same idempotency key produced different tasks: %s != %s", first.ID, second.ID)
	}
	all, err := r.Tasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("idempotent create persisted %d tasks, want 1", len(all))
	}
	if got := len(r.queue); got != 1 {
		t.Fatalf("idempotent repeat enqueued duplicate work: queue=%d want=1", got)
	}
}

func TestCreateIdempotentRejectsSameKeyWithDifferentInput(t *testing.T) {
	r := bareRuntimeForCreateTest(t, 4)
	if _, err := r.CreateIdempotent("first payload", nil, "stable-key"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CreateIdempotent("different payload", nil, "stable-key"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting input error=%v, want ErrIdempotencyConflict", err)
	}
	all, err := r.Tasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("conflicting idempotent create persisted %d tasks, want 1", len(all))
	}
}
