package workforce

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

type fakeRuntime struct {
	mu    sync.Mutex
	tasks []*task.Task
}

func (f *fakeRuntime) Create(input string, meta map[string]any) (*task.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t := &task.Task{ID: "task_local_1", Input: input, Output: "private result", Status: task.Completed, Metadata: meta, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	f.tasks = append(f.tasks, t)
	return t, nil
}

func (f *fakeRuntime) Tasks() ([]*task.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*task.Task(nil), f.tasks...), nil
}

func TestBridgeRoutesCloudTaskThroughLocalRuntime(t *testing.T) {
	var mu sync.Mutex
	pullCount := 0
	resultBody := map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/workforce/runtime/heartbeat":
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/workforce/runtime/sync":
			_, _ = w.Write([]byte(`{"ok":true,"schema":"kingai.workforce.v1","employees":[{"id":"emp_1","name":"Emma","title":"AI Sales Manager","status":"active","autonomy_level":"execute","risk_ceiling":"medium","skills":["crm.read"],"goals":["Follow up leads"]}],"workflows":[],"policy":{"cloud_never_bypasses_local_approval":true,"arbitrary_shell":false,"execution_boundary":"KINGAIBOT customer-local"}}`))
		case "/api/workforce/runtime/tasks/pull":
			mu.Lock()
			pullCount++
			count := pullCount
			mu.Unlock()
			if count == 1 {
				_, _ = w.Write([]byte(`{"ok":true,"task":{"id":"cloud_1","organization_id":"org_1","employee_id":"emp_1","title":"Follow up leads","instructions":"Prepare the approved follow-up.","priority":"normal","risk_level":"low","action_fingerprint":"abc"}}`))
			} else {
				_, _ = w.Write([]byte(`{"ok":true,"task":null}`))
			}
		case "/api/workforce/runtime/tasks/cloud_1/result":
			defer r.Body.Close()
			_ = json.NewDecoder(r.Body).Decode(&resultBody)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	settings := Settings{Enabled: true, ControlPlaneURL: server.URL, NodeToken: testToken(), AllowInsecureHTTP: true, HeartbeatInterval: time.Minute, SyncInterval: time.Minute, PollInterval: time.Minute, RequestTimeout: 2 * time.Second, ReportOutput: false, MaxReportBytes: 1024}
	rt := &fakeRuntime{}
	bridge, err := NewBridge(settings, "test", t.TempDir(), rt)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := bridge.sync(ctx); err != nil {
		t.Fatal(err)
	}
	if err := bridge.pull(ctx); err != nil {
		t.Fatal(err)
	}
	if len(rt.tasks) != 1 {
		t.Fatalf("expected one local task, got %d", len(rt.tasks))
	}
	if !strings.Contains(rt.tasks[0].Input, "local allow/ask/deny tool policy is authoritative") {
		t.Fatal("local security boundary missing from task prompt")
	}
	if rt.tasks[0].Metadata["workforce_cloud_task_id"] != "cloud_1" {
		t.Fatal("cloud task id not persisted in local task metadata")
	}
	if err := bridge.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if resultBody["status"] != "succeeded" {
		t.Fatalf("unexpected reported status: %#v", resultBody)
	}
	if _, ok := resultBody["output"]; ok {
		t.Fatal("private local output must not be uploaded by default")
	}
}
