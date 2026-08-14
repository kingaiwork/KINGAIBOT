package authority

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/storage"
)

const maxAuthorityGrants = 100000

type Grant struct {
	Envelope  Envelope   `json:"envelope"`
	ParentID  string     `json:"parent_id,omitempty"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type Store struct {
	dir    string
	events *eventlog.Log
	mu     sync.RWMutex
}

func NewStore(dir string, events *eventlog.Log) (*Store, error) {
	if events == nil {
		return nil, errors.New("authority store requires audit log")
	}
	if err := os.MkdirAll(filepath.Join(dir, "grants"), 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir, events: events}, nil
}

func (s *Store) grantPath(id string) (string, error) {
	if err := storage.ValidateID(id); err != nil {
		return "", err
	}
	return filepath.Join(s.dir, "grants", id+".json"), nil
}

func (s *Store) saveLocked(grant *Grant) error {
	if grant == nil {
		return errors.New("authority grant required")
	}
	if strings.TrimSpace(grant.Envelope.ID) == "" {
		return errors.New("authority grant requires id")
	}
	path, err := s.grantPath(grant.Envelope.ID)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(grant, "", "  ")
	if err != nil {
		return err
	}
	return storage.AtomicWriteFile(path, b, 0o600)
}

func (s *Store) loadLocked(id string) (*Grant, error) {
	path, err := s.grantPath(id)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var grant Grant
	if err := json.Unmarshal(b, &grant); err != nil {
		return nil, err
	}
	return &grant, nil
}

func cloneGrant(grant *Grant) (*Grant, error) {
	b, err := json.Marshal(grant)
	if err != nil {
		return nil, err
	}
	var out Grant
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Store) countLocked() (int, error) {
	entries, err := os.ReadDir(filepath.Join(s.dir, "grants"))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			n++
		}
	}
	return n, nil
}

// CreateRoot persists a new grant as pending first. The grant is promoted to
// active only after the authority.created audit event is durable. This ordering
// ensures a crash between persistence and audit cannot expose unaudited
// authority after restart.
func (s *Store) CreateRoot(envelope Envelope) (*Grant, error) {
	now := time.Now().UTC()
	if err := envelope.Validate(now); err != nil {
		return nil, err
	}
	id, err := storage.RandomID("auth")
	if err != nil {
		return nil, err
	}
	envelope.ID = id
	grant := &Grant{Envelope: envelope, Status: "pending", CreatedAt: now, UpdatedAt: now}

	s.mu.Lock()
	defer s.mu.Unlock()
	if n, countErr := s.countLocked(); countErr != nil {
		return nil, countErr
	} else if n >= maxAuthorityGrants {
		return nil, errors.New("authority grant limit reached")
	}
	if err := s.saveLocked(grant); err != nil {
		return nil, err
	}
	path, _ := s.grantPath(id)
	if err := s.events.Append(eventlog.Event{Type: "authority.created", Data: map[string]any{"authority_id": id, "subject_id": envelope.SubjectID}}); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("authority creation rolled back because audit failed: %w", err)
	}
	grant.Status = "active"
	grant.UpdatedAt = time.Now().UTC()
	if err := s.saveLocked(grant); err != nil {
		// The previously persisted pending grant remains non-effective if the
		// atomic promotion cannot be committed.
		return nil, fmt.Errorf("authority was audited but activation persistence failed: %w", err)
	}
	return cloneGrant(grant)
}

func (s *Store) Get(id string) (*Grant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	grant, err := s.loadLocked(id)
	if err != nil {
		return nil, err
	}
	return cloneGrant(grant)
}

func (s *Store) List() ([]*Grant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(filepath.Join(s.dir, "grants"))
	if err != nil {
		return nil, err
	}
	out := make([]*Grant, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		grant, loadErr := s.loadLocked(strings.TrimSuffix(entry.Name(), ".json"))
		if loadErr != nil {
			return nil, loadErr
		}
		out = append(out, grant)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// Derive follows the same audit-before-activation rule as CreateRoot. The
// child remains pending and non-effective until the delegation audit is
// durable, including across process crashes.
func (s *Store) Derive(parentID string, child Envelope) (*Grant, error) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	parent, err := s.loadLocked(parentID)
	if err != nil {
		return nil, err
	}
	if err := s.effectiveLocked(parent, now, map[string]struct{}{}); err != nil {
		return nil, fmt.Errorf("parent authority is not effective: %w", err)
	}
	derived, err := parent.Envelope.Derive(child, now)
	if err != nil {
		return nil, err
	}
	id, err := storage.RandomID("auth")
	if err != nil {
		return nil, err
	}
	derived.ID = id
	grant := &Grant{Envelope: derived, ParentID: parent.Envelope.ID, Status: "pending", CreatedAt: now, UpdatedAt: now}
	if n, countErr := s.countLocked(); countErr != nil {
		return nil, countErr
	} else if n >= maxAuthorityGrants {
		return nil, errors.New("authority grant limit reached")
	}
	if err := s.saveLocked(grant); err != nil {
		return nil, err
	}
	if err := s.events.Append(eventlog.Event{Type: "authority.delegated", Data: map[string]any{"authority_id": id, "parent_id": parent.Envelope.ID, "subject_id": derived.SubjectID}}); err != nil {
		path, _ := s.grantPath(id)
		_ = os.Remove(path)
		return nil, fmt.Errorf("authority delegation rolled back because audit failed: %w", err)
	}
	grant.Status = "active"
	grant.UpdatedAt = time.Now().UTC()
	if err := s.saveLocked(grant); err != nil {
		return nil, fmt.Errorf("authority delegation was audited but activation persistence failed: %w", err)
	}
	return cloneGrant(grant)
}

func (s *Store) Revoke(id string) (*Grant, error) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	grant, err := s.loadLocked(id)
	if err != nil {
		return nil, err
	}
	if grant.Status != "active" {
		return nil, errors.New("authority grant is not active")
	}
	original, err := cloneGrant(grant)
	if err != nil {
		return nil, err
	}
	grant.Status = "revoked"
	grant.RevokedAt = &now
	grant.UpdatedAt = now
	if err := s.saveLocked(grant); err != nil {
		return nil, err
	}
	if err := s.events.Append(eventlog.Event{Type: "authority.revoked", Data: map[string]any{"authority_id": id, "subject_id": grant.Envelope.SubjectID}}); err != nil {
		if rollbackErr := s.saveLocked(original); rollbackErr != nil {
			return nil, fmt.Errorf("audit failed and authority rollback failed: audit=%v rollback=%w", err, rollbackErr)
		}
		return nil, fmt.Errorf("authority revocation rolled back because audit failed: %w", err)
	}
	return cloneGrant(grant)
}

func (s *Store) Effective(id string, now time.Time) (*Grant, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	grant, err := s.loadLocked(id)
	if err != nil {
		return nil, err
	}
	if err := s.effectiveLocked(grant, now, map[string]struct{}{}); err != nil {
		return nil, err
	}
	return cloneGrant(grant)
}

func (s *Store) effectiveLocked(grant *Grant, now time.Time, seen map[string]struct{}) error {
	if grant == nil {
		return errors.New("authority grant required")
	}
	id := grant.Envelope.ID
	if _, ok := seen[id]; ok {
		return errors.New("authority parent cycle detected")
	}
	seen[id] = struct{}{}
	if grant.Status != "active" {
		return errors.New("authority grant is not active")
	}
	if grant.RevokedAt != nil {
		return errors.New("authority grant is revoked")
	}
	if err := grant.Envelope.Validate(now); err != nil {
		return err
	}
	if grant.ParentID == "" {
		return nil
	}
	parent, err := s.loadLocked(grant.ParentID)
	if err != nil {
		return fmt.Errorf("authority parent unavailable: %w", err)
	}
	return s.effectiveLocked(parent, now, seen)
}

func (s *Store) Check(id, capability, dataScope, tool string) error {
	grant, err := s.Effective(id, time.Now().UTC())
	if err != nil {
		return err
	}
	if capability != "" && !grant.Envelope.AllowsCapability(capability) {
		return errors.New("capability denied")
	}
	if dataScope != "" && !grant.Envelope.AllowsDataScope(dataScope) {
		return errors.New("data scope denied")
	}
	if tool != "" && !grant.Envelope.AllowsTool(tool) {
		return errors.New("tool denied")
	}
	return nil
}
