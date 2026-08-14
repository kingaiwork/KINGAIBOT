package runtime

import (
	"sync"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

func saveReconciliationTask(t *testing.T, r *Runtime, candidate *task.Task) {
	t.Helper()
	if err := r.tasks.Save(candidate); err != nil {
		t.Fatal(err)
	}
}

func TestResolveReconciliationAcceptCompletedRequiresDurableOutput(t *testing.T) {
	r := bareRuntimeForCreateTest(t, 2)
	candidate := &task.Task{ID: "task_reconcile_accept", Input: "done externally", Output: "verified output", Provider: "provider-a", Status: task.Reconciliation, Attempts: 1}
	saveReconciliationTask(t, r, candidate)

	resolved, err := r.ResolveReconciliation(candidate.ID, "accept_completed", "verified provider receipt", false)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != task.Completed || resolved.Output != candidate.Output || resolved.Provider != candidate.Provider {
		t.Fatalf("unexpected accepted task: %#v", resolved)
	}
}

func TestResolveReconciliationAcceptCompletedRejectsMissingOutput(t *testing.T) {
	r := bareRuntimeForCreateTest(t, 2)
	candidate := &task.Task{ID: "task_reconcile_no_output", Input: "ambiguous", Status: task.Reconciliation, Attempts: 1}
	saveReconciliationTask(t, r, candidate)

	if _, err := r.ResolveReconciliation(candidate.ID, "accept_completed", "no output exists", false); err == nil {
		t.Fatal("expected accept_completed without durable output to fail")
	}
	stored, err := r.Task(candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != task.Reconciliation {
		t.Fatalf("failed acceptance changed status to %s", stored.Status)
	}
}

func TestResolveReconciliationMarkFailedIsTerminal(t *testing.T) {
	r := bareRuntimeForCreateTest(t, 2)
	candidate := &task.Task{ID: "task_reconcile_fail", Input: "ambiguous", Status: task.Reconciliation, Attempts: 1}
	saveReconciliationTask(t, r, candidate)

	resolved, err := r.ResolveReconciliation(candidate.ID, "mark_failed", "operator verified task must not continue", false)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != task.Failed {
		t.Fatalf("status=%s, want failed", resolved.Status)
	}
}

func TestResolveReconciliationRetryRequiresExplicitReplayAndNoOutput(t *testing.T) {
	r := bareRuntimeForCreateTest(t, 2)
	candidate := &task.Task{ID: "task_reconcile_retry", Input: "safe to retry after review", Status: task.Reconciliation, Attempts: 1}
	saveReconciliationTask(t, r, candidate)

	if _, err := r.ResolveReconciliation(candidate.ID, "retry", "reviewed", false); err == nil {
		t.Fatal("retry without allow_replay should fail")
	}
	if got := len(r.queue); got != 0 {
		t.Fatalf("unauthorized retry enqueued work: %d", got)
	}

	resolved, err := r.ResolveReconciliation(candidate.ID, "retry", "confirmed no external side effect occurred", true)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != task.Queued {
		t.Fatalf("retry status=%s, want queued", resolved.Status)
	}
	if got := len(r.queue); got != 1 {
		t.Fatalf("authorized retry queue=%d, want 1", got)
	}
}

func TestResolveReconciliationRetryRejectsDurableOutput(t *testing.T) {
	r := bareRuntimeForCreateTest(t, 2)
	candidate := &task.Task{ID: "task_reconcile_output_retry", Input: "already has result", Output: "evidence", Status: task.Reconciliation, Attempts: 1}
	saveReconciliationTask(t, r, candidate)

	if _, err := r.ResolveReconciliation(candidate.ID, "retry", "try again", true); err == nil {
		t.Fatal("retry with durable output should fail")
	}
	if got := len(r.queue); got != 0 {
		t.Fatalf("output-bearing reconciliation task was enqueued: %d", got)
	}
}

func TestResolveReconciliationConcurrentDecisionsOnlyOneCommits(t *testing.T) {
	r := bareRuntimeForCreateTest(t, 2)
	candidate := &task.Task{ID: "task_reconcile_race", Input: "ambiguous", Output: "verified output", Status: task.Reconciliation, Attempts: 1}
	saveReconciliationTask(t, r, candidate)

	var wg sync.WaitGroup
	wg.Add(2)
	results := make(chan error, 2)
	go func() {
		defer wg.Done()
		_, err := r.ResolveReconciliation(candidate.ID, "accept_completed", "reviewer accepted evidence", false)
		results <- err
	}()
	go func() {
		defer wg.Done()
		_, err := r.ResolveReconciliation(candidate.ID, "mark_failed", "reviewer rejected execution", false)
		results <- err
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
		t.Fatalf("concurrent reconciliation successes=%d, want exactly 1", successes)
	}
	stored, err := r.Task(candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != task.Completed && stored.Status != task.Failed {
		t.Fatalf("unexpected terminal status after reconciliation race: %s", stored.Status)
	}
}
