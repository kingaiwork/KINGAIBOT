package orchestration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/authority"
	"github.com/kingaiwork/KINGAIBOT/internal/cluster"
	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/storage"
	"github.com/kingaiwork/KINGAIBOT/internal/workgraph"
)

const (
	bindingHeld        = "held"
	bindingActive      = "active"
	bindingReconciling = "reconciling"
	bindingCompleted   = "completed"
	bindingFailed      = "failed"
	maxBindings        = 10000
)

type Binding struct {
	ID          string    `json:"id"`
	GraphID     string    `json:"graph_id"`
	NodeID      string    `json:"node_id"`
	JobID       string    `json:"job_id"`
	AuthorityID string    `json:"authority_id"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type clusterSpec struct {
	Kind                 string          `json:"kind"`
	Payload              json.RawMessage `json:"payload,omitempty"`
	RequiredCapabilities []string        `json:"required_capabilities,omitempty"`
	RequiredDataScopes   []string        `json:"required_data_scopes,omitempty"`
	RequiredTool         string          `json:"required_tool,omitempty"`
	Priority             int             `json:"priority,omitempty"`
}

type Bridge struct {
	dir       string
	graphs    *workgraph.Store
	cluster   *cluster.Coordinator
	authority *authority.Store
	events    *eventlog.Log
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mu        sync.Mutex
}

func New(dir string, graphs *workgraph.Store, coordinator *cluster.Coordinator, authorities *authority.Store, events *eventlog.Log) (*Bridge, error) {
	if graphs == nil || coordinator == nil || authorities == nil || events == nil {
		return nil, errors.New("orchestration requires workgraph, cluster, authority and audit stores")
	}
	if err := os.MkdirAll(filepath.Join(dir, "bindings"), 0o700); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	b := &Bridge{dir: dir, graphs: graphs, cluster: coordinator, authority: authorities, events: events, ctx: ctx, cancel: cancel}
	if err := b.Recover(); err != nil {
		cancel()
		return nil, err
	}
	b.wg.Add(1)
	go b.syncLoop()
	return b, nil
}

func (b *Bridge) Close() {
	if b == nil {
		return
	}
	b.cancel()
	b.wg.Wait()
}

func (b *Bridge) syncLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			_ = b.Sync()
		}
	}
}

func bindingID(graphID, nodeID string) string {
	h := sha256.Sum256([]byte(graphID + "\x00" + nodeID))
	return "dispatch_" + hex.EncodeToString(h[:16])
}

func (b *Bridge) bindingPath(id string) (string, error) {
	if err := storage.ValidateID(id); err != nil {
		return "", err
	}
	return filepath.Join(b.dir, "bindings", id+".json"), nil
}

func (b *Bridge) saveBindingLocked(binding *Binding) error {
	if binding == nil {
		return errors.New("orchestration binding required")
	}
	if binding.ID == "" || binding.GraphID == "" || binding.NodeID == "" || binding.JobID == "" || binding.AuthorityID == "" {
		return errors.New("orchestration binding is incomplete")
	}
	switch binding.State {
	case bindingHeld, bindingActive, bindingReconciling, bindingCompleted, bindingFailed:
	default:
		return errors.New("orchestration binding has invalid state")
	}
	path, err := b.bindingPath(binding.ID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(binding, "", "  ")
	if err != nil {
		return err
	}
	return storage.AtomicWriteFile(path, data, 0o600)
}

func (b *Bridge) loadBindingLocked(id string) (*Binding, error) {
	path, err := b.bindingPath(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var binding Binding
	if err := json.Unmarshal(data, &binding); err != nil {
		return nil, err
	}
	if binding.ID != id {
		return nil, errors.New("orchestration binding identifier mismatch")
	}
	return &binding, nil
}

func (b *Bridge) listBindingsLocked() ([]*Binding, error) {
	entries, err := os.ReadDir(filepath.Join(b.dir, "bindings"))
	if err != nil {
		return nil, err
	}
	out := make([]*Binding, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		binding, loadErr := b.loadBindingLocked(id)
		if loadErr != nil {
			return nil, loadErr
		}
		out = append(out, binding)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func cloneBinding(binding *Binding) (*Binding, error) {
	data, err := json.Marshal(binding)
	if err != nil {
		return nil, err
	}
	var out Binding
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (b *Bridge) Binding(graphID, nodeID string) (*Binding, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.loadBindingLocked(bindingID(strings.TrimSpace(graphID), strings.TrimSpace(nodeID)))
}

func (b *Bridge) Bindings() ([]*Binding, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	bindings, err := b.listBindingsLocked()
	if err != nil {
		return nil, err
	}
	out := make([]*Binding, 0, len(bindings))
	for _, binding := range bindings {
		copy, cloneErr := cloneBinding(binding)
		if cloneErr != nil {
			return nil, cloneErr
		}
		out = append(out, copy)
	}
	return out, nil
}

func decodeClusterSpec(node *workgraph.Node) (clusterSpec, error) {
	if node == nil {
		return clusterSpec{}, errors.New("work node required")
	}
	raw, ok := node.Inputs["cluster"]
	if !ok {
		return clusterSpec{}, errors.New("execute/delegate node requires inputs.cluster")
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return clusterSpec{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var spec clusterSpec
	if err := dec.Decode(&spec); err != nil {
		return clusterSpec{}, fmt.Errorf("invalid inputs.cluster: %w", err)
	}
	if strings.TrimSpace(spec.Kind) == "" {
		return clusterSpec{}, errors.New("inputs.cluster.kind required")
	}
	return spec, nil
}

func clusterReplay(node *workgraph.Node) string {
	if node != nil && node.Replay == workgraph.ReplaySafe {
		return "safe"
	}
	return "manual"
}

// Dispatch creates a remote job that cannot be leased until the WorkGraph node
// is durably Running. The Agent/model cannot choose an authority identifier:
// authority is resolved from the node's trusted Owner identity.
func (b *Bridge) Dispatch(graphID, nodeID string) (*Binding, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	graphID, nodeID = strings.TrimSpace(graphID), strings.TrimSpace(nodeID)
	if err := storage.ValidateID(graphID); err != nil {
		return nil, err
	}
	if err := storage.ValidateID(nodeID); err != nil {
		return nil, err
	}
	id := bindingID(graphID, nodeID)
	if existing, err := b.loadBindingLocked(id); err == nil {
		return cloneBinding(existing)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if bindings, err := b.listBindingsLocked(); err != nil {
		return nil, err
	} else if len(bindings) >= maxBindings {
		return nil, errors.New("orchestration binding limit reached")
	}

	graph, err := b.graphs.Get(graphID)
	if err != nil {
		return nil, err
	}
	node, ok := graph.Nodes[nodeID]
	if !ok || node == nil {
		return nil, errors.New("workgraph node not found")
	}
	if node.State != workgraph.StateReady {
		return nil, errors.New("workgraph node is not ready")
	}
	if node.Type != workgraph.TypeExecute && node.Type != workgraph.TypeDelegate {
		return nil, errors.New("only execute or delegate nodes can dispatch to cluster")
	}
	owner := strings.TrimSpace(node.Owner)
	if owner == "" {
		return nil, errors.New("cluster-dispatched work node requires trusted owner")
	}
	grant, err := b.authority.ActiveForSubject(owner)
	if err != nil {
		return nil, fmt.Errorf("work node authority unavailable: %w", err)
	}
	spec, err := decodeClusterSpec(node)
	if err != nil {
		return nil, err
	}
	job, err := b.cluster.SubmitHeldAuthorized(cluster.Job{
		Kind:                 spec.Kind,
		Payload:              spec.Payload,
		RequiredCapabilities: spec.RequiredCapabilities,
		Priority:             spec.Priority,
		ReplayPolicy:         clusterReplay(node),
	}, grant.Envelope.ID, spec.RequiredDataScopes, spec.RequiredTool, id)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	binding := &Binding{ID: id, GraphID: graphID, NodeID: nodeID, JobID: job.ID, AuthorityID: grant.Envelope.ID, State: bindingHeld, CreatedAt: now, UpdatedAt: now}
	if err := b.saveBindingLocked(binding); err != nil {
		_, _ = b.cluster.CancelHeld(job.ID, id, "orchestration binding persistence failed")
		return nil, err
	}
	if err := b.events.Append(eventlog.Event{Type: "orchestration.dispatch.held", Data: map[string]any{"dispatch_id": id, "workgraph_id": graphID, "node_id": nodeID, "job_id": job.ID}}); err != nil {
		_, _ = b.cluster.CancelHeld(job.ID, id, "orchestration audit failed")
		path, _ := b.bindingPath(id)
		_ = os.Remove(path)
		return nil, fmt.Errorf("dispatch rolled back because audit failed: %w", err)
	}

	if _, err := b.graphs.Start(graphID, nodeID); err != nil {
		_, cancelErr := b.cluster.CancelHeld(job.ID, id, "workgraph start failed")
		if cancelErr != nil {
			return nil, fmt.Errorf("workgraph start failed and held job cancellation failed: start=%v cancel=%w", err, cancelErr)
		}
		binding.State = bindingFailed
		binding.UpdatedAt = time.Now().UTC()
		_ = b.saveBindingLocked(binding)
		return nil, err
	}

	activated, activateErr := b.cluster.ActivateHeld(job.ID, id)
	if activateErr != nil {
		current, getErr := b.cluster.Job(job.ID)
		if getErr == nil && current.Status == "held" {
			if _, cancelErr := b.cluster.CancelHeld(job.ID, id, "cluster activation failed"); cancelErr == nil {
				_, abortErr := b.graphs.AbortUnleased(graphID, nodeID, "cluster held job never activated")
				binding.State = bindingFailed
				binding.UpdatedAt = time.Now().UTC()
				_ = b.saveBindingLocked(binding)
				if abortErr != nil {
					return nil, fmt.Errorf("activation failed and workgraph rollback failed: activate=%v rollback=%w", activateErr, abortErr)
				}
				return nil, activateErr
			}
		}
		// We cannot prove that the job remained unleased. Keep the graph Running
		// and let recovery/synchronization reconcile the durable job state.
		return nil, fmt.Errorf("cluster activation outcome requires recovery: %w", activateErr)
	}
	if activated.Status != "queued" {
		return nil, errors.New("activated cluster job is not queued")
	}
	if err := b.setBindingStateLocked(binding, bindingActive, "orchestration.dispatch.activated"); err != nil {
		return nil, err
	}
	return cloneBinding(binding)
}

func (b *Bridge) setBindingStateLocked(binding *Binding, state, eventType string) error {
	if binding == nil {
		return errors.New("orchestration binding required")
	}
	original := *binding
	binding.State = state
	binding.UpdatedAt = time.Now().UTC()
	if err := b.saveBindingLocked(binding); err != nil {
		*binding = original
		return err
	}
	if err := b.events.Append(eventlog.Event{Type: eventType, Data: map[string]any{"dispatch_id": binding.ID, "workgraph_id": binding.GraphID, "node_id": binding.NodeID, "job_id": binding.JobID, "state": state}}); err != nil {
		*binding = original
		_ = b.saveBindingLocked(binding)
		return fmt.Errorf("orchestration state transition rolled back because audit failed: %w", err)
	}
	return nil
}

func (b *Bridge) Sync() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	bindings, err := b.listBindingsLocked()
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		if binding.State == bindingCompleted || binding.State == bindingFailed {
			continue
		}
		if err := b.syncBindingLocked(binding); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bridge) syncBindingLocked(binding *Binding) error {
	job, err := b.cluster.Job(binding.JobID)
	if err != nil {
		return err
	}
	graph, err := b.graphs.Get(binding.GraphID)
	if err != nil {
		return err
	}
	node, ok := graph.Nodes[binding.NodeID]
	if !ok || node == nil {
		return errors.New("orchestration binding references missing work node")
	}

	switch job.Status {
	case "held":
		return b.recoverHeldBindingLocked(binding, graph, node)
	case "queued", "leased":
		if node.State == workgraph.StateReconciling {
			if _, err := b.graphs.ResumeReconciliation(binding.GraphID, binding.NodeID); err != nil {
				return err
			}
		}
		if binding.State != bindingActive {
			return b.setBindingStateLocked(binding, bindingActive, "orchestration.dispatch.active")
		}
		return nil
	case "completing", "reconciliation":
		if node.State == workgraph.StateRunning {
			if _, err := b.graphs.RequireReconciliation(binding.GraphID, binding.NodeID, "cluster job requires reconciliation: "+job.Error); err != nil {
				return err
			}
		}
		if binding.State != bindingReconciling {
			return b.setBindingStateLocked(binding, bindingReconciling, "orchestration.dispatch.reconciling")
		}
		return nil
	case "completed":
		if node.State != workgraph.StateCompleted {
			outputs, evidence, err := clusterCompletion(job)
			if err != nil {
				return err
			}
			if _, err := b.graphs.Complete(binding.GraphID, binding.NodeID, outputs, evidence); err != nil {
				return err
			}
		}
		return b.setBindingStateLocked(binding, bindingCompleted, "orchestration.dispatch.completed")
	case "failed":
		if node.State != workgraph.StateFailed {
			if _, err := b.graphs.Fail(binding.GraphID, binding.NodeID, "cluster job failed: "+job.Error); err != nil {
				return err
			}
		}
		return b.setBindingStateLocked(binding, bindingFailed, "orchestration.dispatch.failed")
	default:
		return fmt.Errorf("unsupported cluster job state %q", job.Status)
	}
}

func clusterCompletion(job *cluster.Job) (map[string]any, []workgraph.Evidence, error) {
	if job == nil {
		return nil, nil, errors.New("cluster job required")
	}
	outputs := map[string]any{"cluster_job_id": job.ID}
	if len(job.Result) > 0 {
		var result any
		if err := json.Unmarshal(job.Result, &result); err != nil {
			return nil, nil, err
		}
		outputs["result"] = result
	}
	h := sha256.Sum256(job.Result)
	created := job.UpdatedAt
	if job.CompletedAt != nil {
		created = *job.CompletedAt
	}
	evidence := []workgraph.Evidence{{Kind: "cluster_job", Reference: job.ID, SHA256: hex.EncodeToString(h[:]), CreatedAt: created}}
	return outputs, evidence, nil
}

func (b *Bridge) recoverHeldBindingLocked(binding *Binding, graph *workgraph.Graph, node *workgraph.Node) error {
	switch node.State {
	case workgraph.StateReady:
		if _, err := b.graphs.Start(binding.GraphID, binding.NodeID); err != nil {
			return err
		}
	case workgraph.StateRunning:
		// Continue activation below.
	default:
		if _, err := b.cluster.CancelHeld(binding.JobID, binding.ID, "workgraph is no longer dispatchable"); err != nil {
			return err
		}
		return b.setBindingStateLocked(binding, bindingFailed, "orchestration.dispatch.canceled")
	}
	if _, err := b.cluster.ActivateHeld(binding.JobID, binding.ID); err != nil {
		return err
	}
	return b.setBindingStateLocked(binding, bindingActive, "orchestration.dispatch.recovered")
}

// Recover resolves durable held/active bindings after restart and also removes
// orphaned held jobs whose control reference belongs to this bridge but whose
// binding did not survive a crash before persistence.
func (b *Bridge) Recover() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	bindings, err := b.listBindingsLocked()
	if err != nil {
		return err
	}
	byID := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		byID[binding.ID] = struct{}{}
		if binding.State == bindingCompleted || binding.State == bindingFailed {
			continue
		}
		if err := b.syncBindingLocked(binding); err != nil {
			return err
		}
	}
	holds, err := b.cluster.HeldJobs()
	if err != nil {
		return err
	}
	for _, hold := range holds {
		if hold == nil || !strings.HasPrefix(hold.ControlRef, "dispatch_") {
			continue
		}
		if _, ok := byID[hold.ControlRef]; ok {
			continue
		}
		if _, err := b.cluster.CancelHeld(hold.JobID, hold.ControlRef, "orphaned orchestration hold recovered without durable binding"); err != nil {
			return err
		}
		if err := b.events.Append(eventlog.Event{Type: "orchestration.orphan_hold.canceled", Data: map[string]any{"dispatch_id": hold.ControlRef, "job_id": hold.JobID}}); err != nil {
			return err
		}
	}
	return nil
}
