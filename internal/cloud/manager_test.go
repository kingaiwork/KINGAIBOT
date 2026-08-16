package cloud

import (
	"bytes"
	"testing"

	"github.com/kingaiwork/KINGAIBOT/internal/config"
)

func TestValidateBaseURLPinsKINGAI(t *testing.T) {
	if _, err := validateBaseURL("https://api.kingai.work", false); err != nil {
		t.Fatal(err)
	}
	if _, err := validateBaseURL("https://example.com", false); err == nil {
		t.Fatal("expected host pin rejection")
	}
	if _, err := validateBaseURL("http://api.kingai.work", false); err == nil {
		t.Fatal("expected https rejection")
	}
}

func TestPolicyCanOnlyTighten(t *testing.T) {
	cfg := &config.Config{
		Runtime:   config.Runtime{MaxSteps: 12, WorkerCount: 8, TaskTimeoutSeconds: 900},
		Security:  config.Security{DefaultToolPolicy: "ask", ToolPolicies: map[string]string{"shell": "deny", "http": "allow"}},
		Providers: []config.Provider{{Name: "OpenAI", Enabled: true}, {Name: "Local", Enabled: true}},
	}
	ApplyRestrictions(cfg, Policy{Version: 3, DisabledProviders: []string{"OpenAI"}, MaxSteps: 6, MaxWorkerCount: 2, MaxTaskTimeoutSeconds: 300, DefaultToolPolicy: "allow", ToolPolicies: map[string]string{"shell": "allow", "http": "deny"}})
	if cfg.Runtime.MaxSteps != 6 || cfg.Runtime.WorkerCount != 2 || cfg.Runtime.TaskTimeoutSeconds != 300 {
		t.Fatal("runtime ceilings were not contracted")
	}
	if cfg.Security.DefaultToolPolicy != "ask" || cfg.Security.ToolPolicies["shell"] != "deny" || cfg.Security.ToolPolicies["http"] != "deny" {
		t.Fatal("policy expansion was accepted or contraction was missed")
	}
	if cfg.Providers[0].Enabled || !cfg.Providers[1].Enabled {
		t.Fatal("provider disable policy incorrect")
	}
}

func TestEnvelopeEncryptionBindsAAD(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	plain := []byte("private memory")
	aad := syncAAD("workspace-a", syncStream, "0011223344556677")
	nonce, ciphertext, err := encryptEnvelope(key, plain, aad)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decryptEnvelope(key, nonce, ciphertext, aad)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatal("decrypt mismatch")
	}
	if _, err := decryptEnvelope(key, nonce, ciphertext, aad+"-tampered"); err == nil {
		t.Fatal("tampered AAD should fail")
	}
}
