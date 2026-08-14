package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateAPIBase(t *testing.T) {
	if _, err := validateAPIBase("http://example.com:18888"); err == nil {
		t.Fatal("public plaintext API unexpectedly accepted")
	}
	if _, err := validateAPIBase("http://localhost:18888"); err != nil {
		t.Fatalf("loopback API should be accepted: %v", err)
	}
	if _, err := validateAPIBase("https://agent.example.com"); err != nil {
		t.Fatalf("HTTPS API rejected: %v", err)
	}
}

func TestConsoleProxyForwardsOnlySelectedHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/test" || r.URL.Query().Get("q") != "1" {
			t.Errorf("unexpected upstream URL: %s", r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("authorization not forwarded")
		}
		if r.Header.Get("X-Evil") != "" {
			t.Errorf("arbitrary browser header was forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/test?q=1", nil)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("X-Evil", "do-not-forward")
	rr := httptest.NewRecorder()
	newAPIProxy(upstream.URL).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("unexpected proxy response: %d %s", rr.Code, rr.Body.String())
	}
}
