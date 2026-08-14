package cluster

import (
	"errors"
	"fmt"
)

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

func (c *Coordinator) releaseAuthorityWork(jobID string) error {
	usage := c.authorityUsageController()
	if usage == nil {
		return nil
	}
	binding, err := c.loadAuthorityBinding(jobID)
	if err != nil {
		return err
	}
	if err := usage.ReleaseWork(binding.AuthorityID, binding.JobID); err != nil {
		return fmt.Errorf("authority work budget release failed: %w", err)
	}
	return nil
}
