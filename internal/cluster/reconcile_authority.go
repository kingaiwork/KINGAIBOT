package cluster

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ReconcileAuthorized preserves the human/admin ability to record an already
// observed remote outcome as complete/failed, but prevents re-launching work
// after its execution authority has expired or been revoked.
func (c *Coordinator) ReconcileAuthorized(jobID, action, note string, result json.RawMessage) (*Job, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "requeue" && c.authorityChecker() != nil {
		binding, err := c.loadAuthorityBinding(jobID)
		if err != nil {
			return nil, fmt.Errorf("requeue blocked because authority binding is unavailable: %w", err)
		}
		if err := c.authorizeBinding(binding); err != nil {
			return nil, fmt.Errorf("requeue blocked because execution authority is not effective: %w", err)
		}
	}
	return c.Reconcile(jobID, action, note, result)
}
