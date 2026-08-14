package workgraph

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type NodeType string

type State string

type ReplayPolicy string

type Risk string

const (
	TypeThink     NodeType = "think"
	TypeRead      NodeType = "read"
	TypeTransform NodeType = "transform"
	TypeDecide    NodeType = "decide"
	TypeApprove   NodeType = "approve"
	TypeExecute   NodeType = "execute"
	TypeWait      NodeType = "wait"
	TypeDelegate  NodeType = "delegate"
	TypeVerify    NodeType = "verify"
	TypeReconcile NodeType = "reconcile"
	TypeReport    NodeType = "report"
)

const (
	StatePending      State = "pending"
	StateReady        State = "ready"
	StateRunning      State = "running"
	StateWaiting      State = "waiting"
	StateApproval     State = "approval_required"
	StateReconciling  State = "reconciling"
	StateCompleted    State = "completed"
	StateFailed       State = "failed"
	StateCancelled    State = "cancelled"
)

const (
	ReplayManual ReplayPolicy = "manual"
	ReplaySafe   ReplayPolicy = "safe"
	ReplayNever  ReplayPolicy = "never"
)

const (
	RiskLow      Risk = "low"
	RiskModerate Risk = "moderate"
	RiskHigh     Risk = "high"
	RiskCritical Risk = "critical"
)

