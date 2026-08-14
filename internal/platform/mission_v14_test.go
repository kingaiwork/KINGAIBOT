package platform

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

func TestMissionV14DispatchesStableChildTasks(t *testing.T) {
	dir := t.TempDir()
	runtime := newCallbackRuntime()
	m := newManagerWithRuntimeForV14Test(t, dir, runtime)
	mission, err := m.DispatchMissionV14(Mission{Objective: "parallel check", AgentIDs: []string{"", ""}})
	if err != nil {
		t.Fatal(err)
	}
	if mission.Status != "running" || len(mission.Tasks) != 2 {
		t.Fatalf("unexpected dispatched mission: %#v", mission)
	}
	if runtime.next != 2 {
		t.Fatalf("mission created %d runtime tasks, want 2", runtime.next)
	}
	for i, child := range mission.Tasks {
		if child.TaskID == "" {
			t.Fatalf("mission child %d missing task id", i)
		}
	}
	finished, err := m.Mission(mission.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "completed" {
		t.Fatalf("completed fake child tasks did not complete mission: %#v", finished)
	}
}

func TestMissionV14RecoveryReusesOrphanedChildTask(t *testing.T) {
	dir := t.TempDir()
	runtime := newCallbackRuntime()
	m := newManagerWithRuntimeForV14Test(t, dir, runtime)
	missionID := "mission_recovery_test"
	mission := &Mission{
		ID:        missionID,
		Objective: "parallel check",
		AgentIDs:  []string{"", ""},
		Mode:      "parallel",
		Status:    missionDispatchStatusV14,
		Tasks: []MissionTask{
			{Status: "pending"},
			{Status: "pending"},
		},
		CreatedAt: now(),
		UpdatedAt: now(),
	}
	m.mu.Lock()
	if err := m.save("missions", missionID, mission); err != nil {
		m.mu.Unlock()
		t.Fatal(err)
	}
	m.mu.Unlock()

	key := missionTaskIdempotencyKey(missionID, 0, "")
	preexisting, err := runtime.CreateIdempotent(mission.Objective, map[string]any{
		"source":           "mission_v14",
		"mission_id":       missionID,
		"mission_task_idx": 0,
		"agent_id":         "",
	}, key)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.next != 1 {
		t.Fatalf("setup created %d runtime tasks, want 1", runtime.next)
	}

	// The Mission does not know about preexisting yet, simulating a crash after
	// Runtime task creation but before the child TaskID was persisted.
	m.RecoverMissionsV14()
	recovered, err := m.Mission(missionID)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.next != 2 {
		t.Fatalf("recovery created duplicate/orphan child tasks: count=%d want=2 total slots", runtime.next)
	}
	if recovered.Tasks[0].TaskID != preexisting.ID {
		t.Fatalf("recovery failed to relink preexisting child %s: %#v", preexisting.ID, recovered.Tasks)
	}
	if recovered.Tasks[1].TaskID == "" || recovered.Tasks[1].TaskID == preexisting.ID {
		t.Fatalf("second mission slot was not independently created: %#v", recovered.Tasks)
	}
	if recovered.Status != "completed" {
		t.Fatalf("recovered mission did not complete: %#v", recovered)
	}
}

func TestMissionV14RecoveryPropagatesChildReconciliation(t *testing.T) {
	dir := t.TempDir()
	runtime := newCallbackRuntime()
	m := newManagerWithRuntimeForV14Test(t, dir, runtime)
	missionID := "mission_child_reconciliation"
	mission := &Mission{
		ID:        missionID,
		Objective: "ambiguous child",
		AgentIDs:  []string{""},
		Mode:      "parallel",
		Status:    missionDispatchStatusV14,
		Tasks:     []MissionTask{{Status: "pending"}},
		CreatedAt: now(),
		UpdatedAt: now(),
	}
	m.mu.Lock()
	if err := m.save("missions", missionID, mission); err != nil {
		m.mu.Unlock()
		t.Fatal(err)
	}
	m.mu.Unlock()

	key := missionTaskIdempotencyKey(missionID, 0, "")
	preexisting, err := runtime.CreateIdempotent(mission.Objective, map[string]any{
		"source":           "mission_v14",
		"mission_id":       missionID,
		"mission_task_idx": 0,
		"agent_id":         "",
	}, key)
	if err != nil {
		t.Fatal(err)
	}
	runtime.mu.Lock()
	runtime.tasks[preexisting.ID].Status = task.Reconciliation
	runtime.tasks[preexisting.ID].Error = "ambiguous external side effect"
	runtime.mu.Unlock()

	m.RecoverMissionsV14()
	recovered, err := m.Mission(missionID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "reconciliation" {
		t.Fatalf("mission status=%s, want reconciliation: %#v", recovered.Status, recovered)
	}
	if len(recovered.Tasks) != 1 || recovered.Tasks[0].TaskID != preexisting.ID || recovered.Tasks[0].Status != "reconciliation" {
		t.Fatalf("child reconciliation evidence not propagated: %#v", recovered.Tasks)
	}
	if runtime.next != 1 {
		t.Fatalf("reconciliation recovery created duplicate work: count=%d", runtime.next)
	}
}

func TestV14ExtensionRoutesMissionToolToIdempotentDispatcher(t *testing.T) {
	dir := t.TempDir()
	runtime := newCallbackRuntime()
	m := newManagerWithRuntimeForV14Test(t, dir, runtime)
	extension, err := NewV14Extension(m)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(Mission{Objective: "tool mission", AgentIDs: []string{""}})
	result, err := extension.ExecuteTool(context.Background(), "task_parent", "platform_mission_dispatch", raw)
	if err != nil {
		t.Fatal(err)
	}
	var mission Mission
	if err := json.Unmarshal([]byte(result), &mission); err != nil {
		t.Fatal(err)
	}
	if len(mission.Tasks) != 1 || mission.Tasks[0].TaskID == "" || runtime.next != 1 {
		t.Fatalf("v1.4 extension did not use idempotent mission dispatcher: result=%#v count=%d", mission, runtime.next)
	}
}
