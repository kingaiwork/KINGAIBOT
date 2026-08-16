package task

import (
	"testing"
	"time"
)

func TestListUpdatedSinceReturnsBoundedOldestFirst(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"task_a", "task_b", "task_c"} {
		if err := store.Save(&Task{ID: id, Input: id, Status: Completed}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	items, err := store.ListUpdatedSince(time.Time{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].ID != "task_a" || items[1].ID != "task_b" {
		t.Fatalf("expected oldest tasks first, got %q then %q", items[0].ID, items[1].ID)
	}

	cutoff := items[1].UpdatedAt.Add(-time.Second)
	items, err = store.ListUpdatedSince(cutoff, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 2 {
		t.Fatalf("expected overlapping recent items, got %d", len(items))
	}
}
