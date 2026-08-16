package api

import "net/http"

// ProbeHandler exposes only liveness/readiness without the normal API request
// quota. Production supervisors must be able to probe the daemon frequently
// without accidentally rate-limiting it into a restart loop. The outer daemon
// HTTP guard still applies security headers.
func (s *Server) ProbeHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /readyz", s.ready)
	return s.security(s.cors(mux))
}
