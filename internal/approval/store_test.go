package approval

import (
	"encoding/json"
	"testing"
)

func TestCanonicalArgumentsHashIgnoresObjectKeyOrder(t *testing.T) {
	a := CanonicalArgumentsHash(json.RawMessage(`{"a":1,"b":2}`))
	b := CanonicalArgumentsHash(json.RawMessage(`{"b":2,"a":1}`))
	if a != b {
		t.Fatalf("canonical hashes differ: %s vs %s", a, b)
	}
}

func TestApprovalExecutionIsAtMostOnce(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := &Approval{ID: "appr_1", TaskID: "task_1", Tool: "file_write", Arguments: json.RawMessage(`{"path":"a"}`), Status: "approved"}
	if err := s.Save(a); err != nil {
		t.Fatal(err)
	}
	first, err := s.BeginExecution(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.ExecutionState != "executing" {
		t.Fatalf("unexpected state %q", first.ExecutionState)
	}
	if _, err := s.BeginExecution(a.ID); err == nil {
		t.Fatal("expected second begin to require reconciliation")
	}
	if err := s.FinishExecution(a.ID, "ok", nil); err != nil {
		t.Fatal(err)
	}
	final, err := s.BeginExecution(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.ExecutionState != "completed" || final.Result != "ok" {
		t.Fatalf("unexpected completed approval: %#v", final)
	}
}
