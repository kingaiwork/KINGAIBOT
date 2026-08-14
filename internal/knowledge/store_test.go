package knowledge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
)

func newStoreForTest(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	el, err := eventlog.New(dir + "/events")
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(dir+"/knowledge", el)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestProposalIsNotTrustedUntilReviewed(t *testing.T) {
	s := newStoreForTest(t)
	p, err := s.CreateProposal(Item{Kind: "relation", Subject: "KINGAIBOT", Predicate: "integrates", Object: "KING AI", Content: "future execution-layer integration", Confidence: 0.9})
	if err != nil {
		t.Fatal(err)
	}
	results, err := s.Search("KING AI", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatal("unreviewed knowledge appeared in trusted search")
	}
	approved, err := s.Review(p.ID, "approved", "operator verified")
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != "approved" || approved.ReviewedAt == nil {
		t.Fatalf("unexpected review result: %#v", approved)
	}
	results, err = s.Search("KING AI", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Item.ID != p.ID {
		t.Fatalf("approved knowledge not returned: %#v", results)
	}
	neighbors, err := s.Neighbors("KINGAIBOT", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(neighbors) != 1 || neighbors[0].Object != "KING AI" {
		t.Fatalf("graph neighbor lookup failed: %#v", neighbors)
	}
}

func TestRejectedKnowledgeStaysHidden(t *testing.T) {
	s := newStoreForTest(t)
	p, err := s.CreateProposal(Item{Kind: "fact", Content: "untrusted claim", Confidence: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Review(p.ID, "rejected", "not verified"); err != nil {
		t.Fatal(err)
	}
	results, err := s.Search("untrusted claim", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatal("rejected knowledge appeared in trusted search")
	}
}

func TestKnowledgeSanitizesSecrets(t *testing.T) {
	s := newStoreForTest(t)
	p, err := s.CreateProposal(Item{Kind: "note", Content: "api_key=abcdefghijklmnop12345678", Confidence: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p.Content, "abcdefghijklmnop12345678") || !strings.Contains(p.Content, "REDACTED") {
		t.Fatalf("secret was not sanitized: %q", p.Content)
	}
}

func TestAgentToolCanOnlyPropose(t *testing.T) {
	s := newStoreForTest(t)
	raw := json.RawMessage(`{"kind":"fact","content":"candidate fact","confidence":0.7}`)
	out, err := s.ExecuteTool(context.Background(), "task_x", "knowledge_propose", raw)
	if err != nil {
		t.Fatal(err)
	}
	var item Item
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		t.Fatal(err)
	}
	if item.Status != "proposed" {
		t.Fatalf("agent-created knowledge unexpectedly trusted: %s", item.Status)
	}
}
