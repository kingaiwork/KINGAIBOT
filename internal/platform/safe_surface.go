package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/memory"
	"github.com/kingaiwork/KINGAIBOT/internal/provider"
	"github.com/kingaiwork/KINGAIBOT/internal/storage"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

// NewSafe creates the production control-plane manager with crash-safe schedule
// execution and workflow recovery. New remains available for compatibility and
// focused tests, while kingagentd uses this constructor.
func NewSafe(dir string, rt TaskRuntime, events *eventlog.Log) (*Manager, error) {
	if rt == nil {
		return nil, errors.New("platform requires task runtime")
	}
	if events == nil {
		return nil, errors.New("platform requires audit log")
	}
	for _, name := range []string{"agents", "sessions", "schedules", "workflows", "workflow-runs", "nodes", "plugins", "channels", "skills", "missions"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o700); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{dir: dir, rt: rt, events: events, ctx: ctx, cancel: cancel, running: map[string]bool{}, tick: defaultSchedulerTick, offline: defaultNodeOffline}
	m.wg.Add(1)
	go m.safeControlLoop()
	m.safeRecoverWorkflowRuns()
	return m, nil
}

func (m *Manager) safeControlLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.tick)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.safeTickSchedules()
			m.syncRunningMissions()
		}
	}
}

func (m *Manager) safeRecoverWorkflowRuns() {
	runs, err := m.WorkflowRuns()
	if err != nil {
		return
	}
	for _, run := range runs {
		if run.Status != "running" {
			continue
		}
		if err := m.audit("workflow.run.recovery_authorized", map[string]any{"workflow_id": run.WorkflowID, "run_id": run.ID}); err != nil {
			continue
		}
		m.startWorkflowRun(run.ID)
	}
}

func (m *Manager) CreateAgentSafe(a AgentProfile) (*AgentProfile, error) {
	name, err := cleanText(a.Name, maxNameLen, "name")
	if err != nil {
		return nil, err
	}
	if len(a.SystemPrompt) > maxPromptLen {
		return nil, errors.New("system_prompt exceeds limit")
	}
	id, err := storage.RandomID("agent")
	if err != nil {
		return nil, err
	}
	t := now()
	a.ID, a.Name, a.Enabled, a.CreatedAt, a.UpdatedAt = id, name, false, t, t
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.save("agents", id, &a); err != nil {
		return nil, err
	}
	if err := m.audit("agent.created", map[string]any{"agent_id": id, "name": name}); err != nil {
		return nil, fmt.Errorf("agent remains disabled because creation audit failed: %w", err)
	}
	a.Enabled = true
	a.UpdatedAt = now()
	if err := m.save("agents", id, &a); err != nil {
		return nil, fmt.Errorf("agent creation was audited but activation persistence failed: %w", err)
	}
	return &a, nil
}

func (m *Manager) SetAgentEnabledSafe(id string, enabled bool) (*AgentProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var a AgentProfile
	if err := m.read("agents", id, &a); err != nil {
		return nil, err
	}
	if a.Enabled == enabled {
		return &a, nil
	}
	if enabled {
		if err := m.audit("agent.enabled", map[string]any{"agent_id": id, "enabled": true}); err != nil {
			return nil, fmt.Errorf("agent remains disabled because enable audit failed: %w", err)
		}
		a.Enabled = true
		a.UpdatedAt = now()
		if err := m.save("agents", id, &a); err != nil {
			return nil, fmt.Errorf("agent enable was audited but persistence failed: %w", err)
		}
		return &a, nil
	}
	a.Enabled = false
	a.UpdatedAt = now()
	if err := m.save("agents", id, &a); err != nil {
		return nil, err
	}
	if err := m.audit("agent.enabled", map[string]any{"agent_id": id, "enabled": false}); err != nil {
		return nil, fmt.Errorf("agent remains disabled but disable audit failed: %w", err)
	}
	return &a, nil
}

func (m *Manager) SendSessionSafe(id, text string) (*task.Task, error) {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > maxPromptLen {
		return nil, errors.New("message required and must be within prompt limit")
	}
	m.mu.RLock()
	var session Session
	err := m.read("sessions", id, &session)
	m.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	if session.AgentID != "" {
		a, err := m.Agent(session.AgentID)
		if err != nil {
			return nil, err
		}
		if !a.Enabled {
			return nil, errors.New("agent disabled")
		}
	}
	h := sha256.Sum256([]byte(text))
	if err := m.audit("session.turn.authorized", map[string]any{"session_id": id, "agent_id": session.AgentID, "message_sha256": hex.EncodeToString(h[:])}); err != nil {
		return nil, fmt.Errorf("session task blocked because authorization audit failed: %w", err)
	}
	return m.SendSession(id, text)
}

