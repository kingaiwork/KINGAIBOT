package runtime

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/config"
	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

func bareRuntimeForCreateTest(t *testing.T, queueCapacity int) *Runtime {
	t.Helper()
	dir := t.TempDir()
	ts, err := task.NewStore(filepath.Join(dir, "tasks"))
	if err != nil {
		t.Fatal(err)
	}
	el, err := eventlog.New(filepath.Join(dir, "events"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &Runtime{
		tasks:      ts,
		events:     el,
		cfg:        &config.Config{Runtime: config.Runtime{MaxRequestBytes: 1024}},
		queue:      make(chan string, queueCapacity),
		ctx:        ctx,
		cancel:     cancel,
		running:    map[string]context.CancelFunc{},
		processing: map[string]bool{},
	}
}

func TestCreateClassifiesInvalidInput(t *testing.T) {
	r := bareRuntimeForCreateTest(t, 1)
	if _, err := r.Create("", nil); !errors.Is(err, ErrInvalidTaskInput) {
		t.Fatalf("empty input error=%v, want ErrInvalidTaskInput", err)
	}
	if _, err := r.Create(string(make([]byte, 1025)), nil); !errors.Is(err, ErrInvalidTaskInput) {
		t.Fatalf("oversized input error=%v, want ErrInvalidTaskInput", err)
	}
}

func TestCreateClassifiesQueueUnavailableAndPersistsFailure(t *testing.T) {
	r := bareRuntimeForCreateTest(t, 1)
	r.queue <- "occupied"
	if _, err := r.Create("do work", map[string]any{"source": "test"}); !errors.Is(err, ErrQueueUnavailable) {
		t.Fatalf("queue-full error=%v, want ErrQueueUnavailable", err)
	}
	tasks, err := r.Tasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks=%d want=1", len(tasks))
	}
	if tasks[0].Status != task.Failed || tasks[0].Error != "runtime queue unavailable" {
		t.Fatalf("queue-full task not terminal failed: %#v", tasks[0])
	}
}
