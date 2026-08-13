package device

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestPairingIsOneTimeAndDeviceCredentialIsScoped(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	pairing, secret, err := s.CreatePairing([]string{ScopeTasksRead, ScopeApprovalsRead}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" || pairing.SecretHash == "" {
		t.Fatal("expected one-time secret and persisted hash")
	}
	device, token, err := s.ConsumePairing(pairing.ID, secret, "My phone", "android")
	if err != nil {
		t.Fatal(err)
	}
	if device.TokenHash != "" {
		t.Fatal("public device must not expose token hash")
	}
	if token == "" {
		t.Fatal("expected device token")
	}
	if _, _, err := s.ConsumePairing(pairing.ID, secret, "Replay", "android"); err == nil {
		t.Fatal("one-time pairing was replayed")
	}
	if _, err := s.Authorize(token, ScopeTasksRead); err != nil {
		t.Fatalf("granted scope rejected: %v", err)
	}
	if _, err := s.Authorize(token, ScopeTasksCreate); err == nil {
		t.Fatal("credential was accepted for an ungranted scope")
	}
	if err := s.Revoke(device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authorize(token, ScopeTasksRead); err == nil {
		t.Fatal("revoked credential remained valid")
	}
}

func TestPlaintextSecretsAreNeverPersisted(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	pairing, secret, err := s.CreatePairing(nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := s.ConsumePairing(pairing.ID, secret, "Desktop", "windows")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if strings.Contains(text, secret) {
		t.Fatal("pairing secret was persisted in plaintext")
	}
	if strings.Contains(text, token) {
		t.Fatal("device token was persisted in plaintext")
	}
}

func TestExpiredPairingRejected(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pairing, secret, err := s.CreatePairing(nil, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, _, err := s.ConsumePairing(pairing.ID, secret, "Phone", "android"); err == nil {
		t.Fatal("expired pairing was accepted")
	}
}

func TestWrongPairingSecretRejected(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pairing, _, err := s.CreatePairing(nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ConsumePairing(pairing.ID, strings.Repeat("x", 43), "Phone", "android"); err == nil {
		t.Fatal("wrong pairing secret was accepted")
	}
}
