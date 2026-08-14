package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/memory"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

var runtimeReconciliationGates sync.Map // map[*Runtime]*sync.Mutex

func (r *Runtime) reconciliationLock() *sync.Mutex {
	value, _ := runtimeReconciliationGates.LoadOrStore(r, &sync.Mutex{})
	return value.(*sync.Mutex)
}

// ResolveReconciliation is the only supported operator path out of the Runtime
// reconciliation state. It never silently replays ambiguous work.
//
// Decisions:
//   - accept_completed: requires durable output and audit-before-completed.
//   - mark_failed: fail-closed state-first; audit failure never restores work.
//   - retry: requires allowReplay=true, no durable output, and an exact audit
//     before the Task becomes queued again.
func (r *Runtime) ResolveReconciliation(id, decision, note string, allowReplay bool) (*task.Task, error) {
	if r == nil || r.tasks == nil || r.events == nil {
		return nil, errors.New("runtime reconciliation is unavailable")
	}
	decision = strings.ToLower(strings.TrimSpace(decision))
	note = strings.TrimSpace(memory.SanitizeContent(note))
	if note == "" {
		return nil, errors.New("reconciliation note required")
	}
	if len(note) > 4096 {
		return nil, errors.New("reconciliation note exceeds limit")
	}
	if decision != "accept_completed" && decision != "mark_failed" && decision != "retry" {
		return nil, errors.New("decision must be accept_completed, mark_failed, or retry")
	}

	gate := r.reconciliationLock()
	gate.Lock()
	defer gate.Unlock()

	current, err := r.tasks.Get(id)
	if err != nil {
		return nil, err
	}
	if current.Status != task.Reconciliation {
		return nil, errors.New("task is not in reconciliation")
	}

	switch decision {
	case "accept_completed":
		if strings.TrimSpace(current.Output) == "" {
			return nil, errors.New("accept_completed requires durable task output")
		}
		outputHash := sha256.Sum256([]byte(current.Output))
		if err := r.events.Append(eventlog.Event{Type: "task.reconciliation.accept_completed", TaskID: id, Data: map[string]any{
			"note":          note,
			"provider":      current.Provider,
			"output_sha256": hex.EncodeToString(outputHash[:]),
		}}); err != nil {
			return nil, fmt.Errorf("task remains in reconciliation because acceptance audit failed: %w", err)
		}
		updated, err := r.tasks.Update(id, func(t *task.Task) error {
			if t.Status != task.Reconciliation {
				return errors.New("task reconciliation state changed")
			}
			if strings.TrimSpace(t.Output) == "" {
				return errors.New("durable task output disappeared")
			}
			t.Status = task.Completed
			t.Error = ""
			t.PendingApproval = ""
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("completion acceptance was audited but task persistence failed: %w", err)
		}
		if r.cfg != nil && r.cfg.Memory.Enabled && r.memory != nil && r.cfg.Memory.StoreTaskOutputs {
			_ = r.memory.Add(memory.Record{Kind: "episodic", Content: updated.Output, Source: "task:" + id, Importance: 0.5, Confidence: 0.8})
		}
		return updated, nil

	case "mark_failed":
		updated, err := r.tasks.Update(id, func(t *task.Task) error {
			if t.Status != task.Reconciliation {
				return errors.New("task reconciliation state changed")
			}
			t.Status = task.Failed
			t.PendingApproval = ""
			t.Error = "operator reconciliation: " + note
			return nil
		})
		if err != nil {
			return nil, err
		}
		if err := r.events.Append(eventlog.Event{Type: "task.reconciliation.mark_failed", TaskID: id, Data: map[string]any{"note": note}}); err != nil {
			return nil, fmt.Errorf("task remains failed but reconciliation audit failed: %w", err)
		}
		return updated, nil

	case "retry":
		if !allowReplay {
			return nil, errors.New("retry requires allow_replay=true after operator side-effect review")
		}
		if strings.TrimSpace(current.Output) != "" {
			return nil, errors.New("retry is blocked while durable output exists; accept_completed or mark_failed")
		}
		if err := r.events.Append(eventlog.Event{Type: "task.reconciliation.retry_authorized", TaskID: id, Data: map[string]any{
			"note":         note,
			"allow_replay": true,
			"attempts":     current.Attempts,
		}}); err != nil {
			return nil, fmt.Errorf("task remains in reconciliation because retry audit failed: %w", err)
		}
		updated, err := r.tasks.Update(id, func(t *task.Task) error {
			if t.Status != task.Reconciliation {
				return errors.New("task reconciliation state changed")
			}
			if strings.TrimSpace(t.Output) != "" {
				return errors.New("durable output appeared; retry blocked")
			}
			t.Status = task.Queued
			t.Error = ""
			t.Provider = ""
			t.PendingApproval = ""
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("retry was audited but queued-state persistence failed: %w", err)
		}
		if !r.enqueueBlocking(id) {
			return nil, errors.New("runtime stopped after retry was authorized; queued task remains recoverable")
		}
		return updated, nil
	}

	return nil, errors.New("unreachable reconciliation decision")
}
