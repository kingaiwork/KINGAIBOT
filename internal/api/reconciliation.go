package api

import (
	"net/http"
	"strings"
)

// TaskReconciliationHandler is mounted as an exact admin route by kingagentd.
// It is intentionally separate from MCP/A2A/model tools: resolving ambiguous
// side effects is an operator governance action, never an agent capability.
func (s *Server) TaskReconciliationHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /v1/tasks/{id}/reconcile", s.authEnv(s.cfg.Server.AdminTokenEnv, http.HandlerFunc(s.reconcileTask)))
	return s.security(s.cors(s.limit(mux)))
}

func (s *Server) reconcileTask(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Decision    string `json:"decision"`
		Note        string `json:"note"`
		AllowReplay bool   `json:"allow_replay,omitempty"`
	}
	if decode(r, w, 64<<10, &in) != nil {
		return
	}
	resolved, err := s.rt.ResolveReconciliation(
		r.PathValue("id"),
		strings.ToLower(strings.TrimSpace(in.Decision)),
		in.Note,
		in.AllowReplay,
	)
	if err != nil {
		problem(w, http.StatusBadRequest, "reconciliation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resolved)
}
