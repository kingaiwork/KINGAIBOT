package cluster

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/authority"
	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
)

type budgetHarness struct {
	cluster     *Coordinator
	authorities *authority.Store
}

func newBudgetHarness(t *testing.T) *budgetHarness {
	t.Helper()
	dir := t.TempDir()
	events, err := eventlog.New(dir + "/events")
	if err != nil {
		t.Fatal(err)
	}
	authorities, err := authority.NewStore(dir+"/authority", events)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := New(dir+"/cluster", events)
	if err != nil {
		t.Fatal(err)
	}
	if err := coordinator.SetAuthorityChecker(authorities); err != nil {
		t.Fatal(err)
	}
	return &budgetHarness{cluster: coordinator, authorities: authorities}
}

func (h *budgetHarness) root(t *testing.T, concurrency int, cost int64) *authority.Grant {
	t.Helper()
	grant, err := h.authorities.CreateRoot(authority.Envelope{
		SubjectID:         "agent:budget",
		Capabilities:      []string{"task.execute"},
		ToolScopes:        []string{"file.write"},
		MaxConcurrentWork: concurrency,
		MaxCostUnits:      cost,
	})
	if err != nil {
		t.Fatal(err)
	}
	return grant
}

func budgetJob() Job {
	return Job{
		Kind:                 "file.write",
		Payload:              json.RawMessage(`{"path":"report.txt","content":"ok"}`),
		RequiredCapabilities: []string{"task.execute"},
		ReplayPolicy:         "manual",
	}
}

func TestAuthorizedBudgetedJobEnforcesConcurrencyAndReleasesOnCompletion(t *testing.T) {
	h := newBudgetHarness(t)
	grant := h.root(t, 1, 10)
	first, err := h.cluster.SubmitAuthorizedBudgeted(budgetJob(), grant.Envelope.ID, nil, "file.write", 3)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "queued" {
		t.Fatalf("expected queued job, got %s", first.Status)
	}
	usage, err := h.authorities.Usage(grant.Envelope.ID)
	if err != nil {
		t.Fatal(err)
	}
	if usage.ActiveWork != 1 {
		t.Fatalf("expected one reserved work slot, got %d", usage.ActiveWork)
	}
	if _, err := h.cluster.SubmitAuthorizedBudgeted(budgetJob(), grant.Envelope.ID, nil, "file.write", 3); err == nil {
		t.Fatal("second concurrent job unexpectedly bypassed authority concurrency budget")
	}
	issued, err := h.cluster.RegisterWorker("writer", []string{"task.execute"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := h.cluster.AuthenticateWorker(issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := h.cluster.LeaseJobAuthorized(worker, 30)
	if err != nil {
		t.Fatal(err)
	}
	usage, err = h.authorities.Usage(grant.Envelope.ID)
	if err != nil {
		t.Fatal(err)
	}
	if usage.ConsumedCostUnits != 3 {
		t.Fatalf("expected 3 consumed cost units after lease, got %d", usage.ConsumedCostUnits)
	}
	if _, err := h.cluster.CompleteAuthorized(worker, lease.Job.ID, lease.LeaseToken, json.RawMessage(`{"ok":true}`), ""); err != nil {
		t.Fatal(err)
	}
	usage, err = h.authorities.Usage(grant.Envelope.ID)
	if err != nil {
		t.Fatal(err)
	}
	if usage.ActiveWork != 0 {
		t.Fatalf("terminal completion must release work slot, got %d", usage.ActiveWork)
	}
}

func TestCostBudgetFailureNeverDeliversLeaseToWorker(t *testing.T) {
	h := newBudgetHarness(t)
	grant := h.root(t, 2, 5)
	first, err := h.cluster.SubmitAuthorizedBudgeted(budgetJob(), grant.Envelope.ID, nil, "file.write", 4)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := h.cluster.RegisterWorker("writer", []string{"task.execute"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := h.cluster.AuthenticateWorker(issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := h.cluster.LeaseJobAuthorized(worker, 30)
	if err != nil {
		t.Fatal(err)
	}
	if lease.Job.ID != first.ID {
		t.Fatal("unexpected first lease")
	}
	if _, err := h.cluster.CompleteAuthorized(worker, lease.Job.ID, lease.LeaseToken, nil, "done"); err != nil {
		t.Fatal(err)
	}

	second, err := h.cluster.SubmitAuthorizedBudgeted(budgetJob(), grant.Envelope.ID, nil, "file.write", 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.cluster.LeaseJobAuthorized(worker, 30); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("worker must not receive a lease whose charge exceeds remaining budget; got %v", err)
	}
	stored, err := h.cluster.Job(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "failed" {
		t.Fatalf("budget-blocked job must fail closed before delivery, got %s", stored.Status)
	}
	usage, err := h.authorities.Usage(grant.Envelope.ID)
	if err != nil {
		t.Fatal(err)
	}
	if usage.ConsumedCostUnits != 4 {
		t.Fatalf("failed pre-delivery charge must not consume unavailable cost; got %d", usage.ConsumedCostUnits)
	}
	if usage.ActiveWork != 0 {
		t.Fatalf("budget-blocked terminal job must release concurrency slot, got %d", usage.ActiveWork)
	}
}

func TestBoundedCostRejectsUnspecifiedZeroBeforeLeaseDelivery(t *testing.T) {
	h := newBudgetHarness(t)
	grant := h.root(t, 1, 5)
	job, err := h.cluster.SubmitAuthorizedBudgeted(budgetJob(), grant.Envelope.ID, nil, "file.write", 0)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := h.cluster.RegisterWorker("writer", []string{"task.execute"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := h.cluster.AuthenticateWorker(issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.cluster.LeaseJobAuthorized(worker, 30); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bounded zero-cost job must not be delivered; got %v", err)
	}
	stored, err := h.cluster.Job(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "failed" {
		t.Fatalf("expected failed zero-cost job, got %s", stored.Status)
	}
}
