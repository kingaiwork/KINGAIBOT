package api

import (
	"net/http"
	"strings"
)

// ApprovalDecisionV14Handler is mounted on exact admin routes by kingagentd.
// Both the compatibility endpoint and the explicit /decision endpoint use the
// staged, audit-backed V14 state machine so production traffic cannot fall back
// to the legacy persist-first approval transition.
func (s *Server) ApprovalDecisionV14Handler() http.Handler {
	mux := http.NewServeMux()
	handler := s.authEnv(s.cfg.Server.AdminTokenEnv, http.HandlerFunc(s.approvalDecisionV14))
	mux.Handle("POST /v1/approvals/{id}", handler)
	mux.Handle("POST /v1/approvals/{id}/decision", handler)
	return s.security(s.cors(s.limit(mux)))
}

func (s *Server) approvalDecisionV14(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status string `json:"status"`
	}
	if decode(r, w, 64<<10, &in) != nil {
		return
	}
	if err := s.rt.DecideApprovalV14(r.PathValue("id"), strings.ToLower(strings.TrimSpace(in.Status))); err != nil {
		problem(w, http.StatusBadRequest, "approval_decision_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
