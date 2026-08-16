package task

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/storage"
)

// ListUpdatedSince returns tasks whose durable file changed at or after since,
// oldest first. It intentionally uses directory metadata as a cheap prefilter
// so long-running cognition/reconciliation loops do not re-read and decode the
// entire task history on every poll. A small caller-side overlap is recommended
// because filesystem timestamp precision varies by platform.
func (s *Store) ListUpdatedSince(since time.Time, limit int) ([]*Task, error) {
	if limit <= 0 {
		limit = 1024
	}
	if limit > 10000 {
		limit = 10000
	}

	type candidate struct {
		name    string
		modTime time.Time
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	candidates := make([]candidate, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if !since.IsZero() && info.ModTime().Before(since) {
			continue
		}
		candidates = append(candidates, candidate{name: entry.Name(), modTime: info.ModTime()})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].modTime.Equal(candidates[j].modTime) {
			return candidates[i].name < candidates[j].name
		}
		return candidates[i].modTime.Before(candidates[j].modTime)
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	out := make([]*Task, 0, len(candidates))
	for _, candidate := range candidates {
		b, err := os.ReadFile(filepath.Join(s.dir, candidate.name))
		if err != nil {
			continue
		}
		var t Task
		if json.Unmarshal(b, &t) != nil || storage.ValidateID(t.ID) != nil {
			continue
		}
		out = append(out, &t)
	}
	return out, nil
}
