package cluster

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/memory"
	"github.com/kingaiwork/KINGAIBOT/internal/storage"
)

// HeldJob records the trusted control-plane reference that owns a staged job.
// A held job is durable but is deliberately invisible to LeaseJob until the
// control plane activates it after its surrounding WorkGraph transition is
// committed.
type HeldJob struct {
	JobID      string    `json:"job_id"`
	ControlRef string    `json:"control_ref"`
	CreatedAt  time.Time `json:"created_at"`
}

func (c *Coordinator) holdsDir() string {
	return filepath.Join(c.dir, "holds")
}

func (c *Coordinator) holdPath(jobID string) (string, error) {
	if err := storage.ValidateID(jobID); err != nil {
		return "", err
	}
	return filepath.Join(c.holdsDir(), jobID+".json"), nil
}

func normalizeControlRef(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 256 {
		return "", errors.New("control_ref required and must be <= 256 bytes")
	}
	return raw, nil
}

func (c *Coordinator) prepareHeldAuthorized(in Job, authorityID string, dataScopes []string, requiredTool string) (Job, *JobAuthorityBinding, error) {
	if c.authorityChecker() == nil {
		return Job{}, nil, errors.New("authority checker required for held job submission")
	}
	authorityID = strings.TrimSpace(authorityID)
	if authorityID == "" {
		return Job{}, nil, errors.New("authority_id required for held job submission")
	}
	normalized, err := normalizeJob(in)
	if err != nil {
		return Job{}, nil, err
	}
	dataScopes, err = normalizeDataScopes(dataScopes)
	if err != nil {
		return Job{}, nil, err
	}
	requiredTool = strings.TrimSpace(requiredTool)
	if len(requiredTool) > 256 {
		return Job{}, nil, errors.New("required_tool must be <= 256 bytes")
	}
	if len(normalized.RequiredCapabilities) == 0 && len(dataScopes) == 0 && requiredTool == "" {
		return Job{}, nil, errors.New("authority-bound job must declare a required capability, data scope or tool")
	}
	binding := &JobAuthorityBinding{
		AuthorityID:          authorityID,
		RequiredCapabilities: append([]string(nil), normalized.RequiredCapabilities...),
		RequiredDataScopes:   append([]string(nil), dataScopes...),
		RequiredTool:         requiredTool,
		CreatedAt:            time.Now().UTC(),
	}
	if err := c.authorizeBinding(binding); err != nil {
		return Job{}, nil, fmt.Errorf("cluster job authority denied: %w", err)
	}
	return normalized, binding, nil
}

// SubmitHeldAuthorized creates an authority-bound job in the held state. Held
// jobs are not leaseable. This is the first half of the KINGAIBOT orchestration
// handoff: callers persist their own graph/binding state before ActivateHeld.
func (c *Coordinator) SubmitHeldAuthorized(in Job, authorityID string, dataScopes []string, requiredTool, controlRef string) (*Job, error) {
	controlRef, err := normalizeControlRef(controlRef)
	if err != nil {
		return nil, err
	}
	in, binding, err := c.prepareHeldAuthorized(in, authorityID, dataScopes, requiredTool)
	if err != nil {
		return nil, err
	}
	id, err := storage.RandomID("job")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	in.ID, in.Status, in.CreatedAt, in.UpdatedAt = id, "held", now, now
	in.LeaseOwner, in.LeaseTokenHash, in.LeaseExpiresAt = "", "", nil
	binding.JobID = id
	hold := HeldJob{JobID: id, ControlRef: controlRef, CreatedAt: now}

	jobPath, err := c.jobPath(id)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(c.holdsDir(), 0o700); err != nil {
		return nil, err
	}
	holdPath, err := c.holdPath(id)
	if err != nil {
		return nil, err
	}
	bindingPath, err := c.authorityBindingPath(id)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if err := save(jobPath, &in); err != nil {
		return nil, err
	}
	if err := save(bindingPath, binding); err != nil {
		_ = os.Remove(jobPath)
		return nil, err
	}
	if err := save(holdPath, &hold); err != nil {
		_ = os.Remove(bindingPath)
		_ = os.Remove(jobPath)
		return nil, err
	}
	if err := c.events.Append(eventlog.Event{Type: "cluster.job.held", Data: map[string]any{
		"job_id":                id,
		"control_ref":           controlRef,
		"authority_id":          authorityID,
		"kind":                  in.Kind,
		"required_capabilities": binding.RequiredCapabilities,
		"required_data_scopes":  binding.RequiredDataScopes,
		"required_tool":         binding.RequiredTool,
	}}); err != nil {
		_ = os.Remove(holdPath)
		_ = os.Remove(bindingPath)
		_ = os.Remove(jobPath)
		return nil, fmt.Errorf("held job creation rolled back because audit failed: %w", err)
	}
	public := in
	public.LeaseTokenHash = ""
	return &public, nil
}

