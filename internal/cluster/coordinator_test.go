package cluster

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
)

func newCoordinatorForTest(t *testing.T) *Coordinator {
	t.Helper()
	dir := t.TempDir()
	el, err := eventlog.New(dir + "/events")
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(dir+"/cluster", el)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestWorkerTokenNotPersistedAndCapabilityMatching(t *testing.T) {
	c := newCoordinatorForTest(t)
	issued, err := c.RegisterWorker("browser-1", []string{"browser", "linux"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(issued.Token, "kaw_worker_") {
		t.Fatalf("unexpected worker token: %q", issued.Token)
	}
	path, err := c.workerPath(issued.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), issued.Token) {
		t.Fatal("raw worker token was persisted")
	}
	job, err := c.Submit(Job{Kind: "browser.navigate", Payload: json.RawMessage(`{"url":"https://example.com"}`), RequiredCapabilities: []string{"browser"}})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := c.AuthenticateWorker(issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := c.LeaseJob(worker, 30)
	if err != nil {
		t.Fatal(err)
	}
	if lease.Job.ID != job.ID || lease.LeaseToken == "" {
		t.Fatalf("unexpected lease: %#v", lease)
	}
}

func TestCapabilityMismatchDoesNotLease(t *testing.T) {
	c := newCoordinatorForTest(t)
	issued, err := c.RegisterWorker("text-only", []string{"text"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Submit(Job{Kind: "browser.screenshot", RequiredCapabilities: []string{"browser"}}); err != nil {
		t.Fatal(err)
	}
	worker, err := c.AuthenticateWorker(issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.LeaseJob(worker, 30); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no matching job, got %v", err)
	}
}

func expireLease(t *testing.T, c *Coordinator, jobID string) {
	t.Helper()
	path, err := c.jobPath(jobID)
	if err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var j Job
	if err := read(path, &j); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Minute)
	j.LeaseExpiresAt = &past
	if err := save(path, &j); err != nil {
		t.Fatal(err)
	}
}

func jobByID(t *testing.T, jobs []*Job, id string) *Job {
	t.Helper()
	for _, j := range jobs {
		if j.ID == id {
			return j
		}
	}
	t.Fatalf("job %s not found", id)
	return nil
}

func TestManualReplayPolicyRequiresReconciliation(t *testing.T) {
	c := newCoordinatorForTest(t)
	issued, err := c.RegisterWorker("device", []string{"device"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	job, err := c.Submit(Job{Kind: "device.action", RequiredCapabilities: []string{"device"}})
	if err != nil {
		t.Fatal(err)
	}
	if job.ReplayPolicy != "manual" {
		t.Fatalf("unsafe default replay policy: %s", job.ReplayPolicy)
	}
	worker, _ := c.AuthenticateWorker(issued.Token)
	if _, err := c.LeaseJob(worker, 30); err != nil {
		t.Fatal(err)
	}
	expireLease(t, c, job.ID)
	jobs, err := c.Jobs()
	if err != nil {
		t.Fatal(err)
	}
	got := jobByID(t, jobs, job.ID)
	if got.Status != "reconciliation" {
		t.Fatalf("manual job was replayed after ambiguous lease expiry: %#v", got)
	}
	if _, err := c.Reconcile(job.ID, "requeue", "operator confirmed no side effect", nil); err != nil {
		t.Fatal(err)
	}
	jobs, _ = c.Jobs()
	if got = jobByID(t, jobs, job.ID); got.Status != "queued" {
		t.Fatalf("operator reconciliation did not requeue job: %#v", got)
	}
}

func TestSafeReplayPolicyRequeuesExpiredLease(t *testing.T) {
	c := newCoordinatorForTest(t)
	issued, err := c.RegisterWorker("reader", []string{"read"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	job, err := c.Submit(Job{Kind: "safe.read", RequiredCapabilities: []string{"read"}, ReplayPolicy: "safe"})
	if err != nil {
		t.Fatal(err)
	}
	worker, _ := c.AuthenticateWorker(issued.Token)
	if _, err := c.LeaseJob(worker, 30); err != nil {
		t.Fatal(err)
	}
	expireLease(t, c, job.ID)
	jobs, err := c.Jobs()
	if err != nil {
		t.Fatal(err)
	}
	if got := jobByID(t, jobs, job.ID); got.Status != "queued" {
		t.Fatalf("safe job did not requeue: %#v", got)
	}
}

func TestLeaseCanOnlyCompleteOnce(t *testing.T) {
	c := newCoordinatorForTest(t)
	issued, err := c.RegisterWorker("worker", []string{"task"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Submit(Job{Kind: "task", RequiredCapabilities: []string{"task"}})
	if err != nil {
		t.Fatal(err)
	}
	worker, _ := c.AuthenticateWorker(issued.Token)
	lease, err := c.LeaseJob(worker, 30)
	if err != nil {
		t.Fatal(err)
	}
	result := json.RawMessage(`{"ok":true}`)
	completed, err := c.Complete(worker, lease.Job.ID, lease.LeaseToken, result, "")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "completed" {
		t.Fatalf("unexpected status: %s", completed.Status)
	}
	if _, err := c.Complete(worker, lease.Job.ID, lease.LeaseToken, result, ""); err == nil {
		t.Fatal("same lease completed twice")
	}
}
