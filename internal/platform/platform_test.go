package platform

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

type fakeRuntime struct {
	mu    sync.Mutex
	next  int
	tasks map[string]*task.Task
}

func newFakeRuntime() *fakeRuntime { return &fakeRuntime{tasks: map[string]*task.Task{}} }

func (f *fakeRuntime) Create(input string, meta map[string]any) (*task.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	id := fmt.Sprintf("task_%d", f.next)
	n := time.Now().UTC()
	t := &task.Task{ID: id, Input: input, Output: "done:" + input, Status: task.Completed, Metadata: meta, CreatedAt: n, UpdatedAt: n}
	f.tasks[id] = t
	cp := *t
	return &cp, nil
}

func (f *fakeRuntime) Task(id string) (*task.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tasks[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	cp := *t
	return &cp, nil
}

func newManagerForTest(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	el, err := eventlog.New(dir + "/events")
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(dir+"/platform", newFakeRuntime(), el)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m
}

func TestAgentSessionAndMission(t *testing.T) {
	m := newManagerForTest(t)
	a, err := m.CreateAgent(AgentProfile{Name: "researcher", SystemPrompt: "Be concise."})
	if err != nil {
		t.Fatal(err)
	}
	s, err := m.CreateSession(Session{AgentID: a.ID, Channel: "web", Sender: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := m.SendSession(s.ID, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != task.Completed {
		t.Fatalf("unexpected task status: %s", created.Status)
	}
	got, err := m.Session(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Turns) != 1 || got.Turns[0].Assistant == "" || got.Turns[0].DoneAt == nil {
		t.Fatalf("session did not synchronize task result: %#v", got.Turns)
	}
	mission, err := m.DispatchMission(Mission{Objective: "check system", AgentIDs: []string{a.ID}})
	if err != nil {
		t.Fatal(err)
	}
	mission, err = m.Mission(mission.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mission.Status != "completed" {
		t.Fatalf("unexpected mission status: %s", mission.Status)
	}
}

func TestWorkflowRunsToCompletion(t *testing.T) {
	m := newManagerForTest(t)
	wf, err := m.CreateWorkflow(Workflow{Name: "two-step", Steps: []WorkflowStep{{Prompt: "one"}, {Prompt: "two"}}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := m.RunWorkflow(wf.ID)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		runs, er := m.WorkflowRuns()
		if er != nil {
			t.Fatal(er)
		}
		for _, r := range runs {
			if r.ID == run.ID && r.Status == "completed" {
				if len(r.TaskIDs) != 2 || len(r.Outputs) != 2 {
					t.Fatalf("unexpected run: %#v", r)
				}
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("workflow did not complete")
}

func TestRemoteRegistrationRejectsUnsafeHTTP(t *testing.T) {
	m := newManagerForTest(t)
	if _, err := m.CreatePlugin(Plugin{Name: "bad", Endpoint: "http://example.com/plugin"}); err == nil {
		t.Fatal("expected insecure public HTTP plugin to be rejected")
	}
	if _, err := m.CreateNode(Node{Name: "local", Endpoint: "http://127.0.0.1:9000", AllowInsecureHTTP: true}); err != nil {
		t.Fatalf("loopback HTTP should be allowed when explicitly enabled: %v", err)
	}
}

func TestScheduleBounds(t *testing.T) {
	m := newManagerForTest(t)
	if _, err := m.CreateSchedule(Schedule{Name: "too-fast", Prompt: "x", IntervalSeconds: 10}); err == nil {
		t.Fatal("expected sub-minute schedule rejection")
	}
	s, err := m.CreateSchedule(Schedule{Name: "hourly", Prompt: "x", IntervalSeconds: 3600})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Enabled || s.NextRunAt.IsZero() {
		t.Fatalf("invalid schedule: %#v", s)
	}
}
