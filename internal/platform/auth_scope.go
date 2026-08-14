package platform

import (
	"net/http"
	"strings"
)

// AutomationAuthHandler protects operations that create or trigger autonomous
// work. platform.write may edit ordinary control-plane data, but it is not
// sufficient to launch schedules, workflow runs or missions.
func (m *Manager) AutomationAuthHandler(adminTokenEnv string, next http.Handler) http.Handler {
	return m.authWithPermission(adminTokenEnv, "platform.automation", next)
}

// governedPlatformPermission assigns the minimum scoped permission required by
// a platform request. Reads are read-only; routine durable configuration uses
// write; operations that can launch work require automation; and trust-surface
// registration/activation requires admin.
func governedPlatformPermission(r *http.Request) string {
	if r == nil {
		return "platform.admin"
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return "platform.read"
	}
	path := ""
	if r.URL != nil {
		path = r.URL.Path
	}

	// Re-enabling or disabling an execution/trust surface is administrative.
	if strings.HasSuffix(path, "/enabled") {
		return "platform.admin"
	}

	// Creating resources that can directly extend remote execution or trusted
	// behavior is administrative even if the resource starts inert/audited.
	switch path {
	case "/v1/platform/agents", "/v1/platform/nodes", "/v1/platform/plugins", "/v1/platform/channels", "/v1/platform/skills":
		if r.Method == http.MethodPost {
			return "platform.admin"
		}
	}

	// Triggering or installing autonomous work requires the dedicated automation
	// permission. This keeps ordinary operators from launching work by virtue of
	// generic platform.write access.
	if r.Method == http.MethodPost {
		if path == "/v1/platform/schedules" || path == "/v1/platform/missions" ||
			strings.HasSuffix(path, "/messages") || strings.HasSuffix(path, "/run") ||
			strings.HasSuffix(path, "/heartbeat") {
			return "platform.automation"
		}
	}

	return "platform.write"
}

// GovernedScopedAuthHandler applies the path-level permission policy used by
// the v1.4 production control surface. The legacy environment admin token keeps
// backward compatibility while scoped access keys receive least privilege.
func (m *Manager) GovernedScopedAuthHandler(adminTokenEnv string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.authWithPermission(adminTokenEnv, governedPlatformPermission(r), next).ServeHTTP(w, r)
	})
}
