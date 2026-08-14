package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type InboundReceipt struct {
	ID        string         `json:"id"`
	ChannelID string         `json:"channel_id"`
	EventID   string         `json:"event_id"`
	Sender    string         `json:"sender"`
	SessionID string         `json:"session_id,omitempty"`
	TaskID    string         `json:"task_id,omitempty"`
	Status    string         `json:"status"`
	Error     string         `json:"error,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

func (m *Manager) ensureInboundDir() error {
	return os.MkdirAll(filepath.Join(m.dir, "inbound-receipts"), 0o700)
}

func inboundReceiptID(channelID, eventID string) string {
	h := sha256.Sum256([]byte(channelID + "\x00" + eventID))
	return "in_" + hex.EncodeToString(h[:16])
}

func senderDigest(sender string) string {
	h := sha256.Sum256([]byte(sender))
	return hex.EncodeToString(h[:12])
}

func senderAllowed(allowed []string, sender string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, v := range allowed {
		if strings.TrimSpace(v) == sender {
			return true
		}
	}
	return false
}

func (m *Manager) InboundHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/inbound/{id}", m.httpInbound)
	return mux
}

func (m *Manager) channel(id string) (*Channel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var c Channel
	if err := m.read("channels", id, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (m *Manager) authenticateInbound(r *http.Request, c *Channel) error {
	if c == nil || !c.Enabled {
		return errors.New("channel disabled or unavailable")
	}
	if c.BearerTokenEnv == "" {
		return errors.New("channel has no inbound bearer token configured")
	}
	expected := os.Getenv(c.BearerTokenEnv)
	if len(expected) < 32 {
		return errors.New("channel inbound bearer token is missing or too short")
	}
	got := bearerToken(r)
	if !constantTokenEqual(got, expected) {
		return errors.New("invalid channel bearer token")
	}
	return nil
}

func (m *Manager) findInboundSession(channelID, sender string) (*Session, error) {
	sessions, err := m.Sessions()
	if err != nil {
		return nil, err
	}
	for _, s := range sessions {
		if s.Channel == channelID && s.Sender == sender {
			return s, nil
		}
	}
	return nil, os.ErrNotExist
}

func (m *Manager) reserveInbound(c *Channel, eventID, sender string, metadata map[string]any) (*InboundReceipt, bool, error) {
	if err := m.ensureInboundDir(); err != nil {
		return nil, false, err
	}
	id := inboundReceiptID(c.ID, eventID)
	m.mu.Lock()
	defer m.mu.Unlock()
	var existing InboundReceipt
	if err := m.read("inbound-receipts", id, &existing); err == nil {
		if existing.Status == "accepted" || existing.Status == "processing" {
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
	return receipt, false, nil
}

func (m *Manager) finishInbound(receipt *InboundReceipt, sessionID, taskID, status, errText string) error {
	if receipt == nil {
		return errors.New("receipt required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	receipt.SessionID = sessionID
	receipt.TaskID = taskID
	receipt.Status = status
	receipt.Error = errText
	receipt.UpdatedAt = now()
	return m.save("inbound-receipts", receipt.ID, receipt)
}

func (m *Manager) httpInbound(w http.ResponseWriter, r *http.Request) {
	channelID := r.PathValue("id")
	c, err := m.channel(channelID)
	if err != nil {
		writePlatformJSON(w, http.StatusNotFound, map[string]any{"error": "channel_not_found"})
		return
	}
	if err := m.authenticateInbound(r, c); err != nil {
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
	if !senderAllowed(c.AllowedSenders, in.Sender) {
		writePlatformJSON(w, http.StatusForbidden, map[string]any{"error": "sender_not_allowed"})
		return
	}

	receipt, duplicate, err := m.reserveInbound(c, in.EventID, in.Sender, in.Metadata)
	if err != nil {
		platformProblem(w, err)
		return
	}
	if duplicate {
		writePlatformJSON(w, http.StatusAccepted, map[string]any{"duplicate": true, "receipt_id": receipt.ID, "session_id": receipt.SessionID, "task_id": receipt.TaskID, "status": receipt.Status})
		return
	}

	var session *Session
	if in.SessionID != "" {
		session, err = m.Session(in.SessionID)
		if err == nil && (session.Channel != c.ID || session.Sender != in.Sender) {
			err = errors.New("session does not belong to this channel and sender")
		}
	} else {
		session, err = m.findInboundSession(c.ID, in.Sender)
		if errors.Is(err, os.ErrNotExist) {
			session, err = m.CreateSession(Session{Channel: c.ID, Sender: in.Sender})
		}
	}
	if err != nil {
		_ = m.finishInbound(receipt, "", "", "failed", err.Error())
		platformProblem(w, err)
		return
	}

	t, err := m.SendSession(session.ID, text)
	if err != nil {
		_ = m.finishInbound(receipt, session.ID, "", "failed", err.Error())
		platformProblem(w, err)
		return
	}
	if err := m.finishInbound(receipt, session.ID, t.ID, "accepted", ""); err != nil {
		// Task creation itself is already durably audited by Runtime. Surface the
		// receipt persistence failure rather than pretending idempotency succeeded.
		writePlatformJSON(w, http.StatusInternalServerError, map[string]any{"error": "receipt_persistence_failed", "task_id": t.ID})
		return
	}
	if err := m.audit("channel.inbound.accepted", map[string]any{"channel_id": c.ID, "receipt_id": receipt.ID, "session_id": session.ID, "task_id": t.ID, "sender_sha256_96": senderDigest(in.Sender)}); err != nil {
		writePlatformJSON(w, http.StatusInternalServerError, map[string]any{"error": "audit_failed", "task_id": t.ID})
		return
	}
	writePlatformJSON(w, http.StatusAccepted, map[string]any{"duplicate": false, "receipt_id": receipt.ID, "session_id": session.ID, "task_id": t.ID, "status": string(t.Status)})
}

func (r *InboundReceipt) String() string {
	return fmt.Sprintf("%s:%s", r.ChannelID, r.EventID)
}
