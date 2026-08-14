package authority

import (
	"testing"
	"time"
)

func TestDeriveAllowsNarrowerAuthority(t *testing.T) {
	now := time.Now().UTC()
	parentExpiry := now.Add(2 * time.Hour)
	childExpiry := now.Add(time.Hour)
	parent := Envelope{
		SubjectID:         "agent:planner",
		Capabilities:      []string{"task.*", "knowledge.read"},
		DataScopes:        []string{"project.alpha.*"},
		ToolScopes:        []string{"file.*", "http.get"},
		MaxConcurrentWork: 8,
		MaxCostUnits:      1000,
		AllowDelegation:   true,
		DelegationDepth:   2,
		ExpiresAt:         &parentExpiry,
	}
	child := Envelope{
		SubjectID:         "worker:alpha",
		Capabilities:      []string{"task.execute", "knowledge.read"},
		DataScopes:        []string{"project.alpha.docs"},
		ToolScopes:        []string{"file.read"},
		MaxConcurrentWork: 2,
		MaxCostUnits:      200,
		AllowDelegation:   true,
		DelegationDepth:   1,
		ExpiresAt:         &childExpiry,
	}

	got, err := parent.Derive(child, now)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if !got.AllowsCapability("task.execute") {
		t.Fatal("expected delegated capability")
	}
}

func TestDeriveRejectsCapabilityEscalation(t *testing.T) {
	now := time.Now().UTC()
	parent := Envelope{
		SubjectID:       "agent:a",
		Capabilities:    []string{"knowledge.read"},
		AllowDelegation: true,
		DelegationDepth: 1,
	}
	child := Envelope{SubjectID: "agent:b", Capabilities: []string{"admin.write"}}
	if _, err := parent.Derive(child, now); err == nil {
		t.Fatal("expected capability escalation to be rejected")
	}
}

func TestDeriveRejectsUnlimitedChildFromBoundedParent(t *testing.T) {
	now := time.Now().UTC()
	parent := Envelope{
		SubjectID:         "agent:a",
		Capabilities:      []string{"task.execute"},
		MaxConcurrentWork: 4,
		MaxCostUnits:      500,
		AllowDelegation:   true,
		DelegationDepth:   1,
	}
	child := Envelope{
		SubjectID:    "worker:b",
		Capabilities: []string{"task.execute"},
		// zero means unlimited and therefore must not be derivable from bounded parent.
		MaxConcurrentWork: 0,
		MaxCostUnits:      0,
	}
	if _, err := parent.Derive(child, now); err == nil {
		t.Fatal("expected unlimited child budget to be rejected")
	}
}

func TestWildcardIsNamespaceBound(t *testing.T) {
	if !Allows([]string{"file.*"}, "file.read") {
		t.Fatal("expected namespace wildcard match")
	}
	if Allows([]string{"file.*"}, "filesystem.read") {
		t.Fatal("wildcard must not escape namespace")
	}
}
