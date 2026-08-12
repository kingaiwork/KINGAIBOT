package evolution

import "testing"

func TestProposalRejectsTraversalID(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(&Proposal{ID: "../escape", Kind: "test", Status: "proposed"}); err == nil {
		t.Fatal("expected traversal id to be rejected")
	}
}
