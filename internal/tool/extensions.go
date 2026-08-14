package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/kingaiwork/KINGAIBOT/internal/approval"
	"github.com/kingaiwork/KINGAIBOT/internal/memory"
	"github.com/kingaiwork/KINGAIBOT/internal/policy"
	"github.com/kingaiwork/KINGAIBOT/internal/provider"
	"github.com/kingaiwork/KINGAIBOT/internal/storage"
)

// Extension is a narrow capability boundary for platform-level tools.
// Extension implementations never bypass Registry policy, approval or audit.
type Extension interface {
	ToolDefinitions() []provider.ToolDef
	ExecuteTool(ctx context.Context, taskID, name string, args json.RawMessage) (string, error)
}

type extensionSet struct {
	mu   sync.RWMutex
	list []Extension
}

var extensions sync.Map // map[*Registry]*extensionSet

func (r *Registry) extensionSet() *extensionSet {
	v, _ := extensions.LoadOrStore(r, &extensionSet{})
	return v.(*extensionSet)
}

func (r *Registry) RegisterExtension(ext Extension) {
	if r == nil || ext == nil {
		return
	}
	s := r.extensionSet()
	s.mu.Lock()
	s.list = append(s.list, ext)
	s.mu.Unlock()
}

func (r *Registry) ExtensionDefinitions() []provider.ToolDef {
	if r == nil {
		return nil
	}
	s := r.extensionSet()
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []provider.ToolDef
	for _, ext := range s.list {
		out = append(out, ext.ToolDefinitions()...)
	}
	return out
}

func (r *Registry) AllDefinitions() []provider.ToolDef {
	out := append([]provider.ToolDef(nil), r.Definitions()...)
	out = append(out, r.ExtensionDefinitions()...)
	return out
}

func (r *Registry) findExtension(name string) Extension {
	s := r.extensionSet()
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ext := range s.list {
		for _, d := range ext.ToolDefinitions() {
			if d.Function.Name == name {
				return ext
			}
	}
	}
	return nil
}

// ExecuteAny executes built-in tools or registered extensions. Extensions pass
// through the same exact-action approval and hash-chained audit boundary.
func (r *Registry) ExecuteAny(ctx context.Context, taskID, name string, args json.RawMessage) (string, error) {
	ext := r.findExtension(name)
	if ext == nil {
		return r.Execute(ctx, taskID, name, args)
	}
	if !json.Valid(args) {
		return "", errors.New("tool arguments must be valid JSON")
	}
	argHash := approval.CanonicalArgumentsHash(args)
	decision := r.policy.Evaluate(name)
	if decision == policy.Deny {
		if err := r.audit("tool.denied", taskID, map[string]any{"tool": name, "arguments_hash": argHash, "extension": true}); err != nil {
			return "", fmt.Errorf("extension denied and audit write failed: %w", err)
		}
		return "", fmt.Errorf("tool %s denied by policy", name)
	}
	if decision != policy.Ask {
		if err := r.audit("tool.execution.requested", taskID, map[string]any{"tool": name, "arguments_hash": argHash, "decision": "allow", "extension": true}); err != nil {
			return "", fmt.Errorf("audit unavailable; extension execution blocked: %w", err)
		}
		result, execErr := ext.ExecuteTool(ctx, taskID, name, args)
		eventType := "tool.execution.completed"
		data := map[string]any{"tool": name, "arguments_hash": argHash, "extension": true}
		if execErr != nil {
			eventType = "tool.execution.failed"
			data["error"] = memory.SanitizeContent(execErr.Error())
		}
		if auditErr := r.audit(eventType, taskID, data); auditErr != nil {
			return result, fmt.Errorf("extension outcome could not be audited; action may have executed and requires reconciliation: %w", auditErr)
		}
		return result, execErr
	}

	a, err := r.approvals.FindMatching(taskID, name, args)
	if errors.Is(err, os.ErrNotExist) {
		approvalID, idErr := storage.RandomID("appr")
		if idErr != nil {
			return "", idErr
		}
		a = &approval.Approval{ID: approvalID, TaskID: taskID, Tool: name, Capability: "extension:" + name, Arguments: args, ArgumentsHash: argHash, Status: "pending"}
		if err = r.approvals.Save(a); err != nil {
			return "", err
		}
		if err = r.audit("tool.approval.requested", taskID, map[string]any{"approval_id": a.ID, "tool": name, "arguments_hash": argHash, "extension": true}); err != nil {
			return "", fmt.Errorf("approval created but audit write failed; execution blocked: %w", err)
		}
		return "", &ApprovalRequired{ApprovalID: a.ID}
	}
	if err != nil {
		return "", err
	}
	switch a.Status {
	case "pending":
		return "", &ApprovalRequired{ApprovalID: a.ID}
	case "denied":
		return "", errors.New("approval denied for this exact extension action")
	case "approved":
	default:
		return "", errors.New("invalid approval state")
	}

	switch a.ExecutionState {
	case "completed":
		if err = r.audit("tool.execution.replayed", taskID, map[string]any{"approval_id": a.ID, "tool": name, "arguments_hash": argHash, "cached": true, "extension": true}); err != nil {
			return "", fmt.Errorf("audit unavailable; cached extension result withheld: %w", err)
		}
		return a.Result, nil
	case "failed":
		if err = r.audit("tool.execution.replayed", taskID, map[string]any{"approval_id": a.ID, "tool": name, "arguments_hash": argHash, "cached_failure": true, "extension": true}); err != nil {
			return "", fmt.Errorf("audit unavailable; cached extension failure withheld: %w", err)
		}
		return a.Result, fmt.Errorf("previous approved extension execution failed: %s", a.ExecutionError)
	}
	if err = r.audit("tool.execution.requested", taskID, map[string]any{"approval_id": a.ID, "tool": name, "arguments_hash": argHash, "decision": "approved", "extension": true}); err != nil {
		return "", fmt.Errorf("audit unavailable; approved extension execution blocked: %w", err)
	}
	a, err = r.approvals.BeginExecution(a.ID)
	if err != nil {
		return "", err
	}
	switch a.ExecutionState {
	case "completed":
		return a.Result, nil
	case "failed":
		return a.Result, fmt.Errorf("previous approved extension execution failed: %s", a.ExecutionError)
	case "executing":
	default:
		return "", errors.New("invalid approved extension execution state")
	}

	result, execErr := ext.ExecuteTool(ctx, taskID, name, args)
	if finishErr := r.approvals.FinishExecution(a.ID, result, execErr); finishErr != nil {
		return result, fmt.Errorf("extension outcome could not be durably recorded; action may have executed and requires reconciliation: %w", finishErr)
	}
	eventType := "tool.execution.completed"
	data := map[string]any{"approval_id": a.ID, "tool": name, "arguments_hash": argHash, "extension": true}
	if execErr != nil {
		eventType = "tool.execution.failed"
		data["error"] = memory.SanitizeContent(execErr.Error())
	}
	if auditErr := r.audit(eventType, taskID, data); auditErr != nil {
		return result, fmt.Errorf("extension outcome was recorded but audit append failed; reconciliation required: %w", auditErr)
	}
	return result, execErr
}
