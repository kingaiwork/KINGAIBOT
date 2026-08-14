package evolution

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
)

func newControllerForTest(t *testing.T) (*Controller, *Store) {
	t.Helper()
	dir := t.TempDir()
	store, err := New(dir + "/evolution")
	if err != nil {
		t.Fatal(err)
	}
	log, err := eventlog.New(dir + "/events")
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewController(store, log)
	if err != nil {
		t.Fatal(err)
	}
	return controller, store
}

func TestControlledEvolutionLifecycle(t *testing.T) {
	c, store := newControllerForTest(t)
	p, err := c.Propose(Proposal{Kind: "repair", Title: "Fix queue edge case", Rationale: "regression evidence", Risk: "medium"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != "proposed" {
		t.Fatalf("unexpected initial status: %s", p.Status)
	}
	if _, err := c.Decide(p.ID, Decision{Action: "approve", Reason: "too early"}); err == nil {
		t.Fatal("proposal approved without evaluation")
	}
	if _, err := c.SubmitEvaluation(p.ID, Evaluation{Suite: "unit-regression", Score: 0.4, Passed: false}); err != nil {
		t.Fatal(err)
	}
	p, _ = store.Get(p.ID)
	if p.Status != "evaluation_failed" {
		t.Fatalf("failed evaluation did not block proposal: %s", p.Status)
	}
	if _, err := c.SubmitEvaluation(p.ID, Evaluation{Suite: "unit-regression-v2", Score: 0.98, Passed: true}); err != nil {
		t.Fatal(err)
	}
	p, _ = store.Get(p.ID)
	if p.Status != "review_required" {
		t.Fatalf("passed evaluation did not require review: %s", p.Status)
	}
	if _, err := c.Decide(p.ID, Decision{Action: "approve", Reason: "reviewed and tests passed"}); err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	if _, err := c.Decide(p.ID, Decision{Action: "stage", Reason: "signed candidate built", ArtifactDigest: digest}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Decide(p.ID, Decision{Action: "release", Reason: "missing gates", ArtifactDigest: digest}); err == nil {
		t.Fatal("release succeeded without signature and health gates")
	}
	if _, err := c.Decide(p.ID, Decision{Action: "release", Reason: "wrong artifact", ArtifactDigest: strings.Repeat("b", 64), SignatureVerified: true, HealthStatus: "passed"}); err == nil {
		t.Fatal("release accepted artifact different from staged digest")
	}
	if _, err := c.Decide(p.ID, Decision{Action: "release", Reason: "signature and health verified", ArtifactDigest: digest, SignatureVerified: true, HealthStatus: "passed"}); err != nil {
		t.Fatal(err)
	}
	p, _ = store.Get(p.ID)
	if p.Status != "released" {
		t.Fatalf("expected released, got %s", p.Status)
	}
	if _, err := c.Decide(p.ID, Decision{Action: "rollback", Reason: "canary health regression"}); err != nil {
		t.Fatal(err)
	}
	p, _ = store.Get(p.ID)
	if p.Status != "rolled_back" {
		t.Fatalf("expected rolled_back, got %s", p.Status)
	}
}

func TestEvolutionProposalSanitizesSecrets(t *testing.T) {
	c, _ := newControllerForTest(t)
	p, err := c.Propose(Proposal{Kind: "repair", Title: "sanitize", Rationale: "token=abcdefghijklmnop12345678", Risk: "low", Evidence: map[string]any{"nested": map[string]any{"password": "password=abcdefghijklmnop12345678"}}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(p)
	if strings.Contains(string(b), "abcdefghijklmnop12345678") || !strings.Contains(string(b), "REDACTED") {
		t.Fatalf("proposal contains unsanitized secret: %s", b)
	}
}

func TestAgentEvolutionToolCanOnlyPropose(t *testing.T) {
	c, _ := newControllerForTest(t)
	out, err := c.ExecuteTool(context.Background(), "task_x", "evolution_propose_improvement", json.RawMessage(`{"kind":"optimization","title":"reduce retries","rationale":"observed duplicate transient failures","risk":"low"}`))
	if err != nil {
		t.Fatal(err)
	}
	var p Proposal
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		t.Fatal(err)
	}
	if p.Status != "proposed" {
		t.Fatalf("agent proposal unexpectedly gained authority: %s", p.Status)
	}
	for _, def := range c.ToolDefinitions() {
		if strings.Contains(def.Function.Name, "approve") || strings.Contains(def.Function.Name, "release") || strings.Contains(def.Function.Name, "stage") {
			t.Fatalf("agent exposed privileged evolution tool: %s", def.Function.Name)
		}
	}
}
