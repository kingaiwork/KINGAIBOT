package runtime

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/kingaiwork/KINGAIBOT/internal/approval"
	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

const (
	approvalApprovingV14 = "approving"
	approvalDenyingV14   = "denying"
)

var approvalDecisionV14Gates sync.Map // map[*Runtime]*sync.Mutex

func (r *Runtime) approvalDecisionV14Lock() *sync.Mutex {
	value, _ := approvalDecisionV14Gates.LoadOrStore(r, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func approvalStageV14(decision string) string {
	if decision == "approved" {
		return approvalApprovingV14
	}
	return approvalDenyingV14
}

// DecideApprovalV14 makes approval trust directional and crash-safe:
//
//	pending -> approving/denying -> audit -> approved/denied -> task transition
//
// The staged states are intentionally non-executable. If the process or audit
// subsystem fails, the task remains WaitingApproval and the same decision can
// be resumed safely. An opposite decision cannot overtake an in-progress stage.
func (r *Runtime) DecideApprovalV14(id, decision string) error {
	if r == nil || r.approvals == nil || r.tasks == nil || r.events == nil {
		return errors.New("runtime approval subsystem is unavailable")
	}
	decision = strings.ToLower(strings.TrimSpace(decision))
	if decision != "approved" && decision != "denied" {
		return errors.New("status must be approved or denied")
	}
	stage := approvalStageV14(decision)

	gate := r.approvalDecisionV14Lock()
	gate.Lock()
	defer gate.Unlock()

	a, err := r.approvals.Get(id)
	if err != nil {
		return err
	}

	switch a.Status {
	case "pending":
		a, err = r.approvals.Update(id, func(cur *approval.Approval) error {
			if cur.Status != "pending" {
				return errors.New("approval state changed")
			}
			cur.Status = stage
			return nil
		})
		if err != nil {
			return err
		}
	case stage:
		// Resume the same decision after a crash/audit outage.
	case decision:
		// Final decision was already durably reached. Resume only the derived
		// Task transition, which is safe because V14 reaches this state only after
		// the decision audit was appended.
		return r.applyApprovalDecisionV14(a, decision)
	case approvalApprovingV14, approvalDenyingV14:
		return fmt.Errorf("approval is already staged as %s; opposite decision requires operator reconciliation", a.Status)
	default:
		return fmt.Errorf("approval already decided as %s", a.Status)
	}

	if err := r.events.Append(eventlog.Event{Type: "approval." + decision, TaskID: a.TaskID, Data: map[string]any{
		"approval_id":    a.ID,
		"tool":           a.Tool,
		"arguments_hash": a.ArgumentsHash,
		"staged_from":    stage,
	}}); err != nil {
		return fmt.Errorf("approval remains %s because decision audit failed: %w", stage, err)
	}

	a, err = r.approvals.Update(id, func(cur *approval.Approval) error {
		if cur.Status != stage {
			return fmt.Errorf("approval stage changed from %s to %s", stage, cur.Status)
		}
		cur.Status = decision
		return nil
	})
	if err != nil {
		return fmt.Errorf("approval decision was audited but final state persistence failed: %w", err)
	}
	return r.applyApprovalDecisionV14(a, decision)
}

func (r *Runtime) applyApprovalDecisionV14(a *approval.Approval, decision string) error {
	if a == nil {
		return errors.New("approval required")
	}
	if decision == "denied" {
		updated, err := r.tasks.Update(a.TaskID, func(t *task.Task) error {
			switch t.Status {
			case task.WaitingApproval:
				if t.PendingApproval != a.ID {
					return errors.New("task is waiting for another approval")
				}
				t.Status = task.Failed
				t.PendingApproval = ""
				t.Error = "approval denied"
				return nil
			case task.Failed:
				return nil
			case task.Queued, task.Running, task.Completing, task.Completed:
				// A denied approval paired with an already-executing/executed task is
				// inconsistent and potentially side-effecting. Preserve evidence and
				// force explicit reconciliation rather than hiding it as failure.
				t.Status = task.Reconciliation
				t.PendingApproval = ""
				t.Error = "approval denied but task had already progressed; reconciliation required"
				return nil
			case task.Reconciliation, task.Canceled:
				return nil
			default:
				return fmt.Errorf("task has incompatible approval state %s", t.Status)
			}
		})
		if err != nil {
			return err
		}
		if updated.Status == task.Reconciliation {
			_ = r.events.Append(eventlog.Event{Type: "task.reconciliation_required", TaskID: a.TaskID, Data: map[string]any{
				"reason":      "denied_approval_after_task_progress",
				"approval_id": a.ID,
			}})
		}
		return nil
	}

	shouldEnqueue := false
	_, err := r.tasks.Update(a.TaskID, func(t *task.Task) error {
		switch t.Status {
		case task.WaitingApproval:
			if t.PendingApproval != a.ID {
				return errors.New("task is waiting for another approval")
			}
			t.Status = task.Queued
			t.PendingApproval = ""
			t.Error = ""
			shouldEnqueue = true
			return nil
		case task.Queued, task.Running, task.Completing, task.Completed, task.Failed, task.Canceled, task.Reconciliation:
			// Idempotent recovery after the final approval was persisted. Do not
			// move already-progressed work backward or enqueue an already-queued Task twice.
			return nil
		default:
			return fmt.Errorf("task has incompatible approval state %s", t.Status)
		}
	})
	if err != nil {
		return err
	}
	if !shouldEnqueue {
		return nil
	}
	if !r.enqueue(a.TaskID) {
		// Queued is durable and Recover will safely enqueue it after restart. Do
		// not downgrade an audited approval because the in-memory queue is full.
		return ErrQueueUnavailable
	}
	return nil
}
