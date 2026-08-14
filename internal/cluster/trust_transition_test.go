package cluster

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
)

func breakAuditFile(t *testing.T, eventsDir string) {
	t.Helper()
	path := filepath.Join(eventsDir, "events.jsonl")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerRegistrationAuditFailureLeavesCredentialDisabled(t *testing.T) {
	dir := t.TempDir()
	eventsDir := filepath.Join(dir, "events")
	events, err := eventlog.New(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := New(filepath.Join(dir, "cluster"), events)
	if err != nil {
		t.Fatal(err)
	}
	breakAuditFile(t, eventsDir)
	if _, err := coordinator.RegisterWorker("blocked", []string{"task.execute"}, nil); err == nil {
		t.Fatal("registration unexpectedly succeeded with broken audit log")
	}
	workers, err := coordinator.Workers()
	if err != nil {
		t.Fatal(err)
	}
	if len(workers) != 1 {
		t.Fatalf("expected one inert persisted worker, got %d", len(workers))
	}
	if workers[0].Enabled {
		t.Fatal("worker credential became enabled before durable registration audit")
	}
}

func TestWorkerEnableAuditFailureStaysDisabled(t *testing.T) {
	dir := t.TempDir()
	eventsDir := filepath.Join(dir, "events")
	events, err := eventlog.New(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := New(filepath.Join(dir, "cluster"), events)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := coordinator.RegisterWorker("writer", []string{"task.execute"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.SetWorkerEnabled(issued.ID, false); err != nil {
		t.Fatal(err)
	}
	breakAuditFile(t, eventsDir)
	if _, err := coordinator.SetWorkerEnabled(issued.ID, true); err == nil {
		t.Fatal("enable unexpectedly succeeded with broken audit log")
	}
	if _, err := coordinator.AuthenticateWorker(issued.Token); err == nil {
		t.Fatal("disabled worker token became usable despite failed enable audit")
	}
}

func TestPendingAuditJobIsNeverLeaseable(t *testing.T) {
	dir := t.TempDir()
	events, err := eventlog.New(filepath.Join(dir, "events"))
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := New(filepath.Join(dir, "cluster"), events)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := coordinator.RegisterWorker("writer", []string{"task.execute"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := coordinator.AuthenticateWorker(issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	job := &Job{
		ID:                   "job_pending_audit_test",
		Kind:                 "test",
		RequiredCapabilities: []string{"task.execute"},
		ReplayPolicy:         "manual",
		Status:               "pending_audit",
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	path, err := coordinator.jobPath(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	coordinator.mu.Lock()
	if err := save(path, job); err != nil {
		coordinator.mu.Unlock()
		t.Fatal(err)
	}
	coordinator.mu.Unlock()
	if _, err := coordinator.LeaseJob(worker, 30); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending_audit job must never be leaseable, got %v", err)
	}
}
