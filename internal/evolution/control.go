package evolution

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/memory"
	"github.com/kingaiwork/KINGAIBOT/internal/provider"
	"github.com/kingaiwork/KINGAIBOT/internal/storage"
)

type Evaluation struct {
	ID         string         `json:"id"`
	ProposalID string         `json:"proposal_id"`
	Suite      string         `json:"suite"`
	Score      float64        `json:"score"`
	Passed     bool           `json:"passed"`
	Evidence   map[string]any `json:"evidence,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

type Decision struct {
	ID                string    `json:"id"`
	ProposalID        string    `json:"proposal_id"`
	Action            string    `json:"action"`
	Reason            string    `json:"reason"`
	ArtifactDigest    string    `json:"artifact_digest,omitempty"`
	SignatureVerified bool      `json:"signature_verified,omitempty"`
	HealthStatus      string    `json:"health_status,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type Snapshot struct {
	Proposal    *Proposal     `json:"proposal"`
	Evaluations []*Evaluation `json:"evaluations"`
	Decisions   []*Decision   `json:"decisions"`
}

type Controller struct {
	store  *Store
	events *eventlog.Log
	dir    string
	mu     sync.Mutex
}

func NewController(store *Store, events *eventlog.Log) (*Controller, error) {
	if store == nil || events == nil {
		return nil, errors.New("evolution controller requires store and audit log")
	}
	dir := filepath.Join(store.dir, ".control")
	for _, name := range []string{"evaluations", "decisions"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o700); err != nil {
			return nil, err
		}
	}
	return &Controller{store: store, events: events, dir: dir}, nil
}

func sanitizeAny(v any) any {
	switch x := v.(type) {
	case string:
		return memory.SanitizeContent(x)
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = sanitizeAny(x[i])
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = sanitizeAny(val)
		}
		return out
	default:
		return v
	}
}

func cleanText(s string, limit int) string {
	s = strings.TrimSpace(memory.SanitizeContent(s))
	if len(s) > limit {
		s = s[:limit]
	}
	return s
}

func validRisk(risk string) bool {
	switch risk {
	case "low", "medium", "high", "critical":
		return true
	default:
		return false
	}
}

func (c *Controller) Propose(p Proposal) (*Proposal, error) {
	p.Kind = strings.ToLower(cleanText(p.Kind, 128))
	p.Title = cleanText(p.Title, 512)
	p.Rationale = cleanText(p.Rationale, 32<<10)
	p.Risk = strings.ToLower(cleanText(p.Risk, 32))
	if p.Kind == "" || p.Title == "" || p.Rationale == "" {
		return nil, errors.New("kind, title and rationale are required")
	}
	if p.Risk == "" {
		p.Risk = "medium"
	}
	if !validRisk(p.Risk) {
		return nil, errors.New("risk must be low, medium, high or critical")
	}
	if p.Evidence != nil {
		if sanitized, ok := sanitizeAny(p.Evidence).(map[string]any); ok {
			p.Evidence = sanitized
		}
	}
	p.Status = "proposed"
	if err := c.store.Save(&p); err != nil {
		return nil, err
	}
	if err := c.events.Append(eventlog.Event{Type: "evolution.proposed", Data: map[string]any{"proposal_id": p.ID, "kind": p.Kind, "risk": p.Risk}}); err != nil {
		_, _ = c.store.Update(p.ID, func(x *Proposal) error {
			x.Status = "blocked_audit"
			return nil
		})
		return nil, fmt.Errorf("proposal blocked because audit append failed: %w", err)
	}
	return c.store.Get(p.ID)
}

func recordPath(dir, collection, id string) (string, error) {
	if err := storage.ValidateID(id); err != nil {
		return "", err
	}
	return filepath.Join(dir, collection, id+".json"), nil
}

func writeRecord(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return storage.AtomicWriteFile(path, b, 0o600)
}

func listRecords[T any](dir string) ([]*T, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]*T, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		b, er := os.ReadFile(filepath.Join(dir, entry.Name()))
		if er != nil {
			continue
		}
		var v T
		if json.Unmarshal(b, &v) == nil {
			out = append(out, &v)
		}
	}
	return out, nil
}

