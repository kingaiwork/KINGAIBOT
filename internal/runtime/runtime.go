package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/agent"
	"github.com/kingaiwork/KINGAIBOT/internal/approval"
	"github.com/kingaiwork/KINGAIBOT/internal/config"
	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/evolution"
	"github.com/kingaiwork/KINGAIBOT/internal/memory"
	"github.com/kingaiwork/KINGAIBOT/internal/storage"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
	"github.com/kingaiwork/KINGAIBOT/internal/tool"
)

var (
	ErrInvalidTaskInput = errors.New("invalid task input")
	ErrQueueUnavailable = errors.New("runtime queue unavailable")
)

type Runtime struct {
	tasks      *task.Store
	approvals  *approval.Store
	events     *eventlog.Log
	memory     *memory.Store
	agent      *agent.Engine
	evolution  *evolution.Store
	cfg        *config.Config
	queue      chan string
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.Mutex
	running    map[string]context.CancelFunc
	processing map[string]bool
}

func New(ts *task.Store, as *approval.Store, el *eventlog.Log, ms *memory.Store, ae *agent.Engine, es *evolution.Store, cfg *config.Config) *Runtime {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runtime{tasks: ts, approvals: as, events: el, memory: ms, agent: ae, evolution: es, cfg: cfg, queue: make(chan string, cfg.Runtime.QueueCapacity), ctx: ctx, cancel: cancel, running: map[string]context.CancelFunc{}, processing: map[string]bool{}}
	for i := 0; i < cfg.Runtime.WorkerCount; i++ {
		r.wg.Add(1)
		go r.worker()
	}
	r.wg.Add(1)
	go r.integrityLoop()
	return r
}

func (r *Runtime) Recover() error {
	ts, err := r.tasks.Recoverable()
	if err != nil {
		return err
	}
	for _, t := range ts {
		_, er := r.tasks.Update(t.ID, func(x *task.Task) error {
			if x.Status == task.Running || x.Status == task.Queued {
				x.Status = task.Queued
				x.Error = ""
				x.PendingApproval = ""
			}
			return nil
		})
		if er != nil {
			return er
		}
		if !r.enqueueBlocking(t.ID) {
			return errors.New("runtime stopped during recovery")
		}
	}
	return nil
}

func (r *Runtime) Close() {
	r.cancel()
	r.mu.Lock()
	for _, cancel := range r.running {
		cancel()
	}
	r.mu.Unlock()
	r.wg.Wait()
}

func (r *Runtime) Create(input string, meta map[string]any) (*task.Task, error) {
	if input == "" {
		return nil, fmt.Errorf("%w: input required", ErrInvalidTaskInput)
	}
	if len(input) > int(r.cfg.Runtime.MaxRequestBytes) {
		return nil, fmt.Errorf("%w: input exceeds configured limit", ErrInvalidTaskInput)
	}
	id, err := storage.RandomID("task")
	if err != nil {
		return nil, err
	}
	t := &task.Task{ID: id, Input: input, Status: task.Queued, Metadata: meta}
	if err := r.tasks.Save(t); err != nil {
		return nil, err
	}
	if err := r.events.Append(eventlog.Event{Type: "task.created", TaskID: t.ID}); err != nil {
		_, _ = r.tasks.Update(t.ID, func(x *task.Task) error {
			x.Status = task.Failed
			x.Error = "audit log unavailable; task was not scheduled"
			return nil
		})
		return nil, err
	}
	if !r.enqueue(t.ID) {
		_, _ = r.tasks.Update(t.ID, func(x *task.Task) error {
			x.Status = task.Failed
			x.Error = "runtime queue unavailable"
			return nil
		})
		_ = r.events.Append(eventlog.Event{Type: "task.rejected", TaskID: t.ID, Data: map[string]any{"reason": "queue_unavailable"}})
		return nil, ErrQueueUnavailable
	}
	return t, nil
}

func (r *Runtime) enqueue(id string) bool {
	select {
	case r.queue <- id:
		return true
	case <-r.ctx.Done():
		return false
	default:
		return false
	}
}

func (r *Runtime) enqueueBlocking(id string) bool {
	select {
	case r.queue <- id:
		return true
	case <-r.ctx.Done():
		return false
	}
}

func (r *Runtime) claim(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.processing[id] {
		return false
	}
	r.processing[id] = true
	return true
}

func (r *Runtime) release(id string) {
	r.mu.Lock()
	delete(r.processing, id)
	delete(r.running, id)
	r.mu.Unlock()
}

func (r *Runtime) integrityLoop() {
	defer r.wg.Done()
	interval := time.Duration(r.cfg.Runtime.AuditVerifyIntervalSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			_ = r.events.Verify()
		}
	}
}

func (r *Runtime) Healthy() error {
	if r == nil || r.events == nil {
		return errors.New("runtime audit subsystem is not initialized")
	}
	return r.events.Healthy()
}

func (r *Runtime) worker() {
	defer r.wg.Done()
	for {
		select {
		case <-r.ctx.Done():
			return
		case id := <-r.queue:
			r.process(id)
		}
	}
}

