package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/memory"
	"github.com/kingaiwork/KINGAIBOT/internal/provider"
	"github.com/kingaiwork/KINGAIBOT/internal/storage"
)

// CreateProposalSafe stages a proposal as pending_audit. It becomes reviewable
// only after its proposal audit is durable. A crash between persistence and
// audit therefore cannot leave unaudited knowledge eligible for approval.
func (s *Store) CreateProposalSafe(in Item) (*Item, error) {
	in, err := normalizeItem(in)
	if err != nil {
		return nil, err
	}
	id, err := storage.RandomID("know")
	if err != nil {
		return nil, err
	}
	t := time.Now().UTC()
	in.ID, in.Status, in.CreatedAt, in.UpdatedAt = id, "pending_audit", t, t
	in.SHA256 = itemHash(in)

	s.mu.Lock()
	defer s.mu.Unlock()
	if n, err := s.countLocked(); err != nil {
		return nil, err
	} else if n >= maxItems {
		return nil, errors.New("knowledge store item limit reached")
	}
	if err := s.saveLocked(&in); err != nil {
		return nil, err
	}
	if err := s.events.Append(eventlog.Event{Type: "knowledge.proposed", Data: map[string]any{"knowledge_id": id, "scope": in.Scope, "kind": in.Kind, "sha256": in.SHA256}}); err != nil {
		return nil, fmt.Errorf("knowledge remains pending_audit because proposal audit failed: %w", err)
	}
	in.Status = "proposed"
	in.UpdatedAt = time.Now().UTC()
	if err := s.saveLocked(&in); err != nil {
		return nil, fmt.Errorf("knowledge proposal was audited but activation persistence failed: %w", err)
	}
	return cloneKnowledgeItem(&in)
}

// CreateApprovedSafe stages direct administrative trust in pending_audit. The
// item is invisible to approved reads until the exact approval event is durable.
func (s *Store) CreateApprovedSafe(in Item, reviewNote string) (*Item, error) {
	in, err := normalizeItem(in)
	if err != nil {
		return nil, err
	}
	id, err := storage.RandomID("know")
	if err != nil {
		return nil, err
	}
	t := time.Now().UTC()
	in.ID, in.Status, in.CreatedAt, in.UpdatedAt, in.ReviewedAt = id, "pending_audit", t, t, &t
	in.ReviewNote = clean(memory.SanitizeContent(reviewNote), 4096)
	in.SHA256 = itemHash(in)

	s.mu.Lock()
	defer s.mu.Unlock()
	if n, err := s.countLocked(); err != nil {
		return nil, err
	} else if n >= maxItems {
		return nil, errors.New("knowledge store item limit reached")
	}
	if err := s.saveLocked(&in); err != nil {
		return nil, err
	}
	if err := s.events.Append(eventlog.Event{Type: "knowledge.approved", Data: map[string]any{"knowledge_id": id, "scope": in.Scope, "kind": in.Kind, "sha256": in.SHA256}}); err != nil {
		return nil, fmt.Errorf("knowledge remains pending_audit because approval audit failed: %w", err)
	}
	in.Status = "approved"
	in.UpdatedAt = time.Now().UTC()
	if err := s.saveLocked(&in); err != nil {
		return nil, fmt.Errorf("knowledge approval was audited but trusted-state persistence failed: %w", err)
	}
	return cloneKnowledgeItem(&in)
}

// ReviewSafe serializes the complete read -> audit -> state transition under
// the Store lock. Two simultaneous reviewers can no longer both audit opposite
// decisions against the same proposed version.
func (s *Store) ReviewSafe(id, decision, note string) (*Item, error) {
	decision = strings.ToLower(strings.TrimSpace(memory.SanitizeContent(decision)))
	if decision != "approved" && decision != "rejected" {
		return nil, errors.New("decision must be approved or rejected")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.itemPath(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var item Item
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, err
	}
	if item.Status != "proposed" {
		return nil, errors.New("only audited proposed knowledge can be reviewed")
	}
	if err := s.events.Append(eventlog.Event{Type: "knowledge.reviewed", Data: map[string]any{"knowledge_id": id, "decision": decision, "sha256": item.SHA256}}); err != nil {
		return nil, fmt.Errorf("knowledge review blocked because audit failed: %w", err)
	}
	t := time.Now().UTC()
	item.Status, item.UpdatedAt, item.ReviewedAt = decision, t, &t
	item.ReviewNote = clean(memory.SanitizeContent(note), 4096)
	if err := s.saveLocked(&item); err != nil {
		return nil, fmt.Errorf("knowledge review was audited but state persistence failed: %w", err)
	}
	return cloneKnowledgeItem(&item)
}

func cloneKnowledgeItem(item *Item) (*Item, error) {
	data, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}
	var out Item
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SafeExtension preserves the existing model-facing knowledge schema but routes
// proposals through the crash-safe staging path. Models still cannot approve.
type SafeExtension struct {
	store *Store
}

func NewSafeExtension(store *Store) (*SafeExtension, error) {
	if store == nil {
		return nil, errors.New("knowledge store required")
	}
	return &SafeExtension{store: store}, nil
}

func (s *SafeExtension) ToolDefinitions() []provider.ToolDef {
	return s.store.ToolDefinitions()
}

func (s *SafeExtension) ExecuteTool(_ context.Context, _ string, name string, raw json.RawMessage) (string, error) {
	switch name {
	case "knowledge_search":
		var in struct {
			Query string `json:"query"`
			Scope string `json:"scope"`
			Limit int    `json:"limit"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return "", err
		}
		v, err := s.store.Search(in.Query, in.Scope, in.Limit)
		return marshal(v, err)
	case "knowledge_neighbors":
		var in struct {
			Entity string `json:"entity"`
			Scope  string `json:"scope"`
			Limit  int    `json:"limit"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return "", err
		}
		v, err := s.store.Neighbors(in.Entity, in.Scope, in.Limit)
		return marshal(v, err)
	case "knowledge_propose":
		var in Item
		if err := json.Unmarshal(raw, &in); err != nil {
			return "", err
		}
		v, err := s.store.CreateProposalSafe(in)
		return marshal(v, err)
	default:
		return "", errors.New("unknown knowledge tool")
	}
}
