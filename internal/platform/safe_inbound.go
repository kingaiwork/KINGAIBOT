package platform

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// InboundHandlerSafe keeps webhook retries idempotent without pretending that
// task creation and receipt persistence form a distributed transaction. A
// receipt that is known to have a Task is replayed as the same Task reference;
// an ambiguous processing receipt is surfaced for reconciliation rather than
// blindly creating a second side effect.
func (m *Manager) InboundHandlerSafe() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/inbound/{id}", m.httpInboundSafe)
	return mux
}

func (m *Manager) reserveInboundSafe(c *Channel, eventID, sender string, metadata map[string]any) (*InboundReceipt, bool, error) {
	if err := m.ensureInboundDir(); err != nil {
		return nil, false, err
	}
	id := inboundReceiptID(c.ID, eventID)
	m.mu.Lock()
	defer m.mu.Unlock()
	var existing InboundReceipt
	if err := m.read("inbound-receipts", id, &existing); err == nil {
		switch existing.Status {
		case "accepted", "task_created", "processing", "reconciliation":
			return &existing, true, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	t := now()
	receipt := &InboundReceipt{ID: id, ChannelID: c.ID, EventID: eventID, Sender: sender, Status: "processing", Metadata: metadata, CreatedAt: t, UpdatedAt: t}
	if err := m.save("inbound-receipts", id, receipt); err != nil {
		return nil, false, err
	}
	if err := m.audit("channel.inbound.reserved", map[string]any{"channel_id": c.ID, "receipt_id": id, "sender_sha256_96": senderDigest(sender)}); err != nil {
		receipt.Status = "failed"
		receipt.Error = "inbound reservation audit failed"
		receipt.UpdatedAt = now()
		_ = m.save("inbound-receipts", id, receipt)
		return nil, false, fmt.Errorf("inbound event blocked because reservation audit failed: %w", err)
	}
	return receipt, false, nil
}

func (m *Manager) httpInboundSafe(w http.ResponseWriter, r *http.Request) {
	channelID := r.PathValue("id")
	channel, err := m.channel(channelID)
	if err != nil {
		writePlatformJSON(w, http.StatusNotFound, map[string]any{"error": "channel_not_found"})
		return
	}
	if err := m.authenticateInbound(r, channel); err != nil {
		writePlatformJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	var in struct {
		EventID   string         `json:"event_id"`
		Sender    string         `json:"sender"`
		Text      string         `json:"text"`
		SessionID string         `json:"session_id,omitempty"`
		Metadata  map[string]any `json:"metadata,omitempty"`
	}
	if !decodePlatform(w, r, &in) {
		return
	}
	in.EventID = strings.TrimSpace(in.EventID)
	in.Sender = strings.TrimSpace(in.Sender)
	if in.EventID == "" || len(in.EventID) > 256 {
		writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "event_id_required"})
		return
	}
	if in.Sender == "" || len(in.Sender) > 512 {
		writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "sender_required"})
		return
	}
	text, err := cleanText(in.Text, maxPromptLen, "text")
	if err != nil {
		platformProblem(w, err)
		return
	}
	if !senderAllowed(channel.AllowedSenders, in.Sender) {
		writePlatformJSON(w, http.StatusForbidden, map[string]any{"error": "sender_not_allowed"})
		return
	}

	receipt, duplicate, err := m.reserveInboundSafe(channel, in.EventID, in.Sender, in.Metadata)
	if err != nil {
		platformProblem(w, err)
		return
	}
	if duplicate {
		switch receipt.Status {
		case "accepted", "task_created":
			writePlatformJSON(w, http.StatusAccepted, map[string]any{"duplicate": true, "receipt_id": receipt.ID, "session_id": receipt.SessionID, "task_id": receipt.TaskID, "status": receipt.Status})
		default:
			writePlatformJSON(w, http.StatusConflict, map[string]any{"error": "inbound_reconciliation_required", "receipt_id": receipt.ID, "session_id": receipt.SessionID, "task_id": receipt.TaskID, "status": receipt.Status})
		}
		return
	}

	var session *Session
	if in.SessionID != "" {
		session, err = m.Session(in.SessionID)
		if err == nil && (session.Channel != channel.ID || session.Sender != in.Sender) {
			err = errors.New("session does not belong to this channel and sender")
		}
	} else {
		session, err = m.findInboundSession(channel.ID, in.Sender)
		if errors.Is(err, os.ErrNotExist) {
			session, err = m.CreateSession(Session{Channel: channel.ID, Sender: in.Sender})
		}
	}
	if err != nil {
		_ = m.finishInbound(receipt, "", "", "failed", err.Error())
		platformProblem(w, err)
		return
	}

	taskResult, err := m.SendSessionSafe(session.ID, text)
	if err != nil {
		_ = m.finishInbound(receipt, session.ID, "", "failed", err.Error())
		platformProblem(w, err)
		return
	}
	// Persist the exact Task identity before the final acceptance audit. If the
	// process dies after this point, a retry can safely return the same Task ID.
	if err := m.finishInbound(receipt, session.ID, taskResult.ID, "task_created", ""); err != nil {
		writePlatformJSON(w, http.StatusInternalServerError, map[string]any{"error": "receipt_task_link_failed", "task_id": taskResult.ID})
		return
	}
	if err := m.audit("channel.inbound.accepted", map[string]any{"channel_id": channel.ID, "receipt_id": receipt.ID, "session_id": session.ID, "task_id": taskResult.ID, "sender_sha256_96": senderDigest(in.Sender)}); err != nil {
		writePlatformJSON(w, http.StatusInternalServerError, map[string]any{"error": "audit_failed", "receipt_id": receipt.ID, "task_id": taskResult.ID})
		return
	}
	if err := m.finishInbound(receipt, session.ID, taskResult.ID, "accepted", ""); err != nil {
		writePlatformJSON(w, http.StatusInternalServerError, map[string]any{"error": "receipt_acceptance_persistence_failed", "task_id": taskResult.ID})
		return
	}
	writePlatformJSON(w, http.StatusAccepted, map[string]any{"duplicate": false, "receipt_id": receipt.ID, "session_id": session.ID, "task_id": taskResult.ID, "status": string(taskResult.Status)})
}