func (r *Runtime) process(id string) {
	if !r.claim(id) {
		return
	}
	defer r.release(id)
	t, err := r.tasks.Get(id)
	if err != nil || t.Status == task.Canceled || t.Status == task.Completed || t.Status == task.Failed || t.Status == task.WaitingApproval {
		return
	}
	t, err = r.tasks.Update(id, func(x *task.Task) error {
		if x.Status != task.Queued {
			return fmt.Errorf("task not queued")
		}
		x.Status = task.Running
		x.Attempts++
		x.PendingApproval = ""
		x.Error = ""
		return nil
	})
	if err != nil {
		return
	}
	if auditErr := r.events.Append(eventlog.Event{Type: "task.running", TaskID: id, Data: map[string]any{"attempt": t.Attempts}}); auditErr != nil {
		_, _ = r.tasks.Update(id, func(x *task.Task) error {
			x.Status = task.Failed
			x.Error = "audit log unavailable; execution blocked"
			return nil
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.ctx, time.Duration(r.cfg.Runtime.TaskTimeoutSeconds)*time.Second)
	r.mu.Lock()
	r.running[id] = cancel
	r.mu.Unlock()
	defer cancel()
	out, providerName, runErr := r.agent.Run(ctx, id, t.Input)
	cur, _ := r.tasks.Get(id)
	if cur != nil && cur.Status == task.Canceled {
		_ = r.events.Append(eventlog.Event{Type: "task.canceled", TaskID: id})
		return
	}
	if runErr != nil {
		safeErr := memory.SanitizeContent(runErr.Error())
		var ar *tool.ApprovalRequired
		if errors.As(runErr, &ar) {
			_, _ = r.tasks.Update(id, func(x *task.Task) error {
				if x.Status == task.Canceled {
					return nil
				}
				x.Status = task.WaitingApproval
				x.PendingApproval = ar.ApprovalID
				x.Error = "approval required"
				return nil
			})
			_ = r.events.Append(eventlog.Event{Type: "task.waiting_approval", TaskID: id, Data: map[string]any{"approval_id": ar.ApprovalID}})
			return
		}
		if errors.Is(runErr, context.Canceled) && cur != nil && cur.Status == task.Canceled {
			return
		}
		_, _ = r.tasks.Update(id, func(x *task.Task) error {
			if x.Status == task.Canceled {
				return nil
			}
			x.Status = task.Failed
			x.Error = safeErr
			return nil
		})
		_ = r.events.Append(eventlog.Event{Type: "task.failed", TaskID: id, Data: map[string]any{"error": safeErr}})
		if r.cfg.Evolution.Enabled && r.evolution != nil {
			proposalID, idErr := storage.RandomID("evo")
			if idErr == nil {
				p := &evolution.Proposal{ID: proposalID, Kind: "runtime-failure", Title: "Improve handling for failed task", Rationale: "A production task failed and generated a review-only improvement proposal.", Evidence: map[string]any{"task_id": id, "error": safeErr, "attempts": t.Attempts}, Risk: "medium", Status: "proposed"}
				_ = r.evolution.Save(p)
			} else {
				_ = r.events.Append(eventlog.Event{Type: "evolution.proposal_failed", TaskID: id, Data: map[string]any{"reason": "secure_random_unavailable"}})
			}
		}
		return
	}
	_, err = r.tasks.Update(id, func(x *task.Task) error {
		if x.Status == task.Canceled {
			return errors.New("task canceled")
		}
		x.Status = task.Completed
		x.Output = out
		x.Provider = providerName
		x.Error = ""
		return nil
	})
	if err != nil {
		return
	}
	if r.cfg.Memory.Enabled && r.memory != nil && r.cfg.Memory.StoreTaskOutputs {
		_ = r.memory.Add(memory.Record{Kind: "episodic", Content: out, Source: "task:" + id, Importance: 0.5, Confidence: 0.8})
	}
	_ = r.events.Append(eventlog.Event{Type: "task.completed", TaskID: id, Data: map[string]any{"provider": providerName}})
}

func (r *Runtime) Task(id string) (*task.Task, error) { return r.tasks.Get(id) }
func (r *Runtime) Tasks() ([]*task.Task, error)       { return r.tasks.List() }
func (r *Runtime) Approvals() ([]*approval.Approval, error) {
	return r.approvals.List()
}
func (r *Runtime) Evolutions() ([]*evolution.Proposal, error) { return r.evolution.List() }

func (r *Runtime) Cancel(id string) error {
	_, err := r.tasks.Update(id, func(t *task.Task) error {
		switch t.Status {
		case task.Completed, task.Failed, task.Canceled:
			return errors.New("task already terminal")
		}
		t.Status = task.Canceled
		t.PendingApproval = ""
		return nil
	})
	if err != nil {
		return err
	}
	r.mu.Lock()
	if cancel, ok := r.running[id]; ok {
		cancel()
	}
	r.mu.Unlock()
	return nil
}

func (r *Runtime) DecideApproval(id, status string) error {
	if status != "approved" && status != "denied" {
		return errors.New("status must be approved or denied")
	}
	a, err := r.approvals.Update(id, func(a *approval.Approval) error {
		if a.Status != "pending" {
			return errors.New("approval already decided")
		}
		a.Status = status
		return nil
	})
	if err != nil {
		return err
	}
	if err := r.events.Append(eventlog.Event{Type: "approval." + status, TaskID: a.TaskID, Data: map[string]any{"approval_id": a.ID, "tool": a.Tool, "arguments_hash": a.ArgumentsHash}}); err != nil {
		return fmt.Errorf("approval state persisted but audit append failed; execution remains blocked: %w", err)
	}
	if status == "approved" {
		_, err = r.tasks.Update(a.TaskID, func(t *task.Task) error {
			if t.Status != task.WaitingApproval || t.PendingApproval != a.ID {
				return errors.New("task is not waiting for this approval")
			}
			t.Status = task.Queued
			t.PendingApproval = ""
			t.Error = ""
			return nil
		})
		if err != nil {
			return err
		}
		if !r.enqueue(a.TaskID) {
			_, _ = r.tasks.Update(a.TaskID, func(t *task.Task) error {
				t.Status = task.Failed
				t.Error = "runtime queue unavailable after approval"
				return nil
			})
			return ErrQueueUnavailable
		}
	}
	return nil
}
