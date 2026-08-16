package platform

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestInboundSignatureV161(t *testing.T) {
	channel := &Channel{ID: "chan_secure", BearerTokenEnv: "KING_TEST_CHANNEL_TOKEN"}
	secret := "0123456789abcdef0123456789abcdef"
	t.Setenv("KING_TEST_CHANNEL_TOKEN_SIGNING_SECRET", secret)
	body := []byte(`{"event_id":"evt_1","sender":"user_1","text":"hello"}`)
	now := time.Unix(1786874400, 0).UTC()
	sig, err := SignInboundBody(secret, channel.ID, now.Unix(), body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/v1/inbound/chan_secure", nil)
	r.Header.Set("X-KINGAI-Timestamp", "1786874400")
	r.Header.Set("X-KINGAI-Signature", sig)
	if err := verifyInboundSignatureV161(r, channel, body, now); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := verifyInboundSignatureV161(r, channel, append(body, ' '), now); err == nil {
		t.Fatal("tampered body should be rejected")
	}
	if err := verifyInboundSignatureV161(r, channel, body, now.Add(6*time.Minute)); err == nil {
		t.Fatal("stale signature should be rejected")
	}
}

func TestInboundSignatureV161CompatibilityWithoutSecret(t *testing.T) {
	channel := &Channel{ID: "chan_legacy", BearerTokenEnv: "KING_TEST_LEGACY_TOKEN"}
	t.Setenv("KING_TEST_LEGACY_TOKEN_SIGNING_SECRET", "")
	r := httptest.NewRequest("POST", "/v1/inbound/chan_legacy", nil)
	if err := verifyInboundSignatureV161(r, channel, []byte(`{"text":"hello"}`), time.Now().UTC()); err != nil {
		t.Fatalf("legacy bearer-only channel should remain compatible: %v", err)
	}
}

func TestInboundSignatureV161RejectsWeakProvisionedSecret(t *testing.T) {
	channel := &Channel{ID: "chan_weak", BearerTokenEnv: "KING_TEST_WEAK_TOKEN"}
	t.Setenv("KING_TEST_WEAK_TOKEN_SIGNING_SECRET", "short")
	r := httptest.NewRequest("POST", "/v1/inbound/chan_weak", nil)
	if err := verifyInboundSignatureV161(r, channel, []byte(`{}`), time.Now().UTC()); err == nil {
		t.Fatal("weak provisioned signing secret should fail closed")
	}
}
