package provider

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/config"
)

func testTool() ToolDef {
	return ToolDef{Type: "function", Function: FunctionDef{Name: "echo", Description: "echo input", Parameters: map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "integer"}}, "required": []string{"x"}}}}
}

func TestAnthropicNativeToolRoundTrip(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "secret" || r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("missing Anthropic headers: %#v", r.Header)
		}
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		call := len(bodies)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			_, _ = w.Write([]byte(`{"content":[{"type":"tool_use","id":"toolu_1","name":"echo","input":{"x":1}}],"stop_reason":"tool_use"}`))
			return
		}
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"done"}],"stop_reason":"end_turn"}`))
	}))
	defer server.Close()

	client := New(nil, 2*time.Second)
	provider := config.Provider{Type: "anthropic", BaseURL: server.URL, Model: "claude-test", AllowPrivateNetwork: true}
	system := "system"
	user := "use echo"
	first, _, err := client.chatOne(t.Context(), provider, "secret", []Message{{Role: "system", Content: &system}, {Role: "user", Content: &user}}, []ToolDef{testTool()})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.ToolCalls) != 1 || first.ToolCalls[0].ID != "toolu_1" || first.ToolCalls[0].Function.Name != "echo" {
		t.Fatalf("unexpected first response: %#v", first)
	}
	result := `{"ok":true}`
	second, _, err := client.chatOne(t.Context(), provider, "secret", []Message{{Role: "system", Content: &system}, {Role: "user", Content: &user}, first, {Role: "tool", ToolCallID: "toolu_1", Content: &result}}, []ToolDef{testTool()})
	if err != nil {
		t.Fatal(err)
	}
	if second.Content == nil || *second.Content != "done" {
		t.Fatalf("unexpected second response: %#v", second)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 || !strings.Contains(bodies[1], `"type":"tool_result"`) || !strings.Contains(bodies[1], `"tool_use_id":"toolu_1"`) {
		t.Fatalf("tool result not preserved in Anthropic request: %v", bodies)
	}
}

func TestGeminiNativeToolRoundTrip(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-test:generateContent" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "secret" {
			t.Errorf("missing Gemini API key header")
		}
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		call := len(bodies)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"echo","args":{"x":1}}}]}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]}}]}`))
	}))
	defer server.Close()

	client := New(nil, 2*time.Second)
	provider := config.Provider{Type: "gemini", BaseURL: server.URL, Model: "gemini-test", AllowPrivateNetwork: true}
	system := "system"
	user := "use echo"
	first, _, err := client.chatOne(t.Context(), provider, "secret", []Message{{Role: "system", Content: &system}, {Role: "user", Content: &user}}, []ToolDef{testTool()})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.ToolCalls) != 1 || first.ToolCalls[0].ID == "" || first.ToolCalls[0].Function.Name != "echo" {
		t.Fatalf("unexpected first response: %#v", first)
	}
	result := `{"ok":true}`
	second, _, err := client.chatOne(t.Context(), provider, "secret", []Message{{Role: "system", Content: &system}, {Role: "user", Content: &user}, first, {Role: "tool", ToolCallID: first.ToolCalls[0].ID, Content: &result}}, []ToolDef{testTool()})
	if err != nil {
		t.Fatal(err)
	}
	if second.Content == nil || *second.Content != "done" {
		t.Fatalf("unexpected second response: %#v", second)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 || !strings.Contains(bodies[1], `"functionResponse"`) || !strings.Contains(bodies[1], `"name":"echo"`) {
		t.Fatalf("function response not preserved in Gemini request: %v", bodies)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(bodies[1]), &payload); err != nil {
		t.Fatalf("invalid Gemini request JSON: %v", err)
	}
}
