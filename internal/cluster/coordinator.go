package cluster

import (
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
	ReplayPolicy         string          `json:"replay_policy"`
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

// RegisterWorker persists the credential verifier in a disabled state first.
// The worker is enabled only after the registration audit is durable, so a
// crash can never leave an unaudited credential usable after restart.
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
	w := Worker{ID: id, Name: name, Capabilities: normalizeCaps(caps), TokenPrefix: secret[:12], TokenHash: sha(token), Enabled: false, Metadata: metadata, CreatedAt: n, UpdatedAt: n, LastSeenAt: n}
	path, err := c.workerPath(id)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if err := save(path, &w); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	if err := c.events.Append(eventlog.Event{Type: "cluster.worker.registered", Data: map[string]any{"worker_id": id, "capabilities": w.Capabilities}}); err != nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("worker remains disabled because registration audit failed: %w", err)
	}
	w.Enabled = true
	w.UpdatedAt = time.Now().UTC()
	if err := save(path, &w); err != nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("worker was audited but enable persistence failed: %w", err)
	}
	c.mu.Unlock()
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

// SetWorkerEnabled orders trust transitions conservatively. Enabling is audited
// before persistence; disabling is persisted before audit and is never rolled
// back on audit failure because that would re-expand authority.
func (c *Coordinator) SetWorkerEnabled(id string, enabled bool) (*Worker, error) {
	path, err := c.workerPath(id)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var w Worker
	if err := read(path, &w); err != nil {
		return nil, err
	}
	if w.Enabled == enabled {
		w.TokenHash = ""
		return &w, nil
	}
	if enabled {
		if err := c.events.Append(eventlog.Event{Type: "cluster.worker.enabled", Data: map[string]any{"worker_id": id, "enabled": true}}); err != nil {
			return nil, fmt.Errorf("worker remains disabled because enable audit failed: %w", err)
		}
		w.Enabled = true
		w.UpdatedAt = time.Now().UTC()
		if err := save(path, &w); err != nil {
			return nil, fmt.Errorf("worker enable was audited but persistence failed: %w", err)
		}
	} else {
		w.Enabled = false
		w.UpdatedAt = time.Now().UTC()
		if err := save(path, &w); err != nil {
			return nil, err
		}
		if err := c.events.Append(eventlog.Event{Type: "cluster.worker.enabled", Data: map[string]any{"worker_id": id, "enabled": false}}); err != nil {
			return nil, fmt.Errorf("worker remains disabled but disable audit failed: %w", err)
		}
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
	in.ReplayPolicy = strings.ToLower(strings.TrimSpace(in.ReplayPolicy))
	if in.ReplayPolicy == "" {
		in.ReplayPolicy = "manual"
	}
	if in.ReplayPolicy != "manual" && in.ReplayPolicy != "safe" {
		return Job{}, errors.New("replay_policy must be manual or safe")
	}
	if in.Priority < -1000 {
		in.Priority = -1000
	}
	if in.Priority > 1000 {
		in.Priority = 1000
	}
	return in, nil
}

// Submit uses an inert pending_audit state until the submission audit is
// durable. LeaseJob only selects queued jobs, so neither a concurrent Worker
// nor a crash can expose unaudited work.
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
	in.ID, in.Status, in.CreatedAt, in.UpdatedAt = id, "pending_audit", n, n
	in.LeaseOwner, in.LeaseTokenHash, in.LeaseExpiresAt = "", "", nil
	path, err := c.jobPath(id)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := save(path, &in); err != nil {
		return nil, err
	}
	if err := c.events.Append(eventlog.Event{Type: "cluster.job.submitted", Data: map[string]any{"job_id": id, "kind": in.Kind, "required_capabilities": in.RequiredCapabilities, "replay_policy": in.ReplayPolicy}}); err != nil {
		in.Status = "failed"
		in.Error = "audit unavailable; job will not be leased"
		in.UpdatedAt = time.Now().UTC()
		_ = save(path, &in)
		return nil, fmt.Errorf("job disabled because submit audit failed: %w", err)
	}
	in.Status = "queued"
	in.UpdatedAt = time.Now().UTC()
	if err := save(path, &in); err != nil {
		return nil, fmt.Errorf("job was audited but queue activation persistence failed: %w", err)
	}
	return &in, nil
}

