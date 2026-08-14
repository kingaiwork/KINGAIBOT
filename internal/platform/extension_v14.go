package platform

import (
	"context"
	"encoding/json"

	"github.com/kingaiwork/KINGAIBOT/internal/provider"
)

// V14Extension is a thin production overlay on the already-hardened
// SafeExtension. Only operations with newer v1.4 execution semantics are
// intercepted here; all other tools continue through the proven safe surface.
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
	if name == "platform_mission_dispatch" {
		var in Mission
		if err := json.Unmarshal(raw, &in); err != nil {
			return "", err
		}
		mission, err := x.manager.DispatchMissionV14(in)
		return marshalResult(mission, err)
	}
	return x.safe.ExecuteTool(ctx, taskID, name, raw)
}
