package authority

import (
	"errors"
	"os"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

type captureTaskRuntime struct {
	meta map[string]any
}

func (r *captureTaskRuntime) Create(_ string, meta map[string]any) (*task.Task, error) {
	r.meta = copyMetadata(meta)
	return &task.Task{ID: "task_capture", Status: task.Queued, Metadata: copyMetadata(meta)}, nil
}

func (r *captureTaskRuntime) Task(id string) (*task.Task, error) {
	if id != "task_capture" {
		return nil, os.ErrNotExist
	}
	return &task.Task{ID: id, Status: task.Queued, Metadata: copyMetadata(r.meta)}, nil
}

func TestBoundTaskRuntimePropagatesUniqueAgentAuthority(t *testing.T) {
	store := newAuthorityTestStore(t)
	grant, err := store.CreateRoot(Envelope{SubjectID: "agent_alpha", Capabilities: []string{"task.execute"}})
	if err != nil {
		t.Fatal(err)
	}
	base := &captureTaskRuntime{}
	bound, err := NewBoundTaskRuntime(base, store)
	if err != nil {
		t.Fatal(err)
	}
	created, err := bound.Create("do work", map[string]any{"agent_id": "agent_alpha", "source": "session"})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := created.Metadata["authority_id"].(string); got != grant.Envelope.ID {
		t.Fatalf("expected trusted authority %q, got %q", grant.Envelope.ID, got)
	}
}

func TestBoundTaskRuntimeLeavesUnprivilegedAgentWithoutAuthority(t *testing.T) {
	store := newAuthorityTestStore(t)
	base := &captureTaskRuntime{}
	bound, err := NewBoundTaskRuntime(base, store)
	if err != nil {
		t.Fatal(err)
	}
	created, err := bound.Create("reason only", map[string]any{"agent_id": "agent_no_grant"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := created.Metadata["authority_id"]; ok {
		t.Fatal("unprivileged agent unexpectedly received authority")
	}
}

func TestBoundTaskRuntimeRejectsAmbiguousAgentAuthority(t *testing.T) {
	store := newAuthorityTestStore(t)
	for i := 0; i < 2; i++ {
		if _, err := store.CreateRoot(Envelope{SubjectID: "agent_ambiguous", Capabilities: []string{"task.execute"}}); err != nil {
			t.Fatal(err)
		}
	}
	bound, err := NewBoundTaskRuntime(&captureTaskRuntime{}, store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bound.Create("do work", map[string]any{"agent_id": "agent_ambiguous"}); err == nil {
		t.Fatal("ambiguous authority must fail closed")
	}
}

func TestTaskAuthorityResolverReadsOnlyDurableTaskMetadata(t *testing.T) {
	dir := t.TempDir()
	tasks, err := task.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := tasks.Save(&task.Task{ID: "task_bound", Status: task.Queued, Metadata: map[string]any{"authority_id": "auth_trusted"}}); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewTaskAuthorityResolver(tasks)
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolver.AuthorityForTask("task_bound")
	if err != nil {
		t.Fatal(err)
	}
	if got != "auth_trusted" {
		t.Fatalf("unexpected authority: %q", got)
	}
	if _, err := resolver.AuthorityForTask("task_missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected missing task to fail closed, got %v", err)
	}
}
