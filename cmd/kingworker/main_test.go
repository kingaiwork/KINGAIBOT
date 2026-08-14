package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestValidateCoordinatorRequiresTLSOffLoopback(t *testing.T) {
	if _, err := validateCoordinator("http://example.com:18888"); err == nil {
		t.Fatal("public plaintext coordinator unexpectedly accepted")
	}
	if _, err := validateCoordinator("http://127.0.0.1:18888"); err != nil {
		t.Fatalf("loopback HTTP should be accepted: %v", err)
	}
	if _, err := validateCoordinator("https://agent.example.com"); err != nil {
		t.Fatalf("HTTPS coordinator rejected: %v", err)
	}
}

func TestWorkerFileSandboxRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	client := &workerClient{workspace: root}
	if _, err := client.fileWrite(json.RawMessage(`{"path":"../escape.txt","content":"no"}`)); err == nil {
		t.Fatal("worker allowed path traversal")
	}
	path := filepath.Join(root, "safe.txt")
	if _, err := client.fileWrite(json.RawMessage(`{"path":"` + path + `","content":"ok"}`)); err != nil {
		t.Fatalf("safe write failed: %v", err)
	}
	out, err := client.fileRead(json.RawMessage(`{"path":"` + path + `"}`))
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]any)
	if m["content"] != "ok" {
		t.Fatalf("unexpected read result: %#v", out)
	}
}

func TestWorkerHTTPGetRequiresAllowlist(t *testing.T) {
	client := &workerClient{allowHosts: map[string]struct{}{}}
	if _, err := client.httpGet(context.Background(), json.RawMessage(`{"url":"https://example.com"}`)); err == nil {
		t.Fatal("HTTP request to unlisted host was accepted")
	}
}
