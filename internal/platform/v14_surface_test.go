package platform

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

type callbackRuntime struct {
	mu       sync.Mutex
	next     int
	tasks    map[string]*task.Task
	onCreate func()
}

func newCallbackRuntime() *callbackRuntime {
	return &callbackRuntime{tasks: map[string]*task.Task{}}
}

func (f *callbackRuntime) Create(input string, meta map[string]any) (*task.Task, error) {
	f.mu.Lock()
	f.next++
	id := fmt.Sprintf("task_%d", f.next)
	n := time.Now().UTC()
	created := &task.Task{ID: id, Input: input, Status: task.Queued, Metadata: meta, CreatedAt: n, UpdatedAt: n}
	f.tasks[id] = created
	callback := f.onCreate
	f.mu.Unlock()
	if callback != nil {
		callback()
	}
	copy := *created
	return &copy, nil
}

func (f *callbackRuntime) Task(id string) (*task.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	found, ok := f.tasks[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	copy := *found
	return &copy, nil
}

func TestHandlerV14DoesNotReturnRetryableFailureAfterTaskCreation(t *testing.T) {
	dir := t.TempDir()
	events, err := eventlog.New(dir + "/events")
	if err != nil {
		t.Fatal(err)
	}
	runtime := newCallbackRuntime()
	manager, err := New(dir+"/platform", runtime, events)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	session, err := manager.CreateSession(Session{Channel: "web", Sender: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	sessionPath, err := manager.path("sessions", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	runtime.onCreate = func() { _ = os.Remove(sessionPath) }

	req := httptest.NewRequest(http.MethodPost, "/v1/platform/sessions/"+session.ID+"/messages", strings.NewReader(`{"message":"execute once"}`))
	rec := httptest.NewRecorder()
	manager.HandlerV14().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("v1.4 route returned retryable failure after task creation: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if runtime.next != 1 {
		t.Fatalf("expected exactly one Runtime.Create call, got %d", runtime.next)
	}
}
