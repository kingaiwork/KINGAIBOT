package policy

import "testing"

func TestPolicyDefaultsAndOverrides(t *testing.T) {
	e := New("deny", map[string]string{"file_read": "allow", "file_write": "ask"})
	if e.Evaluate("file_read") != Allow {
		t.Fatal("file_read should be allow")
	}
	if e.Evaluate("file_write") != Ask {
		t.Fatal("file_write should be ask")
	}
	if e.Evaluate("shell_exec") != Deny {
		t.Fatal("unknown tool should inherit deny")
	}
}
