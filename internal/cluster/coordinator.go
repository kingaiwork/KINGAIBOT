package cluster

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

const (
	maxPayloadBytes  = 1 << 20
	maxResultBytes   = 4 << 20
	maxCapabilities  = 128
	defaultLeaseSecs = 120
)

type Worker struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Capabilities []string       `json:"capabilities,omitempty"`
	TokenPrefix  string         `json:"token_prefix"`
	TokenHash    string         `json:"token_hash"`
	Enabled      bool           `json:"enabled"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	LastSeenAt   time.Time      `json:"last_seen_at"`
}

type IssuedWorker struct {
	Worker
	Token string `json:"token"`
}

type Job struct {
	ID                   string          `json:"id"`
	Kind                 string          `json:"kind"`
	Payload              json.RawMessage `json:"payload"`
	RequiredCapabilities []string        `json:"required_capabilities,omitempty"`
	Priority             int             `json:"priority"`
	Status               string          `json:"status"`
	LeaseOwner           string          `json:"lease_owner,omitempty"`
	LeaseTokenHash       string          `json:"lease_token_hash,omitempty"`
	LeaseExpiresAt       *time.Time      `json:"lease_expires_at,omitempty"`
	Attempts             int             `json:"attempts"`
	Result               json.RawMessage `json:"result,omitempty"`
	Error                string          `json:"error,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
	CompletedAt          *time.Time      `json:"completed_at,omitempty"`
}

type Lease struct {
	Job        Job    `json:"job"`
	LeaseToken string `json:"lease_token"`
}

type Coordinator struct {
	dir    string
	events *eventlog.Log
	mu     sync.Mutex
}

func New(dir string, events *eventlog.Log) (*Coordinator, error) {
	if events == nil {
		return nil, errors.New("cluster coordinator requires audit log")
	}
	for _, name := range []string{"workers", "jobs"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o700); err != nil {
			return nil, err
		}
	}
	return &Coordinator{dir: dir, events: events}, nil
}

func idPath(dir, collection, id string) (string, error) {
	if err := storage.ValidateID(id); err != nil {
		return "", err
	}
	return filepath.Join(dir, collection, id+".json"), nil
}

func save(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return storage.AtomicWriteFile(path, b, 0o600)
}

func read(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func normalizeCaps(in []string) []string {
	if len(in) > maxCapabilities {
		in = in[:maxCapabilities]
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, cap := range in {
		cap = strings.ToLower(strings.TrimSpace(cap))
		if cap == "" || len(cap) > 128 {
			continue
		}
		if _, ok := seen[cap]; !ok {
			seen[cap] = struct{}{}
			out = append(out, cap)
		}
	}
	sort.Strings(out)
	return out
}

func randomSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", errors.New("secure random generator unavailable")
	}
	return hex.EncodeToString(b), nil
}

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func constantHashEqual(raw, expectedHash string) bool {
	got := sha(raw)
	return len(got) == len(expectedHash) && subtle.ConstantTimeCompare([]byte(got), []byte(expectedHash)) == 1
}

func (c *Coordinator) workerPath(id string) (string, error) { return idPath(c.dir, "workers", id) }
func (c *Coordinator) jobPath(id string) (string, error)    { return idPath(c.dir, "jobs", id) }

func (c *Coordinator) RegisterWorker(name string, caps []string, metadata map[string]any) (*IssuedWorker, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 {
		return nil, errors.New("worker name required and must be <= 128 bytes")
	}
	id, err := storage.RandomID("worker")
	if err != nil {
		return nil, err
	}
	secret, err := randomSecret(32)
	if err != nil {
		return nil, err
	}
	token := "kaw_" + id + "_" + secret
	n := time.Now().UTC()
	w := Worker{ID: id, Name: name, Capabilities: normalizeCaps(caps), TokenPrefix: secret[:12], TokenHash: sha(token), Enabled: true, Metadata: metadata, CreatedAt: n, UpdatedAt: n, LastSeenAt: n}
	path, err := c.workerPath(id)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	err = save(path, &w)
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if err := c.events.Append(eventlog.Event{Type: "cluster.worker.registered", Data: map[string]any{"worker_id": id, "capabilities": w.Capabilities}}); err != nil {
		w.Enabled = false
		w.UpdatedAt = time.Now().UTC()
		c.mu.Lock()
		_ = save(path, &w)
		c.mu.Unlock()
		return nil, fmt.Errorf("worker disabled because registration audit failed: %w", err)
	}
	return &IssuedWorker{Worker: w, Token: token}, nil
}

