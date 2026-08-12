package eventlog

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestAuditHashChainDetectsTampering(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Append(Event{Type: "task.created", TaskID: "task_1"}); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(Event{Type: "task.completed", TaskID: "task_1"}); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "events.jsonl")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	b = bytes.Replace(b, []byte(`"task.created"`), []byte(`"task.changed"`), 1)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(dir); err == nil {
		t.Fatal("expected tampered audit chain to be rejected")
	}
}

func TestVerifyTamperMarksLogUnhealthyAndFailClosed(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Append(Event{Type: "task.created", TaskID: "task_2"}); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "events.jsonl")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	b = bytes.Replace(b, []byte(`"task.created"`), []byte(`"task.changed"`), 1)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := l.Verify(); err == nil {
		t.Fatal("expected verification failure")
	}
	if err := l.Healthy(); err == nil {
		t.Fatal("expected unhealthy audit log")
	}
	if err := l.Append(Event{Type: "should.not.append"}); err == nil {
		t.Fatal("expected fail-closed append")
	}
}
