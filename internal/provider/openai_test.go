package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/config"
)

func TestChatCompatibleProvider(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer ts.Close()
	c := New([]config.Provider{{Name: "mock", BaseURL: ts.URL, Model: "mock", Enabled: true, AllowPrivateNetwork: true}}, 5*time.Second)
	msg, name, err := c.Chat(context.Background(), []Message{{Role: "user", Content: str("hi")}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if name != "mock" || msg.Content == nil || *msg.Content != "ok" {
		t.Fatalf("unexpected result: %#v %s", msg, name)
	}
}
func str(s string) *string { return &s }
