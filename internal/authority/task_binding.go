package authority

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

// TaskRuntime is the minimal runtime contract needed by the platform control
// plane. BoundTaskRuntime decorates it without moving authority decisions into
// the model or platform prompt layer.
type TaskRuntime interface {
	Create(input string, meta map[string]any) (*task.Task, error)
	Task(id string) (*task.Task, error)
}

type idempotentTaskRuntime interface {
	CreateIdempotent(input string, meta map[string]any, key string) (*task.Task, error)
}

type BoundTaskRuntime struct {
	base  TaskRuntime
	store *Store
}

func NewBoundTaskRuntime(base TaskRuntime, store *Store) (*BoundTaskRuntime, error) {
	if base == nil {
		return nil, errors.New("base task runtime required")
	}
	if store == nil {
		return nil, errors.New("authority store required")
	}
	return &BoundTaskRuntime{base: base, store: store}, nil
}

func copyMetadata(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}

// bindTrustedMetadata removes any caller-supplied authority binding and derives
// the effective authority exclusively from durable Agent identity state.
func (b *BoundTaskRuntime) bindTrustedMetadata(in map[string]any) (map[string]any, error) {
	meta := copyMetadata(in)
	delete(meta, "authority_id")
	agentID, _ := meta["agent_id"].(string)
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return meta, nil
	}
	grant, err := b.store.ActiveForSubject(agentID)
	switch {
	case err == nil:
		meta["authority_id"] = grant.Envelope.ID
	case errors.Is(err, os.ErrNotExist):
		// An agent without a grant may still reason and perform capabilities that
		// do not require authority-bound execution. Privileged delegation fails
		// closed later because no authority_id is present.
	default:
		return nil, fmt.Errorf("agent authority resolution failed: %w", err)
	}
	return meta, nil
}

func (b *BoundTaskRuntime) Create(input string, meta map[string]any) (*task.Task, error) {
	bound, err := b.bindTrustedMetadata(meta)
	if err != nil {
		return nil, err
	}
	return b.base.Create(input, bound)
}

// CreateIdempotent preserves the same trusted authority derivation while
// delegating stable task identity to a Runtime that explicitly supports it.
func (b *BoundTaskRuntime) CreateIdempotent(input string, meta map[string]any, key string) (*task.Task, error) {
	base, ok := b.base.(idempotentTaskRuntime)
	if !ok {
		return nil, errors.New("base task runtime does not support idempotent creation")
	}
	bound, err := b.bindTrustedMetadata(meta)
	if err != nil {
		return nil, err
	}
	return base.CreateIdempotent(input, bound, key)
}

func (b *BoundTaskRuntime) Task(id string) (*task.Task, error) {
	return b.base.Task(id)
}

// TaskAuthorityResolver reads only durable task metadata produced by trusted
// runtime code. A model cannot supply an authority identifier through tool
// arguments and thereby widen its own permissions.
type TaskAuthorityResolver struct {
	tasks *task.Store
}

func NewTaskAuthorityResolver(tasks *task.Store) (*TaskAuthorityResolver, error) {
	if tasks == nil {
		return nil, errors.New("task store required")
	}
	return &TaskAuthorityResolver{tasks: tasks}, nil
}

func (r *TaskAuthorityResolver) AuthorityForTask(taskID string) (string, error) {
	t, err := r.tasks.Get(taskID)
	if err != nil {
		return "", err
	}
	if t.Metadata == nil {
		return "", os.ErrNotExist
	}
	raw, ok := t.Metadata["authority_id"]
	if !ok {
		return "", os.ErrNotExist
	}
	authorityID, ok := raw.(string)
	if !ok || strings.TrimSpace(authorityID) == "" {
		return "", errors.New("task authority metadata is invalid")
	}
	return strings.TrimSpace(authorityID), nil
}
