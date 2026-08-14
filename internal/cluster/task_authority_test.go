package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fixedTaskAuthorityResolver struct {
	taskID      string
	authorityID string
}

func (r *fixedTaskAuthorityResolver) AuthorityForTask(taskID string) (string, error) {
	if taskID != r.taskID {
		return "", errors.New("task authority unavailable")
	}
	return r.authorityID, nil
}

func TestClusterToolUsesAuthorityBoundToTrustedTask(t *testing.T) {
	c := newCoordinatorForTest(t)
	checker := &fakeAuthorityChecker{allowed: true, id: "auth_task"}
	if err := c.SetAuthorityChecker(checker); err != nil {
		t.Fatal(err)
	}
	if err := c.SetTaskAuthorityResolver(&fixedTaskAuthorityResolver{taskID: "task_trusted", authorityID: "auth_task"}); err != nil {
		t.Fatal(err)
	}

	raw := json.RawMessage(`{"kind":"file.write","payload":{"path":"report.txt"},"required_capabilities":["task.execute"],"required_tool":"file.write","replay_policy":"manual"}`)
	if _, err := c.ExecuteTool(context.Background(), "task_trusted", "cluster_job_submit", raw); err != nil {
		t.Fatal(err)
	}
	jobs, err := c.Jobs()
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected one job, got %d", len(jobs))
	}
	binding, err := c.loadAuthorityBinding(jobs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if binding.AuthorityID != "auth_task" {
		t.Fatalf("unexpected authority binding: %#v", binding)
	}
}

func TestClusterToolCannotSubmitWithoutTrustedTaskAuthority(t *testing.T) {
	c := newCoordinatorForTest(t)
	checker := &fakeAuthorityChecker{allowed: true, id: "auth_task"}
	if err := c.SetAuthorityChecker(checker); err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"kind":"file.write","required_capabilities":["task.execute"]}`)
	if _, err := c.ExecuteTool(context.Background(), "task_unbound", "cluster_job_submit", raw); err == nil {
		t.Fatal("model-triggered cluster job unexpectedly bypassed trusted task authority")
	}
}

func TestAuthorizedJobCannotOmitAuthorityConstraints(t *testing.T) {
	c := newCoordinatorForTest(t)
	checker := &fakeAuthorityChecker{allowed: true, id: "auth_task"}
	if err := c.SetAuthorityChecker(checker); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SubmitAuthorized(Job{Kind: "opaque.remote.action"}, "auth_task", nil, ""); err == nil {
		t.Fatal("authority-bound remote job without declared constraints must fail closed")
	}
}
