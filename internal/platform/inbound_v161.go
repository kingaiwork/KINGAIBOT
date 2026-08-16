package platform

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/memory"
)

type inboundEnvelopeV161 struct {
	EventID   string         `json:"event_id"`
	Sender    string         `json:"sender"`
	Text      string         `json:"text"`
	SessionID string         `json:"session_id,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

func readInboundEnvelopeV161(w http.ResponseWriter, r *http.Request) ([]byte, inboundEnvelopeV161, bool) {
	var in inboundEnvelopeV161
	r.Body = http.MaxBytesReader(w, r.Body, maxAPIRequest)
	defer r.Body.Close()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writePlatformJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "request_too_large"})
		} else {
			writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_body"})
		}
		return nil, in, false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return nil, in, false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return nil, in, false
	}
	return raw, in, true
}

// InboundHandlerV161 is the production normalized Channel gateway. It keeps
// v1.4 crash/idempotency semantics and adds opt-in body signing. When the
// channel's <BEARER_TOKEN_ENV>_SIGNING_SECRET environment variable is set, a
// gateway must send X-KINGAI-Timestamp and X-KINGAI-Signature headers.
func (m *Manager) InboundHandlerV161() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/inbound/{id}", m.httpInboundV161)
	return mux
}

func (m *Manager) httpInboundV161(w http.ResponseWriter, r *http.Request) {
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

	raw, in, ok := readInboundEnvelopeV161(w, r)
	if !ok {
		return
	}
	if err := verifyInboundSignatureV161(r, channel, raw, time.Now().UTC()); err != nil {
		if errors.Is(err, errInboundSigningMisconfigured) {
			writePlatformJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "channel_misconfigured"})
			return
		}
		writePlatformJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_signature"})
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
			writePlatformJSON(w, http.StatusAccepted, map[string]any{
				"duplicate":  true,
				"receipt_id": receipt.ID,
				"session_id": receipt.SessionID,
				"task_id":    receipt.TaskID,
				"status":     receipt.Status,
			})
		default:
			writePlatformJSON(w, http.StatusConflict, map[string]any{
				"error":      "inbound_reconciliation_required",
				"receipt_id": receipt.ID,
				"session_id": receipt.SessionID,
				"task_id":    receipt.TaskID,
				"status":     receipt.Status,
			})
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
		_ = m.finishInbound(receipt, "", "", "failed", memorySafeError(err))
		platformProblem(w, err)
		return
	}

	created, err := m.SendSessionV14(session.ID, text)
	if err != nil {
		_ = m.finishInbound(receipt, session.ID, "", "failed", memorySafeError(err))
		platformProblem(w, err)
		return
	}
	if err := m.finishInbound(receipt, session.ID, created.ID, "task_created", ""); err != nil {
		writePlatformJSON(w, http.StatusInternalServerError, map[string]any{"error": "receipt_task_link_failed", "task_id": created.ID})
		return
	}
	if err := m.audit("channel.inbound.accepted", map[string]any{
		"channel_id":       channel.ID,
		"receipt_id":       receipt.ID,
		"session_id":       session.ID,
		"task_id":          created.ID,
		"sender_sha256_96": senderDigest(in.Sender),
		"signed":           os.Getenv(inboundSigningSecretEnv(channel)) != "",
	}); err != nil {
		writePlatformJSON(w, http.StatusInternalServerError, map[string]any{"error": "audit_failed", "receipt_id": receipt.ID, "task_id": created.ID})
		return
	}
	if err := m.finishInbound(receipt, session.ID, created.ID, "accepted", ""); err != nil {
		writePlatformJSON(w, http.StatusInternalServerError, map[string]any{"error": "receipt_acceptance_persistence_failed", "task_id": created.ID})
		return
	}
	writePlatformJSON(w, http.StatusAccepted, map[string]any{
		"duplicate":  false,
		"receipt_id": receipt.ID,
		"session_id": session.ID,
		"task_id":    created.ID,
		"status":     string(created.Status),
	})
}

func memorySafeError(err error) string {
	if err == nil {
		return ""
	}
	s := strings.TrimSpace(memory.SanitizeContent(err.Error()))
	if len(s) > 512 {
		s = s[:512]
	}
	return s
}
