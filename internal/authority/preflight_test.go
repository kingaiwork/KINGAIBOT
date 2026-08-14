package authority

import "testing"

func TestPreflightExplainsAncestorBottleneckWithoutMutatingUsage(t *testing.T) {
	store := newAuthorityTestStore(t)
	root, err := store.CreateRoot(Envelope{
		SubjectID:         "agent:root",
		Capabilities:      []string{"task.*"},
		MaxConcurrentWork: 1,
		MaxCostUnits:      10,
		AllowDelegation:   true,
		DelegationDepth:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.Derive(root.Envelope.ID, Envelope{
		SubjectID:         "agent:child",
		Capabilities:      []string{"task.execute"},
		MaxConcurrentWork: 1,
		MaxCostUnits:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReserveWork(child.Envelope.ID, "job-existing"); err != nil {
		t.Fatal(err)
	}
	before, err := store.Usage(root.Envelope.ID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Preflight(child.Envelope.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed {
		t.Fatal("preflight unexpectedly allowed work when ancestor concurrency is exhausted")
	}
	if len(result.Lineage) != 2 {
		t.Fatalf("expected child+parent lineage, got %d", len(result.Lineage))
	}
	if len(result.Reasons) == 0 {
		t.Fatal("denied preflight must explain its bottleneck")
	}
	after, err := store.Usage(root.Envelope.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.ActiveWork != after.ActiveWork || before.ConsumedCostUnits != after.ConsumedCostUnits {
		t.Fatal("read-only preflight mutated authority usage")
	}
}

func TestPreflightRequiresTrustedCostForBoundedLineage(t *testing.T) {
	store := newAuthorityTestStore(t)
	root, err := store.CreateRoot(Envelope{
		SubjectID:    "agent:bounded",
		Capabilities: []string{"task.execute"},
		MaxCostUnits: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Preflight(root.Envelope.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed || len(result.Reasons) == 0 {
		t.Fatal("bounded authority must reject unspecified preflight cost")
	}
	result, err = store.Preflight(root.Envelope.ID, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Allowed {
		t.Fatalf("expected cost 4 to fit budget: %#v", result.Reasons)
	}
	if err := store.ChargeCost(root.Envelope.ID, "existing", 3); err != nil {
		t.Fatal(err)
	}
	result, err = store.Preflight(root.Envelope.ID, 4)
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed {
		t.Fatal("preflight ignored already consumed cost")
	}
}

func TestUsageOverviewIncludesParentAndChildAccounting(t *testing.T) {
	store := newAuthorityTestStore(t)
	root, err := store.CreateRoot(Envelope{
		SubjectID:         "agent:root",
		Capabilities:      []string{"task.*"},
		MaxConcurrentWork: 2,
		MaxCostUnits:      20,
		AllowDelegation:   true,
		DelegationDepth:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.Derive(root.Envelope.ID, Envelope{
		SubjectID:         "agent:child",
		Capabilities:      []string{"task.execute"},
		MaxConcurrentWork: 1,
		MaxCostUnits:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReserveWork(child.Envelope.ID, "job-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.ChargeCost(child.Envelope.ID, "job-1.attempt.1", 3); err != nil {
		t.Fatal(err)
	}
	overview, err := store.UsageOverview()
	if err != nil {
		t.Fatal(err)
	}
	if len(overview) != 2 {
		t.Fatalf("expected two usage rows, got %d", len(overview))
	}
	seen := map[string]*UsageSnapshot{}
	for _, row := range overview {
		seen[row.AuthorityID] = row
	}
	if seen[root.Envelope.ID] == nil || seen[root.Envelope.ID].ActiveWork != 1 || seen[root.Envelope.ID].ConsumedCostUnits != 3 {
		t.Fatalf("parent usage does not include child accounting: %#v", seen[root.Envelope.ID])
	}
	if seen[child.Envelope.ID] == nil || seen[child.Envelope.ID].ActiveWork != 1 || seen[child.Envelope.ID].ConsumedCostUnits != 3 {
		t.Fatalf("child usage missing: %#v", seen[child.Envelope.ID])
	}
}
