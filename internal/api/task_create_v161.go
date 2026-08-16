package api

import (
	"errors"
	"net/http"
	"strings"

	karuntime "github.com/kingaiwork/KINGAIBOT/internal/runtime"
)

// TaskCreateHandlerV161 preserves the existing authenticated task API while
// distinguishing caller errors from temporary runtime backpressure.
func (s *Server) TaskCreateHandlerV161() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /v1/tasks", s.authEnv(s.cfg.Server.AdminTokenEnv, http.HandlerFunc(s.createTaskV161)))
	return s.security(s.cors(s.limit(mux)))
}

func (s *Server) createTaskV161(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Input    string         `json:"input"`
		Metadata map[string]any `json:"metadata"`
	}
	if decode(r, w, s.cfg.Runtime.MaxRequestBytes, &in) != nil {
		return
	}
	t, err := s.rt.Create(strings.TrimSpace(in.Input), in.Metadata)
	if err == nil {
		writeJSON(w, http.StatusAccepted, t)
		return
	}
	if errors.Is(err, karuntime.ErrQueueUnavailable) {
		w.Header().Set("Retry-After", "1")
		problem(w, http.StatusServiceUnavailable, "runtime_busy", "runtime queue is temporarily unavailable")
		return
	}
	if errors.Is(err, karuntime.ErrInvalidTaskInput) {
		problem(w, http.StatusBadRequest, "invalid_task", err.Error())
		return
	}
	problem(w, http.StatusInternalServerError, "task_create_failed", "task could not be created")
}
