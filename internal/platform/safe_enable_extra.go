package platform

import (
	"errors"
	"fmt"
)

func (m *Manager) SetWorkflowEnabledSafe(id string, enabled bool) (*Workflow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var workflow Workflow
	if err := m.read("workflows", id, &workflow); err != nil {
		return nil, err
	}
	if workflow.Enabled == enabled {
		return &workflow, nil
	}
	if enabled {
		for _, step := range workflow.Steps {
			if step.AgentID == "" {
				continue
			}
			var agent AgentProfile
			if err := m.read("agents", step.AgentID, &agent); err != nil {
				return nil, err
			}
			if !agent.Enabled {
				return nil, fmt.Errorf("workflow agent %s disabled", step.AgentID)
			}
		}
		if err := m.audit("workflow.enabled", map[string]any{"workflow_id": id, "enabled": true}); err != nil {
			return nil, fmt.Errorf("workflow remains disabled because enable audit failed: %w", err)
		}
		workflow.Enabled = true
		workflow.UpdatedAt = now()
		if err := m.save("workflows", id, &workflow); err != nil {
			return nil, fmt.Errorf("workflow enable was audited but persistence failed: %w", err)
		}
		return &workflow, nil
	}
	workflow.Enabled = false
	workflow.UpdatedAt = now()
	if err := m.save("workflows", id, &workflow); err != nil {
		return nil, err
	}
	if err := m.audit("workflow.enabled", map[string]any{"workflow_id": id, "enabled": false}); err != nil {
		return nil, fmt.Errorf("workflow remains disabled but disable audit failed: %w", err)
	}
	return &workflow, nil
}

var _ = errors.New
