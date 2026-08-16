package platform

import (
	"net/http"
	"path/filepath"
)

func (m *Manager) ChannelGatewayAdminHandlerV170() http.Handler {
	g := m.gatewayV170()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/platform/channel-gateway/status", g.httpStatus)
	mux.HandleFunc("GET /v1/platform/channel-gateway/pending", g.httpPending)
	mux.HandleFunc("GET /v1/platform/channel-gateway/receipts", g.httpReceipts)
	mux.HandleFunc("POST /v1/platform/channel-gateway/pending/{id}/retry", g.httpRetry)
	mux.HandleFunc("POST /v1/platform/channel-gateway/pending/{id}/resolve", g.httpResolve)
	return mux
}

func (g *channelGatewayV170) httpStatus(w http.ResponseWriter, _ *http.Request) {
	m := g.manager
	m.mu.RLock()
	pending, _ := listJSON[OutboundDelivery](filepath.Join(m.dir, "outbound-pending"))
	routes, _ := listJSON[ChannelRoute](filepath.Join(m.dir, "channel-routes"))
	m.mu.RUnlock()
	counts := map[string]int{}
	for _, d := range pending {
		counts[d.Status]++
	}
	writePlatformJSON(w, http.StatusOK, map[string]any{
		"version":           channelGatewayVersion,
		"native_channels":   []string{"telegram", "slack", "discord", "whatsapp"},
		"routes":            len(routes),
		"pending":           len(pending),
		"pending_by_status": counts,
	})
}

func publicDelivery(d *OutboundDelivery) map[string]any {
	return map[string]any{
		"id": d.ID, "channel_id": d.ChannelID, "session_id": d.SessionID,
		"task_id": d.TaskID, "route_id": d.RouteID, "status": d.Status,
		"attempts": d.Attempts, "next_attempt_at": d.NextAttemptAt,
		"last_error": d.LastError, "created_at": d.CreatedAt, "updated_at": d.UpdatedAt,
	}
}

func (g *channelGatewayV170) httpPending(w http.ResponseWriter, _ *http.Request) {
	m := g.manager
	m.mu.RLock()
	pending, err := listJSON[OutboundDelivery](filepath.Join(m.dir, "outbound-pending"))
	m.mu.RUnlock()
	if err != nil {
		platformProblem(w, err)
		return
	}
	out := make([]map[string]any, 0, len(pending))
	for _, d := range pending {
		out = append(out, publicDelivery(d))
	}
	writePlatformJSON(w, http.StatusOK, out)
}

func (g *channelGatewayV170) httpReceipts(w http.ResponseWriter, _ *http.Request) {
	m := g.manager
	m.mu.RLock()
	receipts, err := listJSON[OutboundReceipt](filepath.Join(m.dir, "outbound-receipts"))
	m.mu.RUnlock()
	respondPlatform(w, receipts, err)
}

func (g *channelGatewayV170) httpRetry(w http.ResponseWriter, r *http.Request) {
	m := g.manager
	m.mu.Lock()
	defer m.mu.Unlock()
	var d OutboundDelivery
	if err := m.read("outbound-pending", r.PathValue("id"), &d); err != nil {
		platformProblem(w, err)
		return
	}
	if d.Status != "reconciliation" && d.Status != "failed" {
		writePlatformJSON(w, http.StatusConflict, map[string]any{"error": "delivery_not_retryable"})
		return
	}
	if err := m.audit("channel.outbound.retry_authorized", map[string]any{"delivery_id": d.ID, "channel_id": d.ChannelID, "task_id": d.TaskID}); err != nil {
		writePlatformJSON(w, http.StatusInternalServerError, map[string]any{"error": "audit_failed"})
		return
	}
	d.Status = "retry_wait"
	d.NextAttemptAt = now()
	d.LastError = ""
	d.UpdatedAt = now()
	if err := m.save("outbound-pending", d.ID, &d); err != nil {
		platformProblem(w, err)
		return
	}
	writePlatformJSON(w, http.StatusOK, publicDelivery(&d))
}

func (g *channelGatewayV170) httpResolve(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status string `json:"status"`
	}
	if !decodePlatform(w, r, &in) {
		return
	}
	if in.Status != "delivered" && in.Status != "failed" {
		writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "status_must_be_delivered_or_failed"})
		return
	}
	m := g.manager
	m.mu.RLock()
	var d OutboundDelivery
	err := m.read("outbound-pending", r.PathValue("id"), &d)
	m.mu.RUnlock()
	if err != nil {
		platformProblem(w, err)
		return
	}
	if d.Status != "reconciliation" {
		writePlatformJSON(w, http.StatusConflict, map[string]any{"error": "delivery_not_in_reconciliation"})
		return
	}
	if err := m.audit("channel.outbound.resolve_authorized", map[string]any{"delivery_id": d.ID, "channel_id": d.ChannelID, "task_id": d.TaskID, "status": in.Status}); err != nil {
		writePlatformJSON(w, http.StatusInternalServerError, map[string]any{"error": "audit_failed"})
		return
	}
	g.complete(&d, in.Status, "operator reconciled")
	writePlatformJSON(w, http.StatusOK, map[string]any{"id": d.ID, "status": in.Status})
}
