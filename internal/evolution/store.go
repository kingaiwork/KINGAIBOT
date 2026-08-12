package evolution

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/storage"
)

type Proposal struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	Title     string         `json:"title"`
	Rationale string         `json:"rationale"`
	Evidence  map[string]any `json:"evidence,omitempty"`
	Risk      string         `json:"risk"`
	Status    string         `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type Store struct {
	dir string
	mu  sync.RWMutex
}

func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) path(id string) (string, error) {
	if err := storage.ValidateID(id); err != nil {
		return "", err
	}
	return filepath.Join(s.dir, id+".json"), nil
}

func (s *Store) Save(p *Proposal) error {
	if p == nil {
		return errors.New("proposal required")
	}
	if p.ID == "" {
		id, err := storage.RandomID("evo")
		if err != nil {
			return err
		}
		p.ID = id
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(p.ID)
	if err != nil {
		return err
	}
	p.UpdatedAt = time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = p.UpdatedAt
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return storage.AtomicWriteFile(path, b, 0o600)
}

func (s *Store) List() ([]*Proposal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	es, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var out []*Proposal
	for _, e := range es {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, er := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if er != nil {
			continue
		}
		var p Proposal
		if json.Unmarshal(b, &p) == nil && storage.ValidateID(p.ID) == nil {
			out = append(out, &p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