func (m *Manager) CreateScheduleSafe(s Schedule) (*Schedule, error) {
	name, err := cleanText(s.Name, maxNameLen, "name")
	if err != nil {
		return nil, err
	}
	prompt, err := cleanText(s.Prompt, maxPromptLen, "prompt")
	if err != nil {
		return nil, err
	}
	if s.IntervalSeconds < 60 || s.IntervalSeconds > 31*24*3600 {
		return nil, errors.New("interval_seconds must be between 60 and 2678400")
	}
	if s.AgentID != "" {
		a, err := m.Agent(s.AgentID)
		if err != nil {
			return nil, err
		}
		if !a.Enabled {
			return nil, errors.New("schedule agent disabled")
		}
	}
	id, err := storage.RandomID("sched")
	if err != nil {
		return nil, err
	}
	t := now()
	s.ID, s.Name, s.Prompt, s.Enabled, s.CreatedAt, s.UpdatedAt = id, name, prompt, false, t, t
	if s.NextRunAt.IsZero() || s.NextRunAt.Before(t) {
		s.NextRunAt = t.Add(time.Duration(s.IntervalSeconds) * time.Second)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.save("schedules", id, &s); err != nil {
		return nil, err
	}
	if err := m.audit("schedule.created", map[string]any{"schedule_id": id, "interval_seconds": s.IntervalSeconds, "agent_id": s.AgentID}); err != nil {
		return nil, fmt.Errorf("schedule remains disabled because creation audit failed: %w", err)
	}
	s.Enabled = true
	s.UpdatedAt = now()
	if err := m.save("schedules", id, &s); err != nil {
		return nil, fmt.Errorf("schedule creation was audited but activation persistence failed: %w", err)
	}
	return &s, nil
}

func (m *Manager) SetScheduleEnabledSafe(id string, enabled bool) (*Schedule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var s Schedule
	if err := m.read("schedules", id, &s); err != nil {
		return nil, err
	}
	if s.Enabled == enabled {
		return &s, nil
	}
	if enabled {
		if s.AgentID != "" {
			var a AgentProfile
			if err := m.read("agents", s.AgentID, &a); err != nil {
				return nil, err
			}
			if !a.Enabled {
				return nil, errors.New("schedule agent disabled")
			}
		}
		if err := m.audit("schedule.enabled", map[string]any{"schedule_id": id, "enabled": true}); err != nil {
			return nil, fmt.Errorf("schedule remains disabled because enable audit failed: %w", err)
		}
		s.Enabled = true
		if s.NextRunAt.Before(now()) {
			s.NextRunAt = now().Add(time.Duration(s.IntervalSeconds) * time.Second)
		}
		s.UpdatedAt = now()
		if err := m.save("schedules", id, &s); err != nil {
			return nil, fmt.Errorf("schedule enable was audited but persistence failed: %w", err)
		}
		return &s, nil
	}
	s.Enabled = false
	s.UpdatedAt = now()
	if err := m.save("schedules", id, &s); err != nil {
		return nil, err
	}
	if err := m.audit("schedule.enabled", map[string]any{"schedule_id": id, "enabled": false}); err != nil {
		return nil, fmt.Errorf("schedule remains disabled but disable audit failed: %w", err)
	}
	return &s, nil
}

func (m *Manager) CreateWorkflowSafe(w Workflow) (*Workflow, error) {
	name, err := cleanText(w.Name, maxNameLen, "name")
	if err != nil {
		return nil, err
	}
	if len(w.Steps) == 0 || len(w.Steps) > maxWorkflowSteps {
		return nil, fmt.Errorf("workflow requires 1-%d steps", maxWorkflowSteps)
	}
	for i := range w.Steps {
		if w.Steps[i].ID == "" {
			w.Steps[i].ID = fmt.Sprintf("step_%02d", i+1)
		}
		if err := storage.ValidateID(w.Steps[i].ID); err != nil {
			return nil, fmt.Errorf("step %d: %w", i+1, err)
		}
		if _, err := cleanText(w.Steps[i].Prompt, maxPromptLen, "step prompt"); err != nil {
			return nil, err
		}
		if w.Steps[i].AgentID != "" {
			if _, err := m.Agent(w.Steps[i].AgentID); err != nil {
				return nil, fmt.Errorf("step %d agent: %w", i+1, err)
			}
		}
	}
	id, err := storage.RandomID("wf")
	if err != nil {
		return nil, err
	}
	t := now()
	w.ID, w.Name, w.Enabled, w.CreatedAt, w.UpdatedAt = id, name, false, t, t
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.save("workflows", id, &w); err != nil {
		return nil, err
	}
	if err := m.audit("workflow.created", map[string]any{"workflow_id": id, "steps": len(w.Steps)}); err != nil {
		return nil, fmt.Errorf("workflow remains disabled because creation audit failed: %w", err)
	}
	w.Enabled = true
	w.UpdatedAt = now()
	if err := m.save("workflows", id, &w); err != nil {
		return nil, fmt.Errorf("workflow creation was audited but activation persistence failed: %w", err)
	}
	return &w, nil
}

func (m *Manager) RunWorkflowSafe(id string) (*WorkflowRun, error) {
	w, err := m.Workflow(id)
	if err != nil {
		return nil, err
	}
	if !w.Enabled {
		return nil, errors.New("workflow disabled")
	}
	for _, step := range w.Steps {
		if step.AgentID == "" {
			continue
		}
		a, err := m.Agent(step.AgentID)
		if err != nil || !a.Enabled {
			return nil, fmt.Errorf("workflow agent %s unavailable", step.AgentID)
		}
	}
	rid, err := storage.RandomID("wfrun")
	if err != nil {
		return nil, err
	}
	t := now()
	run := &WorkflowRun{ID: rid, WorkflowID: id, Status: "running", CreatedAt: t, UpdatedAt: t}
	if err := m.audit("workflow.run.created", map[string]any{"workflow_id": id, "run_id": rid}); err != nil {
		return nil, fmt.Errorf("workflow run blocked because audit failed: %w", err)
	}
	m.mu.Lock()
	err = m.save("workflow-runs", rid, run)
	m.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("workflow run was audited but persistence failed: %w", err)
	}
	m.startWorkflowRun(rid)
	return run, nil
}

