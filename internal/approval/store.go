package approval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/storage"
)

type Approval struct {
	ID             string          `json:"id"`
	TaskID         string          `json:"task_id"`
	Tool           string          `json:"tool"`
	Capability     string          `json:"capability"`
	Arguments      json.RawMessage `json:"arguments"`
	ArgumentsHash  string          `json:"arguments_hash"`
	Status         string          `json:"status"`
	ExecutionState string          `json:"execution_state,omitempty"`
	Result         string          `json:"result,omitempty"`
	ExecutionError string          `json:"execution_error,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	ExecutedAt     *time.Time      `json:"executed_at,omitempty"`
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

func CanonicalArgumentsHash(raw json.RawMessage) string {
	var v any
	canonical := raw
	if json.Unmarshal(raw, &v) == nil {
		if b, err := json.Marshal(v); err == nil {
			canonical = b
		}
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func (s *Store) path(id string) (string, error) {
	if err := storage.ValidateID(id); err != nil {
		return "", err
	}
	return filepath.Join(s.dir, id+".json"), nil
}

func (s *Store) saveLocked(a *Approval) error {
	if a == nil {
		return errors.New("approval required")
	}
	p, err := s.path(a.ID)
	if err != nil {
		return err
	}
	if a.ArgumentsHash == "" {
		a.ArgumentsHash = CanonicalArgumentsHash(a.Arguments)
	}
	a.UpdatedAt = time.Now().UTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = a.UpdatedAt
	}
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return storage.AtomicWriteFile(p, b, 0o600)
}

func (s *Store) Save(a *Approval) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(a)
}

func (s *Store) getLocked(id string) (*Approval, error) {
	p, err := s.path(id)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var a Approval
	if err = json.Unmarshal(b, &a); err != nil {
		return nil, err
	}
	if a.ArgumentsHash == "" {
		a.ArgumentsHash = CanonicalArgumentsHash(a.Arguments)
	}
	return &a, nil
}

func (s *Store) Get(id string) (*Approval, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getLocked(id)
}

func (s *Store) Update(id string, fn func(*Approval) error) (*Approval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, err := s.getLocked(id)
	if err != nil {
		return nil, err
	}
	if err = fn(a); err != nil {
		return nil, err
	}
	if err = s.saveLocked(a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Store) List() ([]*Approval, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listLocked()
}

func (s *Store) listLocked() ([]*Approval, error) {
	es, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var out []*Approval
	for _, e := range es {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, er := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if er != nil {
			continue
		}
		var a Approval
		if json.Unmarshal(b, &a) == nil && storage.ValidateID(a.ID) == nil {
			if a.ArgumentsHash == "" {
				a.ArgumentsHash = CanonicalArgumentsHash(a.Arguments)
			}
			out = append(out, &a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) FindMatching(taskID, tool string, args json.RawMessage) (*Approval, error) {
	h := CanonicalArgumentsHash(args)
	s.mu.RLock()
	defer s.mu.RUnlock()
	all, err := s.listLocked()
	if err != nil {
		return nil, err
	}
	for _, a := range all {
		if a.TaskID == taskID && a.Tool == tool && a.ArgumentsHash == h {
			return a, nil
		}
	}
	return nil, os.ErrNotExist
}

func (s *Store) BeginExecution(id string) (*Approval, error) {
	return s.Update(id, func(a *Approval) error {
		if a.Status != "approved" {
			return errors.New("approval is not approved")
		}
		switch a.ExecutionState {
		case "":
			a.ExecutionState = "executing"
		case "executing":
			return errors.New("approved action is already executing; manual reconciliation required before retry")
		case "completed", "failed":
			return nil
		default:
			return errors.New("invalid approval execution state")
		}
		return nil
	})
}

func (s *Store) FinishExecution(id, result string, execErr error) error {
	_, err := s.Update(id, func(a *Approval) error {
		now := time.Now().UTC()
		a.ExecutedAt = &now
		a.Result = result
		if execErr != nil {
			a.ExecutionState = "failed"
			a.ExecutionError = execErr.Error()
		} else {
			a.ExecutionState = "completed"
			a.ExecutionError = ""
		}
		return nil
	})
	return err
}
