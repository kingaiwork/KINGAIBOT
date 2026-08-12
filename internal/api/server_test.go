package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Name:    "KINGAIBOT",
		Version: "1.1.0",
		Server: config.Server{
			Listen:        "127.0.0.1:18888",
			AdminTokenEnv: "TEST_ADMIN_TOKEN",
			MCPTokenEnv:   "TEST_MCP_TOKEN",
			A2ATokenEnv:   "TEST_A2A_TOKEN",
		},
		Runtime:   config.Runtime{MaxRequestBytes: 1 << 20},
		Providers: []config.Provider{{Name: "p", Enabled: true, APIKeyEnv: "TEST_MODEL_TOKEN"}},
		Protocols: config.Protocols{MCP: true, A2A: true},
	}
}

func TestAgentCardNeverReflectsUntrustedHost(t *testing.T) {
	c := testConfig()
	s := New(c, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "http://evil.example/.well-known/agent-card.json", nil)
	req.Host = "evil.example"
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "evil.example") {
		t.Fatalf("agent card reflected untrusted Host header: %s", rr.Body.String())
	}
}

func TestMCPDiscoverAndA2AVersionedMethodNames(t *testing.T) {
	c := testConfig()
	t.Setenv("TEST_MCP_TOKEN", strings.Repeat("m", 32))
	t.Setenv("TEST_A2A_TOKEN", strings.Repeat("a", 32))
	s := New(c, nil, nil)

	mcpReq := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{}}`))
	mcpReq.Header.Set("Authorization", "Bearer "+strings.Repeat("m", 32))
	mcpRR := httptest.NewRecorder()
	s.Handler().ServeHTTP(mcpRR, mcpReq)
	if mcpRR.Code != http.StatusOK || !strings.Contains(mcpRR.Body.String(), `"protocolVersion":"2026-07-28"`) {
		t.Fatalf("unexpected MCP discover response: %d %s", mcpRR.Code, mcpRR.Body.String())
	}

	oldReq := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"message/send","params":{}}`))
	oldReq.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	oldReq.Header.Set("A2A-Version", "1.0")
	oldRR := httptest.NewRecorder()
	s.Handler().ServeHTTP(oldRR, oldReq)
	var body map[string]any
	if err := json.Unmarshal(oldRR.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] == nil {
		t.Fatalf("legacy A2A method unexpectedly accepted: %s", oldRR.Body.String())
	}
}

func TestReadyRequiresSeparatedSecretsAndModelCredential(t *testing.T) {
	c := testConfig()
	for _, k := range []string{"TEST_ADMIN_TOKEN", "TEST_MCP_TOKEN", "TEST_A2A_TOKEN", "TEST_MODEL_TOKEN"} {
		_ = os.Unsetenv(k)
	}
	t.Setenv("TEST_ADMIN_TOKEN", strings.Repeat("d", 32))
	t.Setenv("TEST_MCP_TOKEN", strings.Repeat("m", 32))
	t.Setenv("TEST_A2A_TOKEN", strings.Repeat("a", 32))
	s := New(c, nil, nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected not ready without model key, got %d", rr.Code)
	}
	t.Setenv("TEST_MODEL_TOKEN", "model")
	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected ready, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestReadyRejectsReusedProtocolSecrets(t *testing.T) {
	c := testConfig()
	shared := strings.Repeat("s", 32)
	t.Setenv("TEST_ADMIN_TOKEN", shared)
	t.Setenv("TEST_MCP_TOKEN", shared)
	t.Setenv("TEST_A2A_TOKEN", strings.Repeat("a", 32))
	t.Setenv("TEST_MODEL_TOKEN", "model")
	s := New(c, nil, nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusServiceUnavailable || !strings.Contains(rr.Body.String(), "auth_secret_reuse") {
		t.Fatalf("expected reused-secret readiness failure, got %d %s", rr.Code, rr.Body.String())
	}
}
