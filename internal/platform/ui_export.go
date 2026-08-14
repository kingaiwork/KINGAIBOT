package platform

import "net/http"

// ControlCenterHandler returns the static zero-dependency web UI. It contains
// no platform state and can therefore be served by the standalone kingconsole.
func ControlCenterHandler() http.Handler {
	return (&Manager{}).UIHandler()
}
