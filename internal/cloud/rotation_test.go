package cloud

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestAmbiguousRotationCommitRecovers(t *testing.T) {
	m, err := New(t.TempDir(), Config{Enabled: true, BaseURL: "https://api.kingai.work"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.mu.Lock()
	m.state.Enrolled = true
	m.state.NodeID = "11111111-1111-4111-8111-111111111111"
	m.state.KeyID = "22222222-2222-4222-8222-222222222222"
	m.mu.Unlock()
	if err := m.saveState(); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	commitCalls := 0
	m.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/ops/nodes/key-rotation/prepare":
			return jsonResponse(http.StatusCreated, `{"rotation_id":"33333333-3333-4333-8333-333333333333","new_key_id":"44444444-4444-4444-8444-444444444444","phase":"prepared"}`), nil
		case "/api/v1/ops/nodes/key-rotation/commit":
			mu.Lock()
			commitCalls++
			call := commitCalls
			mu.Unlock()
			if call == 1 {
				// Simulate the classic ambiguous transport window: the remote side may
				// have committed, but the local caller never received the response.
				return nil, errors.New("connection dropped after request write")
			}
			return jsonResponse(http.StatusOK, `{"rotation_id":"33333333-3333-4333-8333-333333333333","new_key_id":"44444444-4444-4444-8444-444444444444","phase":"committed","idempotent":true}`), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not_found"}`), nil
		}
	})}

	if _, err := m.RotateKey(t.Context()); err == nil {
		t.Fatal("first rotation should surface ambiguous commit")
	}
	if got := m.RotationStatus(); got.Status != "prepared" || got.RotationID == "" {
		t.Fatalf("rotation recovery marker lost: %+v", got)
	}
	if err := m.RecoverKeyRotation(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := m.RotationStatus(); got.Status != "idle" {
		t.Fatalf("rotation marker not cleaned after recovery: %+v", got)
	}
	state := m.Snapshot()
	if state.KeyID != "44444444-4444-4444-8444-444444444444" || state.LastKeyRotationAt.IsZero() {
		t.Fatalf("active key state was not committed: %+v", state)
	}
	mu.Lock()
	calls := commitCalls
	mu.Unlock()
	if calls != 2 {
		t.Fatalf("expected one ambiguous commit plus one idempotent recovery, got %d", calls)
	}
}
