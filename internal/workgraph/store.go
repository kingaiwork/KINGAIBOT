package workgraph

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

const (
	maxGraphsPerStore = 10000
	maxNodesPerGraph  = 256
)

// Store persists KINGAIBOT WorkGraphs independently from model context. Trust-
// expanding transitions are audited before they become durable; fail-closed
// transitions become durable before audit and are never rolled back into a more
// executable state merely because the audit sink is unhealthy.
type Store struct {
	dir    string
	events *eventlog.Log
	mu     sync.RWMutex
}

func NewStore(dir string, events *eventlog.Log) (*Store, error) {
	if events == nil {
		return nil, errors.New("workgraph store requires audit log")
	}
	if err := os.MkdirAll(filepath.Join(dir, "graphs"), 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir, events: events}, nil
}

func (s *Store) graphPath(id string) (string, error) {
	if err := storage.ValidateID(id); err != nil {
		return "", err
	}
	return filepath.Join(s.dir, "graphs", id+".json"), nil
}

func (s *Store) saveLocked(g *Graph) error {
	if g == nil {
		return errors.New("workgraph required")
	}
	if err := g.Validate(); err != nil {
		return err
	}
	path, err := s.graphPath(g.ID)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	return storage.AtomicWriteFile(path, b, 0o600)
}

func (s *Store) loadLocked(id string) (*Graph, error) {
	path, err := s.graphPath(id)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var g Graph
	if err := json.Unmarshal(b, &g); err != nil {
		return nil, err
	}
	if err := g.Validate(); err != nil {
		return nil, fmt.Errorf("stored workgraph is invalid: %w", err)
	}
	return &g, nil
}

