package tool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/approval"
	"github.com/kingaiwork/KINGAIBOT/internal/config"
	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/policy"
	"github.com/kingaiwork/KINGAIBOT/internal/provider"
)

type fakeExtension struct{ calls int }

func (f *fakeExtension) ToolDefinitions() []provider.ToolDef {
	return []provider.ToolDef{{Type: "function", Function: provider.FunctionDef{Name: "ext_echo", Description: "echo", Parameters: map[string]any{"type": "object"}}}}
}
func (f *fakeExtension) ExecuteTool(_ context.Context, _ string, _ string, args json.RawMessage) (string, error) {
	f.calls++
	return string(args), nil
}

func newExtensionRegistry(t *testing.T, decision string) (*Registry, *fakeExtension, *approval.Store) {
	t.Helper()
	d := t.TempDir()
	a, err := approval.New(d + "/approvals")
	if err != nil {
		t.Fatal(err)
	}
	e, err := eventlog.New(d + "/events")
	if err != nil {
		t.Fatal(err)
	}
	p := policy.New("deny", map[string]string{"ext_echo": decision})
	r := New(&config.Config{}, p, a, e)
	ext := &fakeExtension{}
	r.RegisterExtension(ext)
	return r, ext, a
}

func TestExtensionAllowUsesAuditBoundary(t *testing.T) {
	r, ext, _ := newExtensionRegistry(t, "allow")
	out, err := r.ExecuteAny(context.Background(), "task_x", "ext_echo", json.RawMessage(`{"x":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if out != `{"x":1}` || ext.calls != 1 {
		t.Fatalf("unexpected result %q calls=%d", out, ext.calls)
	}
}

func TestExtensionAskRequiresExactApproval(t *testing.T) {
	r, ext, store := newExtensionRegistry(t, "ask")
	args := json.RawMessage(`{"x":1}`)
	_, err := r.ExecuteAny(context.Background(), "task_x", "ext_echo", args)
	ar, ok := err.(*ApprovalRequired)
	if !ok {
		t.Fatalf("expected ApprovalRequired, got %v", err)
	}
	if ext.calls != 0 {
		t.Fatal("extension executed before approval")
	}
	_, err = store.Update(ar.ApprovalID, func(a *approval.Approval) error { a.Status = "approved"; return nil })
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.ExecuteAny(context.Background(), "task_x", "ext_echo", args)
	if err != nil {
		t.Fatal(err)
	}
	if out != `{"x":1}` || ext.calls != 1 {
		t.Fatalf("unexpected result %q calls=%d", out, ext.calls)
	}
}

func TestExtensionDenied(t *testing.T) {
	r, ext, _ := newExtensionRegistry(t, "deny")
	if _, err := r.ExecuteAny(context.Background(), "task_x", "ext_echo", json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected deny")
	}
	if ext.calls != 0 {
		t.Fatal("denied extension executed")
	}
}
