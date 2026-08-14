package authority

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBudgetHTTPPreflightAndOverview(t *testing.T) {
	store := newAuthorityTestStore(t)
	grant, err := store.CreateRoot(Envelope{
		SubjectID:         "agent:http-budget",
		Capabilities:      []string{"task.execute"},
		MaxConcurrentWork: 2,
		MaxCostUnits:      10,
	})
	if err != nil {
		t.Fatal(err)
	}

	body := bytes.NewBufferString(`{"cost_units":3}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/authority/envelopes/"+grant.Envelope.ID+"/preflight", body)
	rec := httptest.NewRecorder()
	store.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preflight status=%d body=%s", rec.Code, rec.Body.String())
	}
	var preflight BudgetPreflight
	if err := json.Unmarshal(rec.Body.Bytes(), &preflight); err != nil {
		t.Fatal(err)
	}
	if !preflight.Allowed || preflight.CostUnits != 3 || len(preflight.Lineage) != 1 {
		t.Fatalf("unexpected preflight response: %#v", preflight)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/authority/usage", nil)
	rec = httptest.NewRecorder()
	store.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("usage overview status=%d body=%s", rec.Code, rec.Body.String())
	}
	var overview struct {
		Usage []*UsageSnapshot `json:"usage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &overview); err != nil {
		t.Fatal(err)
	}
	if len(overview.Usage) != 1 || overview.Usage[0].AuthorityID != grant.Envelope.ID {
		t.Fatalf("unexpected usage overview: %#v", overview.Usage)
	}
}

func TestBudgetHTTPPreflightRejectsUnknownFields(t *testing.T) {
	store := newAuthorityTestStore(t)
	grant, err := store.CreateRoot(Envelope{SubjectID: "agent:http", Capabilities: []string{"task.execute"}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/authority/envelopes/"+grant.Envelope.ID+"/preflight", bytes.NewBufferString(`{"cost_units":1,"authority_id":"attacker"}`))
	rec := httptest.NewRecorder()
	store.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("model-style authority override field should be rejected, got status=%d body=%s", rec.Code, rec.Body.String())
	}
}
