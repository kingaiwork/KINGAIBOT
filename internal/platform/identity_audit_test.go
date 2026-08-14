package platform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
)

func newIdentityAuditManager(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	eventsDir := filepath.Join(dir, "events")
	events, err := eventlog.New(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(filepath.Join(dir, "platform"), newFakeRuntime(), events)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m, eventsDir
}

func breakPlatformAudit(t *testing.T, eventsDir string) {
	t.Helper()
	path := filepath.Join(eventsDir, "events.jsonl")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestIdentityCreationAuditFailureLeavesIdentityDisabled(t *testing.T) {
	m, eventsDir := newIdentityAuditManager(t)
	breakPlatformAudit(t, eventsDir)
	if _, err := m.CreateIdentity(Identity{Name: "blocked-admin", Roles: []string{"admin"}}); err == nil {
		t.Fatal("identity creation unexpectedly succeeded with broken audit log")
	}
	ids, err := m.Identities()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected one staged identity, got %d", len(ids))
	}
	if ids[0].Enabled {
		t.Fatal("audit-failed identity became enabled")
	}
}

func TestIdentityEnableAuditFailureCannotRestoreAuthority(t *testing.T) {
	m, eventsDir := newIdentityAuditManager(t)
	id, err := m.CreateIdentity(Identity{Name: "operator", Roles: []string{"operator"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetIdentityEnabled(id.ID, false); err != nil {
		t.Fatal(err)
	}
	breakPlatformAudit(t, eventsDir)
	if _, err := m.SetIdentityEnabled(id.ID, true); err == nil {
		t.Fatal("identity enable unexpectedly succeeded with broken audit log")
	}
	stored, err := m.Identity(id.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Enabled {
		t.Fatal("failed enable audit restored identity authority")
	}
}

func TestAccessKeyIssuanceAuditFailureLeavesKeyRevoked(t *testing.T) {
	m, eventsDir := newIdentityAuditManager(t)
	id, err := m.CreateIdentity(Identity{Name: "viewer", Roles: []string{"viewer"}})
	if err != nil {
		t.Fatal(err)
	}
	breakPlatformAudit(t, eventsDir)
	if _, err := m.IssueAccessKey(id.ID, 3600); err == nil {
		t.Fatal("access key issuance unexpectedly succeeded with broken audit log")
	}
	keys, err := m.AccessKeys()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected one staged key, got %d", len(keys))
	}
	if keys[0].RevokedAt == nil {
		t.Fatal("audit-failed access key became active")
	}
}
