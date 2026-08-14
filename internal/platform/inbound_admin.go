package platform

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/kingaiwork/KINGAIBOT/internal/memory"
)

// InboundAdminHandler exposes reconciliation-only administration for durable
// inbound receipts. It intentionally has no "retry" action: an ambiguous
// processing window cannot prove that Runtime.Create did not already succeed.
func (m *Manager) InboundAdminHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/platform/inbound-receipts", m.httpInboundReceipts)
	mux.HandleFunc("GET /v1/platform/inbound-receipts/{id}", m.httpInboundReceipt)
	mux.HandleFunc("POST /v1/platform/inbound-receipts/{id}/reconcile", m.httpInboundReconcile)
	return mux
}

func (m *Manager) InboundReceipt(id string) (*InboundReceipt, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var receipt InboundReceipt
	if err := m.read("inbound-receipts", id, &receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}

func (m *Manager) InboundReceipts(status string, limit int) ([]*InboundReceipt, error) {
	if err := m.ensureInboundDir(); err != nil {
		return nil, err
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	m.mu.RLock()
	out, err := listJSON[InboundReceipt](filepath.Join(m.dir, "inbound-receipts"))
	m.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	filtered := out[:0]
	for _, receipt := range out {
		if receipt == nil {
			continue
		}
		if status != "" && receipt.Status != status {
			continue
		}
		filtered = append(filtered, receipt)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt) })
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

func taskMetadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

// ReconcileInboundReceipt resolves an ambiguous receipt without creating new
// work. link_task requires an already-existing Session task whose durable
// metadata proves it belongs to the same channel/sender. mark_failed is a
// fail-closed terminal decision and never cancels or rewrites an existing Task.
func (m *Manager) ReconcileInboundReceipt(id, decision, taskID, note string) (*InboundReceipt, error) {
	decision = strings.ToLower(strings.TrimSpace(decision))
	note = strings.TrimSpace(memory.SanitizeContent(note))
	if note == "" {
		return nil, errors.New("reconciliation note required")
	}
	if len(note) > 4096 {
		return nil, errors.New("reconciliation note exceeds limit")
	}
	if decision != "link_task" && decision != "mark_failed" {
		return nil, errors.New("decision must be link_task or mark_failed")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	var receipt InboundReceipt
	if err := m.read("inbound-receipts", id, &receipt); err != nil {
		return nil, err
	}

	if decision == "mark_failed" {
		if receipt.Status == "accepted" || receipt.TaskID != "" {
			return nil, errors.New("receipt with known task cannot be marked failed")
		}
		receipt.Status = "failed"
		receipt.Error = "operator reconciliation: " + note
		receipt.UpdatedAt = now()
		if err := m.save("inbound-receipts", id, &receipt); err != nil {
			return nil, err
		}
		if err := m.audit("channel.inbound.reconciled.failed", map[string]any{
			"receipt_id":       id,
			"channel_id":       receipt.ChannelID,
			"sender_sha256_96": senderDigest(receipt.Sender),
			"note":             note,
		}); err != nil {
			return nil, fmt.Errorf("receipt remains failed but reconciliation audit failed: %w", err)
		}
		return &receipt, nil
	}

	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, errors.New("task_id required for link_task")
	}
	t, err := m.rt.Task(taskID)
	if err != nil {
		return nil, fmt.Errorf("task lookup failed: %w", err)
	}
	if taskMetadataString(t.Metadata, "source") != "session" {
		return nil, errors.New("task is not a session task")
	}
	sessionID := taskMetadataString(t.Metadata, "session_id")
	if sessionID == "" {
		return nil, errors.New("task has no session identity")
	}
	if taskMetadataString(t.Metadata, "channel") != receipt.ChannelID {
		return nil, errors.New("task channel does not match receipt")
	}
	if taskMetadataString(t.Metadata, "sender") != receipt.Sender {
		return nil, errors.New("task sender does not match receipt")
	}
	if receipt.SessionID != "" && receipt.SessionID != sessionID {
		return nil, errors.New("task session does not match receipt")
	}
	if receipt.Status == "accepted" {
		if receipt.TaskID == t.ID {
			return &receipt, nil
		}
		return nil, errors.New("accepted receipt is already linked to another task")
	}
	if receipt.TaskID != "" && receipt.TaskID != t.ID {
		return nil, errors.New("receipt is already linked to another task")
	}

	// Linking an existing Task promotes ambiguous evidence to accepted evidence,
	// so the exact decision is audited before the receipt becomes accepted.
	if err := m.audit("channel.inbound.reconciled.linked", map[string]any{
		"receipt_id":       id,
		"channel_id":       receipt.ChannelID,
		"session_id":       sessionID,
		"task_id":          t.ID,
		"sender_sha256_96": senderDigest(receipt.Sender),
		"note":             note,
	}); err != nil {
		return nil, fmt.Errorf("receipt remains unresolved because reconciliation audit failed: %w", err)
	}
	receipt.SessionID = sessionID
	receipt.TaskID = t.ID
	receipt.Status = "accepted"
	receipt.Error = ""
	receipt.UpdatedAt = now()
	if err := m.save("inbound-receipts", id, &receipt); err != nil {
		return nil, fmt.Errorf("reconciliation was audited but receipt persistence failed: %w", err)
	}
	return &receipt, nil
}

func (m *Manager) httpInboundReceipts(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 500 {
			writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "limit must be between 1 and 500"})
			return
		}
		limit = parsed
	}
	items, err := m.InboundReceipts(r.URL.Query().Get("status"), limit)
	respondPlatform(w, items, err)
}

func (m *Manager) httpInboundReceipt(w http.ResponseWriter, r *http.Request) {
	receipt, err := m.InboundReceipt(r.PathValue("id"))
	respondPlatform(w, receipt, err)
}

func (m *Manager) httpInboundReconcile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Decision string `json:"decision"`
		TaskID   string `json:"task_id,omitempty"`
		Note     string `json:"note"`
	}
	if !decodePlatform(w, r, &in) {
		return
	}
	receipt, err := m.ReconcileInboundReceipt(r.PathValue("id"), in.Decision, in.TaskID, in.Note)
	respondPlatform(w, receipt, err)
}