func (m *Manager) CreateNodeSafe(n Node) (*Node, error) {
	name, err := cleanText(n.Name, maxNameLen, "name")
	if err != nil {
		return nil, err
	}
	if n.Endpoint != "" {
		if err := validateRemoteURL(n.Endpoint, n.AllowInsecureHTTP); err != nil {
			return nil, err
		}
	}
	id, err := storage.RandomID("node")
	if err != nil {
		return nil, err
	}
	t := now()
	n.ID, n.Name, n.Online, n.CreatedAt, n.UpdatedAt = id, name, false, t, t
	n.LastSeenAt = time.Time{}
	if err := m.audit("node.registered", map[string]any{"node_id": id, "platform": n.Platform}); err != nil {
		return nil, fmt.Errorf("node registration blocked because audit failed: %w", err)
	}
	m.mu.Lock()
	err = m.save("nodes", id, &n)
	m.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("node registration was audited but persistence failed: %w", err)
	}
	return &n, nil
}

func (m *Manager) HeartbeatNodeSafe(id string, metadata map[string]any) (*Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n Node
	if err := m.read("nodes", id, &n); err != nil {
		return nil, err
	}
	if err := m.audit("node.heartbeat", map[string]any{"node_id": id, "metadata_present": metadata != nil}); err != nil {
		return nil, fmt.Errorf("node heartbeat blocked because audit failed: %w", err)
	}
	t := now()
	n.LastSeenAt, n.UpdatedAt, n.Online = t, t, true
	if metadata != nil {
		n.Metadata = metadata
	}
	if err := m.save("nodes", id, &n); err != nil {
		return nil, fmt.Errorf("node heartbeat was audited but persistence failed: %w", err)
	}
	return &n, nil
}

