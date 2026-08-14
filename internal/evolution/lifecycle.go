package evolution

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/storage"
)

func (s *Store) Get(id string) (*Proposal, error) {
	if s == nil {
		return nil, errors.New("evolution store unavailable")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	path, err := s.path(id)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Proposal
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) Update(id string, fn func(*Proposal) error) (*Proposal, error) {
	if s == nil {
		return nil, errors.New("evolution store unavailable")
	}
	if fn == nil {
		return nil, errors.New("update function required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(id)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Proposal
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	if err := fn(&p); err != nil {
		return nil, err
	}
	p.UpdatedAt = time.Now().UTC()
	b, err = json.MarshalIndent(&p, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := storage.AtomicWriteFile(path, b, 0o600); err != nil {
		return nil, err
	}
	return &p, nil
}
