package platform

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPGuardAddsSecurityHeaders(t *testing.T) {
	h := HTTPGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "http://example.test/v1/platform/status", nil)
	req.RemoteAddr = "192.0.2.10:12345"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d", rr.Code)
	}
	for _, name := range []string{"Cache-Control", "X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "Permissions-Policy"} {
		if rr.Header().Get(name) == "" {
			t.Fatalf("missing security header %s", name)
		}
	}
}

func TestHTTPGuardRateLimitsByRemoteAddressNotForwardedHeader(t *testing.T) {
	calls := 0
	h := HTTPGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))
	for i := 0; i < rootRateLimit; i++ {
		req := httptest.NewRequest(http.MethodGet, "http://example.test/v1/platform/status", nil)
		req.RemoteAddr = "192.0.2.20:4444"
		req.Header.Set("X-Forwarded-For", "198.51.100.1")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("request %d unexpectedly limited: %d", i, rr.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.test/v1/platform/status", nil)
	req.RemoteAddr = "192.0.2.20:9999"
	req.Header.Set("X-Forwarded-For", "203.0.113.200")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}
	if calls != rootRateLimit {
		t.Fatalf("handler calls=%d want=%d", calls, rootRateLimit)
	}
}
