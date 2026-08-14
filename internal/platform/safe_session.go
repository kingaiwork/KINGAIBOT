package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/kingaiwork/KINGAIBOT/internal/memory"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

// SendSessionDurable prevents a post-Create persistence failure from being
// mistaken for a safe-to-retry submission. Once Runtime.Create succeeds, the
// Task is durable execution truth and this method returns that Task even if the
// derived Session counters cannot be synchronized immediately. Session state is
// reloaded under the write lock before updating so concurrent turns do not lose
// increments or overwrite each other from stale snapshots.
func (m *Manager) SendSessionDurable(id, text string) (*task.Task, error) {
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
	prompt := text
	if session.AgentID != "" {
		a, err := m.Agent(session.AgentID)
		if err != nil {
			return nil, err
		}
		if !a.Enabled {
			return nil, errors.New("agent disabled")
		}
		if strings.TrimSpace(a.SystemPrompt) != "" {
			prompt = "Operator-defined agent role:\n" + a.SystemPrompt + "\n\nCurrent user message:\n" + text
		}
	}
	h := sha256.Sum256([]byte(text))
	if err := m.audit("session.turn.authorized", map[string]any{"session_id": id, "agent_id": session.AgentID, "message_sha256": hex.EncodeToString(h[:])}); err != nil {
		return nil, fmt.Errorf("session task blocked because authorization audit failed: %w", err)
	}
	created, err := m.rt.Create(prompt, map[string]any{"source": "session", "session_id": id, "agent_id": session.AgentID})
	if err != nil {
		return nil, err
	}

	// From this point on, returning an error could cause a caller/webhook to
	// retry and duplicate a task that already exists. Derived state failures are
	// therefore surfaced through audit rather than as a retryable submission
	// error.
	m.mu.Lock()
	var current Session
	stateErr := m.read("sessions", id, &current)
	if stateErr == nil {
		current.LastTaskID = created.ID
		current.Turns++
		current.UpdatedAt = now()
		stateErr = m.save("sessions", id, &current)
	}
	m.mu.Unlock()
	if stateErr != nil {
		_ = m.audit("session.turn.state_sync_pending", map[string]any{"session_id": id, "task_id": created.ID, "reason": memory.SanitizeContent(stateErr.Error())})
	} else {
		_ = m.audit("session.turn.created", map[string]any{"session_id": id, "task_id": created.ID})
	}
	return created, nil
}
