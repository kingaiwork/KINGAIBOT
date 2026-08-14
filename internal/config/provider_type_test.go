package config

import (
	"os"
	"path/filepath"
	"testing"
)

func minimalProviderConfig(t *testing.T, providerType string) *Config {
	t.Helper()
	dir := t.TempDir()
	return &Config{
		Server: Server{},
		Runtime: Runtime{DataDir: filepath.Join(dir, "data"), WorkspaceDir: filepath.Join(dir, "workspace")},
		Providers: []Provider{{
			Name:      "provider",
			Type:      providerType,
			BaseURL:   "https://example.com/v1",
			Model:     "model",
			Priority:  10,
			Enabled:   true,
			APIKeyEnv: "TEST_PROVIDER_KEY",
		}},
		Security: Security{DefaultToolPolicy: "deny"},
		Evolution: Evolution{Mode: "proposal-only"},
	}
}

func TestProviderTypeAliasesNormalize(t *testing.T) {
	cases := map[string]string{
		"":                  "openai-compatible",
		"openai":            "openai-compatible",
		"openai_compatible": "openai-compatible",
		"claude":            "anthropic",
		"anthropic":         "anthropic",
		"google":            "gemini",
		"google-gemini":     "gemini",
		"gemini":            "gemini",
	}
	for input, want := range cases {
		t.Run(input+"=>"+want, func(t *testing.T) {
			cfg := minimalProviderConfig(t, input)
			if err := cfg.Normalize(t.TempDir()); err != nil {
				t.Fatal(err)
			}
			if got := cfg.Providers[0].Type; got != want {
				t.Fatalf("type=%q want=%q", got, want)
			}
		})
	}
}

func TestUnknownProviderTypeFailsAtStartup(t *testing.T) {
	cfg := minimalProviderConfig(t, "mystery-vendor")
	if err := cfg.Normalize(t.TempDir()); err == nil {
		t.Fatal("unknown provider type was accepted")
	}
}

func TestDisabledUnknownProviderDoesNotBlockStartup(t *testing.T) {
	cfg := minimalProviderConfig(t, "mystery-vendor")
	cfg.Providers[0].Enabled = false
	cfg.Providers = append(cfg.Providers, Provider{Name: "good", Type: "openai-compatible", BaseURL: "https://example.com/v1", Model: "model", Enabled: true})
	if err := cfg.Normalize(t.TempDir()); err != nil {
		t.Fatalf("disabled unknown provider should not block startup: %v", err)
	}
}

func TestProviderTypeValidationDoesNotRequireSecretAtNormalize(t *testing.T) {
	_ = os.Unsetenv("TEST_PROVIDER_KEY")
	cfg := minimalProviderConfig(t, "anthropic")
	if err := cfg.Normalize(t.TempDir()); err != nil {
		t.Fatalf("config normalization should validate env name, not require secret value: %v", err)
	}
}
