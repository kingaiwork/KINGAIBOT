package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

func (m *Manager) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/cloud/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"state": m.Snapshot(), "policy": m.Policy(), "security_boundary": "cloud policy can only narrow local runtime authority; cloud cannot approve or bypass local policy, exact approvals, authority envelopes or audit"})
	})
	mux.HandleFunc("POST /v1/cloud/policy/pull", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := contextWithTimeout(r, 30*time.Second)
		defer cancel()
		p, err := m.PullPolicy(ctx)
		if err != nil {
			http.Error(w, "cloud policy pull failed", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(p)
	})
	mux.HandleFunc("POST /v1/cloud/sync", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := contextWithTimeout(r, 60*time.Second)
		defer cancel()
		if err := m.SyncOnce(ctx); err != nil {
			http.Error(w, "cloud sync failed", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(m.Snapshot())
	})
	return mux
}

func contextWithTimeout(r *http.Request, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), timeout)
}
