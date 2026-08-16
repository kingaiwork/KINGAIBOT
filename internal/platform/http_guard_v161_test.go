package platform

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPGuardDoesNotRateLimitHealthProbes(t *testing.T) {
	h := HTTPGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for i := 0; i < rootRateLimit*2; i++ {
		req := httptest.NewRequest(http.MethodGet, "http://example.test/healthz", nil)
		req.RemoteAddr = "192.0.2.30:12345"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("health probe %d unexpectedly limited: %d", i, rr.Code)
		}
	}
	// Probe traffic must not consume the same caller's normal API allowance.
	req := httptest.NewRequest(http.MethodGet, "http://example.test/v1/platform/status", nil)
	req.RemoteAddr = "192.0.2.30:12345"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("normal request was affected by probe quota: %d", rr.Code)
	}
}
