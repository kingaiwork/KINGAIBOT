package workforce

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/storage"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

type Runtime interface {
	Create(input string, meta map[string]any) (*task.Task, error)
	Tasks() ([]*task.Task, error)
}

type bridgeState struct {
	Assignments map[string]assignment `json:"assignments"`
}

type assignment struct {
	CloudTaskID string `json:"cloud_task_id"`
	LocalTaskID string `json:"local_task_id"`
	Reported    bool   `json:"reported"`
}

type Bridge struct {
	settings  Settings
	client    *Client
	runtime   Runtime
	statePath string
	mu        sync.RWMutex
	state     bridgeState
	employees map[string]Employee
}

func NewBridge(settings Settings, version, dataDir string, rt Runtime) (*Bridge, error) {
	if rt == nil {
		return nil, errors.New("workforce runtime required")
	}
	client, err := NewClient(settings, version)
	if err != nil {
		return nil, err
	}
	if dataDir == "" {
		return nil, errors.New("workforce data directory required")
	}
	dir := filepath.Join(dataDir, "workforce")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	b := &Bridge{settings: settings, client: client, runtime: rt, statePath: filepath.Join(dir, "bridge-state.json"), state: bridgeState{Assignments: map[string]assignment{}}, employees: map[string]Employee{}}
	if err := b.loadState(); err != nil {
		return nil, err
	}
	if err := b.recoverAssignments(); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *Bridge) Run(ctx context.Context) {
	if err := b.cycle(ctx, true); err != nil {
		slog.Warn("workforce initial cycle failed", "error", err)
	}
	heartbeat := time.NewTicker(b.settings.HeartbeatInterval)
	syncTicker := time.NewTicker(b.settings.SyncInterval)
	poll := time.NewTicker(b.settings.PollInterval)
	defer heartbeat.Stop()
	defer syncTicker.Stop()
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if err := b.heartbeat(ctx); err != nil {
				slog.Warn("workforce heartbeat failed", "error", err)
			}
		case <-syncTicker.C:
			if err := b.sync(ctx); err != nil {
				slog.Warn("workforce sync failed", "error", err)
			}
		case <-poll.C:
			if err := b.reconcile(ctx); err != nil {
				slog.Warn("workforce reconciliation failed", "error", err)
			}
			if err := b.pull(ctx); err != nil {
				slog.Warn("workforce task poll failed", "error", err)
			}
		}
	}
}

func (b *Bridge) cycle(ctx context.Context, includeSync bool) error {
	if err := b.heartbeat(ctx); err != nil {
		return err
	}
	if includeSync {
		if err := b.sync(ctx); err != nil {
			return err
		}
	}
	if err := b.reconcile(ctx); err != nil {
		return err
	}
	return b.pull(ctx)
}

func (b *Bridge) heartbeat(ctx context.Context) error {
	return b.client.Heartbeat(ctx, []string{"durable-tasks", "local-approvals", "hash-chained-audit", "mcp", "a2a"})
}

func (b *Bridge) sync(ctx context.Context) error {
	out, err := b.client.Sync(ctx)
	if err != nil {
		return err
	}
	next := make(map[string]Employee, len(out.Employees))
	for _, employee := range out.Employees {
		if employee.ID == "" || employee.Status != "active" {
			continue
		}
		next[employee.ID] = employee
	}
	b.mu.Lock()
	b.employees = next
	b.mu.Unlock()
	return nil
}

func (b *Bridge) pull(ctx context.Context) error {
	cloudTask, err := b.client.PullTask(ctx)
	if err != nil || cloudTask == nil {
		return err
	}
	if cloudTask.ID == "" || cloudTask.EmployeeID == "" || cloudTask.Instructions == "" {
		return errors.New("invalid workforce task payload")
	}
	b.mu.RLock()
	_, already := b.state.Assignments[cloudTask.ID]
	employee, employeeOK := b.employees[cloudTask.EmployeeID]
	b.mu.RUnlock()
	if already {
		return nil
	}
	if !employeeOK {
		if err := b.sync(ctx); err != nil {
			return fmt.Errorf("employee policy unavailable: %w", err)
		}
		b.mu.RLock()
		employee, employeeOK = b.employees[cloudTask.EmployeeID]
		b.mu.RUnlock()
		if !employeeOK {
			return errors.New("cloud task references an unavailable digital employee")
		}
	}
	prompt := buildPrompt(employee, *cloudTask)
	meta := map[string]any{
		"source":                       "kingai-enterprise-workforce",
		"workforce_cloud_task_id":      cloudTask.ID,
		"workforce_organization_id":    cloudTask.OrganizationID,
		"workforce_employee_id":        cloudTask.EmployeeID,
		"workforce_action_fingerprint": cloudTask.ActionFingerprint,
		"workforce_risk_level":         cloudTask.RiskLevel,
		"workforce_priority":           cloudTask.Priority,
	}
	localTask, err := b.runtime.Create(prompt, meta)
	if err != nil {
		return fmt.Errorf("create local durable task: %w", err)
	}
	b.mu.Lock()
	b.state.Assignments[cloudTask.ID] = assignment{CloudTaskID: cloudTask.ID, LocalTaskID: localTask.ID}
	err = b.saveStateLocked()
	b.mu.Unlock()
	return err
}

