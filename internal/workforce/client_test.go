package workforce

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testToken() string { return "knode_" + strings.Repeat("a", 64) }

func TestClientHeartbeatAndSync(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken() {
			t.Fatalf("missing bearer token")
		}
		switch r.URL.Path {
		case "/api/workforce/runtime/heartbeat":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/workforce/runtime/sync":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"schema":"kingai.workforce.v1","skills_schema":"kingai.workforce.skills.v1","employees":[],"workflows":[],"connectors":[],"connector_bindings":[],"policy":{"cloud_never_bypasses_local_approval":true,"arbitrary_shell":false,"credentials_in_cloud":false,"connector_config_grants_permission":false,"execution_boundary":"KINGAIBOT customer-local"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	settings := Settings{Enabled: true, ControlPlaneURL: server.URL, NodeToken: testToken(), AllowInsecureHTTP: true, RequestTimeout: 2 * time.Second}
	client, err := NewClient(settings, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Heartbeat(context.Background(), []string{"durable-tasks"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestClientRejectsUnsafeSyncPolicy(t *testing.T) {
	cases := []string{
		`{"ok":true,"policy":{"cloud_never_bypasses_local_approval":false,"arbitrary_shell":true}}`,
		`{"ok":true,"policy":{"cloud_never_bypasses_local_approval":true,"arbitrary_shell":false,"credentials_in_cloud":true}}`,
		`{"ok":true,"policy":{"cloud_never_bypasses_local_approval":true,"arbitrary_shell":false,"connector_config_grants_permission":true}}`,
	}
	for _, body := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		client, err := NewClient(Settings{Enabled: true, ControlPlaneURL: server.URL, NodeToken: testToken(), AllowInsecureHTTP: true, RequestTimeout: 2 * time.Second}, "test")
		if err != nil {
			server.Close()
			t.Fatal(err)
		}
		if _, err := client.Sync(context.Background()); err == nil {
			server.Close()
			t.Fatal("expected unsafe cloud policy to be rejected")
		}
		server.Close()
	}
}

func TestAuthenticatedRedirectIsNotFollowed(t *testing.T) {
	followed := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		followed = true
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirector.Close()
	client, err := NewClient(Settings{Enabled: true, ControlPlaneURL: redirector.URL, NodeToken: testToken(), AllowInsecureHTTP: true, RequestTimeout: 2 * time.Second}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Heartbeat(context.Background(), nil); err == nil {
		t.Fatal("expected redirect response to fail")
	}
	if followed {
		t.Fatal("authenticated redirect was followed")
	}
}

func TestControlPlaneHTTPRestrictedToLoopback(t *testing.T) {
	if err := validateControlPlaneURL("http://example.com", true); err == nil {
		t.Fatal("non-loopback insecure HTTP should be rejected")
	}
	if err := validateControlPlaneURL("http://127.0.0.1:8787", true); err != nil {
		t.Fatalf("loopback development URL should be allowed: %v", err)
	}
}
