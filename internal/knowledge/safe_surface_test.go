package knowledge

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
)

func newSafeKnowledgeStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	eventsDir := filepath.Join(dir, "events")
	events, err := eventlog.New(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(filepath.Join(dir, "knowledge"), events)
	if err != nil {
		t.Fatal(err)
	}
	return store, eventsDir
}

func breakKnowledgeAudit(t *testing.T, eventsDir string) {
	t.Helper()
	path := filepath.Join(eventsDir, "events.jsonl")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func knowledgeFact() Item {
	return Item{Kind: "fact", Scope: "company", Subject: "KINGAIBOT", Predicate: "status", Object: "safe"}
}

func TestProposalAuditFailureLeavesUnreviewablePendingItem(t *testing.T) {
	store, eventsDir := newSafeKnowledgeStore(t)
	breakKnowledgeAudit(t, eventsDir)
	if _, err := store.CreateProposalSafe(knowledgeFact()); err == nil {
		t.Fatal("proposal unexpectedly succeeded with broken audit log")
	}
	items, err := store.List(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Status != "pending_audit" {
		t.Fatalf("audit-failed proposal became reviewable: %#v", items)
	}
	if _, err := store.ReviewSafe(items[0].ID, "approved", "must not trust"); err == nil {
		t.Fatal("pending_audit knowledge was approved")
	}
	trusted, err := store.List(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(trusted) != 0 {
		t.Fatal("pending_audit knowledge leaked into trusted reads")
	}
}

func TestConcurrentOppositeReviewsOnlyOneCanCommit(t *testing.T) {
	store, _ := newSafeKnowledgeStore(t)
	item, err := store.CreateProposalSafe(knowledgeFact())
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, decision := range []string{"approved", "rejected"} {
		decision := decision
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := store.ReviewSafe(item.ID, decision, "concurrent review")
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	failures := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			failures++
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("expected exactly one review commit, successes=%d failures=%d", successes, failures)
	}
	stored, err := store.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "approved" && stored.Status != "rejected" {
		t.Fatalf("unexpected final review state %q", stored.Status)
	}
}

func TestDirectApprovalAuditFailureNeverBecomesTrusted(t *testing.T) {
	store, eventsDir := newSafeKnowledgeStore(t)
	breakKnowledgeAudit(t, eventsDir)
	if _, err := store.CreateApprovedSafe(knowledgeFact(), "admin import"); err == nil {
		t.Fatal("direct approval unexpectedly succeeded with broken audit log")
	}
	trusted, err := store.List(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(trusted) != 0 {
		t.Fatal("audit-failed direct approval became trusted")
	}
	all, err := store.List(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Status != "pending_audit" {
		t.Fatalf("expected inert pending_audit item, got %#v", all)
	}
}
