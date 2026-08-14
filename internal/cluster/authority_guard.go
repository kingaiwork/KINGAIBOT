package cluster

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/memory"
	"github.com/kingaiwork/KINGAIBOT/internal/storage"
)

// AuthorityChecker is intentionally small so the cluster execution layer does
// not depend on the authority package. The daemon wires the KINGAIBOT
// Capability Envelope store into this boundary.
type AuthorityChecker interface {
	Check(id, capability, dataScope, tool string) error
}

type JobAuthorityBinding struct {
	JobID                 string    `json:"job_id"`
	AuthorityID           string    `json:"authority_id"`
	RequiredCapabilities  []string  `json:"required_capabilities,omitempty"`
	RequiredDataScopes    []string  `json:"required_data_scopes,omitempty"`
	RequiredTool          string    `json:"required_tool,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
}

// AuthorizedJobRequest is the external job-submission shape when an authority
// checker is enabled. Job fields remain top-level for API compatibility.
type AuthorizedJobRequest struct {
	Job
	AuthorityID        string   `json:"authority_id,omitempty"`
	RequiredDataScopes []string `json:"required_data_scopes,omitempty"`
	RequiredTool       string   `json:"required_tool,omitempty"`
}

var coordinatorAuthority sync.Map

func (c *Coordinator) SetAuthorityChecker(checker AuthorityChecker) error {
	if c == nil {
		return errors.New("cluster coordinator required")
	}
	if checker == nil {
		coordinatorAuthority.Delete(c)
		return nil
	}
	if err := os.MkdirAll(filepath.Join(c.dir, "authority_bindings"), 0o700); err != nil {
		return err
	}
	coordinatorAuthority.Store(c, checker)
	return nil
}

func (c *Coordinator) authorityChecker() AuthorityChecker {
	if c == nil {
		return nil
	}
	v, ok := coordinatorAuthority.Load(c)
	if !ok {
		return nil
	}
	checker, _ := v.(AuthorityChecker)
	return checker
}

func (c *Coordinator) authorityBindingPath(jobID string) (string, error) {
	if err := storage.ValidateID(jobID); err != nil {
		return "", err
	}
	return filepath.Join(c.dir, "authority_bindings", jobID+".json"), nil
}

func normalizeDataScopes(in []string) ([]string, error) {
	if len(in) > maxCapabilities {
		return nil, fmt.Errorf("required_data_scopes exceeds %d entries", maxCapabilities)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" || len(v) > 256 {
			return nil, errors.New("required_data_scopes contains invalid value")
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out, nil
}

func (c *Coordinator) saveAuthorityBinding(binding *JobAuthorityBinding) error {
	if binding == nil {
		return errors.New("authority binding required")
	}
	path, err := c.authorityBindingPath(binding.JobID)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(binding, "", "  ")
	if err != nil {
		return err
	}
	return storage.AtomicWriteFile(path, b, 0o600)
}

func (c *Coordinator) loadAuthorityBinding(jobID string) (*JobAuthorityBinding, error) {
	path, err := c.authorityBindingPath(jobID)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var binding JobAuthorityBinding
	if err := json.Unmarshal(b, &binding); err != nil {
		return nil, err
	}
	if binding.JobID != jobID || strings.TrimSpace(binding.AuthorityID) == "" {
		return nil, errors.New("invalid job authority binding")
	}
	return &binding, nil
}

func (c *Coordinator) authorizeBinding(binding *JobAuthorityBinding) error {
	checker := c.authorityChecker()
	if checker == nil {
		return nil
	}
	if binding == nil || strings.TrimSpace(binding.AuthorityID) == "" {
		return errors.New("authority binding required")
	}
	for _, capability := range binding.RequiredCapabilities {
		if err := checker.Check(binding.AuthorityID, capability, "", ""); err != nil {
			return fmt.Errorf("capability %q denied: %w", capability, err)
		}
	}
	for _, scope := range binding.RequiredDataScopes {
		if err := checker.Check(binding.AuthorityID, "", scope, ""); err != nil {
			return fmt.Errorf("data scope %q denied: %w", scope, err)
		}
	}
	if binding.RequiredTool != "" {
		if err := checker.Check(binding.AuthorityID, "", "", binding.RequiredTool); err != nil {
			return fmt.Errorf("tool %q denied: %w", binding.RequiredTool, err)
		}
	}
	return nil
}

func (c *Coordinator) SubmitAuthorized(in Job, authorityID string, dataScopes []string, requiredTool string) (*Job, error) {
	checker := c.authorityChecker()
	if checker == nil {
		return c.Submit(in)
	}
	authorityID = strings.TrimSpace(authorityID)
	if authorityID == "" {
		return nil, errors.New("authority_id required for cluster job submission")
	}
	normalized, err := normalizeJob(in)
	if err != nil {
		return nil, err
	}
	dataScopes, err = normalizeDataScopes(dataScopes)
	if err != nil {
		return nil, err
	}
	requiredTool = strings.TrimSpace(requiredTool)
	if len(requiredTool) > 256 {
		return nil, errors.New("required_tool must be <= 256 bytes")
	}
	binding := &JobAuthorityBinding{
		AuthorityID:          authorityID,
		RequiredCapabilities: append([]string(nil), normalized.RequiredCapabilities...),
		RequiredDataScopes:   append([]string(nil), dataScopes...),
		RequiredTool:         requiredTool,
		CreatedAt:            time.Now().UTC(),
	}
	if err := c.authorizeBinding(binding); err != nil {
		return nil, fmt.Errorf("cluster job authority denied: %w", err)
	}
	job, err := c.Submit(normalized)
	if err != nil {
		return nil, err
	}
	binding.JobID = job.ID
	if err := c.saveAuthorityBinding(binding); err != nil {
		_ = c.failQueuedJob(job.ID, "authority binding persistence failed")
		return nil, fmt.Errorf("job disabled because authority binding failed: %w", err)
	}
	if err := c.events.Append(eventlog.Event{Type: "cluster.job.authority_bound", Data: map[string]any{"job_id": job.ID, "authority_id": authorityID, "required_capabilities": binding.RequiredCapabilities, "required_data_scopes": binding.RequiredDataScopes, "required_tool": binding.RequiredTool}}); err != nil {
		path, _ := c.authorityBindingPath(job.ID)
		_ = os.Remove(path)
		_ = c.failQueuedJob(job.ID, "authority binding audit failed")
		return nil, fmt.Errorf("job disabled because authority binding audit failed: %w", err)
	}
	return job, nil
}

func (c *Coordinator) failQueuedJob(jobID, reason string) error {
	path, err := c.jobPath(jobID)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var job Job
	if err := read(path, &job); err != nil {
		return err
	}
	if job.Status != "queued" {
		return nil
	}
	job.Status = "failed"
	job.Error = memory.SanitizeContent(reason)
	job.UpdatedAt = time.Now().UTC()
	now := job.UpdatedAt
	job.CompletedAt = &now
	return save(path, &job)
}

// LeaseJobAuthorized revalidates authority immediately before a worker sees a
// lease. Jobs whose authority was revoked or expired are failed without being
// disclosed to the worker, then the coordinator continues looking for work.
func (c *Coordinator) LeaseJobAuthorized(worker *Worker, leaseSeconds int) (*Lease, error) {
	if c.authorityChecker() == nil {
		return c.LeaseJob(worker, leaseSeconds)
	}
	for attempts := 0; attempts < 256; attempts++ {
		lease, err := c.LeaseJob(worker, leaseSeconds)
		if err != nil {
			return nil, err
		}
		binding, bindErr := c.loadAuthorityBinding(lease.Job.ID)
		if bindErr == nil {
			bindErr = c.authorizeBinding(binding)
		}
		if bindErr == nil {
			return lease, nil
		}
		_, completeErr := c.Complete(worker, lease.Job.ID, lease.LeaseToken, nil, "authority denied before lease delivery: "+bindErr.Error())
		if completeErr != nil {
			return nil, fmt.Errorf("unauthorized lease could not be failed closed: %w", completeErr)
		}
		if err := c.events.Append(eventlog.Event{Type: "cluster.job.authority_blocked", Data: map[string]any{"job_id": lease.Job.ID, "worker_id": worker.ID, "reason": bindErr.Error()}}); err != nil {
			return nil, fmt.Errorf("authority block audit failed: %w", err)
		}
	}
	return nil, os.ErrNotExist
}

// CompleteAuthorized revalidates authority before a remote result becomes a
// terminal success. If authority changed while the worker was executing, the
// result is retained as evidence and moved to reconciliation instead.
func (c *Coordinator) CompleteAuthorized(worker *Worker, jobID, leaseToken string, result json.RawMessage, jobErr string) (*Job, error) {
	if c.authorityChecker() == nil {
		return c.Complete(worker, jobID, leaseToken, result, jobErr)
	}
	binding, err := c.loadAuthorityBinding(jobID)
	if err == nil {
		err = c.authorizeBinding(binding)
	}
	if err == nil {
		return c.Complete(worker, jobID, leaseToken, result, jobErr)
	}
	return c.moveAuthorityLossToReconciliation(worker, jobID, leaseToken, result, err)
}

func (c *Coordinator) moveAuthorityLossToReconciliation(worker *Worker, jobID, leaseToken string, result json.RawMessage, authorityErr error) (*Job, error) {
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
	now := time.Now().UTC()
	c.mu.Lock()
	var job Job
	if err := read(path, &job); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	if job.Status != "leased" || job.LeaseOwner != worker.ID || job.LeaseExpiresAt == nil || !job.LeaseExpiresAt.After(now) || !constantHashEqual(leaseToken, job.LeaseTokenHash) {
		c.mu.Unlock()
		return nil, errors.New("invalid or expired lease")
	}
	reason := "authority changed before completion commit; manual reconciliation required"
	if authorityErr != nil {
		reason += ": " + authorityErr.Error()
	}
	reason = memory.SanitizeContent(reason)
	if len(reason) > 8192 {
		reason = reason[:8192]
	}
	job.Status = "reconciliation"
	job.Result = append(json.RawMessage(nil), result...)
	job.Error = reason
	job.UpdatedAt = now
	job.LeaseOwner, job.LeaseTokenHash, job.LeaseExpiresAt = "", "", nil
	if err := save(path, &job); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	c.mu.Unlock()
	if err := c.events.Append(eventlog.Event{Type: "cluster.job.authority_reconciliation", Data: map[string]any{"job_id": job.ID, "worker_id": worker.ID, "attempt": job.Attempts, "reason": reason}}); err != nil {
		return nil, fmt.Errorf("job remains in reconciliation but audit failed: %w", err)
	}
	return &job, nil
}
