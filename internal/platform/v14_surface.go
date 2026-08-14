package platform

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

// SendSessionV14 is the production v1.4 session submission path. It delegates
// to the durable implementation so a Task that was successfully created can
// never be mistaken for a safe-to-retry failure merely because derived Session
// state could not be synchronized afterward.
func (m *Manager) SendSessionV14(id, text string) (*task.Task, error) {
	return m.SendSessionDurable(id, text)
}

// HandlerV14 overlays the execution-sensitive routes that need v1.4 semantics
// and delegates the remaining control-plane API to the compatibility handler.
// Exact method/path patterns win over the fallback root handler.
func (m *Manager) HandlerV14() http.Handler {
	// New v1.4 workflow runs and missions use distinct durable states, so
	// compatibility recovery/synchronization ignores them. Recover and start the
	// dedicated v1.4 mission synchronizer before serving requests.
	m.RecoverWorkflowRunsV14()
	m.RecoverMissionsV14()
	m.startMissionSyncV14()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/platform/sessions/{id}/messages", m.httpSessionMessageV14)
	mux.HandleFunc("POST /v1/platform/workflows/{id}/run", m.httpWorkflowRunV14)
	mux.HandleFunc("GET /v1/platform/missions", m.httpMissionsV14)
	mux.HandleFunc("POST /v1/platform/missions", m.httpMissionDispatchV14)
	mux.HandleFunc("GET /v1/platform/missions/{id}", m.httpMissionV14)
	mux.Handle("/", m.Handler())
	return mux
}

func (m *Manager) httpSessionMessageV14(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Message string `json:"message"`
	}
	if !decodePlatform(w, r, &in) {
		return
	}
	created, err := m.SendSessionV14(r.PathValue("id"), in.Message)
	if err != nil {
		platformProblem(w, err)
		return
	}
	writePlatformJSON(w, http.StatusAccepted, created)
}

func (m *Manager) httpWorkflowRunV14(w http.ResponseWriter, r *http.Request) {
	run, err := m.RunWorkflowV14(r.PathValue("id"))
	if err != nil {
		platformProblem(w, err)
		return
	}
	writePlatformJSON(w, http.StatusAccepted, run)
}

func (m *Manager) httpMissionsV14(w http.ResponseWriter, _ *http.Request) {
	missions, err := m.MissionsV14()
	respondPlatform(w, missions, err)
}

func (m *Manager) httpMissionV14(w http.ResponseWriter, r *http.Request) {
	mission, err := m.MissionV14(r.PathValue("id"))
	respondPlatform(w, mission, err)
}

func (m *Manager) httpMissionDispatchV14(w http.ResponseWriter, r *http.Request) {
	var in Mission
	if !decodePlatform(w, r, &in) {
		return
	}
	mission, err := m.DispatchMissionV14(in)
	if err != nil {
		platformProblem(w, err)
		return
	}
	writePlatformJSON(w, http.StatusAccepted, mission)
}

// InboundHandlerV14 keeps the conservative receipt/reconciliation behavior of
// the safe inbound gateway while routing actual Session task creation through
// SendSessionDurable. This removes the remaining production dependency on the
// legacy post-create Session persistence semantics.
func (m *Manager) InboundHandlerV14() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/inbound/{id}", m.httpInboundV14)
	return mux
}

func (m *Manager) httpInboundV14(w http.ResponseWriter, r *http.Request) {
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
		_ = m.finishInbound(receipt, "", "", "failed", err.Error())
		platformProblem(w, err)
		return
	}

	created, err := m.SendSessionV14(session.ID, text)
	if err != nil {
		_ = m.finishInbound(receipt, session.ID, "", "failed", err.Error())
		platformProblem(w, err)
		return
	}
	if err := m.finishInbound(receipt, session.ID, created.ID, "task_created", ""); err != nil {
		writePlatformJSON(w, http.StatusInternalServerError, map[string]any{
			"error":   "receipt_task_link_failed",
			"task_id": created.ID,
		})
		return
	}
	if err := m.audit("channel.inbound.accepted", map[string]any{
		"channel_id":       channel.ID,
		"receipt_id":       receipt.ID,
		"session_id":       session.ID,
		"task_id":          created.ID,
		"sender_sha256_96": senderDigest(in.Sender),
	}); err != nil {
		writePlatformJSON(w, http.StatusInternalServerError, map[string]any{
			"error":      "audit_failed",
			"receipt_id": receipt.ID,
			"task_id":    created.ID,
		})
		return
	}
	if err := m.finishInbound(receipt, session.ID, created.ID, "accepted", ""); err != nil {
		writePlatformJSON(w, http.StatusInternalServerError, map[string]any{
			"error":   "receipt_acceptance_persistence_failed",
			"task_id": created.ID,
		})
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
