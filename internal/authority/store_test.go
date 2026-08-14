package authority

import (
	"testing"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
)

func newAuthorityTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	events, err := eventlog.New(dir + "/events")
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(dir+"/authority", events)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestStoreDeriveCannotEscalate(t *testing.T) {
	store := newAuthorityTestStore(t)
	root, err := store.CreateRoot(Envelope{
		SubjectID:       "agent:planner",
		Capabilities:    []string{"task.*", "knowledge.read"},
		DataScopes:      []string{"project.alpha.*"},
		ToolScopes:      []string{"file.*"},
		AllowDelegation: true,
		DelegationDepth: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.Derive(root.Envelope.ID, Envelope{
		SubjectID:       "worker:alpha",
		Capabilities:    []string{"task.execute"},
		DataScopes:      []string{"project.alpha.docs"},
		ToolScopes:      []string{"file.read"},
		AllowDelegation: true,
		DelegationDepth: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentID != root.Envelope.ID {
		t.Fatal("delegated grant must retain parent")
	}
	if _, err := store.Derive(root.Envelope.ID, Envelope{
		SubjectID:    "worker:evil",
		Capabilities: []string{"admin.write"},
	}); err == nil {
		t.Fatal("expected privilege escalation to fail")
	}
}

func TestRevokedParentInvalidatesDescendant(t *testing.T) {
	store := newAuthorityTestStore(t)
	root, err := store.CreateRoot(Envelope{
		SubjectID:       "agent:root",
		Capabilities:    []string{"task.*"},
		ToolScopes:      []string{"file.*"},
		AllowDelegation: true,
		DelegationDepth: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.Derive(root.Envelope.ID, Envelope{
		SubjectID:    "worker:child",
		Capabilities: []string{"task.execute"},
		ToolScopes:   []string{"file.read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Check(child.Envelope.ID, "task.execute", "", "file.read"); err != nil {
		t.Fatalf("expected child to be effective before parent revoke: %v", err)
	}
	if _, err := store.Revoke(root.Envelope.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Check(child.Envelope.ID, "task.execute", "", "file.read"); err == nil {
		t.Fatal("revoked parent must invalidate descendant")
	}
}

func TestExpiredEnvelopeIsNotEffective(t *testing.T) {
	store := newAuthorityTestStore(t)
	expires := time.Now().UTC().Add(20 * time.Millisecond)
	grant, err := store.CreateRoot(Envelope{
		SubjectID:    "agent:short",
		Capabilities: []string{"task.read"},
		ExpiresAt:    &expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Effective(grant.Envelope.ID, expires.Add(time.Second)); err == nil {
		t.Fatal("expired envelope must not be effective")
	}
}