// NodesSafe only demotes stale nodes. Listing nodes must never promote an
// offline registration to Online; only an audited heartbeat can do that.
func (m *Manager) NodesSafe() ([]*Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out, err := listJSON[Node](filepath.Join(m.dir, "nodes"))
	if err != nil {
		return nil, err
	}
	t := now()
	for _, n := range out {
		if n.Online && (n.LastSeenAt.IsZero() || t.Sub(n.LastSeenAt) > m.offline) {
			n.Online = false
			n.UpdatedAt = t
			if err := m.save("nodes", n.ID, n); err != nil {
				return nil, err
			}
			_ = m.audit("node.offline", map[string]any{"node_id": n.ID, "reason": "heartbeat_timeout"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *Manager) nodeForAction(id string) (*Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n Node
	if err := m.read("nodes", id, &n); err != nil {
		return nil, err
	}
	if !n.Online || n.LastSeenAt.IsZero() || now().Sub(n.LastSeenAt) > m.offline {
		if n.Online {
			n.Online = false
			n.UpdatedAt = now()
			_ = m.save("nodes", id, &n)
			_ = m.audit("node.offline", map[string]any{"node_id": id, "reason": "heartbeat_timeout"})
		}
		return nil, errors.New("node offline")
	}
	if n.Endpoint == "" {
		return nil, errors.New("node has no remote endpoint")
	}
	return &n, nil
}

func (m *Manager) CreatePluginSafe(p Plugin) (*Plugin, error) {
	name, err := cleanText(p.Name, maxNameLen, "name")
	if err != nil {
		return nil, err
	}
	if err := validateRemoteURL(p.Endpoint, p.AllowInsecureHTTP); err != nil {
		return nil, err
	}
	if p.Version == "" {
		p.Version = "1"
	}
	id, err := storage.RandomID("plugin")
	if err != nil {
		return nil, err
	}
	digest, err := canonicalSHA(map[string]any{"name": name, "version": p.Version, "endpoint": p.Endpoint, "capabilities": p.Capabilities})
	if err != nil {
		return nil, err
	}
	t := now()
	p.ID, p.Name, p.Enabled, p.ManifestSHA256, p.CreatedAt, p.UpdatedAt = id, name, false, digest, t, t
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.save("plugins", id, &p); err != nil {
		return nil, err
	}
	if err := m.audit("plugin.registered", map[string]any{"plugin_id": id, "sha256": digest}); err != nil {
		return nil, fmt.Errorf("plugin remains disabled because registration audit failed: %w", err)
	}
	p.Enabled = true
	p.UpdatedAt = now()
	if err := m.save("plugins", id, &p); err != nil {
		return nil, fmt.Errorf("plugin registration was audited but activation persistence failed: %w", err)
	}
	return &p, nil
}

func (m *Manager) SetPluginEnabledSafe(id string, enabled bool) (*Plugin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var p Plugin
	if err := m.read("plugins", id, &p); err != nil {
		return nil, err
	}
	if p.Enabled == enabled {
		return &p, nil
	}
	if enabled {
		if err := m.audit("plugin.enabled", map[string]any{"plugin_id": id, "enabled": true}); err != nil {
			return nil, fmt.Errorf("plugin remains disabled because enable audit failed: %w", err)
		}
		p.Enabled = true
		p.UpdatedAt = now()
		if err := m.save("plugins", id, &p); err != nil {
			return nil, fmt.Errorf("plugin enable was audited but persistence failed: %w", err)
		}
		return &p, nil
	}
	p.Enabled = false
	p.UpdatedAt = now()
	if err := m.save("plugins", id, &p); err != nil {
		return nil, err
	}
	if err := m.audit("plugin.enabled", map[string]any{"plugin_id": id, "enabled": false}); err != nil {
		return nil, fmt.Errorf("plugin remains disabled but disable audit failed: %w", err)
	}
	return &p, nil
}

func (m *Manager) CreateChannelSafe(c Channel) (*Channel, error) {
	name, err := cleanText(c.Name, maxNameLen, "name")
	if err != nil {
		return nil, err
	}
	if err := validateRemoteURL(c.Endpoint, c.AllowInsecureHTTP); err != nil {
		return nil, err
	}
	if c.Kind == "" {
		c.Kind = "webhook"
	}
	id, err := storage.RandomID("chan")
	if err != nil {
		return nil, err
	}
	t := now()
	c.ID, c.Name, c.Enabled, c.CreatedAt, c.UpdatedAt = id, name, false, t, t
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.save("channels", id, &c); err != nil {
		return nil, err
	}
	if err := m.audit("channel.registered", map[string]any{"channel_id": id, "kind": c.Kind}); err != nil {
		return nil, fmt.Errorf("channel remains disabled because registration audit failed: %w", err)
	}
	c.Enabled = true
	c.UpdatedAt = now()
	if err := m.save("channels", id, &c); err != nil {
		return nil, fmt.Errorf("channel registration was audited but activation persistence failed: %w", err)
	}
	return &c, nil
}

func (m *Manager) SetChannelEnabledSafe(id string, enabled bool) (*Channel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var c Channel
	if err := m.read("channels", id, &c); err != nil {
		return nil, err
	}
	if c.Enabled == enabled {
		return &c, nil
	}
	if enabled {
		if err := m.audit("channel.enabled", map[string]any{"channel_id": id, "enabled": true}); err != nil {
			return nil, fmt.Errorf("channel remains disabled because enable audit failed: %w", err)
		}
		c.Enabled = true
		c.UpdatedAt = now()
		if err := m.save("channels", id, &c); err != nil {
			return nil, fmt.Errorf("channel enable was audited but persistence failed: %w", err)
		}
		return &c, nil
	}
	c.Enabled = false
	c.UpdatedAt = now()
	if err := m.save("channels", id, &c); err != nil {
		return nil, err
	}
	if err := m.audit("channel.enabled", map[string]any{"channel_id": id, "enabled": false}); err != nil {
		return nil, fmt.Errorf("channel remains disabled but disable audit failed: %w", err)
	}
	return &c, nil
}

func (m *Manager) CreateSkillSafe(s Skill) (*Skill, error) {
	name, err := cleanText(s.Name, maxNameLen, "name")
	if err != nil {
		return nil, err
	}
	instructions, err := cleanText(s.Instructions, maxPromptLen, "instructions")
	if err != nil {
		return nil, err
	}
	id, err := storage.RandomID("skill")
	if err != nil {
		return nil, err
	}
	digest, err := canonicalSHA(map[string]any{"name": name, "instructions": instructions, "tools": s.Tools})
	if err != nil {
		return nil, err
	}
	t := now()
	s.ID, s.Name, s.Instructions, s.Enabled, s.ContentSHA256, s.CreatedAt, s.UpdatedAt = id, name, instructions, false, digest, t, t
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.save("skills", id, &s); err != nil {
		return nil, err
	}
	if err := m.audit("skill.created", map[string]any{"skill_id": id, "sha256": digest}); err != nil {
		return nil, fmt.Errorf("skill remains disabled because creation audit failed: %w", err)
	}
	s.Enabled = true
	s.UpdatedAt = now()
	if err := m.save("skills", id, &s); err != nil {
		return nil, fmt.Errorf("skill creation was audited but activation persistence failed: %w", err)
	}
	return &s, nil
}

func (m *Manager) SetSkillEnabledSafe(id string, enabled bool) (*Skill, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var s Skill
	if err := m.read("skills", id, &s); err != nil {
		return nil, err
	}
	if s.Enabled == enabled {
		return &s, nil
	}
	if enabled {
		if err := m.audit("skill.enabled", map[string]any{"skill_id": id, "enabled": true}); err != nil {
			return nil, fmt.Errorf("skill remains disabled because enable audit failed: %w", err)
		}
		s.Enabled = true
		s.UpdatedAt = now()
		if err := m.save("skills", id, &s); err != nil {
			return nil, fmt.Errorf("skill enable was audited but persistence failed: %w", err)
		}
		return &s, nil
	}
	s.Enabled = false
	s.UpdatedAt = now()
	if err := m.save("skills", id, &s); err != nil {
		return nil, err
	}
	if err := m.audit("skill.enabled", map[string]any{"skill_id": id, "enabled": false}); err != nil {
		return nil, fmt.Errorf("skill remains disabled but disable audit failed: %w", err)
	}
	return &s, nil
}

// DispatchMissionSafe writes an exact mission authorization event before any
// Runtime Task is created. The post-dispatch event is supplementary; the prior
// authorization is the security gate for every child task in this mission.
func (m *Manager) DispatchMissionSafe(in Mission) (*Mission, error) {
	objective, err := cleanText(in.Objective, maxPromptLen, "objective")
	if err != nil {
		return nil, err
	}
	if len(in.AgentIDs) > maxMissionAgents {
		return nil, fmt.Errorf("mission supports at most %d agents", maxMissionAgents)
	}
	if in.Mode == "" {
		in.Mode = "parallel"
	}
	if in.Mode != "parallel" {
		return nil, errors.New("only parallel mission mode is supported")
	}
	agentIDs := append([]string(nil), in.AgentIDs...)
	if len(agentIDs) == 0 {
		agentIDs = []string{""}
	}
	type preparedAgent struct {
		id     string
		prompt string
	}
	prepared := make([]preparedAgent, 0, len(agentIDs))
	for _, aid := range agentIDs {
		prompt := objective
		if aid != "" {
			a, err := m.Agent(aid)
			if err != nil {
				return nil, err
			}
			if !a.Enabled {
				return nil, fmt.Errorf("agent %s disabled", aid)
			}
			if strings.TrimSpace(a.SystemPrompt) != "" {
				prompt = "Operator-defined agent role:\n" + a.SystemPrompt + "\n\nMission objective:\n" + objective
			}
		}
		prepared = append(prepared, preparedAgent{id: aid, prompt: prompt})
	}
	id, err := storage.RandomID("mission")
	if err != nil {
		return nil, err
	}
	objectiveHash := sha256.Sum256([]byte(objective))
	if err := m.audit("mission.dispatch.authorized", map[string]any{"mission_id": id, "mode": in.Mode, "agents": len(prepared), "objective_sha256": hex.EncodeToString(objectiveHash[:])}); err != nil {
		return nil, fmt.Errorf("mission blocked because authorization audit failed: %w", err)
	}
	tm := now()
	in.ID, in.Objective, in.Status, in.CreatedAt, in.UpdatedAt = id, objective, "running", tm, tm
	for _, item := range prepared {
		t, createErr := m.rt.Create(item.prompt, map[string]any{"source": "mission", "mission_id": id, "agent_id": item.id})
		if createErr != nil {
			in.Tasks = append(in.Tasks, MissionTask{AgentID: item.id, Status: "failed", Error: memory.SanitizeContent(createErr.Error())})
			continue
		}
		in.Tasks = append(in.Tasks, MissionTask{AgentID: item.id, TaskID: t.ID, Status: string(t.Status)})
	}
	m.mu.Lock()
	err = m.save("missions", id, &in)
	m.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("authorized mission task creation occurred but mission persistence failed: %w", err)
	}
	_ = m.audit("mission.dispatched", map[string]any{"mission_id": id, "tasks": len(in.Tasks), "mode": in.Mode})
	return &in, nil
}

func (m *Manager) safeTickSchedules() {
	schedules, err := m.Schedules()
	if err != nil {
		return
	}
	t := now()
	for _, candidate := range schedules {
		if !candidate.Enabled || candidate.NextRunAt.After(t) {
			continue
		}
		m.safeFireSchedule(candidate.ID, t)
	}
}

func (m *Manager) safeFireSchedule(id string, firedAt time.Time) {
	m.mu.Lock()
	var current Schedule
	if m.read("schedules", id, &current) != nil || !current.Enabled || current.NextRunAt.After(firedAt) {
		m.mu.Unlock()
		return
	}
	prompt := current.Prompt
	if current.AgentID != "" {
		var agent AgentProfile
		if err := m.read("agents", current.AgentID, &agent); err != nil || !agent.Enabled {
			current.NextRunAt = firedAt.Add(time.Duration(current.IntervalSeconds) * time.Second)
			current.LastError = "scheduled agent unavailable or disabled"
			current.UpdatedAt = firedAt
			_ = m.save("schedules", current.ID, &current)
			m.mu.Unlock()
			_ = m.audit("schedule.skipped", map[string]any{"schedule_id": current.ID, "agent_id": current.AgentID, "reason": current.LastError})
			return
		}
		if strings.TrimSpace(agent.SystemPrompt) != "" {
			prompt = "Operator-defined agent role:\n" + agent.SystemPrompt + "\n\nScheduled objective:\n" + current.Prompt
		}
	}
	if err := m.audit("schedule.fire.authorized", map[string]any{"schedule_id": current.ID, "agent_id": current.AgentID, "scheduled_at": current.NextRunAt}); err != nil {
		current.NextRunAt = firedAt.Add(time.Duration(current.IntervalSeconds) * time.Second)
		current.LastError = "schedule fire blocked because audit is unavailable"
		current.UpdatedAt = firedAt
		_ = m.save("schedules", current.ID, &current)
		m.mu.Unlock()
		return
	}
	current.NextRunAt = firedAt.Add(time.Duration(current.IntervalSeconds) * time.Second)
	current.UpdatedAt = firedAt
	if err := m.save("schedules", current.ID, &current); err != nil {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	created, createErr := m.rt.Create(prompt, map[string]any{"source": "schedule", "schedule_id": current.ID, "agent_id": current.AgentID})
	m.mu.Lock()
	if m.read("schedules", current.ID, &current) == nil {
		runAt := firedAt
		current.LastRunAt = &runAt
		if createErr != nil {
			current.LastError = memory.SanitizeContent(createErr.Error())
		} else {
			current.LastTaskID = created.ID
			current.LastError = ""
		}
		current.UpdatedAt = now()
		_ = m.save("schedules", current.ID, &current)
	}
	m.mu.Unlock()
	_ = m.audit("schedule.fired", map[string]any{"schedule_id": current.ID, "task_id": current.LastTaskID, "error": current.LastError})
}

// SafeExtension is the production model-facing platform extension. It preserves
// the existing tool schema while routing mutating/remote operations through the
// crash-safe control surface.
type SafeExtension struct {
	manager *Manager
}

func NewSafeExtension(manager *Manager) (*SafeExtension, error) {
	if manager == nil {
		return nil, errors.New("platform manager required")
	}
	return &SafeExtension{manager: manager}, nil
}

func (s *SafeExtension) ToolDefinitions() []provider.ToolDef {
	return s.manager.ToolDefinitions()
}

func (s *SafeExtension) ExecuteTool(ctx context.Context, _ string, name string, args json.RawMessage) (string, error) {
	m := s.manager
	switch name {
	case "platform_agents_list":
		v, err := m.Agents()
		return marshalResult(v, err)
	case "platform_skills_list":
		v, err := m.Skills()
		return marshalResult(v, err)
	case "platform_nodes_list":
		v, err := m.NodesSafe()
		return marshalResult(v, err)
	case "platform_schedule_create":
		var in Schedule
		if err := json.Unmarshal(args, &in); err != nil {
			return "", err
		}
		v, err := m.CreateScheduleSafe(in)
		return marshalResult(v, err)
	case "platform_mission_dispatch":
		var in Mission
		if err := json.Unmarshal(args, &in); err != nil {
			return "", err
		}
		v, err := m.DispatchMissionSafe(in)
		return marshalResult(v, err)
	case "platform_plugin_call":
		var in struct {
			PluginID string `json:"plugin_id"`
			Method   string `json:"method"`
			Input    any    `json:"input"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", err
		}
		m.mu.RLock()
		var plugin Plugin
		err := m.read("plugins", in.PluginID, &plugin)
		m.mu.RUnlock()
		if err != nil {
			return "", err
		}
		if !plugin.Enabled {
			return "", errors.New("plugin disabled")
		}
		return m.remotePOST(ctx, plugin.Endpoint, plugin.BearerTokenEnv, plugin.AllowPrivateNetwork, map[string]any{"method": in.Method, "input": in.Input, "plugin_id": plugin.ID})
	case "platform_channel_send":
		var in struct {
			ChannelID string `json:"channel_id"`
			Recipient string `json:"recipient"`
			Text      string `json:"text"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", err
		}
		m.mu.RLock()
		var channel Channel
		err := m.read("channels", in.ChannelID, &channel)
		m.mu.RUnlock()
		if err != nil {
			return "", err
		}
		if !channel.Enabled {
			return "", errors.New("channel disabled")
		}
		return m.remotePOST(ctx, channel.Endpoint, channel.BearerTokenEnv, channel.AllowPrivateNetwork, map[string]any{"recipient": in.Recipient, "text": in.Text, "channel_id": channel.ID, "kind": channel.Kind})
	case "platform_node_action":
		var in struct {
			NodeID string `json:"node_id"`
			Action string `json:"action"`
			Input  any    `json:"input"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", err
		}
		node, err := m.nodeForAction(in.NodeID)
		if err != nil {
			return "", err
		}
		return m.remotePOST(ctx, node.Endpoint, node.BearerTokenEnv, node.AllowPrivateNetwork, map[string]any{"action": in.Action, "input": in.Input, "node_id": node.ID})
	default:
		return "", errors.New("unknown platform tool")
	}
}
