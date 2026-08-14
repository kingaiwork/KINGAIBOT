package runtime

import (
	"errors"
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