func (c *Coordinator) Jobs() ([]*Job, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.handleExpiredLocked(time.Now().UTC()); err != nil {
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
	for _, cap := range worker {
		set[cap] = struct{}{}
	}
	for _, cap := range required {
		if _, ok := set[cap]; !ok {
			return false
		}
	}
	return true
}

func (c *Coordinator) handleExpiredLocked(now time.Time) error {
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
		if j.Status != "leased" || j.LeaseExpiresAt == nil || j.LeaseExpiresAt.After(now) {
			continue
		}
		workerID := j.LeaseOwner
		j.Status = "reconciliation"
		j.Error = "lease expired; remote side effect outcome may be ambiguous"
		j.UpdatedAt = now
		if err := save(path, &j); err != nil {
			return err
		}
		decision := "manual_reconciliation"
		if j.ReplayPolicy == "safe" {
			decision = "safe_requeue"
		}
		if err := c.events.Append(eventlog.Event{Type: "cluster.job.lease_expired", Data: map[string]any{"job_id": j.ID, "worker_id": workerID, "attempt": j.Attempts, "decision": decision}}); err != nil {
			return fmt.Errorf("expired lease moved to reconciliation because audit failed: %w", err)
		}
		if j.ReplayPolicy == "safe" {
			j.Status, j.LeaseOwner, j.LeaseTokenHash, j.LeaseExpiresAt = "queued", "", "", nil
			j.Error = ""
			j.UpdatedAt = time.Now().UTC()
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
	if err := c.handleExpiredLocked(n); err != nil {
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
	if err := c.events.Append(eventlog.Event{Type: "cluster.job.leased", Data: map[string]any{"job_id": selected.ID, "worker_id": worker.ID, "attempt": selected.Attempts, "lease_expires_at": expires, "replay_policy": selected.ReplayPolicy}}); err != nil {
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
	now := time.Now().UTC()
	if j.Status != "leased" || j.LeaseOwner != worker.ID || j.LeaseExpiresAt == nil || !j.LeaseExpiresAt.After(now) || !constantHashEqual(leaseToken, j.LeaseTokenHash) {
		c.mu.Unlock()
		return nil, errors.New("invalid or expired lease")
	}
	j.Status = "completing"
	j.UpdatedAt = now
	if err := save(path, &j); err != nil {
		c.mu.Unlock()
		return nil, err
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
	if err := c.events.Append(eventlog.Event{Type: "cluster.job." + status, Data: map[string]any{"job_id": j.ID, "worker_id": worker.ID, "attempt": j.Attempts}}); err != nil {
		j.Status = "reconciliation"
		j.Error = "completion received but audit failed; manual reconciliation required"
		j.UpdatedAt = time.Now().UTC()
		c.mu.Lock()
		_ = save(path, &j)
		c.mu.Unlock()
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

func (c *Coordinator) Reconcile(jobID, action, note string, result json.RawMessage) (*Job, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "requeue" && action != "fail" && action != "complete" {
		return nil, errors.New("action must be requeue, fail or complete")
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
	if j.Status != "reconciliation" && j.Status != "completing" {
		c.mu.Unlock()
		return nil, errors.New("job is not awaiting reconciliation")
	}
	c.mu.Unlock()
	note = memory.SanitizeContent(strings.TrimSpace(note))
	if len(note) > 8192 {
		note = note[:8192]
	}
	if err := c.events.Append(eventlog.Event{Type: "cluster.job.reconciled", Data: map[string]any{"job_id": j.ID, "action": action, "note": note}}); err != nil {
		return nil, fmt.Errorf("reconciliation blocked because audit failed: %w", err)
	}
	n := time.Now().UTC()
	switch action {
	case "requeue":
		j.Status = "queued"
		j.Result = nil
		j.Error = ""
		j.CompletedAt = nil
	case "fail":
		j.Status = "failed"
		j.Error = note
		j.CompletedAt = &n
	case "complete":
		j.Status = "completed"
		j.Result = append(json.RawMessage(nil), result...)
		j.Error = ""
		j.CompletedAt = &n
	}
	j.LeaseOwner, j.LeaseTokenHash, j.LeaseExpiresAt = "", "", nil
	j.UpdatedAt = n
	c.mu.Lock()
	err = save(path, &j)
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return &j, nil
}
