package api

import (
	"net/http"
	"strings"
)

// ApprovalDecisionV14Handler is mounted as an exact admin route by kingagentd.
// It overrides the compatibility approval decision endpoint so production
// approval trust uses Runtime's staged, audit-backed V14 state machine.
func (s *Server) ApprovalDecisionV14Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /v1/approvals/{id}/decision", s.authEnv(s.cfg.Server.AdminTokenEnv, http.HandlerFunc(s.approvalDecisionV14)))
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