func (c *Coordinator) readHeldLocked(jobID string) (*Job, *HeldJob, error) {
	jobPath, err := c.jobPath(jobID)
	if err != nil {
		return nil, nil, err
	}
	holdPath, err := c.holdPath(jobID)
	if err != nil {
		return nil, nil, err
	}
	var job Job
	if err := read(jobPath, &job); err != nil {
		return nil, nil, err
	}
	var hold HeldJob
	if err := read(holdPath, &hold); err != nil {
		return nil, nil, err
	}
	if hold.JobID != job.ID || job.Status != "held" {
		return nil, nil, errors.New("job is not held")
	}
	return &job, &hold, nil
}

// ActivateHeld atomically makes a staged job leaseable only after the caller's
// surrounding durable state has been committed. The cluster mutex remains held
// through the audit append, so a Worker cannot race the activation audit.
func (c *Coordinator) ActivateHeld(jobID, controlRef string) (*Job, error) {
	controlRef, err := normalizeControlRef(controlRef)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	job, hold, err := c.readHeldLocked(jobID)
	if err != nil {
		return nil, err
	}
	if hold.ControlRef != controlRef {
		return nil, errors.New("held job control reference mismatch")
	}
	binding, err := c.loadAuthorityBinding(jobID)
	if err != nil {
		return nil, err
	}
	if err := c.authorizeBinding(binding); err != nil {
		return nil, fmt.Errorf("held job authority is no longer effective: %w", err)
	}
	jobPath, _ := c.jobPath(jobID)
	original := *job
	job.Status = "queued"
	job.UpdatedAt = time.Now().UTC()
	if err := save(jobPath, job); err != nil {
		return nil, err
	}
	if err := c.events.Append(eventlog.Event{Type: "cluster.job.activated", Data: map[string]any{"job_id": job.ID, "control_ref": controlRef}}); err != nil {
		_ = save(jobPath, &original)
		return nil, fmt.Errorf("held job activation rolled back because audit failed: %w", err)
	}
	// Hold-marker cleanup is bookkeeping only after queued state and activation
	// audit are durable. Failure to delete it must never be reported as a failed
	// activation because a Worker may now legitimately lease the queued job.
	holdPath, _ := c.holdPath(jobID)
	_ = os.Remove(holdPath)
	public := *job
	public.LeaseTokenHash = ""
	return &public, nil
}

// CancelHeld terminates a staged job that was never exposed to a Worker. This
// is used when the surrounding WorkGraph start cannot be committed.
func (c *Coordinator) CancelHeld(jobID, controlRef, reason string) (*Job, error) {
	controlRef, err := normalizeControlRef(controlRef)
	if err != nil {
		return nil, err
	}
	reason = memory.SanitizeContent(strings.TrimSpace(reason))
	if reason == "" {
		reason = "held orchestration dispatch canceled before activation"
	}
	if len(reason) > 8192 {
		reason = reason[:8192]
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	job, hold, err := c.readHeldLocked(jobID)
	if err != nil {
		return nil, err
	}
	if hold.ControlRef != controlRef {
		return nil, errors.New("held job control reference mismatch")
	}
	jobPath, _ := c.jobPath(jobID)
	original := *job
	now := time.Now().UTC()
	job.Status = "failed"
	job.Error = reason
	job.UpdatedAt = now
	job.CompletedAt = &now
	if err := save(jobPath, job); err != nil {
		return nil, err
	}
	if err := c.events.Append(eventlog.Event{Type: "cluster.job.held_canceled", Data: map[string]any{"job_id": job.ID, "control_ref": controlRef, "reason": reason}}); err != nil {
		_ = save(jobPath, &original)
		return nil, fmt.Errorf("held job cancel rolled back because audit failed: %w", err)
	}
	holdPath, _ := c.holdPath(jobID)
	_ = os.Remove(holdPath)
	public := *job
	public.LeaseTokenHash = ""
	return &public, nil
}

func (c *Coordinator) HeldJobs() ([]*HeldJob, error) {
	if err := os.MkdirAll(c.holdsDir(), 0o700); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entries, err := os.ReadDir(c.holdsDir())
	if err != nil {
		return nil, err
	}
	out := make([]*HeldJob, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		holdPath := filepath.Join(c.holdsDir(), entry.Name())
		var hold HeldJob
		if read(holdPath, &hold) != nil {
			continue
		}
		jobPath, pathErr := c.jobPath(hold.JobID)
		if pathErr != nil {
			continue
		}
		var job Job
		if read(jobPath, &job) != nil || job.Status != "held" {
			// Clean stale bookkeeping markers from already-activated/terminal jobs.
			_ = os.Remove(holdPath)
			continue
		}
		out = append(out, &hold)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// Job returns one public job snapshot and applies the same lease-expiry
// reconciliation rules used by Jobs before returning the state.
func (c *Coordinator) Job(id string) (*Job, error) {
	path, err := c.jobPath(id)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.handleExpiredLocked(time.Now().UTC()); err != nil {
		return nil, err
	}
	var job Job
	if err := read(path, &job); err != nil {
		return nil, err
	}
	job.LeaseTokenHash = ""
	return &job, nil
}
