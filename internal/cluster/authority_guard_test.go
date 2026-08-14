package cluster

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
)

type fakeAuthorityChecker struct {
	mu      sync.Mutex
	allowed bool
	id      string
}

func (f *fakeAuthorityChecker) Check(id, capability, dataScope, tool string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.allowed || id != f.id {
		return errors.New("authority denied")
	}
	return nil
}

func (f *fakeAuthorityChecker) setAllowed(v bool) {
	f.mu.Lock()
	f.allowed = v
	f.mu.Unlock()
}

func TestSubmitAuthorizedRequiresBoundAuthority(t *testing.T) {
	c := newCoordinatorForTest(t)
	checker := &fakeAuthorityChecker{allowed: true, id: "auth_test"}
	if err := c.SetAuthorityChecker(checker); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SubmitAuthorized(Job{Kind: "file.write"}, "", nil, ""); err == nil {
		t.Fatal("expected missing authority_id to fail")
	}
	job, err := c.SubmitAuthorized(Job{
		Kind:                 "file.write",
		Payload:              json.RawMessage(`{"path":"report.txt"}`),
		RequiredCapabilities: []string{"task.execute"},
	}, "auth_test", []string{"project.alpha.docs"}, "file.write")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := c.loadAuthorityBinding(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if binding.AuthorityID != "auth_test" || binding.RequiredTool != "file.write" {
		t.Fatalf("unexpected authority binding: %#v", binding)
	}
}

func TestAuthorityRevokedBeforeLeaseBlocksJob(t *testing.T) {
	c := newCoordinatorForTest(t)
	checker := &fakeAuthorityChecker{allowed: true, id: "auth_test"}
	if err := c.SetAuthorityChecker(checker); err != nil {
		t.Fatal(err)
	}
	issued, err := c.RegisterWorker("writer", []string{"task.execute"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	job, err := c.SubmitAuthorized(Job{Kind: "file.write", RequiredCapabilities: []string{"task.execute"}}, "auth_test", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	checker.setAllowed(false)
	worker, err := c.AuthenticateWorker(issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.LeaseJobAuthorized(worker, 30); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected revoked job to be withheld, got %v", err)
	}
	jobs, err := c.Jobs()
	if err != nil {
		t.Fatal(err)
	}
	stored := jobByID(t, jobs, job.ID)
	if stored.Status != "failed" {
		t.Fatalf("expected authority-blocked job to fail closed, got %s", stored.Status)
	}
}

func TestAuthorityRevokedDuringExecutionMovesToReconciliation(t *testing.T) {
	c := newCoordinatorForTest(t)
	checker := &fakeAuthorityChecker{allowed: true, id: "auth_test"}
	if err := c.SetAuthorityChecker(checker); err != nil {
		t.Fatal(err)
	}
	issued, err := c.RegisterWorker("writer", []string{"task.execute"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	job, err := c.SubmitAuthorized(Job{Kind: "external.write", RequiredCapabilities: []string{"task.execute"}, ReplayPolicy: "manual"}, "auth_test", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	worker, err := c.AuthenticateWorker(issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := c.LeaseJobAuthorized(worker, 30)
	if err != nil {
		t.Fatal(err)
	}
	checker.setAllowed(false)
	result := json.RawMessage(`{"remote_id":"abc123"}`)
	completed, err := c.CompleteAuthorized(worker, job.ID, lease.LeaseToken, result, "")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "reconciliation" {
		t.Fatalf("expected reconciliation after mid-flight revocation, got %s", completed.Status)
	}
	if string(completed.Result) != string(result) {
		t.Fatalf("expected remote result retained as reconciliation evidence: %s", completed.Result)
	}
}
