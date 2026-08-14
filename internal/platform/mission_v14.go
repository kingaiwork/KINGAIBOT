package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kingaiwork/KINGAIBOT/internal/memory"
	"github.com/kingaiwork/KINGAIBOT/internal/storage"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

const missionDispatchStatusV14 = "dispatching_v14"

func missionTaskIdempotencyKey(missionID string, index int, agentID string) string {
	return fmt.Sprintf("king-mission:%s:task:%d:%s", missionID, index, agentID)
}

func (m *Manager) missionPromptV14(objective, agentID string) (string, error) {
	prompt := objective
	if agentID == "" {
		return prompt, nil
	}
	agent, err := m.Agent(agentID)
	if err != nil {
		return "", err
	}
	if !agent.Enabled {
		return "", fmt.Errorf("agent %s disabled", agentID)
	}
	if strings.TrimSpace(agent.SystemPrompt) != "" {
		prompt = "Operator-defined agent role:\n" + agent.SystemPrompt + "\n\nMission objective:\n" + objective
	}
	return prompt, nil
}

// DispatchMissionV14 persists the Mission before any child Runtime Task exists,
// then creates each child through a stable idempotency key and persists that
// exact Task identity immediately. A crash between Task creation and Mission
// linkage is therefore recoverable without launching duplicate child work.
func (m *Manager) DispatchMissionV14(in Mission) (*Mission, error) {
	if _, ok := m.rt.(idempotentPlatformRuntime); !ok {
		return nil, errors.New("platform runtime does not support idempotent mission tasks")
	}
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
	for _, agentID := range agentIDs {
		if _, err := m.missionPromptV14(objective, agentID); err != nil {
			return nil, err
		}
	}
	missionID, err := storage.RandomID("mission")
	if err != nil {
		return nil, err
	}
	objectiveHash := sha256.Sum256([]byte(objective))
	if err := m.audit("mission.v14.dispatch.authorized", map[string]any{
		"mission_id":       missionID,
		"mode":             in.Mode,
		"agents":           len(agentIDs),
		"objective_sha256": hex.EncodeToString(objectiveHash[:]),
	}); err != nil {
		return nil, fmt.Errorf("mission blocked because authorization audit failed: %w", err)
	}
	t := now()
	in.ID = missionID
	in.Objective = objective
	in.Status = missionDispatchStatusV14
	in.CreatedAt = t
	in.UpdatedAt = t
	in.Tasks = make([]MissionTask, len(agentIDs))
	for i, agentID := range agentIDs {
		in.Tasks[i] = MissionTask{AgentID: agentID, Status: "pending"}
	}
	m.mu.Lock()
	err = m.save("missions", missionID, &in)
	m.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("mission authorization was audited but initial persistence failed: %w", err)
	}
	return m.resumeMissionDispatchV14(missionID)
}

func (m *Manager) missionReconciliationV14(mission *Mission, index int, reason string) (*Mission, error) {
	mission.Status = "reconciliation"
	mission.UpdatedAt = now()
	if index >= 0 && index < len(mission.Tasks) {
		mission.Tasks[index].Status = "reconciliation"
		mission.Tasks[index].Error = memory.SanitizeContent(reason)
	}
	m.mu.Lock()
	err := m.save("missions", mission.ID, mission)
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	_ = m.audit("mission.v14.reconciliation", map[string]any{
		"mission_id": mission.ID,
		"task_index": index,
		"reason":     memory.SanitizeContent(reason),
	})
	return mission, nil
}

func missionTaskRequiresReconciliation(status task.Status) bool {
	return status == task.PendingAudit || status == task.Reconciliation
}

func (m *Manager) verifyMissionTaskV14(mission *Mission, index int) (*task.Task, error) {
	if mission == nil || index < 0 || index >= len(mission.Tasks) {
		return nil, errors.New("invalid mission task index")
	}
	taskID := strings.TrimSpace(mission.Tasks[index].TaskID)
	if taskID == "" {
		return nil, errors.New("mission task id missing")
	}
	current, err := m.rt.Task(taskID)
	if err != nil {
		return nil, err
	}
	if missionTaskRequiresReconciliation(current.Status) {
		return current, fmt.Errorf("child task %s is %s", current.ID, current.Status)
	}
	return current, nil
}

