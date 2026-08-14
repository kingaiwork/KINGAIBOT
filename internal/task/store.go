package task

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

type Status string

const (
	PendingAudit    Status = "pending_audit"
	Queued          Status = "queued"
	Running         Status = "running"
	WaitingApproval Status = "waiting_approval"
	Completing      Status = "completing"
	Reconciliation  Status = "reconciliation"
	Completed       Status = "completed"
	Failed          Status = "failed"
	Canceled        Status = "canceled"
)

type Task struct {
	ID              string         `json:"id"`
	Input           string         `json:"input"`
	Output          string         `json:"output,omitempty"`
	Provider        string         `json:"provider,omitempty"`
	Status          Status         `json:"status"`
	Error           string         `json:"error,omitempty"`
	Attempts        int            `json:"attempts"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	PendingApproval string         `json:"pending_approval,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type Store struct {
	dir string
	mu  sync.RWMutex
}

func NewStore(dir string) (*Store, error) {
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

func (s *Store) saveLocked(t *Task) error {
	p, err := s.path(t.ID)
	if err != nil {
		return err
	}
	t.UpdatedAt = time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = t.UpdatedAt
	}
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return storage.AtomicWriteFile(p, b, 0o600)
}

func (s *Store) Save(t *Task) error {
	if t == nil {
		return errors.New("task required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(t)
}

// SaveIfAbsent persists a Task only when its ID is not already present in this
// Store. It is serialized with all other Store writes and is the primitive used
// by deterministic/idempotent Runtime task creation. Existing tasks are never
// overwritten by a repeated attempt.
func (s *Store) SaveIfAbsent(t *Task) (bool, error) {
	if t == nil {
		return false, errors.New("task required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.path(t.ID)
	if err != nil {
		return false, err
	}
	if _, err = os.Stat(p); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := s.saveLocked(t); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) getLocked(id string) (*Task, error) {
	p, err := s.path(id)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var t Task
	if err = json.Unmarshal(b, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) Get(id string) (*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getLocked(id)
}

func (s *Store) Update(id string, fn func(*Task) error) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.getLocked(id)
	if err != nil {
		return nil, err
	}
	if err = fn(t); err != nil {
		return nil, err
	}
	if err = s.saveLocked(t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Store) List() ([]*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	es, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	out := []*Task{}
	for _, e := range es {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, er := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if er != nil {
			continue
		}
		var t Task
		if json.Unmarshal(b, &t) == nil && storage.ValidateID(t.ID) == nil {
			out = append(out, &t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) Recoverable() ([]*Task, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	out := []*Task{}
	for _, t := range all {
		if t.Status == PendingAudit || t.Status == Running || t.Status == Completing || t.Status == Queued {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *Store) Cancel(id string) error {
	_, err := s.Update(id, func(t *Task) error {
		if t.Status == Completed || t.Status == Failed || t.Status == Canceled {
			return errors.New("task already terminal")
		}
		if t.Status == Completing || t.Status == Reconciliation {
			return errors.New("task has ambiguous side effects and requires reconciliation")
		}
		t.Status = Canceled
		t.PendingApproval = ""
		t.Error = "canceled"
		return nil
	})
	return err
}
