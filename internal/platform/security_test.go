package platform

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScopedAccessKeyLifecycle(t *testing.T) {
	m := newManagerForTest(t)
	id, err := m.CreateIdentity(Identity{Name: "viewer", Roles: []string{"viewer"}})
	if err != nil {
		t.Fatal(err)
	}
	issued, err := m.IssueAccessKey(id.ID, 3600)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(issued.Token, "kai_key_") {
		t.Fatalf("unexpected token format: %q", issued.Token)
	}
	if _, err := m.AuthenticateAccessToken(issued.Token, "platform.read"); err != nil {
		t.Fatalf("viewer should have read permission: %v", err)
	}
	if _, err := m.AuthenticateAccessToken(issued.Token, "platform.write"); err == nil {
		t.Fatal("viewer unexpectedly received write permission")
	}
	b, err := os.ReadFile(filepath.Join(m.dir, "access-keys", issued.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte(issued.Token)) {
		t.Fatal("raw access token was persisted")
	}
	if !bytes.Contains(b, []byte(`"token_hash"`)) {
		t.Fatal("access-key verifier hash was not persisted")
	}
	if _, err := m.RevokeAccessKey(issued.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AuthenticateAccessToken(issued.Token, "platform.read"); err == nil {
		t.Fatal("revoked access key still authenticates")
	}
}

func TestAdminRoleCanUseAdminPermission(t *testing.T) {
	m := newManagerForTest(t)
	id, err := m.CreateIdentity(Identity{Name: "admin", Roles: []string{"admin"}})
	if err != nil {
		t.Fatal(err)
	}
	issued, err := m.IssueAccessKey(id.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.AuthenticateAccessToken(issued.Token, "platform.admin"); err != nil {
		t.Fatalf("admin role should satisfy platform.admin: %v", err)
	}
}

func TestInboundChannelIsIdempotent(t *testing.T) {
	m := newManagerForTest(t)
	t.Setenv("KINGAIBOT_TEST_CHANNEL_TOKEN", strings.Repeat("x", 40))
	c, err := m.CreateChannel(Channel{
		Name:           "test-webhook",
		Kind:           "webhook",
		Endpoint:       "https://example.com/outbound",
		BearerTokenEnv: "KINGAIBOT_TEST_CHANNEL_TOKEN",
		AllowedSenders: []string{"alice"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"event_id":"evt-1","sender":"alice","text":"hello"}`
	do := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/inbound/"+c.ID, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 40))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		m.InboundHandler().ServeHTTP(rr, req)
		return rr
	}
	first := do()
	if first.Code != http.StatusAccepted {
		t.Fatalf("first delivery status=%d body=%s", first.Code, first.Body.String())
	}
	var firstBody map[string]any
	if err := json.Unmarshal(first.Body.Bytes(), &firstBody); err != nil {
		t.Fatal(err)
	}
	second := do()
	if second.Code != http.StatusAccepted {
		t.Fatalf("duplicate status=%d body=%s", second.Code, second.Body.String())
	}
	var secondBody map[string]any
	if err := json.Unmarshal(second.Body.Bytes(), &secondBody); err != nil {
		t.Fatal(err)
	}
	if secondBody["duplicate"] != true {
		t.Fatalf("expected duplicate response, got %v", secondBody)
	}
	if firstBody["task_id"] == "" || firstBody["task_id"] != secondBody["task_id"] {
		t.Fatalf("duplicate delivery created a different task: first=%v second=%v", firstBody, secondBody)
	}
}

func TestInboundRejectsUnauthorizedAndUnlistedSender(t *testing.T) {
	m := newManagerForTest(t)
	t.Setenv("KINGAIBOT_TEST_CHANNEL_TOKEN", strings.Repeat("y", 40))
	c, err := m.CreateChannel(Channel{
		Name:           "locked-webhook",
		Endpoint:       "https://example.com/outbound",
		BearerTokenEnv: "KINGAIBOT_TEST_CHANNEL_TOKEN",
		AllowedSenders: []string{"alice"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/inbound/"+c.ID, strings.NewReader(`{"event_id":"evt-2","sender":"alice","text":"hello"}`))
	rr := httptest.NewRecorder()
	m.InboundHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/inbound/"+c.ID, strings.NewReader(`{"event_id":"evt-3","sender":"mallory","text":"hello"}`))
	req.Header.Set("Authorization", "Bearer "+strings.Repeat("y", 40))
	rr = httptest.NewRecorder()
	m.InboundHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestStatusSnapshotCountsPlatformObjects(t *testing.T) {
	m := newManagerForTest(t)
	if _, err := m.CreateAgent(AgentProfile{Name: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateSkill(Skill{Name: "s", Instructions: "do one safe thing"}); err != nil {
		t.Fatal(err)
	}
	status, err := m.StatusSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if status.Counts["agents"] != 1 || status.Counts["skills"] != 1 {
		t.Fatalf("unexpected counts: %#v", status.Counts)
	}
}