func parseWorkerID(token string) (string, error) {
	parts := strings.Split(token, "_")
	if len(parts) != 4 || parts[0] != "kaw" || parts[1] != "worker" {
		return "", errors.New("invalid worker token")
	}
	id := "worker_" + parts[2]
	if err := storage.ValidateID(id); err != nil {
		return "", errors.New("invalid worker token")
	}
	return id, nil
}

func bearer(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
}

func (c *Coordinator) AuthenticateWorker(token string) (*Worker, error) {
	id, err := parseWorkerID(token)
	if err != nil {
		return nil, err
	}
	path, err := c.workerPath(id)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var w Worker
	if err := read(path, &w); err != nil {
		return nil, errors.New("invalid worker token")
	}
	if !w.Enabled || !constantHashEqual(token, w.TokenHash) {
		return nil, errors.New("invalid worker token")
	}
	n := time.Now().UTC()
	w.LastSeenAt, w.UpdatedAt = n, n
	if err := save(path, &w); err != nil {
		return nil, err
	}
	return &w, nil
}

func (c *Coordinator) Workers() ([]*Worker, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(c.dir, "workers"))
	if err != nil {
		return nil, err
	}
	out := make([]*Worker, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		var w Worker
		if read(filepath.Join(c.dir, "workers", e.Name()), &w) == nil {
			w.TokenHash = ""
			out = append(out, &w)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (c *Coordinator) SetWorkerEnabled(id string, enabled bool) (*Worker, error) {
	path, err := c.workerPath(id)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	var w Worker
	if err := read(path, &w); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	w.Enabled, w.UpdatedAt = enabled, time.Now().UTC()
	if err := save(path, &w); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	c.mu.Unlock()
	if err := c.events.Append(eventlog.Event{Type: "cluster.worker.enabled", Data: map[string]any{"worker_id": id, "enabled": enabled}}); err != nil {
		if enabled {
			w.Enabled = false
			c.mu.Lock()
			_ = save(path, &w)
			c.mu.Unlock()
		}
		return nil, err
	}
	w.TokenHash = ""
	return &w, nil
}

func normalizeJob(in Job) (Job, error) {
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	if in.Kind == "" || len(in.Kind) > 128 {
		return Job{}, errors.New("job kind required and must be <= 128 bytes")
	}
	if len(in.Payload) == 0 {
		in.Payload = json.RawMessage(`{}`)
	}
	if len(in.Payload) > maxPayloadBytes || !json.Valid(in.Payload) {
		return Job{}, errors.New("job payload must be valid JSON <= 1 MiB")
	}
	in.RequiredCapabilities = normalizeCaps(in.RequiredCapabilities)
	if in.Priority < -1000 {
		in.Priority = -1000
	}
	if in.Priority > 1000 {
		in.Priority = 1000
	}
	return in, nil
}

func (c *Coordinator) Submit(in Job) (*Job, error) {
	in, err := normalizeJob(in)
	if err != nil {
		return nil, err
	}
	id, err := storage.RandomID("job")
	if err != nil {
		return nil, err
	}
	n := time.Now().UTC()
	in.ID, in.Status, in.CreatedAt, in.UpdatedAt = id, "queued", n, n
	in.LeaseOwner, in.LeaseTokenHash, in.LeaseExpiresAt = "", "", nil
	path, err := c.jobPath(id)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	err = save(path, &in)
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if err := c.events.Append(eventlog.Event{Type: "cluster.job.submitted", Data: map[string]any{"job_id": id, "kind": in.Kind, "required_capabilities": in.RequiredCapabilities}}); err != nil {
		in.Status = "failed"
		in.Error = "audit unavailable; job will not be leased"
		in.UpdatedAt = time.Now().UTC()
		c.mu.Lock()
		_ = save(path, &in)
		c.mu.Unlock()
		return nil, fmt.Errorf("job disabled because submit audit failed: %w", err)
	}
	return &in, nil
}

func (c *Coordinator) Jobs() ([]*Job, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.requeueExpiredLocked(time.Now().UTC()); err != nil {
		return nil, err
	}
	return c.jobsLocked()
}

func (c *Coordinator) jobsLocked() ([]*Job, error) {
	entries, err := os.ReadDir(filepath.Join(c.dir, "jobs"))
	if err != nil {
		return nil, err
	}
	out := make([]*Job, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		var j Job
		if read(filepath.Join(c.dir, "jobs", e.Name()), &j) == nil {
			j.LeaseTokenHash = ""
			out = append(out, &j)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority == out[j].Priority {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].Priority > out[j].Priority
	})
	return out, nil
}

func hasCaps(worker, required []string) bool {
	set := map[string]struct{}{}
	for _, c := range worker {
		set[c] = struct{}{}
	}
	for _, c := range required {
		if _, ok := set[c]; !ok {
			return false
		}
	}
	return true
}

func (c *Coordinator) requeueExpiredLocked(now time.Time) error {
	entries, err := os.ReadDir(filepath.Join(c.dir, "jobs"))
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(c.dir, "jobs", e.Name())
		var j Job
		if read(path, &j) != nil {
			continue
		}
		if j.Status == "leased" && j.LeaseExpiresAt != nil && !j.LeaseExpiresAt.After(now) {
			j.Status, j.LeaseOwner, j.LeaseTokenHash, j.LeaseExpiresAt = "queued", "", "", nil
			j.UpdatedAt = now
			if err := save(path, &j); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Coordinator) LeaseJob(worker *Worker, leaseSeconds int) (*Lease, error) {
	if worker == nil || !worker.Enabled {
		return nil, errors.New("enabled worker required")
	}
	if leaseSeconds <= 0 {
		leaseSeconds = defaultLeaseSecs
	}
	if leaseSeconds < 30 || leaseSeconds > 900 {
		return nil, errors.New("lease_seconds must be between 30 and 900")
	}
	n := time.Now().UTC()
	c.mu.Lock()
	if err := c.requeueExpiredLocked(n); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	jobs, err := c.jobsLocked()
	if err != nil {
		c.mu.Unlock()
		return nil, err
	}
	var selected *Job
	for _, j := range jobs {
		if j.Status == "queued" && hasCaps(worker.Capabilities, j.RequiredCapabilities) {
			selected = j
			break
		}
	}
	if selected == nil {
		c.mu.Unlock()
		return nil, os.ErrNotExist
	}
	secret, err := randomSecret(32)
	if err != nil {
		c.mu.Unlock()
		return nil, err
	}
	leaseToken := "lease_" + selected.ID + "_" + secret
	expires := n.Add(time.Duration(leaseSeconds) * time.Second)
	selected.Status = "leased"
	selected.LeaseOwner = worker.ID
	selected.LeaseTokenHash = sha(leaseToken)
	selected.LeaseExpiresAt = &expires
	selected.Attempts++
	selected.UpdatedAt = n
	path, err := c.jobPath(selected.ID)
	if err != nil {
		c.mu.Unlock()
		return nil, err
	}
	if err := save(path, selected); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	c.mu.Unlock()
	if err := c.events.Append(eventlog.Event{Type: "cluster.job.leased", Data: map[string]any{"job_id": selected.ID, "worker_id": worker.ID, "attempt": selected.Attempts, "lease_expires_at": expires}}); err != nil {
		selected.Status, selected.LeaseOwner, selected.LeaseTokenHash, selected.LeaseExpiresAt = "queued", "", "", nil
		selected.UpdatedAt = time.Now().UTC()
		c.mu.Lock()
		_ = save(path, selected)
		c.mu.Unlock()
		return nil, fmt.Errorf("lease reverted because audit failed: %w", err)
	}
	public := *selected
	public.LeaseTokenHash = ""
	return &Lease{Job: public, LeaseToken: leaseToken}, nil
}

func (c *Coordinator) Complete(worker *Worker, jobID, leaseToken string, result json.RawMessage, jobErr string) (*Job, error) {
	if worker == nil || !worker.Enabled {
		return nil, errors.New("enabled worker required")
	}
	if len(result) > maxResultBytes || (len(result) > 0 && !json.Valid(result)) {
		return nil, errors.New("result must be valid JSON <= 4 MiB")
	}
	path, err := c.jobPath(jobID)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	var j Job
	if err := read(path, &j); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	if j.Status != "leased" || j.LeaseOwner != worker.ID || j.LeaseExpiresAt == nil || !j.LeaseExpiresAt.After(time.Now().UTC()) || !constantHashEqual(leaseToken, j.LeaseTokenHash) {
		c.mu.Unlock()
		return nil, errors.New("invalid or expired lease")
	}
	c.mu.Unlock()

	status := "completed"
	if strings.TrimSpace(jobErr) != "" {
		status = "failed"
		jobErr = memory.SanitizeContent(jobErr)
		if len(jobErr) > 8192 {
			jobErr = jobErr[:8192]
		}
	}
	// Completion audit is written before the result becomes trusted terminal state.
	if err := c.events.Append(eventlog.Event{Type: "cluster.job." + status, Data: map[string]any{"job_id": j.ID, "worker_id": worker.ID, "attempt": j.Attempts}}); err != nil {
		return nil, fmt.Errorf("job completion requires reconciliation because audit failed: %w", err)
	}
	n := time.Now().UTC()
	j.Status, j.Result, j.Error, j.UpdatedAt, j.CompletedAt = status, append(json.RawMessage(nil), result...), jobErr, n, &n
	j.LeaseTokenHash, j.LeaseOwner, j.LeaseExpiresAt = "", "", nil
	c.mu.Lock()
	err = save(path, &j)
	c.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("job completion audited but terminal state persistence failed: %w", err)
	}
	return &j, nil
}

func (c *Coordinator) ToolDefinitions() []provider.ToolDef {
	return []provider.ToolDef{
		{Type: "function", Function: provider.FunctionDef{Name: "cluster_workers_list", Description: "List registered remote workers and their declared capabilities", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}},
		{Type: "function", Function: provider.FunctionDef{Name: "cluster_job_submit", Description: "Submit a durable capability-matched job to the remote worker queue", Parameters: map[string]any{"type": "object", "properties": map[string]any{"kind": map[string]any{"type": "string"}, "payload": map[string]any{}, "required_capabilities": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "priority": map[string]any{"type": "integer"}}, "required": []string{"kind"}}}},
		{Type: "function", Function: provider.FunctionDef{Name: "cluster_jobs_list", Description: "List durable remote jobs and lease state", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}},
	}
}

func (c *Coordinator) ExecuteTool(_ context.Context, _ string, name string, raw json.RawMessage) (string, error) {
	var v any
	var err error
	switch name {
	case "cluster_workers_list":
		v, err = c.Workers()
	case "cluster_jobs_list":
		v, err = c.Jobs()
	case "cluster_job_submit":
		var in struct {
			Kind                 string          `json:"kind"`
			Payload              json.RawMessage `json:"payload"`
			RequiredCapabilities []string        `json:"required_capabilities"`
			Priority             int             `json:"priority"`
		}
		if er := json.Unmarshal(raw, &in); er != nil {
			return "", er
		}
		v, err = c.Submit(Job{Kind: in.Kind, Payload: in.Payload, RequiredCapabilities: in.RequiredCapabilities, Priority: in.Priority})
	default:
		return "", errors.New("unknown cluster tool")
	}
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
