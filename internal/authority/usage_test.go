package authority

import (
	"testing"
	"time"
)

func TestPendingGrantIsNeverEffective(t *testing.T) {
	store := newAuthorityTestStore(t)
	now := time.Now().UTC()
	grant := &Grant{
		Envelope: Envelope{
			ID:           "auth_pending_test",
			SubjectID:    "agent:pending",
			Capabilities: []string{"task.execute"},
		},
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
	}
	store.mu.Lock()
	if err := store.saveLocked(grant); err != nil {
		store.mu.Unlock()
		t.Fatal(err)
	}
	store.mu.Unlock()
	if _, err := store.Effective(grant.Envelope.ID, now); err == nil {
		t.Fatal("pending authority must never be effective")
	}
}

func TestCreateAndDerivePromoteToActive(t *testing.T) {
	store := newAuthorityTestStore(t)
	root, err := store.CreateRoot(Envelope{
		SubjectID:       "agent:root",
		Capabilities:    []string{"task.*"},
		AllowDelegation: true,
		DelegationDepth: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if root.Status != "active" {
		t.Fatalf("root must be active after audited creation, got %q", root.Status)
	}
	child, err := store.Derive(root.Envelope.ID, Envelope{
		SubjectID:    "agent:child",
		Capabilities: []string{"task.execute"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if child.Status != "active" {
		t.Fatalf("child must be active after audited delegation, got %q", child.Status)
	}
}

func TestHierarchicalConcurrencyBudget(t *testing.T) {
	store := newAuthorityTestStore(t)
	root, err := store.CreateRoot(Envelope{
		SubjectID:         "agent:root",
		Capabilities:      []string{"task.*"},
		MaxConcurrentWork: 2,
		AllowDelegation:   true,
		DelegationDepth:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.Derive(root.Envelope.ID, Envelope{
		SubjectID:         "agent:child",
		Capabilities:      []string{"task.execute"},
		MaxConcurrentWork: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReserveWork(child.Envelope.ID, "job-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.ReserveWork(child.Envelope.ID, "job-1"); err != nil {
		t.Fatalf("same work reservation must be idempotent: %v", err)
	}
	if err := store.ReserveWork(child.Envelope.ID, "job-2"); err != nil {
		t.Fatal(err)
	}
	if err := store.ReserveWork(child.Envelope.ID, "job-3"); err == nil {
		t.Fatal("third concurrent work item must exceed hierarchical budget")
	}
	rootUsage, err := store.Usage(root.Envelope.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rootUsage.ActiveWork != 2 {
		t.Fatalf("root must account for descendant work, got %d", rootUsage.ActiveWork)
	}
	if err := store.ReleaseWork(child.Envelope.ID, "job-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.ReserveWork(child.Envelope.ID, "job-3"); err != nil {
		t.Fatalf("released capacity must be reusable: %v", err)
	}
}

func TestHierarchicalCostBudgetAcrossSiblings(t *testing.T) {
	store := newAuthorityTestStore(t)
	root, err := store.CreateRoot(Envelope{
		SubjectID:       "agent:root",
		Capabilities:    []string{"task.*"},
		MaxCostUnits:    10,
		AllowDelegation: true,
		DelegationDepth: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	childA, err := store.Derive(root.Envelope.ID, Envelope{
		SubjectID:    "agent:a",
		Capabilities: []string{"task.execute"},
		MaxCostUnits: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	childB, err := store.Derive(root.Envelope.ID, Envelope{
		SubjectID:    "agent:b",
		Capabilities: []string{"task.execute"},
		MaxCostUnits: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ChargeCost(childA.Envelope.ID, "job-a-attempt-1", 6); err != nil {
		t.Fatal(err)
	}
	if err := store.ChargeCost(childA.Envelope.ID, "job-a-attempt-1", 6); err != nil {
		t.Fatalf("same cost charge must be idempotent: %v", err)
	}
	if err := store.ChargeCost(childB.Envelope.ID, "job-b-attempt-1", 5); err == nil {
		t.Fatal("sibling charges must not exceed shared parent budget")
	}
	rootUsage, err := store.Usage(root.Envelope.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rootUsage.ConsumedCostUnits != 6 {
		t.Fatalf("expected parent consumed cost 6, got %d", rootUsage.ConsumedCostUnits)
	}
	if err := store.ChargeCost(childB.Envelope.ID, "unspecified-cost", 0); err == nil {
		t.Fatal("bounded authority must reject unspecified zero cost")
	}
}
