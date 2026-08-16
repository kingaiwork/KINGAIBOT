package memory

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"time"
)

func (s *Store) SnapshotRecords(limit int) ([]Record, error) {
	if s == nil {
		return []Record{}, nil
	}
	if limit <= 0 || limit > 2048 {
		limit = 512
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	now := time.Now().UTC()
	out := make([]Record, 0, limit)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 256<<10)
	for sc.Scan() {
		var r Record
		if json.Unmarshal(sc.Bytes(), &r) != nil || (r.ExpiresAt != nil && !r.ExpiresAt.After(now)) {
			continue
		}
		r.Content = SanitizeContent(r.Content)
		out = append(out, r)
		if len(out) > limit {
			out = out[1:]
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) MergeSyncedRecords(records []Record) (int, error) {
	if s == nil || len(records) == 0 {
		return 0, nil
	}
	if len(records) > 2048 {
		return 0, errors.New("synced memory record limit exceeded")
	}
	existing, err := s.SnapshotRecords(2048)
	if err != nil {
		return 0, err
	}
	ids := make(map[string]struct{}, len(existing))
	for _, r := range existing {
		ids[r.ID] = struct{}{}
	}
	added := 0
	for _, r := range records {
		if r.ID == "" {
			continue
		}
		if _, ok := ids[r.ID]; ok {
			continue
		}
		r.Content = SanitizeContent(r.Content)
		if len(r.Source) > 240 {
			r.Source = r.Source[:240]
		}
		r.Source = "cloud-sync:" + r.Source
		if r.Confidence > 0.95 {
			r.Confidence = 0.95
		}
		if err := s.Add(r); err != nil {
			return added, err
		}
		ids[r.ID] = struct{}{}
		added++
	}
	return added, nil
}
