package platform

import "net/http"

// AutomationAuthHandler protects operations that create or trigger autonomous
// work. platform.write may edit ordinary control-plane data, but it is not
// sufficient to launch schedules, workflow runs or missions.
func (m *Manager) AutomationAuthHandler(adminTokenEnv string, next http.Handler) http.Handler {
	return m.authWithPermission(adminTokenEnv, "platform.automation", next)
}