func (m *Manager) resumeMissionDispatchV14(missionID string) (*Mission, error) {
	idempotent, ok := m.rt.(idempotentPlatformRuntime)
	if !ok {
		return nil, errors.New("platform runtime does not support idempotent mission tasks")
	}
	m.mu.RLock()
	var mission Mission
	err := m.read("missions", missionID, &mission)
	m.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	if mission.Status != missionDispatchStatusV14 {
		return &mission, nil
	}
	agentIDs := append([]string(nil), mission.AgentIDs...)
	if len(agentIDs) == 0 {
		agentIDs = []string{""}
	}
	if len(mission.Tasks) != len(agentIDs) {
		return m.missionReconciliationV14(&mission, -1, "mission task slots do not match agent list")
	}
	for index, agentID := range agentIDs {
		if mission.Tasks[index].TaskID != "" {
			current, verifyErr := m.verifyMissionTaskV14(&mission, index)
			if verifyErr != nil {
				return m.missionReconciliationV14(&mission, index, "linked mission task requires reconciliation: "+verifyErr.Error())
			}
			mission.Tasks[index].Status = string(current.Status)
			continue
		}
		prompt, err := m.missionPromptV14(mission.Objective, agentID)
		if err != nil {
			return m.missionReconciliationV14(&mission, index, "mission agent unavailable during recovery: "+err.Error())
		}
		key := missionTaskIdempotencyKey(mission.ID, index, agentID)
		created, err := idempotent.CreateIdempotent(prompt, map[string]any{
			"source":           "mission_v14",
			"mission_id":       mission.ID,
			"mission_task_idx": index,
			"agent_id":         agentID,
		}, key)
		if err != nil {
			return m.missionReconciliationV14(&mission, index, "idempotent mission task resolution failed: "+err.Error())
		}
		mission.Tasks[index].TaskID = created.ID
		mission.Tasks[index].Status = string(created.Status)
		mission.Tasks[index].Error = ""
		mission.UpdatedAt = now()
		m.mu.Lock()
		err = m.save("missions", mission.ID, &mission)
		m.mu.Unlock()
		if err != nil {
			// Recovery can safely re-run this slot because the Runtime task identity
			// is deterministic for mission+slot+agent.
			return nil, err
		}
		if missionTaskRequiresReconciliation(created.Status) {
			return m.missionReconciliationV14(&mission, index, fmt.Sprintf("child task %s is %s", created.ID, created.Status))
		}
	}
	if err := m.audit("mission.v14.dispatched", map[string]any{"mission_id": mission.ID, "tasks": len(mission.Tasks), "mode": mission.Mode}); err != nil {
		return m.missionReconciliationV14(&mission, -1, "mission dispatch completion audit failed: "+err.Error())
	}
	mission.Status = "running"
	mission.UpdatedAt = now()
	m.mu.Lock()
	err = m.save("missions", mission.ID, &mission)
	m.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("mission dispatch was audited but running-state persistence failed: %w", err)
	}
	return &mission, nil
}

// RecoverMissionsV14 resumes only partially-dispatched v1.4 missions. Once a
// mission reaches ordinary running state, the existing mission result synchronizer
// can safely track its already-linked child Task IDs.
func (m *Manager) RecoverMissionsV14() {
	m.mu.RLock()
	missions, err := listJSON[Mission](filepath.Join(m.dir, "missions"))
	m.mu.RUnlock()
	if err != nil {
		return
	}
	for _, mission := range missions {
		if mission == nil || mission.Status != missionDispatchStatusV14 {
			continue
		}
		if err := m.audit("mission.v14.recovery_authorized", map[string]any{"mission_id": mission.ID}); err != nil {
			continue
		}
		_, _ = m.resumeMissionDispatchV14(mission.ID)
	}
}
