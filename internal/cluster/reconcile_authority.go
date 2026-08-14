package cluster

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ReconcileAuthorized preserves the human/admin ability to record an already
// observed remote outcome as complete/failed, but prevents re-launching work
// after its execution authority has expired, been revoked, or has lost its
// hierarchical concurrency reservation.
func (c *Coordinator) ReconcileAuthorized(jobID, action, note string, result json.RawMessage) (*Job, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	var binding *JobAuthorityBinding
	if c.authorityChecker() != nil {
		var err error
		binding, err = c.loadAuthorityBinding(jobID)
		if err != nil {
			return nil, fmt.Errorf("reconciliation blocked because authority binding is unavailable: %w", err)
		}
		if action == "requeue" {
			if err := c.authorizeBinding(binding); err != nil {
				return nil, fmt.Errorf("requeue blocked because execution authority is not effective: %w", err)
			}
			if err := c.ensureAuthorityWorkReserved(binding); err != nil {
				return nil, fmt.Errorf("requeue blocked because execution budget is unavailable: %w", err)
			}
		}
	}
	job, err := c.Reconcile(jobID, action, note, result)
	if err != nil {
		return nil, err
	}
	if binding != nil && (action == "complete" || action == "fail") {
		if releaseErr := c.releaseAuthorityBindingWork(binding); releaseErr != nil {
			// The admin reconciliation already established terminal truth. Keep
			// that truth and leave stale capacity fail-closed for recovery.
			c.noteBudgetCleanupPending(jobID, releaseErr)
		}
	}
	return job, nil
}
