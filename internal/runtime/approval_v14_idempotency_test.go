package runtime

import "testing"

func TestDecideApprovalV14RepeatedApprovedDecisionDoesNotDuplicateQueue(t *testing.T) {
	r, approvals, _ := newApprovalV14Runtime(t)
	a, _ := seedApprovalV14(t, r, approvals, "pending")

	if err := r.DecideApprovalV14(a.ID, "approved"); err != nil {
		t.Fatal(err)
	}
	if got := len(r.queue); got != 1 {
		t.Fatalf("first approved decision queue=%d, want 1", got)
	}
	if err := r.DecideApprovalV14(a.ID, "approved"); err != nil {
		t.Fatal(err)
	}
	if got := len(r.queue); got != 1 {
		t.Fatalf("repeated approved decision duplicated queued work: queue=%d want=1", got)
	}
}
