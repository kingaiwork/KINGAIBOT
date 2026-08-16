package platform

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kingaiwork/KINGAIBOT/internal/provider"
)

// V14Extension is the production model-facing platform extension. v1.7 keeps
// the existing tool schema but upgrades channel delivery to the native unified
// Channel Gateway while preserving the v1.4 mission execution semantics.
type V14Extension struct {
	manager *Manager
	safe    *SafeExtension
}

func NewV14Extension(manager *Manager) (*V14Extension, error) {
	safe, err := NewSafeExtension(manager)
	if err != nil {
		return nil, err
	}
	return &V14Extension{manager: manager, safe: safe}, nil
}

func (x *V14Extension) ToolDefinitions() []provider.ToolDef {
	return x.safe.ToolDefinitions()
}

func (x *V14Extension) ExecuteTool(ctx context.Context, taskID, name string, raw json.RawMessage) (string, error) {
	switch name {
	case "platform_mission_dispatch":
		var in Mission
		if err := json.Unmarshal(raw, &in); err != nil {
			return "", err
		}
		mission, err := x.manager.DispatchMissionV14(in)
		return marshalResult(mission, err)
	case "platform_channel_send":
		var in struct {
			ChannelID string `json:"channel_id"`
			Recipient string `json:"recipient"`
			Text      string `json:"text"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return "", err
		}
		channel, err := x.manager.channel(in.ChannelID)
		if err != nil {
			return "", err
		}
		if !channel.Enabled {
			return "", errors.New("channel disabled")
		}
		return x.manager.SendChannelV170(ctx, channel, in.Recipient, in.Text)
	default:
		return x.safe.ExecuteTool(ctx, taskID, name, raw)
	}
}
