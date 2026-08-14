package runtime

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/agent"
	"github.com/kingaiwork/KINGAIBOT/internal/approval"
	"github.com/kingaiwork/KINGAIBOT/internal/config"
	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/evolution"
	"github.com/kingaiwork/KINGAIBOT/internal/memory"
	"github.com/kingaiwork/KINGAIBOT/internal/policy"
	"github.com/kingaiwork/KINGAIBOT/internal/provider"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
	"github.com/kingaiwork/KINGAIBOT/internal/tool"
)

func TestCompletionAuditFailureLeavesTaskInReconciliation(t *testing.T) {
	root := t.TempDir()
	eventsDir := filepath.Join(root, "events")
	eventsPath := filepath.Join(eventsDir, "events.jsonl")
	sabotaged := make(chan struct{}, 1)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Provider execution starts only after task.running has been durably
		// audited. Replace the audit file with a directory before returning a
		// successful model result so the completion append deterministically fails.
		if err := os.Remove(eventsPath); err != nil {
			t.Errorf("remove audit file: %v", err)
		}
		if err := os.Mkdir(eventsPath, 0o700); err != nil {
			t.Errorf("replace audit file with directory: %v", err)
		}
		select {
		case sabotaged <- struct{}{}:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"verified-output"},"finish_reason":"stop"}]}`))
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Runtime: config.Runtime{
			DataDir:               root,
			WorkspaceDir:          filepath.Join(root, "workspace"),
			MaxSteps:              1,
			WorkerCount:           1,
			RequestTimeoutSeconds: 30,
			TaskTimeoutSeconds:    30,
			QueueCapacity:         8,
			MaxRequestBytes:       1 << 20,
		},
		Memory: config.Memory{Enabled: false, MaxRecords: 100, MaxContextChars: 1000},
		Providers: []config.Provider{{
			Name:                "test",
			BaseURL:             upstream.URL,
			Model:               "test-model",
			Enabled:             true,
			AllowPrivateNetwork: true,
			AllowInsecureHTTP:   true,
		}},
		Security:  config.Security{DefaultToolPolicy: "deny", ToolPolicies: map[string]string{}},
		Evolution: config.Evolution{Enabled: false, Mode: "proposal-only"},
	}
	if err := cfg.Normalize(root); err != nil {
		t.Fatal(err)
	}
	ts, err := task.NewStore(filepath.Join(root, "tasks"))
	if err != nil {
		t.Fatal(err)
	}
	as, err := approval.New(filepath.Join(root, "approvals"))
	if err != nil {
		t.Fatal(err)
	}
	el, err := eventlog.New(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := memory.New(filepath.Join(root, "memory"), 100)
	if err != nil {
		t.Fatal(err)
	}
	es, err := evolution.New(filepath.Join(root, "evolution"))
	if err != nil {
		t.Fatal(err)
	}
	pe := policy.New("deny", nil)
	tr := tool.New(cfg, pe, as, el)
	pc := provider.New(cfg.Providers, 30*time.Second)
	ae := agent.New(cfg, pc, tr, ms)
	rt := New(ts, as, el, ms, ae, es, cfg)
	defer rt.Close()

	created, err := rt.Create("produce a successful result", nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-sabotaged:
	case <-time.After(3 * time.Second):
		t.Fatal("provider execution did not reach audit sabotage point")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, err := rt.Task(created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == task.Reconciliation {
			if !strings.Contains(got.Output, "verified-output") {
				t.Fatalf("reconciliation lost successful output: %q", got.Output)
			}
			if !strings.Contains(got.Error, "completion audit unavailable") {
				t.Fatalf("missing completion audit failure reason: %q", got.Error)
			}
			return
		}
		if got.Status == task.Completed {
			t.Fatalf("task became completed even though completion audit was unavailable: %#v", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("task did not enter reconciliation after completion audit failure")
}