type Evidence struct {
	Kind      string    `json:"kind"`
	Reference string    `json:"reference"`
	SHA256    string    `json:"sha256,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Node struct {
	ID               string         `json:"id"`
	Type             NodeType       `json:"type"`
	Owner            string         `json:"owner,omitempty"`
	DependsOn        []string       `json:"depends_on,omitempty"`
	State            State          `json:"state"`
	Risk             Risk           `json:"risk"`
	Replay           ReplayPolicy   `json:"replay"`
	RequiresApproval bool           `json:"requires_approval,omitempty"`
	RequiresEvidence bool           `json:"requires_evidence,omitempty"`
	Inputs           map[string]any `json:"inputs,omitempty"`
	Outputs          map[string]any `json:"outputs,omitempty"`
	Evidence         []Evidence     `json:"evidence,omitempty"`
	Error            string         `json:"error,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type Graph struct {
	ID        string           `json:"id"`
	Objective string           `json:"objective"`
	Nodes     map[string]*Node `json:"nodes"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

func New(id, objective string, now time.Time) (*Graph, error) {
	id = strings.TrimSpace(id)
	objective = strings.TrimSpace(objective)
	if id == "" {
		return nil, errors.New("workgraph requires id")
	}
	if objective == "" {
		return nil, errors.New("workgraph requires objective")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return &Graph{
		ID:        id,
		Objective: objective,
		Nodes:     map[string]*Node{},
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (g *Graph) Add(node Node, now time.Time) error {
	if g == nil {
		return errors.New("nil workgraph")
	}
	if g.Nodes == nil {
		g.Nodes = map[string]*Node{}
	}
	node.ID = strings.TrimSpace(node.ID)
	if node.ID == "" {
		return errors.New("work node requires id")
	}
	if _, exists := g.Nodes[node.ID]; exists {
		return fmt.Errorf("duplicate work node %q", node.ID)
	}
	if !validType(node.Type) {
		return fmt.Errorf("invalid work node type %q", node.Type)
	}
	if node.State == "" {
		node.State = StatePending
	}
	if node.Risk == "" {
		node.Risk = RiskLow
	}
	if node.Replay == "" {
		node.Replay = ReplayManual
	}
	if !validState(node.State) || !validRisk(node.Risk) || !validReplay(node.Replay) {
		return errors.New("work node contains invalid state, risk or replay policy")
	}
	if node.Risk == RiskHigh || node.Risk == RiskCritical {
		node.RequiresEvidence = true
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	node.DependsOn = canonical(node.DependsOn)
	node.CreatedAt = now
	node.UpdatedAt = now
	g.Nodes[node.ID] = &node
	if err := g.Validate(); err != nil {
		delete(g.Nodes, node.ID)
		return err
	}
	g.UpdatedAt = now
	return nil
}

func (g *Graph) Validate() error {
	if g == nil {
		return errors.New("nil workgraph")
	}
	if strings.TrimSpace(g.ID) == "" || strings.TrimSpace(g.Objective) == "" {
		return errors.New("workgraph requires id and objective")
	}
	for id, node := range g.Nodes {
		if node == nil {
			return fmt.Errorf("work node %q is nil", id)
		}
		if id != node.ID {
			return fmt.Errorf("work node key %q does not match id %q", id, node.ID)
		}
		if !validType(node.Type) || !validState(node.State) || !validRisk(node.Risk) || !validReplay(node.Replay) {
			return fmt.Errorf("work node %q has invalid enum value", id)
		}
		for _, dep := range node.DependsOn {
			if dep == id {
				return fmt.Errorf("work node %q cannot depend on itself", id)
			}
			if _, ok := g.Nodes[dep]; !ok {
				return fmt.Errorf("work node %q depends on unknown node %q", id, dep)
			}
		}
		if node.State == StateCompleted && node.RequiresEvidence && len(node.Evidence) == 0 {
			return fmt.Errorf("work node %q cannot complete without evidence", id)
		}
	}
	return g.validateAcyclic()
}

func (g *Graph) Refresh(now time.Time) []string {
	if g == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ready := make([]string, 0)
	for id, node := range g.Nodes {
		if node.State != StatePending {
			continue
		}
		depsDone := true
		for _, dep := range node.DependsOn {
			if g.Nodes[dep].State != StateCompleted {
				depsDone = false
				break
			}
		}
		if !depsDone {
			continue
		}
		if node.RequiresApproval {
			node.State = StateApproval
		} else {
			node.State = StateReady
			ready = append(ready, id)
		}
		node.UpdatedAt = now
	}
	sort.Strings(ready)
	g.UpdatedAt = now
	return ready
}

func (g *Graph) Approve(id string, now time.Time) error {
	node, err := g.node(id)
	if err != nil {
		return err
	}
	if node.State != StateApproval {
		return fmt.Errorf("work node %q is not awaiting approval", id)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	node.State = StateReady
	node.UpdatedAt = now
	g.UpdatedAt = now
	return nil
}

func (g *Graph) Start(id string, now time.Time) error {
	node, err := g.node(id)
	if err != nil {
		return err
	}
	if node.State != StateReady {
		return fmt.Errorf("work node %q is not ready", id)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	node.State = StateRunning
	node.UpdatedAt = now
	g.UpdatedAt = now
	return nil
}

func (g *Graph) Complete(id string, outputs map[string]any, evidence []Evidence, now time.Time) error {
	node, err := g.node(id)
	if err != nil {
		return err
	}
	if node.State != StateRunning && node.State != StateReconciling {
		return fmt.Errorf("work node %q is not running or reconciling", id)
	}
	if node.RequiresEvidence && len(evidence) == 0 {
		return fmt.Errorf("work node %q requires completion evidence", id)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	for i := range evidence {
		if strings.TrimSpace(evidence[i].Kind) == "" || strings.TrimSpace(evidence[i].Reference) == "" {
			return errors.New("evidence requires kind and reference")
		}
		if evidence[i].CreatedAt.IsZero() {
			evidence[i].CreatedAt = now
		}
	}
	node.Outputs = outputs
	node.Evidence = append([]Evidence(nil), evidence...)
	node.Error = ""
	node.State = StateCompleted
	node.UpdatedAt = now
	g.UpdatedAt = now
	return g.Validate()
}

// AmbiguousSideEffect prevents a caller from interpreting an uncertain external
// result as success. Replay-safe work may return to pending; all other work
// enters reconciliation or fails closed when replay is forbidden.
func (g *Graph) AmbiguousSideEffect(id string, reason string, now time.Time) error {
	node, err := g.node(id)
	if err != nil {
		return err
	}
	if node.State != StateRunning {
		return fmt.Errorf("work node %q is not running", id)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	node.Error = strings.TrimSpace(reason)
	switch node.Replay {
	case ReplaySafe:
		node.State = StatePending
	case ReplayManual:
		node.State = StateReconciling
	case ReplayNever:
		node.State = StateFailed
	default:
		return fmt.Errorf("work node %q has invalid replay policy", id)
	}
	node.UpdatedAt = now
	g.UpdatedAt = now
	return nil
}

func (g *Graph) node(id string) (*Node, error) {
	if g == nil {
		return nil, errors.New("nil workgraph")
	}
	node, ok := g.Nodes[strings.TrimSpace(id)]
	if !ok || node == nil {
		return nil, fmt.Errorf("unknown work node %q", id)
	}
	return node, nil
}

func (g *Graph) validateAcyclic() error {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	colors := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		switch colors[id] {
		case gray:
			return fmt.Errorf("workgraph contains dependency cycle at %q", id)
		case black:
			return nil
		}
		colors[id] = gray
		for _, dep := range g.Nodes[id].DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}
		colors[id] = black
		return nil
	}
	for id := range g.Nodes {
		if colors[id] == white {
			if err := visit(id); err != nil {
				return err
			}
		}
	}
	return nil
}

func canonical(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, raw := range values {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func validType(v NodeType) bool {
	switch v {
	case TypeThink, TypeRead, TypeTransform, TypeDecide, TypeApprove, TypeExecute, TypeWait, TypeDelegate, TypeVerify, TypeReconcile, TypeReport:
		return true
	default:
		return false
	}
}

func validState(v State) bool {
	switch v {
	case StatePending, StateReady, StateRunning, StateWaiting, StateApproval, StateReconciling, StateCompleted, StateFailed, StateCancelled:
		return true
	default:
		return false
	}
}

func validRisk(v Risk) bool {
	switch v {
	case RiskLow, RiskModerate, RiskHigh, RiskCritical:
		return true
	default:
		return false
	}
}

func validReplay(v ReplayPolicy) bool {
	switch v {
	case ReplayManual, ReplaySafe, ReplayNever:
		return true
	default:
		return false
	}
}