func (c *Controller) Evaluations(proposalID string) ([]*Evaluation, error) {
	all, err := listRecords[Evaluation](filepath.Join(c.dir, "evaluations"))
	if err != nil {
		return nil, err
	}
	out := make([]*Evaluation, 0)
	for _, e := range all {
		if proposalID == "" || e.ProposalID == proposalID {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (c *Controller) Decisions(proposalID string) ([]*Decision, error) {
	all, err := listRecords[Decision](filepath.Join(c.dir, "decisions"))
	if err != nil {
		return nil, err
	}
	out := make([]*Decision, 0)
	for _, d := range all {
		if proposalID == "" || d.ProposalID == proposalID {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (c *Controller) Snapshot(id string) (*Snapshot, error) {
	p, err := c.store.Get(id)
	if err != nil {
		return nil, err
	}
	evals, err := c.Evaluations(id)
	if err != nil {
		return nil, err
	}
	decisions, err := c.Decisions(id)
	if err != nil {
		return nil, err
	}
	return &Snapshot{Proposal: p, Evaluations: evals, Decisions: decisions}, nil
}

func (c *Controller) SubmitEvaluation(proposalID string, in Evaluation) (*Evaluation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, err := c.store.Get(proposalID)
	if err != nil {
		return nil, err
	}
	switch p.Status {
	case "proposed", "evaluation_failed", "review_required":
	default:
		return nil, fmt.Errorf("proposal status %q does not accept evaluation", p.Status)
	}
	in.Suite = cleanText(in.Suite, 512)
	if in.Suite == "" {
		return nil, errors.New("evaluation suite required")
	}
	if in.Score < 0 || in.Score > 1 {
		return nil, errors.New("evaluation score must be between 0 and 1")
	}
	if in.Evidence != nil {
		if sanitized, ok := sanitizeAny(in.Evidence).(map[string]any); ok {
			in.Evidence = sanitized
		}
	}
	id, err := storage.RandomID("eval")
	if err != nil {
		return nil, err
	}
	in.ID, in.ProposalID, in.CreatedAt = id, proposalID, time.Now().UTC()
	next := "evaluation_failed"
	if in.Passed {
		next = "review_required"
	}
	if err := c.events.Append(eventlog.Event{Type: "evolution.evaluated", Data: map[string]any{"proposal_id": proposalID, "evaluation_id": id, "suite": in.Suite, "score": in.Score, "passed": in.Passed, "next_status": next}}); err != nil {
		return nil, fmt.Errorf("evaluation blocked because audit failed: %w", err)
	}
	path, err := recordPath(c.dir, "evaluations", id)
	if err != nil {
		return nil, err
	}
	if err := writeRecord(path, &in); err != nil {
		return nil, err
	}
	if _, err := c.store.Update(proposalID, func(x *Proposal) error {
		x.Status = next
		return nil
	}); err != nil {
		return nil, fmt.Errorf("evaluation recorded but proposal status update failed: %w", err)
	}
	return &in, nil
}

func digestValid(d string) bool {
	if len(d) != 64 {
		return false
	}
	_, err := hex.DecodeString(d)
	return err == nil
}

func latestStageDigest(decisions []*Decision) string {
	for _, d := range decisions {
		if d.Action == "stage" && digestValid(d.ArtifactDigest) {
			return d.ArtifactDigest
		}
	}
	return ""
}

func (c *Controller) Decide(proposalID string, in Decision) (*Decision, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, err := c.store.Get(proposalID)
	if err != nil {
		return nil, err
	}
	in.Action = strings.ToLower(cleanText(in.Action, 32))
	in.Reason = cleanText(in.Reason, 8192)
	in.HealthStatus = strings.ToLower(cleanText(in.HealthStatus, 64))
	in.ArtifactDigest = strings.ToLower(strings.TrimSpace(in.ArtifactDigest))
	if in.Reason == "" {
		return nil, errors.New("decision reason required")
	}
	next := ""
	switch in.Action {
	case "approve":
		if p.Status != "review_required" {
			return nil, errors.New("approve requires review_required status")
		}
		evals, er := c.Evaluations(proposalID)
		if er != nil {
			return nil, er
		}
		passed := false
		for _, e := range evals {
			if e.Passed {
				passed = true
				break
			}
		}
		if !passed {
			return nil, errors.New("approve requires at least one passed evaluation")
		}
		next = "approved"
	case "reject":
		switch p.Status {
		case "proposed", "evaluation_failed", "review_required", "approved":
			next = "rejected"
		default:
			return nil, fmt.Errorf("cannot reject proposal in status %q", p.Status)
		}
	case "stage":
		if p.Status != "approved" {
			return nil, errors.New("stage requires approved status")
		}
		if !digestValid(in.ArtifactDigest) {
			return nil, errors.New("stage requires a 64-hex SHA-256 artifact_digest")
		}
		next = "staged"
	case "release":
		if p.Status != "staged" {
			return nil, errors.New("release requires staged status")
		}
		if !in.SignatureVerified || in.HealthStatus != "passed" {
			return nil, errors.New("release requires signature_verified=true and health_status=passed")
		}
		decisions, er := c.Decisions(proposalID)
		if er != nil {
			return nil, er
		}
		stagedDigest := latestStageDigest(decisions)
		if stagedDigest == "" || in.ArtifactDigest != stagedDigest {
			return nil, errors.New("release artifact_digest must match the staged artifact")
		}
		next = "released"
	case "rollback":
		if p.Status != "staged" && p.Status != "released" {
			return nil, errors.New("rollback requires staged or released status")
		}
		next = "rolled_back"
	default:
		return nil, errors.New("action must be approve, reject, stage, release or rollback")
	}
	id, err := storage.RandomID("decision")
	if err != nil {
		return nil, err
	}
	in.ID, in.ProposalID, in.CreatedAt = id, proposalID, time.Now().UTC()
	if err := c.events.Append(eventlog.Event{Type: "evolution.decision", Data: map[string]any{"proposal_id": proposalID, "decision_id": id, "action": in.Action, "from_status": p.Status, "to_status": next, "artifact_digest": in.ArtifactDigest, "signature_verified": in.SignatureVerified, "health_status": in.HealthStatus}}); err != nil {
		return nil, fmt.Errorf("decision blocked because audit failed: %w", err)
	}
	if _, err := c.store.Update(proposalID, func(x *Proposal) error {
		x.Status = next
		return nil
	}); err != nil {
		return nil, fmt.Errorf("decision audited but proposal status update failed: %w", err)
	}
	path, err := recordPath(c.dir, "decisions", id)
	if err != nil {
		return nil, err
	}
	if err := writeRecord(path, &in); err != nil {
		return nil, fmt.Errorf("proposal transitioned but decision sidecar persistence failed: %w", err)
	}
	return &in, nil
}

func (c *Controller) ToolDefinitions() []provider.ToolDef {
	return []provider.ToolDef{
		{Type: "function", Function: provider.FunctionDef{Name: "evolution_proposals_list", Description: "List controlled-evolution proposals and review status. This does not grant permission to change code or deploy releases.", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}},
		{Type: "function", Function: provider.FunctionDef{Name: "evolution_propose_improvement", Description: "Create an untrusted improvement proposal for evaluation and operator review. It cannot modify or deploy production code directly.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"kind": map[string]any{"type": "string"}, "title": map[string]any{"type": "string"}, "rationale": map[string]any{"type": "string"}, "risk": map[string]any{"type": "string", "enum": []string{"low", "medium", "high", "critical"}}, "evidence": map[string]any{"type": "object"}}, "required": []string{"kind", "title", "rationale"}}}},
	}
}

func (c *Controller) ExecuteTool(_ context.Context, _ string, name string, raw json.RawMessage) (string, error) {
	var value any
	var err error
	switch name {
	case "evolution_proposals_list":
		value, err = c.store.List()
	case "evolution_propose_improvement":
		var p Proposal
		if er := json.Unmarshal(raw, &p); er != nil {
			return "", er
		}
		value, err = c.Propose(p)
	default:
		return "", errors.New("unknown evolution tool")
	}
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (c *Controller) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/evolution/control/proposals", c.httpProposals)
	mux.HandleFunc("POST /v1/evolution/control/proposals", c.httpProposals)
	mux.HandleFunc("GET /v1/evolution/control/proposals/{id}", c.httpProposal)
	mux.HandleFunc("POST /v1/evolution/control/proposals/{id}/evaluations", c.httpEvaluation)
	mux.HandleFunc("POST /v1/evolution/control/proposals/{id}/decisions", c.httpDecision)
	return mux
}

func (c *Controller) httpProposals(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		v, err := c.store.List()
		controlWrite(w, v, err, http.StatusOK)
		return
	}
	var p Proposal
	if !controlDecode(w, r, &p) {
		return
	}
	v, err := c.Propose(p)
	controlWrite(w, v, err, http.StatusCreated)
}

func (c *Controller) httpProposal(w http.ResponseWriter, r *http.Request) {
	v, err := c.Snapshot(r.PathValue("id"))
	controlWrite(w, v, err, http.StatusOK)
}

func (c *Controller) httpEvaluation(w http.ResponseWriter, r *http.Request) {
	var in Evaluation
	if !controlDecode(w, r, &in) {
		return
	}
	v, err := c.SubmitEvaluation(r.PathValue("id"), in)
	controlWrite(w, v, err, http.StatusCreated)
}

func (c *Controller) httpDecision(w http.ResponseWriter, r *http.Request) {
	var in Decision
	if !controlDecode(w, r, &in) {
		return
	}
	v, err := c.Decide(r.PathValue("id"), in)
	controlWrite(w, v, err, http.StatusCreated)
}

func controlDecode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		controlJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json", "detail": err.Error()})
		return false
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			err = errors.New("only one JSON object is allowed")
		}
		controlJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json", "detail": err.Error()})
		return false
	}
	return true
}

func controlWrite(w http.ResponseWriter, v any, err error, success int) {
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		controlJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	controlJSON(w, success, v)
}

func controlJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
