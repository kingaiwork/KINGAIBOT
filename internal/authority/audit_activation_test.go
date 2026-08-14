package authority

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
)

func breakAuthorityAudit(t *testing.T, eventsDir string) {
	t.Helper()
	path := filepath.Join(eventsDir, "events.jsonl")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestRootAuditFailureNeverLeavesEffectiveGrant(t *testing.T) {
	dir := t.TempDir()
	eventsDir := filepath.Join(dir, "events")
	events, err := eventlog.New(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(filepath.Join(dir, "authority"), events)
	if err != nil {
		t.Fatal(err)
	}
	breakAuthorityAudit(t, eventsDir)
	if _, err := store.CreateRoot(Envelope{SubjectID: "agent:blocked", Capabilities: []string{"task.execute"}}); err == nil {
		t.Fatal("root creation unexpectedly succeeded with broken audit log")
	}
	grants, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, grant := range grants {
		if grant.Status == "active" {
			t.Fatalf("audit-failed root left active authority %s", grant.Envelope.ID)
		}
	}
}

func TestDelegationAuditFailureNeverLeavesEffectiveChild(t *testing.T) {
	dir := t.TempDir()
	eventsDir := filepath.Join(dir, "events")
	events, err := eventlog.New(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(filepath.Join(dir, "authority"), events)
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.CreateRoot(Envelope{
		SubjectID:       "agent:root",
		Capabilities:    []string{"task.*"},
		AllowDelegation: true,
		DelegationDepth: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	breakAuthorityAudit(t, eventsDir)
	if _, err := store.Derive(root.Envelope.ID, Envelope{
		SubjectID:    "agent:child",
		Capabilities: []string{"task.execute"},
	}); err == nil {
		t.Fatal("delegation unexpectedly succeeded with broken audit log")
	}
	grants, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	activeChildren := 0
	for _, grant := range grants {
		if grant.ParentID == root.Envelope.ID && grant.Status == "active" {
			activeChildren++
		}
	}
	if activeChildren != 0 {
		t.Fatalf("audit-failed delegation left %d active child grants", activeChildren)
	}
}
