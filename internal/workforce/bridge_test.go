package workforce

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

type fakeBoundRuntime struct {
	mu       sync.Mutex
	task     *task.Task
	input    string
	metadata map[string]any
	key      string
	creates  int
}

func (f *fakeBoundRuntime) CreateIdempotent(input string, meta map[string]any, key string) (*task.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates++
	f.input = input
	f.metadata = map[string]any{}
	for k, v := range meta {
		f.metadata[k] = v
	}
	f.key = key
	if f.task == nil {
		f.task = &task.Task{ID: "task_local_1", Input: input, Status: task.Completed, Output: "private local result", Metadata: f.metadata, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	}
	return f.task, nil
}

func (f *fakeBoundRuntime) Task(id string) (*task.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.task == nil || f.task.ID != id {
		return nil, os.ErrNotExist
	}
	return f.task, nil
}

func writeBindings(t *testing.T, dir string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, "bindings.json")
	raw := []byte(`{"version":1,"employees":{"emp_1":{"agent_id":"agent.local.sales"}}}`)
	if err := os.WriteFile(path, raw, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func workforceTestServer(t *testing.T, resultBody *map[string]any) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	pullCount := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer "+testServiceToken() {
			t.Errorf("missing service identity bearer token")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v1/workforce/runtime/heartbeat":
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/v1/workforce/runtime/sync":
			_, _ = w.Write([]byte(`{"ok":true,"schema":"kingai.workforce.v2","skills_schema":"kingai.workforce.skills.v1","employees":[{"id":"emp_1","name":"Emma","title":"AI Sales Manager","role_key":"sales-manager","status":"active","autonomy_level":"execute","risk_ceiling":"medium","skills":["crm.read","crm.update"],"goals":["Follow up qualified leads"]}],"workflows":[],"connectors":[{"id":"con_1","provider_key":"crm","name":"Company CRM","status":"active","auth_mode":"local-mcp","local_alias":"crm-main","allowed_skills":["crm.read","crm.update"],"config":{"workspace":"west"}}],"connector_bindings":[{"employee_id":"emp_1","connector_id":"con_1","status":"active","skill_scope":["crm.read"]}],"policy":{"cloud_never_bypasses_local_approval":true,"arbitrary_shell":false,"credentials_in_cloud":false,"connector_config_grants_permission":false,"generic_remote_shell":false,"execution_boundary":"KINGAIBOT customer-local capability-envelope-policy-approval"}}`))
		case "/api/v1/workforce/runtime/tasks/pull":
			mu.Lock()
			pullCount++
			count := pullCount
			mu.Unlock()
			if count == 1 {
				_, _ = w.Write([]byte(`{"ok":true,"task":{"id":"cloud_1","organization_id":"org_1","workspace_id":"ws_1","employee_id":"emp_1","title":"Follow up leads","instructions":"Prepare the approved follow-up.","priority":"normal","risk_level":"low","action_fingerprint":"abc"}}`))
			} else {
				_, _ = w.Write([]byte(`{"ok":true,"task":null}`))
			}
		case "/api/v1/workforce/runtime/tasks/cloud_1/result":
			defer r.Body.Close()
			var decoded map[string]any
			if err := json.NewDecoder(r.Body).Decode(&decoded); err != nil {
				t.Errorf("decode result: %v", err)
			}
			*resultBody = decoded
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestConnectorContextUsesIntersectionOnly(t *testing.T) {
	employee := Employee{ID: "emp_1", Skills: []string{"crm.read", "crm.update"}}
	connectors := []Connector{{ID: "con_1", Status: "active", AuthMode: "local-mcp", LocalAlias: "crm-main", AllowedSkills: []string{"crm.read", "crm.update"}, Config: map[string]any{"workspace": "west"}}}
	bindings := []ConnectorBinding{{EmployeeID: "emp_1", ConnectorID: "con_1", Status: "active", SkillScope: []string{"crm.read", "unknown.skill"}}}
	out, err := connectorContextForEmployee(employee, connectors, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || len(out[0].Skills) != 1 || out[0].Skills[0] != "crm.read" {
		t.Fatalf("unexpected least-privilege connector context: %#v", out)
	}
}

func TestConnectorContextRejectsSecretLikeCloudConfig(t *testing.T) {
	employee := Employee{ID: "emp_1", Skills: []string{"crm.read"}}
	connectors := []Connector{{ID: "con_1", Status: "active", AuthMode: "local-mcp", LocalAlias: "crm-main", AllowedSkills: []string{"crm.read"}, Config: map[string]any{"api_token": "must-not-arrive"}}}
	bindings := []ConnectorBinding{{EmployeeID: "emp_1", ConnectorID: "con_1", Status: "active", SkillScope: []string{"crm.read"}}}
	if _, err := connectorContextForEmployee(employee, connectors, bindings); err == nil {
		t.Fatal("secret-like cloud connector config must fail closed locally")
	}
}

func TestBridgeUsesOnlyLocalBindingForAgentIdentity(t *testing.T) {
	result := map[string]any{}
	server := workforceTestServer(t, &result)
	defer server.Close()
	dir := t.TempDir()
	bindings := writeBindings(t, dir, 0o600)
	settings := testSettings(server.URL)
	settings.BindingsFile = bindings
	settings.ReportOutput = false
	settings.MaxReportBytes = 1024
	rt := &fakeBoundRuntime{}
	bridge, err := NewBridge(settings, "test", dir, rt)
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
	if got := rt.metadata["agent_id"]; got != "agent.local.sales" {
		t.Fatalf("local agent binding not applied: %#v", got)
	}
	if _, exists := rt.metadata["authority_id"]; exists {
		t.Fatal("workforce bridge must never supply authority_id")
	}
	if rt.key != "workforce-v2:cloud_1" {
		t.Fatalf("unstable idempotency key: %q", rt.key)
	}
	if !strings.Contains(rt.input, "alias=crm-main") || !strings.Contains(rt.input, "skills=crm.read") {
		t.Fatal("least-privilege connector context missing from task prompt")
	}
	if !strings.Contains(rt.input, "cloud employee ID is not a local authority subject") {
		t.Fatal("v14 authority boundary missing from prompt")
	}
	if err := bridge.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "succeeded" {
		t.Fatalf("unexpected reported result: %#v", result)
	}
	if _, exists := result["output"]; exists {
		t.Fatal("private local output must not be uploaded by default")
	}
}

func TestMissingLocalBindingDoesNotCreateAgentAuthoritySubject(t *testing.T) {
	result := map[string]any{}
	server := workforceTestServer(t, &result)
	defer server.Close()
	dir := t.TempDir()
	settings := testSettings(server.URL)
	settings.BindingsFile = filepath.Join(dir, "missing-bindings.json")
	rt := &fakeBoundRuntime{}
	bridge, err := NewBridge(settings, "test", dir, rt)
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
	if _, exists := rt.metadata["agent_id"]; exists {
		t.Fatal("cloud employee must not become local agent identity without operator mapping")
	}
}

func TestBindingsFileMustBePrivateOnUnix(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Unix permission contract")
	}
	dir := t.TempDir()
	bindings := writeBindings(t, dir, 0o644)
	settings := Settings{Enabled: true, ControlPlaneURL: "http://127.0.0.1:8787", ServiceToken: testServiceToken(), AllowInsecureHTTP: true, RequestTimeout: time.Second, BindingsFile: bindings}
	if _, err := NewBridge(settings, "test", dir, &fakeBoundRuntime{}); err == nil {
		t.Fatal("group/world-readable authority binding file must fail closed")
	}
}

func TestReconciliationStateIsNotReportedAsTerminal(t *testing.T) {
	result := map[string]any{}
	server := workforceTestServer(t, &result)
	defer server.Close()
	dir := t.TempDir()
	settings := testSettings(server.URL)
	settings.BindingsFile = filepath.Join(dir, "missing.json")
	rt := &fakeBoundRuntime{task: &task.Task{ID: "task_local_1", Status: task.Reconciliation}}
	bridge, err := NewBridge(settings, "test", dir, rt)
	if err != nil {
		t.Fatal(err)
	}
	bridge.state.Assignments["cloud_1"] = assignment{CloudTaskID: "cloud_1", LocalTaskID: "task_local_1"}
	if err := bridge.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Fatalf("reconciliation state must stay local until resolved: %#v", result)
	}
}
