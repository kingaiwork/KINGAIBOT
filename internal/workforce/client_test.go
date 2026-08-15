package workforce

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testServiceToken() string { return "ksv_" + strings.Repeat("a", 64) }

func testSettings(url string) Settings {
	return Settings{Enabled: true, ControlPlaneURL: url, ServiceToken: testServiceToken(), AllowInsecureHTTP: true, RequestTimeout: 2 * time.Second}
}

func TestClientHeartbeatAndSync(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testServiceToken() {
			t.Fatalf("missing bearer service token")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/workforce/runtime/heartbeat":
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/v1/workforce/runtime/sync":
			_, _ = w.Write([]byte(`{"ok":true,"schema":"kingai.workforce.v2","skills_schema":"kingai.workforce.skills.v1","employees":[],"workflows":[],"connectors":[],"connector_bindings":[],"policy":{"cloud_never_bypasses_local_approval":true,"arbitrary_shell":false,"credentials_in_cloud":false,"connector_config_grants_permission":false,"generic_remote_shell":false,"execution_boundary":"KINGAIBOT customer-local capability-envelope-policy-approval"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewClient(testSettings(server.URL), "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Heartbeat(context.Background(), []string{"v14-authority"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestClientRejectsUnsafeSyncPolicy(t *testing.T) {
	bodies := []string{
		`{"ok":true,"schema":"kingai.workforce.v2","policy":{"cloud_never_bypasses_local_approval":false,"arbitrary_shell":false,"credentials_in_cloud":false,"connector_config_grants_permission":false,"generic_remote_shell":false}}`,
		`{"ok":true,"schema":"kingai.workforce.v2","policy":{"cloud_never_bypasses_local_approval":true,"arbitrary_shell":true,"credentials_in_cloud":false,"connector_config_grants_permission":false,"generic_remote_shell":false}}`,
		`{"ok":true,"schema":"kingai.workforce.v2","policy":{"cloud_never_bypasses_local_approval":true,"arbitrary_shell":false,"credentials_in_cloud":true,"connector_config_grants_permission":false,"generic_remote_shell":false}}`,
		`{"ok":true,"schema":"kingai.workforce.v2","policy":{"cloud_never_bypasses_local_approval":true,"arbitrary_shell":false,"credentials_in_cloud":false,"connector_config_grants_permission":true,"generic_remote_shell":false}}`,
		`{"ok":true,"schema":"kingai.workforce.v2","policy":{"cloud_never_bypasses_local_approval":true,"arbitrary_shell":false,"credentials_in_cloud":false,"connector_config_grants_permission":false,"generic_remote_shell":true}}`,
	}
	for _, body := range bodies {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		client, err := NewClient(testSettings(server.URL), "test")
		if err != nil {
			server.Close()
			t.Fatal(err)
		}
		if _, err := client.Sync(context.Background()); err == nil {
			server.Close()
			t.Fatal("unsafe workforce cloud policy must be rejected")
		}
		server.Close()
	}
}

func TestClientRejectsUnknownWorkforceSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"schema":"kingai.workforce.v99","policy":{"cloud_never_bypasses_local_approval":true,"arbitrary_shell":false,"credentials_in_cloud":false,"connector_config_grants_permission":false,"generic_remote_shell":false}}`))
	}))
	defer server.Close()
	client, err := NewClient(testSettings(server.URL), "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Sync(context.Background()); err == nil {
		t.Fatal("unknown workforce schema must fail closed")
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
	client, err := NewClient(testSettings(redirector.URL), "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Heartbeat(context.Background(), nil); err == nil {
		t.Fatal("redirect response must fail")
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

func TestServiceTokenFormat(t *testing.T) {
	if !serviceTokenPattern.MatchString(testServiceToken()) {
		t.Fatal("valid workforce service token rejected")
	}
	if serviceTokenPattern.MatchString("knode_" + strings.Repeat("a", 64)) {
		t.Fatal("legacy node token must not be accepted by v14 workforce client")
	}
}
