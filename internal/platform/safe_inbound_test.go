package platform

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func createInboundChannel(t *testing.T, h *safeHarness) *Channel {
	t.Helper()
	t.Setenv("KINGAIBOT_TEST_CHANNEL_TOKEN", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	channel, err := h.manager.CreateChannelSafe(Channel{
		Name:              "webhook",
		Kind:              "webhook",
		Endpoint:          "http://127.0.0.1:19002",
		AllowInsecureHTTP: true,
		BearerTokenEnv:    "KINGAIBOT_TEST_CHANNEL_TOKEN",
		AllowedSenders:    []string{"alice"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return channel
}

func inboundRequest(t *testing.T, handler http.Handler, channelID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/inbound/"+channelID, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestSafeInboundDuplicateReturnsOriginalTaskWithoutReexecution(t *testing.T) {
	h := newSafeHarness(t)
	channel := createInboundChannel(t, h)
	handler := h.manager.InboundHandlerSafe()
	payload := map[string]any{"event_id": "evt-1", "sender": "alice", "text": "hello"}
	first := inboundRequest(t, handler, channel.ID, payload)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first inbound status=%d body=%s", first.Code, first.Body.String())
	}
	if runtimeTaskCount(h.runtime) != 1 {
		t.Fatalf("expected one task after first delivery, got %d", runtimeTaskCount(h.runtime))
	}
	var firstBody map[string]any
	if err := json.Unmarshal(first.Body.Bytes(), &firstBody); err != nil {
		t.Fatal(err)
	}
	second := inboundRequest(t, handler, channel.ID, payload)
	if second.Code != http.StatusAccepted {
		t.Fatalf("duplicate inbound status=%d body=%s", second.Code, second.Body.String())
	}
	if runtimeTaskCount(h.runtime) != 1 {
		t.Fatalf("duplicate inbound re-executed task; got %d tasks", runtimeTaskCount(h.runtime))
	}
	var secondBody map[string]any
	if err := json.Unmarshal(second.Body.Bytes(), &secondBody); err != nil {
		t.Fatal(err)
	}
	if duplicate, _ := secondBody["duplicate"].(bool); !duplicate {
		t.Fatalf("duplicate response not marked duplicate: %#v", secondBody)
	}
	if firstBody["task_id"] == "" || firstBody["task_id"] != secondBody["task_id"] {
		t.Fatalf("duplicate did not preserve task identity: first=%#v second=%#v", firstBody, secondBody)
	}
}

func TestSafeInboundAmbiguousProcessingRequiresReconciliation(t *testing.T) {
	h := newSafeHarness(t)
	channel := createInboundChannel(t, h)
	if _, duplicate, err := h.manager.reserveInboundSafe(channel, "evt-ambiguous", "alice", nil); err != nil || duplicate {
		t.Fatalf("failed to create processing receipt: duplicate=%v err=%v", duplicate, err)
	}
	rec := inboundRequest(t, h.manager.InboundHandlerSafe(), channel.ID, map[string]any{"event_id": "evt-ambiguous", "sender": "alice", "text": "must not replay"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("ambiguous retry must require reconciliation, status=%d body=%s", rec.Code, rec.Body.String())
	}
	if runtimeTaskCount(h.runtime) != 0 {
		t.Fatal("ambiguous processing receipt was blindly re-executed")
	}
}

func TestSafeInboundRespectsDisabledSessionAgent(t *testing.T) {
	h := newSafeHarness(t)
	channel := createInboundChannel(t, h)
	agent, err := h.manager.CreateAgentSafe(AgentProfile{Name: "inbound-agent"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := h.manager.CreateSession(Session{AgentID: agent.ID, Channel: channel.ID, Sender: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.manager.SetAgentEnabledSafe(agent.ID, false); err != nil {
		t.Fatal(err)
	}
	rec := inboundRequest(t, h.manager.InboundHandlerSafe(), channel.ID, map[string]any{"event_id": "evt-disabled", "sender": "alice", "session_id": session.ID, "text": "do not run"})
	if rec.Code == http.StatusAccepted {
		t.Fatalf("disabled agent inbound unexpectedly accepted: %s", rec.Body.String())
	}
	if runtimeTaskCount(h.runtime) != 0 {
		t.Fatal("disabled inbound agent created a runtime task")
	}
}
