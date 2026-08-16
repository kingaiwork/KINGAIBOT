package platform

import (
	"strings"
)

// ApplyCloudChannelRestrictions is intentionally one-way. Cloud policy may
// disable an already-enabled channel, but it cannot create, enable, reconfigure
// or grant authority to a channel. Local operators remain the authority source.
func (m *Manager) ApplyCloudChannelRestrictions(disabled []string) error {
	if m == nil || len(disabled) == 0 {
		return nil
	}
	deny := map[string]struct{}{}
	for _, value := range disabled {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			deny[value] = struct{}{}
		}
	}
	channels, err := m.Channels()
	if err != nil {
		return err
	}
	for _, channel := range channels {
		if channel == nil || !channel.Enabled {
			continue
		}
		_, byID := deny[strings.ToLower(channel.ID)]
		_, byName := deny[strings.ToLower(channel.Name)]
		_, byKind := deny[strings.ToLower(channel.Kind)]
		if !byID && !byName && !byKind {
			continue
		}
		if err := m.audit("channel.cloud_policy.disable", map[string]any{"channel_id": channel.ID, "kind": channel.Kind, "policy_direction": "contraction_only"}); err != nil {
			return err
		}
		if _, err := m.SetChannelEnabledSafe(channel.ID, false); err != nil {
			return err
		}
	}
	return nil
}
