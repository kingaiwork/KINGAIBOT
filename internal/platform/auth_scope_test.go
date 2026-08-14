package platform

import (
	"net/http"
	"net/url"
	"testing"
)

func permissionFor(method, path string) string {
	return governedPlatformPermission(&http.Request{Method: method, URL: &url.URL{Path: path}})
}

func TestGovernedPlatformPermissionClassification(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   string
	}{
		{http.MethodGet, "/v1/platform/agents", "platform.read"},
		{http.MethodPost, "/v1/platform/sessions", "platform.write"},
		{http.MethodPost, "/v1/platform/workflows", "platform.write"},
		{http.MethodPost, "/v1/platform/sessions/sess_1/messages", "platform.automation"},
		{http.MethodPost, "/v1/platform/schedules", "platform.automation"},
		{http.MethodPost, "/v1/platform/workflows/wf_1/run", "platform.automation"},
		{http.MethodPost, "/v1/platform/missions", "platform.automation"},
		{http.MethodPost, "/v1/platform/nodes/node_1/heartbeat", "platform.automation"},
		{http.MethodPost, "/v1/platform/agents", "platform.admin"},
		{http.MethodPost, "/v1/platform/plugins", "platform.admin"},
		{http.MethodPost, "/v1/platform/channels", "platform.admin"},
		{http.MethodPost, "/v1/platform/skills", "platform.admin"},
		{http.MethodPost, "/v1/platform/agents/agent_1/enabled", "platform.admin"},
		{http.MethodPost, "/v1/platform/schedules/sched_1/enabled", "platform.admin"},
		{http.MethodPost, "/v1/platform/plugins/plugin_1/enabled", "platform.admin"},
	}
	for _, tc := range cases {
		if got := permissionFor(tc.method, tc.path); got != tc.want {
			t.Fatalf("%s %s permission=%s want=%s", tc.method, tc.path, got, tc.want)
		}
	}
}