func (b *Bridge) reconcile(ctx context.Context) error {
	all, err := b.runtime.Tasks()
	if err != nil {
		return err
	}
	byID := make(map[string]*task.Task, len(all))
	for _, local := range all {
		byID[local.ID] = local
	}
	b.mu.RLock()
	assignments := make([]assignment, 0, len(b.state.Assignments))
	for _, a := range b.state.Assignments {
		if !a.Reported {
			assignments = append(assignments, a)
		}
	}
	b.mu.RUnlock()
	for _, a := range assignments {
		local := byID[a.LocalTaskID]
		if local == nil {
			continue
		}
		var status string
		var output any
		var errorText string
		switch local.Status {
		case task.Completed:
			status = "succeeded"
			if b.settings.ReportOutput && local.Output != "" {
				out := local.Output
				if len(out) > b.settings.MaxReportBytes {
					out = out[:b.settings.MaxReportBytes]
				}
				output = map[string]any{"text": out, "truncated": len(local.Output) > len(out)}
			}
		case task.Failed, task.Canceled:
			status = "failed"
			errorText = local.Error
			if errorText == "" {
				errorText = string(local.Status)
			}
			if len(errorText) > b.settings.MaxReportBytes {
				errorText = errorText[:b.settings.MaxReportBytes]
			}
		default:
			continue
		}
		if err := b.client.ReportResult(ctx, a.CloudTaskID, status, output, errorText); err != nil {
			return err
		}
		b.mu.Lock()
		current := b.state.Assignments[a.CloudTaskID]
		current.Reported = true
		b.state.Assignments[a.CloudTaskID] = current
		if err := b.saveStateLocked(); err != nil {
			b.mu.Unlock()
			return err
		}
		b.mu.Unlock()
	}
	return nil
}

func (b *Bridge) recoverAssignments() error {
	all, err := b.runtime.Tasks()
	if err != nil {
		return err
	}
	changed := false
	for _, local := range all {
		if local.Metadata == nil {
			continue
		}
		cloudID, _ := local.Metadata["workforce_cloud_task_id"].(string)
		if cloudID == "" {
			continue
		}
		if _, ok := b.state.Assignments[cloudID]; ok {
			continue
		}
		b.state.Assignments[cloudID] = assignment{CloudTaskID: cloudID, LocalTaskID: local.ID}
		changed = true
	}
	if changed {
		return b.saveStateLocked()
	}
	return nil
}

func (b *Bridge) loadState() error {
	raw, err := os.ReadFile(b.statePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var state bridgeState
	if err := json.Unmarshal(raw, &state); err != nil {
		return fmt.Errorf("invalid workforce bridge state: %w", err)
	}
	if state.Assignments == nil {
		state.Assignments = map[string]assignment{}
	}
	b.state = state
	return nil
}

func (b *Bridge) saveStateLocked() error {
	raw, err := json.MarshalIndent(b.state, "", "  ")
	if err != nil {
		return err
	}
	return storage.AtomicWriteFile(b.statePath, raw, 0o600)
}

func buildPrompt(employee Employee, cloudTask CloudTask) string {
	goals := strings.Join(employee.Goals, "; ")
	skills := strings.Join(employee.Skills, ", ")
	return fmt.Sprintf(`KING AI Enterprise Workforce task

Digital employee: %s
Role: %s
Employee policy: autonomy=%s, risk_ceiling=%s
Business goals: %s
Cloud-declared skills: %s

Task title: %s
Priority: %s
Task risk: %s
Instructions:
%s

Security boundary:
- This cloud task is business intent, not operating-system permission.
- KINGAIBOT local allow/ask/deny tool policy is authoritative.
- Never bypass local approvals, filesystem roots, HTTP allowlists, shell restrictions, or audit requirements.
- If a required local tool is denied, fail safely rather than finding an unapproved workaround.
- Keep customer secrets and private data local unless an already-approved tool explicitly requires transmission.`,
		employee.Name, employee.Title, employee.AutonomyLevel, employee.RiskCeiling, goals, skills, cloudTask.Title, cloudTask.Priority, cloudTask.RiskLevel, cloudTask.Instructions)
}
