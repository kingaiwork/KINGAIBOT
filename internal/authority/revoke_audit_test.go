package authority

import (
	"path/filepath"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
)

func TestRevocationAuditFailureNeverRestoresAuthority(t *testing.T) {
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
	grant, err := store.CreateRoot(Envelope{
		SubjectID:    "agent:revoke-me",
		Capabilities: []string{"task.execute"},
	})
	if err != nil {
		t.Fatal(err)
	}
	breakAuthorityAudit(t, eventsDir)
	if _, err := store.Revoke(grant.Envelope.ID); err == nil {
		t.Fatal("expected revocation audit failure")
	}
	stored, err := store.Get(grant.Envelope.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "revoked" || stored.RevokedAt == nil {
		t.Fatalf("failed revocation audit restored authority: %#v", stored)
	}
	if _, err := store.Effective(grant.Envelope.ID, stored.UpdatedAt); err == nil {
		t.Fatal("revoked authority remained effective after audit failure")
	}
}
