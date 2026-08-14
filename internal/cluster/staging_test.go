package cluster

import (
	"errors"
	"os"
	"testing"
)

func TestHeldJobIsNotLeaseableUntilActivated(t *testing.T) {
	c := newCoordinatorForTest(t)
	checker := &fakeAuthorityChecker{allowed: true, id: "auth_hold"}
	if err := c.SetAuthorityChecker(checker); err != nil {
		t.Fatal(err)
	}
	issued, err := c.RegisterWorker("executor", []string{"task.execute"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := c.AuthenticateWorker(issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	job, err := c.SubmitHeldAuthorized(Job{
		Kind:                 "external.write",
		RequiredCapabilities: []string{"task.execute"},
	}, "auth_hold", nil, "", "dispatch_test")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "held" {
		t.Fatalf("expected held job, got %s", job.Status)
	}
	if _, err := c.LeaseJobAuthorized(worker, 30); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("held job became leaseable before activation: %v", err)
	}
	job, err = c.ActivateHeld(job.ID, "dispatch_test")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "queued" {
		t.Fatalf("expected queued job after activation, got %s", job.Status)
	}
	lease, err := c.LeaseJobAuthorized(worker, 30)
	if err != nil {
		t.Fatal(err)
	}
	if lease.Job.ID != job.ID {
		t.Fatalf("leased wrong job: %s", lease.Job.ID)
	}
}

func TestCancelHeldPreventsFutureLease(t *testing.T) {
	c := newCoordinatorForTest(t)
	checker := &fakeAuthorityChecker{allowed: true, id: "auth_hold"}
	if err := c.SetAuthorityChecker(checker); err != nil {
		t.Fatal(err)
	}
	issued, err := c.RegisterWorker("executor", []string{"task.execute"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := c.AuthenticateWorker(issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	job, err := c.SubmitHeldAuthorized(Job{
		Kind:                 "external.write",
		RequiredCapabilities: []string{"task.execute"},
	}, "auth_hold", nil, "", "dispatch_cancel")
	if err != nil {
		t.Fatal(err)
	}
	job, err = c.CancelHeld(job.ID, "dispatch_cancel", "graph start did not commit")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "failed" {
		t.Fatalf("expected canceled held job to fail closed, got %s", job.Status)
	}
	if _, err := c.LeaseJobAuthorized(worker, 30); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled held job became leaseable: %v", err)
	}
}

func TestHeldActivationRevalidatesAuthority(t *testing.T) {
	c := newCoordinatorForTest(t)
	checker := &fakeAuthorityChecker{allowed: true, id: "auth_hold"}
	if err := c.SetAuthorityChecker(checker); err != nil {
		t.Fatal(err)
	}
	job, err := c.SubmitHeldAuthorized(Job{
		Kind:                 "external.write",
		RequiredCapabilities: []string{"task.execute"},
	}, "auth_hold", nil, "", "dispatch_revoke")
	if err != nil {
		t.Fatal(err)
	}
	checker.setAllowed(false)
	if _, err := c.ActivateHeld(job.ID, "dispatch_revoke"); err == nil {
		t.Fatal("revoked authority unexpectedly activated held job")
	}
	stored, err := c.Job(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "held" {
		t.Fatalf("failed activation must leave job held, got %s", stored.Status)
	}
}

func TestHeldControlReferenceMustMatch(t *testing.T) {
	c := newCoordinatorForTest(t)
	checker := &fakeAuthorityChecker{allowed: true, id: "auth_hold"}
	if err := c.SetAuthorityChecker(checker); err != nil {
		t.Fatal(err)
	}
	job, err := c.SubmitHeldAuthorized(Job{
		Kind:                 "external.write",
		RequiredCapabilities: []string{"task.execute"},
	}, "auth_hold", nil, "", "dispatch_owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ActivateHeld(job.ID, "dispatch_other"); err == nil {
		t.Fatal("mismatched control reference unexpectedly activated held job")
	}
}
