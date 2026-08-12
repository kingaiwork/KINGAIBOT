package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/approval"
	"github.com/kingaiwork/KINGAIBOT/internal/config"
	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/netguard"
	"github.com/kingaiwork/KINGAIBOT/internal/policy"
)

func TestSecureWritableNestedPath(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a", "b", "file.txt")
	got, err := secureWritable(target, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(canonicalRoot, "a", "b", "file.txt")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
func TestSecureWritableTraversalDenied(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside.txt")
	if _, err := secureWritable(outside, []string{root}); err == nil {
		t.Fatal("expected traversal denial")
	}
}
func TestSecureExistingSymlinkEscapeDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	root := t.TempDir()
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := secureExisting(link, []string{root}); err == nil {
		t.Fatal("expected symlink escape denial")
	}
}
func TestRejectPrivateIP(t *testing.T) {
	if netguard.PublicIP([]byte{127, 0, 0, 1}) {
		t.Fatal("loopback must be rejected")
	}
}
func TestApprovalBoundToExactArgumentsAndCachedAtMostOnce(t *testing.T) {
	workspace := t.TempDir()
	approvals, err := approval.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	events, err := eventlog.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Runtime: config.Runtime{WorkspaceDir: workspace}, Security: config.Security{DefaultToolPolicy: "deny", ToolPolicies: map[string]string{"file_write": "ask"}, FileWriteRoots: []string{workspace}}}
	reg := New(cfg, policy.New("deny", cfg.Security.ToolPolicies), approvals, events)
	path := filepath.Join(workspace, "result.txt")
	args1, _ := json.Marshal(map[string]any{"path": path, "content": "first"})
	_, err = reg.Execute(context.Background(), "task_1", "file_write", args1)
	var ar *ApprovalRequired
	if !errors.As(err, &ar) {
		t.Fatalf("expected approval required, got %v", err)
	}
	if _, err := approvals.Update(ar.ApprovalID, func(a *approval.Approval) error { a.Status = "approved"; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Execute(context.Background(), "task_1", "file_write", args1); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "first" {
		t.Fatalf("unexpected first write %q", got)
	}
	if err := os.WriteFile(path, []byte("external-change"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Execute(context.Background(), "task_1", "file_write", args1); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "external-change" {
		t.Fatalf("approved action was replayed: %q", got)
	}
	args2, _ := json.Marshal(map[string]any{"path": path, "content": "second"})
	_, err = reg.Execute(context.Background(), "task_1", "file_write", args2)
	var ar2 *ApprovalRequired
	if !errors.As(err, &ar2) {
		t.Fatalf("expected a new approval for changed arguments, got %v", err)
	}
	if ar2.ApprovalID == ar.ApprovalID {
		t.Fatal("changed arguments reused the old approval")
	}
}
func TestShellRejectsPathMasqueradingAsAllowedBinary(t *testing.T) {
	workspace := t.TempDir()
	approvals, _ := approval.New(t.TempDir())
	cfg := &config.Config{Runtime: config.Runtime{WorkspaceDir: workspace}, Security: config.Security{DefaultToolPolicy: "deny", ToolPolicies: map[string]string{"shell_exec": "allow"}, ShellAllowlist: []string{"echo"}}}
	events, err := eventlog.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := New(cfg, policy.New("deny", cfg.Security.ToolPolicies), approvals, events)
	args, _ := json.Marshal(map[string]any{"argv": []string{filepath.Join(workspace, "echo"), "hello"}})
	_, err = reg.Execute(context.Background(), "task_1", "shell_exec", args)
	if err == nil || !strings.Contains(err.Error(), "paths are denied") {
		t.Fatalf("expected path masquerade denial, got %v", err)
	}
}
func TestToolExecutionFailsClosedWithoutAuditLog(t *testing.T) {
	approvals, err := approval.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Security: config.Security{DefaultToolPolicy: "deny", ToolPolicies: map[string]string{"system_info": "allow"}}}
	reg := New(cfg, policy.New("deny", cfg.Security.ToolPolicies), approvals, nil)
	_, err = reg.Execute(context.Background(), "task_1", "system_info", json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "audit unavailable") {
		t.Fatalf("expected fail-closed audit error, got %v", err)
	}
}
