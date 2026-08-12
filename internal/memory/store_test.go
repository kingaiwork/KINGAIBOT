package memory

import (
	"strings"
	"testing"
)

func TestMemorySearch(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Add(Record{ID: "1", Kind: "semantic", Content: "deploy cloudflare worker safely", Importance: 1, Confidence: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(Record{ID: "2", Kind: "semantic", Content: "cook noodles", Importance: 1, Confidence: 1}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Search("cloudflare worker deploy", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0].ID != "1" {
		t.Fatalf("unexpected results: %#v", got)
	}
}

func TestSanitizeContentRedactsSecrets(t *testing.T) {
	in := "api_key=abcdefgh12345678 Authorization: Bearer bearer-secret-12345 sk-abcdefghijklmnopqrstuvwxyz ghp_abcdefghijklmnopqrstuvwxyz1234 eyJabcde12345.abcdefghijk12345.abcdefghijk12345"
	out := SanitizeContent(in)
	for _, secret := range []string{"abcdefgh12345678", "bearer-secret-12345", "sk-abcdefghijklmnopqrstuvwxyz", "ghp_abcdefghijklmnopqrstuvwxyz1234", "eyJabcde12345"} {
		if strings.Contains(out, secret) {
			t.Fatalf("secret was not redacted: %q in %q", secret, out)
		}
	}
}
