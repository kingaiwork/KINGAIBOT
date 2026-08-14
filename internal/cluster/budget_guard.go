package cluster

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
)

const maxCostUnitsPerAttempt int64 = 1_000_000_000_000

// AuthorityUsageController is an optional extension of AuthorityChecker. The
// authority package implements it, while tests or alternate policy engines may
// continue providing check-only implementations. Budget operations remain
// model-independent and are keyed by trusted Job/Authority identifiers.
type AuthorityUsageController interface {
	ReserveWork(authorityID, workID string) error
	ReleaseWork(authorityID, workID string) error
	ChargeCost(authorityID, chargeID string, units int64) error
	HasWorkReservation(authorityID, workID string) error
}

func normalizeCostUnits(units int64) (int64, error) {
	if units < 0 || units > maxCostUnitsPerAttempt {
		return 0, fmt.Errorf("cost_units must be between 0 and %d", maxCostUnitsPerAttempt)
	}
	return units, nil
}

func (c *Coordinator) authorityUsageController() AuthorityUsageController {
	checker := c.authorityChecker()
	if checker == nil {
		return nil
	}
	usage, _ := checker.(AuthorityUsageController)
	return usage
}

func (c *Coordinator) reserveAuthorityWork(binding *JobAuthorityBinding) error {
	usage := c.authorityUsageController()
	if usage == nil {
		return nil
	}
	if binding == nil || binding.JobID == "" || binding.AuthorityID == "" {
		return errors.New("authority budget reservation requires job binding")
	}
	if err := usage.ReserveWork(binding.AuthorityID, binding.JobID); err != nil {
		return fmt.Errorf("authority work budget denied: %w", err)
	}
	return nil
}

func (c *Coordinator) ensureAuthorityWorkReserved(binding *JobAuthorityBinding) error {
	usage := c.authorityUsageController()
	if usage == nil {
		return nil
	}
	if binding == nil || binding.JobID == "" || binding.AuthorityID == "" {
		return errors.New("authority budget reservation requires job binding")
	}
	if err := usage.HasWorkReservation(binding.AuthorityID, binding.JobID); err == nil {
		return nil
	}
	return c.reserveAuthorityWork(binding)
}

func (c *Coordinator) chargeAuthorityAttempt(job *Job, binding *JobAuthorityBinding) error {
	usage := c.authorityUsageController()
	if usage == nil {
		return nil
	}
	if job == nil || binding == nil || job.ID == "" || binding.AuthorityID == "" {
		return errors.New("authority cost charge requires job binding")
	}
	chargeID := fmt.Sprintf("%s.attempt.%d", job.ID, job.Attempts)
	if err := usage.ChargeCost(binding.AuthorityID, chargeID, binding.CostUnits); err != nil {
		return fmt.Errorf("authority cost budget denied: %w", err)
	}
	return nil
}

func (c *Coordinator) releaseAuthorityBindingWork(binding *JobAuthorityBinding) error {
	usage := c.authorityUsageController()
	if usage == nil {
		return nil
	}
	if binding == nil || binding.JobID == "" || binding.AuthorityID == "" {
		return errors.New("authority budget release requires job binding")
	}
	if err := usage.ReleaseWork(binding.AuthorityID, binding.JobID); err != nil {
		return fmt.Errorf("authority work budget release failed: %w", err)
	}
	return nil
}

func (c *Coordinator) releaseAuthorityWork(jobID string) error {
	if c.authorityUsageController() == nil {
		return nil
	}
	binding, err := c.loadAuthorityBinding(jobID)
	if err != nil {
		return err
	}
	return c.releaseAuthorityBindingWork(binding)
}

// SetHeldCost binds trusted per-attempt cost to an already-held job. The held
// state guarantees no Worker can observe the job while the control plane is
// attaching the budget. Cost may never be changed after activation.
func (c *Coordinator) SetHeldCost(jobID, controlRef string, costUnits int64) (*JobAuthorityBinding, error) {
	costUnits, err := normalizeCostUnits(costUnits)
	if err != nil {
		return nil, err
	}
	controlRef = strings.TrimSpace(controlRef)
	if controlRef == "" {
		return nil, errors.New("control_ref required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	job, hold, err := c.readHeldLocked(jobID)
	if err != nil {
		return nil, err
	}
	if hold.ControlRef != controlRef || job.Status != "held" {
		return nil, errors.New("held job control reference mismatch")
	}
	binding, err := c.loadAuthorityBinding(jobID)
	if err != nil {
		return nil, err
	}
	original := *binding
	binding.CostUnits = costUnits
	if err := c.saveAuthorityBinding(binding); err != nil {
		return nil, err
	}
	if err := c.events.Append(eventlog.Event{Type: "cluster.job.cost_bound", Data: map[string]any{
		"job_id":       jobID,
		"authority_id": binding.AuthorityID,
		"cost_units":   costUnits,
	}}); err != nil {
		_ = c.saveAuthorityBinding(&original)
		return nil, fmt.Errorf("held job cost binding rolled back because audit failed: %w", err)
	}
	return binding, nil
}

// reconcileTerminalAuthorityUsage heals conservative stale reservations left
// by a crash or an earlier cleanup failure. Only terminal jobs release slots;
// held, queued, leased, completing and reconciliation jobs continue consuming
// concurrency capacity.
func (c *Coordinator) reconcileTerminalAuthorityUsage() error {
	if c.authorityUsageController() == nil {
		return nil
	}
	c.mu.Lock()
	entries, err := os.ReadDir(filepath.Join(c.dir, "jobs"))
	if err != nil {
		c.mu.Unlock()
		return err
	}
	terminal := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var job Job
		if read(filepath.Join(c.dir, "jobs", entry.Name()), &job) != nil {
			continue
		}
		if job.Status == "completed" || job.Status == "failed" || job.Status == "cancelled" {
			terminal = append(terminal, job.ID)
		}
	}
	c.mu.Unlock()
	for _, jobID := range terminal {
		if err := c.releaseAuthorityWork(jobID); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (c *Coordinator) noteBudgetCleanupPending(jobID string, cleanupErr error) {
	if cleanupErr == nil {
		return
	}
	_ = c.events.Append(eventlog.Event{Type: "cluster.job.budget_cleanup_pending", Data: map[string]any{
		"job_id": jobID,
		"reason": memorySafeBudgetError(cleanupErr),
		"at":     time.Now().UTC(),
	}})
}

func memorySafeBudgetError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 2048 {
		message = message[:2048]
	}
	return message
}
