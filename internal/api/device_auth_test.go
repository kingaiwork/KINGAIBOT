package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/device"
)

func TestScopedDeviceAuthorization(t *testing.T) {
	store, err := device.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pairing, secret, err := store.CreatePairing([]string{device.ScopeTasksRead}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := store.ConsumePairing(pairing.ID, secret, "Phone", "android")
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	admin := strings.Repeat("d", 32)
	t.Setenv(cfg.Server.AdminTokenEnv, admin)
	s := New(cfg, nil, nil, store)

	okHandler := s.authScoped(device.ScopeTasksRead, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	okHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("scoped token rejected: %d %s", rr.Code, rr.Body.String())
	}

	deniedHandler := s.authScoped(device.ScopeApprovalsDecide, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rr = httptest.NewRecorder()
	deniedHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("ungranted scope should be forbidden, got %d", rr.Code)
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/", nil)
	adminReq.Header.Set("Authorization", "Bearer "+admin)
	rr = httptest.NewRecorder()
	deniedHandler.ServeHTTP(rr, adminReq)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("admin fallback rejected: %d %s", rr.Code, rr.Body.String())
	}
}
