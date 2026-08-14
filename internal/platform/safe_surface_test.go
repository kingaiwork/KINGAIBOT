package platform

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
)

type safeHarness struct {
	manager *Manager
	runtime *fakeRuntime
	events  string
}

func newSafeHarness(t *testing.T) *safeHarness {
	t.Helper()
	dir := t.TempDir()
	eventsDir := filepath.Join(dir, "events")
	log, err := eventlog.New(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	runtime := newFakeRuntime()
	manager, err := NewSafe(filepath.Join(dir, "platform"), runtime, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	return &safeHarness{manager: manager, runtime: runtime, events: eventsDir}
}

func runtimeTaskCount(runtime *fakeRuntime) int {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return len(runtime.tasks)
}

func TestSafeScheduleSkipsDisabledAgentWithoutCreatingTask(t *testing.T) {
	h := newSafeHarness(t)
	agent, err := h.manager.CreateAgentSafe(AgentProfile{Name: "scheduler", SystemPrompt: "scheduled role"})
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := h.manager.CreateScheduleSafe(Schedule{Name: "hourly", Prompt: "do work", IntervalSeconds: 3600, AgentID: agent.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.manager.SetAgentEnabledSafe(agent.ID, false); err != nil {
		t.Fatal(err)
	}
	before := runtimeTaskCount(h.runtime)
	h.manager.safeFireSchedule(schedule.ID, schedule.NextRunAt.Add(time.Second))
	after := runtimeTaskCount(h.runtime)
	if after != before {
		t.Fatalf("disabled agent schedule created %d tasks; before=%d", after-before, before)
	}
	schedules, err := h.manager.Schedules()
	if err != nil {
		t.Fatal(err)
	}
	var stored *Schedule
	for _, item := range schedules {
		if item.ID == schedule.ID {
			stored = item
			break
		}
	}
	if stored == nil || !strings.Contains(stored.LastError, "unavailable or disabled") {
		t.Fatalf("schedule did not record disabled-agent skip: %#v", stored)
	}
}

func TestSafeNodeRequiresAuditedHeartbeatBeforeAction(t *testing.T) {
	h := newSafeHarness(t)
	node, err := h.manager.CreateNodeSafe(Node{Name: "desktop", Endpoint: "http://127.0.0.1:19001", AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	if node.Online || !node.LastSeenAt.IsZero() {
		t.Fatalf("new node must start offline: %#v", node)
	}
	nodes, err := h.manager.NodesSafe()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Online {
		t.Fatalf("listing nodes promoted an offline node: %#v", nodes)
	}
	extension, err := NewSafeExtension(h.manager)
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"node_id": node.ID, "action": "inspect"})
	if _, err := extension.ExecuteTool(context.Background(), "task", "platform_node_action", args); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("offline node action was not rejected locally: %v", err)
	}
	heartbeat, err := h.manager.HeartbeatNodeSafe(node.ID, map[string]any{"os": "test"})
	if err != nil {
		t.Fatal(err)
	}
	if !heartbeat.Online || heartbeat.LastSeenAt.IsZero() {
		t.Fatalf("audited heartbeat did not mark node online: %#v", heartbeat)
	}
}

func TestSafeWorkflowRunAuditFailureCreatesNoRunOrTask(t *testing.T) {
	h := newSafeHarness(t)
	workflow, err := h.manager.CreateWorkflowSafe(Workflow{Name: "one-step", Steps: []WorkflowStep{{Prompt: "one"}}})
	if err != nil {
		t.Fatal(err)
	}
	breakPlatformAudit(t, h.events)
	if _, err := h.manager.RunWorkflowSafe(workflow.ID); err == nil {
		t.Fatal("workflow run unexpectedly started with broken audit log")
	}
	if runtimeTaskCount(h.runtime) != 0 {
		t.Fatal("workflow audit failure created runtime task")
	}
	runs, err := h.manager.WorkflowRuns()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("workflow audit failure persisted run: %#v", runs)
	}
}

func TestSafeAgentCreationAuditFailureLeavesAgentDisabled(t *testing.T) {
	h := newSafeHarness(t)
	breakPlatformAudit(t, h.events)
	if _, err := h.manager.CreateAgentSafe(AgentProfile{Name: "blocked"}); err == nil {
		t.Fatal("agent creation unexpectedly succeeded with broken audit log")
	}
	agents, err := h.manager.Agents()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].Enabled {
		t.Fatalf("audit-failed agent became usable: %#v", agents)
	}
}

func TestSafeMissionDisabledAgentCreatesNoChildTasks(t *testing.T) {
	h := newSafeHarness(t)
	agent, err := h.manager.CreateAgentSafe(AgentProfile{Name: "mission-worker"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.manager.SetAgentEnabledSafe(agent.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := h.manager.DispatchMissionSafe(Mission{Objective: "must not run", AgentIDs: []string{agent.ID}}); err == nil {
		t.Fatal("mission unexpectedly dispatched through disabled agent")
	}
	if runtimeTaskCount(h.runtime) != 0 {
		t.Fatal("disabled-agent mission created child runtime tasks")
	}
}
