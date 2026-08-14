package platform

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func persistInboundReceiptForTest(t *testing.T, m *Manager, receipt *InboundReceipt) {
	t.Helper()
	if err := m.ensureInboundDir(); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	err := m.save("inbound-receipts", receipt.ID, receipt)
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
}

func TestInboundReconciliationLinksOnlyMatchingExistingTask(t *testing.T) {
	m := newManagerForTest(t)
	session, err := m.CreateSession(Session{Channel: "chan_alpha", Sender: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := m.SendSessionDurable(session.ID, "hello")
	if err != nil {
		t.Fatal(err)
	}
	receipt := &InboundReceipt{
		ID:        inboundReceiptID("chan_alpha", "evt_1"),
		ChannelID: "chan_alpha",
		EventID:   "evt_1",
		Sender:    "alice",
		Status:    "reconciliation",
		CreatedAt: now(),
		UpdatedAt: now(),
	}
	persistInboundReceiptForTest(t, m, receipt)

	resolved, err := m.ReconcileInboundReceipt(receipt.ID, "link_task", created.ID, "verified durable task metadata")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != "accepted" || resolved.TaskID != created.ID || resolved.SessionID != session.ID {
		t.Fatalf("unexpected resolved receipt: %#v", resolved)
	}
}

func TestInboundReconciliationRejectsMismatchedTaskEvidence(t *testing.T) {
	m := newManagerForTest(t)
	session, err := m.CreateSession(Session{Channel: "chan_alpha", Sender: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := m.SendSessionDurable(session.ID, "hello")
	if err != nil {
		t.Fatal(err)
	}
	receipt := &InboundReceipt{
		ID:        inboundReceiptID("chan_alpha", "evt_2"),
		ChannelID: "chan_alpha",
		EventID:   "evt_2",
		Sender:    "bob",
		Status:    "reconciliation",
		CreatedAt: now(),
		UpdatedAt: now(),
	}
	persistInboundReceiptForTest(t, m, receipt)

	if _, err := m.ReconcileInboundReceipt(receipt.ID, "link_task", created.ID, "wrong sender must fail"); err == nil {
		t.Fatal("expected mismatched task evidence to be rejected")
	}
	got, err := m.InboundReceipt(receipt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "reconciliation" || got.TaskID != "" {
		t.Fatalf("receipt changed despite failed evidence validation: %#v", got)
	}
}

func TestInboundReconciliationCanFailClosedWithoutRetry(t *testing.T) {
	m := newManagerForTest(t)
	receipt := &InboundReceipt{
		ID:        inboundReceiptID("chan_alpha", "evt_3"),
		ChannelID: "chan_alpha",
		EventID:   "evt_3",
		Sender:    "alice",
		Status:    "processing",
		CreatedAt: now(),
		UpdatedAt: now(),
	}
	persistInboundReceiptForTest(t, m, receipt)

	resolved, err := m.ReconcileInboundReceipt(receipt.ID, "mark_failed", "", "operator verified no safe automatic recovery")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != "failed" || resolved.TaskID != "" {
		t.Fatalf("unexpected fail-closed receipt: %#v", resolved)
	}
}

func TestInboundAdminHandlerReconcile(t *testing.T) {
	m := newManagerForTest(t)
	receipt := &InboundReceipt{
		ID:        inboundReceiptID("chan_alpha", "evt_4"),
		ChannelID: "chan_alpha",
		EventID:   "evt_4",
		Sender:    "alice",
		Status:    "processing",
		CreatedAt: now(),
		UpdatedAt: now(),
	}
	persistInboundReceiptForTest(t, m, receipt)

	body := `{"decision":"mark_failed","note":"manual review completed"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/platform/inbound-receipts/"+receipt.ID+"/reconcile", strings.NewReader(body))
	rec := httptest.NewRecorder()
	m.InboundAdminHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got InboundReceipt
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" {
		t.Fatalf("unexpected handler result: %#v", got)
	}
}