func cloneGraph(g *Graph) (*Graph, error) {
	if g == nil {
		return nil, errors.New("workgraph required")
	}
	b, err := json.Marshal(g)
	if err != nil {
		return nil, err
	}
	var out Graph
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Store) countLocked() (int, error) {
	entries, err := os.ReadDir(filepath.Join(s.dir, "graphs"))
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

// Create accepts nodes in any order. Dependencies are resolved before each Add,
// and the request fails if the graph contains unknown dependencies or a cycle.
// The creation audit is durable before the graph becomes visible to dispatchers.
func (s *Store) Create(objective string, nodes []Node) (*Graph, error) {
	objective = strings.TrimSpace(objective)
	if objective == "" {
		return nil, errors.New("objective required")
	}
	if len(nodes) > maxNodesPerGraph {
		return nil, fmt.Errorf("workgraph exceeds %d nodes", maxNodesPerGraph)
	}
	id, err := storage.RandomID("wg")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	g, err := New(id, objective, now)
	if err != nil {
		return nil, err
	}

	remaining := append([]Node(nil), nodes...)
	for len(remaining) > 0 {
		progress := false
		next := remaining[:0]
		for _, node := range remaining {
			ready := true
			for _, dep := range node.DependsOn {
				if _, ok := g.Nodes[strings.TrimSpace(dep)]; !ok {
					ready = false
					break
				}
			}
			if !ready {
				next = append(next, node)
				continue
			}
			if err := g.Add(node, now); err != nil {
				return nil, err
			}
			progress = true
		}
		if !progress {
			return nil, errors.New("workgraph contains an unknown dependency or dependency cycle")
		}
		remaining = next
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if n, countErr := s.countLocked(); countErr != nil {
		return nil, countErr
	} else if n >= maxGraphsPerStore {
		return nil, errors.New("workgraph store limit reached")
	}
	if err := s.events.Append(eventlog.Event{Type: "workgraph.created", Data: map[string]any{"workgraph_id": g.ID, "nodes": len(g.Nodes)}}); err != nil {
		return nil, fmt.Errorf("workgraph creation blocked because audit failed: %w", err)
	}
	if err := s.saveLocked(g); err != nil {
		return nil, fmt.Errorf("workgraph creation was audited but persistence failed: %w", err)
	}
	return cloneGraph(g)
}

func (s *Store) Get(id string) (*Graph, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, err := s.loadLocked(id)
	if err != nil {
		return nil, err
	}
	return cloneGraph(g)
}

func (s *Store) List() ([]*Graph, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(filepath.Join(s.dir, "graphs"))
	if err != nil {
		return nil, err
	}
	out := make([]*Graph, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		g, loadErr := s.loadLocked(id)
		if loadErr != nil {
			return nil, loadErr
		}
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

type transitionFn func(*Graph) (map[string]any, error)

// transition applies one of two persistence orders:
//
//   - auditFirst=true for transitions that make work more executable or assert
//     successful completion. Audit must be durable before that state is visible.
//   - auditFirst=false for fail-closed transitions such as failed/reconciling.
//     The safer state is persisted first and is never rolled back to a more
//     executable state if audit persistence subsequently fails.
func (s *Store) transition(id, eventType string, auditFirst bool, fn transitionFn) (*Graph, error) {
	if fn == nil {
		return nil, errors.New("transition required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	original, err := s.loadLocked(id)
	if err != nil {
		return nil, err
	}
	candidate, err := cloneGraph(original)
	if err != nil {
		return nil, err
	}
	data, err := fn(candidate)
	if err != nil {
		return nil, err
	}
	if err := candidate.Validate(); err != nil {
		return nil, err
	}
	if data == nil {
		data = map[string]any{}
	}
	data["workgraph_id"] = candidate.ID
	if auditFirst {
		if err := s.events.Append(eventlog.Event{Type: eventType, Data: data}); err != nil {
			return nil, fmt.Errorf("workgraph transition blocked because audit failed: %w", err)
		}
		if err := s.saveLocked(candidate); err != nil {
			return nil, fmt.Errorf("workgraph transition was audited but persistence failed: %w", err)
		}
		return cloneGraph(candidate)
	}
	if err := s.saveLocked(candidate); err != nil {
		return nil, err
	}
	if err := s.events.Append(eventlog.Event{Type: eventType, Data: data}); err != nil {
		return nil, fmt.Errorf("workgraph fail-closed transition persisted but audit failed: %w", err)
	}
	return cloneGraph(candidate)
}

func (s *Store) Refresh(id string) (*Graph, error) {
	return s.transition(id, "workgraph.refreshed", true, func(g *Graph) (map[string]any, error) {
		ready := g.Refresh(time.Now().UTC())
		return map[string]any{"ready_nodes": ready}, nil
	})
}

func (s *Store) Approve(id, nodeID string) (*Graph, error) {
	return s.transition(id, "workgraph.node.approved", true, func(g *Graph) (map[string]any, error) {
		if err := g.Approve(nodeID, time.Now().UTC()); err != nil {
			return nil, err
		}
		return map[string]any{"node_id": nodeID}, nil
	})
}

func (s *Store) Start(id, nodeID string) (*Graph, error) {
	return s.transition(id, "workgraph.node.started", true, func(g *Graph) (map[string]any, error) {
		if err := g.Start(nodeID, time.Now().UTC()); err != nil {
			return nil, err
		}
		return map[string]any{"node_id": nodeID}, nil
	})
}

func (s *Store) AbortUnleased(id, nodeID, reason string) (*Graph, error) {
	return s.transition(id, "workgraph.node.dispatch_aborted", true, func(g *Graph) (map[string]any, error) {
		if err := g.AbortUnleasedStart(nodeID, reason, time.Now().UTC()); err != nil {
			return nil, err
		}
		return map[string]any{"node_id": nodeID, "reason": boundedReason(reason)}, nil
	})
}

func (s *Store) RequireReconciliation(id, nodeID, reason string) (*Graph, error) {
	return s.transition(id, "workgraph.node.reconciliation_required", false, func(g *Graph) (map[string]any, error) {
		if err := g.RequireReconciliation(nodeID, reason, time.Now().UTC()); err != nil {
			return nil, err
		}
		return map[string]any{"node_id": nodeID, "reason": boundedReason(reason)}, nil
	})
}

func (s *Store) ResumeReconciliation(id, nodeID string) (*Graph, error) {
	return s.transition(id, "workgraph.node.reconciliation_resumed", true, func(g *Graph) (map[string]any, error) {
		if err := g.ResumeReconciliation(nodeID, time.Now().UTC()); err != nil {
			return nil, err
		}
		return map[string]any{"node_id": nodeID}, nil
	})
}

func (s *Store) Fail(id, nodeID, reason string) (*Graph, error) {
	return s.transition(id, "workgraph.node.failed", false, func(g *Graph) (map[string]any, error) {
		if err := g.Fail(nodeID, reason, time.Now().UTC()); err != nil {
			return nil, err
		}
		return map[string]any{"node_id": nodeID, "reason": boundedReason(reason)}, nil
	})
}

func (s *Store) Complete(id, nodeID string, outputs map[string]any, evidence []Evidence) (*Graph, error) {
	return s.transition(id, "workgraph.node.completed", true, func(g *Graph) (map[string]any, error) {
		if err := g.Complete(nodeID, outputs, evidence, time.Now().UTC()); err != nil {
			return nil, err
		}
		return map[string]any{"node_id": nodeID, "evidence_count": len(evidence)}, nil
	})
}

func (s *Store) Ambiguous(id, nodeID, reason string) (*Graph, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, errors.New("reason required")
	}
	current, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	node, ok := current.Nodes[strings.TrimSpace(nodeID)]
	if !ok || node == nil {
		return nil, errors.New("work node not found")
	}
	// ReplaySafe moves Running back toward executable work and therefore audits
	// first. Manual/Never move into reconciliation/failed and persist fail-closed.
	auditFirst := node.Replay == ReplaySafe
	return s.transition(id, "workgraph.node.ambiguous", auditFirst, func(g *Graph) (map[string]any, error) {
		if err := g.AmbiguousSideEffect(nodeID, reason, time.Now().UTC()); err != nil {
			return nil, err
		}
		return map[string]any{"node_id": nodeID, "reason": reason}, nil
	})
}
