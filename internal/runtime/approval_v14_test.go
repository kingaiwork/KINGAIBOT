package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/approval"
	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

func newApprovalV14Runtime(t *testing.T) (*Runtime, *approval.Store, string) {
	t.Helper()
	root := t.TempDir()
	tasks, err := task.NewStore(filepath.Join(root, "tasks"))
	if err != nil {
		t.Fatal(err)
	}
	approvals, err := approval.New(filepath.Join(root, "approvals"))
	if err != nil {
		t.Fatal(err)
	}
	eventsDir := filepath.Join(root, "events")
	events, err := eventlog.New(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &Runtime{
		tasks:     tasks,
		approvals: approvals,
		events:    events,
		queue:     make(chan string, 8),
		ctx:       context.Background(),
	}
	return runtime, approvals, eventsDir
}

func seedApprovalV14(t *testing.T, r *Runtime, approvals *approval.Store, approvalStatus string) (*approval.Approval, *task.Task) {
	t.Helper()
	a := &approval.Approval{
		ID:            "appr_v14_test",
		TaskID:        "task_v14_approval",
		Tool:          "platform_test_tool",
		ArgumentsHash: "sha256-test",
		Status:        approvalStatus,
	}
	if err := approvals.Save(a); err != nil {
		t.Fatal(err)
	}
	candidate := &task.Task{
		ID:              a.TaskID,
		Input:           "approval gated task",
		Status:          task.WaitingApproval,
		PendingApproval: a.ID,
	}
	if err := r.tasks.Save(candidate); err != nil {
		t.Fatal(err)
	}
	return a, candidate
}

func TestDecideApprovalV14ApprovesOnlyAfterAudit(t *testing.T) {
	r, approvals, _ := newApprovalV14Runtime(t)
	a, _ := seedApprovalV14(t, r, approvals, "pending")

	if err := r.DecideApprovalV14(a.ID, "approved"); err != nil {
		t.Fatal(err)
	}
	storedApproval, err := approvals.Get(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedApproval.Status != "approved" {
		t.Fatalf("approval status=%s, want approved", storedApproval.Status)
	}
	storedTask, err := r.Task(a.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if storedTask.Status != task.Queued || storedTask.PendingApproval != "" {
		t.Fatalf("approved task did not become queued: %#v", storedTask)
	}
	if got := len(r.queue); got != 1 {
		t.Fatalf("approved task queue=%d, want 1", got)
	}
}

func TestDecideApprovalV14AuditFailureLeavesNonExecutableStage(t *testing.T) {
	r, approvals, eventsDir := newApprovalV14Runtime(t)
	a, _ := seedApprovalV14(t, r, approvals, "pending")

	eventsPath := filepath.Join(eventsDir, "events.jsonl")
	if err := os.RemoveAll(eventsPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(eventsPath, 0o700); err != nil {
		t.Fatal(err)
	}

	err := r.DecideApprovalV14(a.ID, "approved")
	if err == nil || !strings.Contains(err.Error(), "remains approving") {
		t.Fatalf("audit failure error=%v, want staged approving failure", err)
	}
	storedApproval, getErr := approvals.Get(a.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if storedApproval.Status != approvalApprovingV14 {
		t.Fatalf("audit failure approval status=%s, want %s", storedApproval.Status, approvalApprovingV14)
	}
	storedTask, getErr := r.Task(a.TaskID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if storedTask.Status != task.WaitingApproval || storedTask.PendingApproval != a.ID {
		t.Fatalf("audit failure expanded task execution state: %#v", storedTask)
	}
	if got := len(r.queue); got != 0 {
		t.Fatalf("audit failure enqueued task: %d", got)
	}
	if err := r.DecideApprovalV14(a.ID, "denied"); err == nil {
		t.Fatal("opposite decision overtook staged approval")
	}
}

func TestDecideApprovalV14ResumesMatchingStagedDecision(t *testing.T) {
	r, approvals, _ := newApprovalV14Runtime(t)
	a, _ := seedApprovalV14(t, r, approvals, approvalApprovingV14)

	if err := r.DecideApprovalV14(a.ID, "approved"); err != nil {
		t.Fatal(err)
	}
	storedApproval, err := approvals.Get(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedApproval.Status != "approved" {
		t.Fatalf("resumed approval status=%s, want approved", storedApproval.Status)
	}
	storedTask, err := r.Task(a.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if storedTask.Status != task.Queued {
		t.Fatalf("resumed approval task status=%s, want queued", storedTask.Status)
	}
}

func TestDecideApprovalV14ConcurrentOppositeDecisionsOnlyOneCommits(t *testing.T) {
	r, approvals, _ := newApprovalV14Runtime(t)
	a, _ := seedApprovalV14(t, r, approvals, "pending")

	var wg sync.WaitGroup
	wg.Add(2)
	results := make(chan error, 2)
	go func() {
		defer wg.Done()
		results <- r.DecideApprovalV14(a.ID, "approved")
	}()
	go func() {
		defer wg.Done()
		results <- r.DecideApprovalV14(a.ID, "denied")
	}()
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent approval decision successes=%d, want exactly 1", successes)
	}
	storedApproval, err := approvals.Get(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedApproval.Status != "approved" && storedApproval.Status != "denied" {
		t.Fatalf("unexpected final approval status: %s", storedApproval.Status)
	}
	storedTask, err := r.Task(a.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if storedApproval.Status == "approved" && storedTask.Status != task.Queued {
		t.Fatalf("approved decision left task in %s", storedTask.Status)
	}
	if storedApproval.Status == "denied" && storedTask.Status != task.Failed {
		t.Fatalf("denied decision left task in %s", storedTask.Status)
	}
}

func TestDecideApprovalV14DeniedAfterProgressForcesReconciliation(t *testing.T) {
	r, approvals, _ := newApprovalV14Runtime(t)
	a, _ := seedApprovalV14(t, r, approvals, "denied")
	if _, err := r.tasks.Update(a.TaskID, func(candidate *task.Task) error {
		candidate.Status = task.Running
		candidate.PendingApproval = ""
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := r.DecideApprovalV14(a.ID, "denied"); err != nil {
		t.Fatal(err)
	}
	stored, err := r.Task(a.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != task.Reconciliation {
		t.Fatalf("denied approval with progressed task status=%s, want reconciliation", stored.Status)
	}
}
