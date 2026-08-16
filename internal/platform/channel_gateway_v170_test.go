package platform

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSplitChannelTextUnicode(t *testing.T) {
	parts := splitChannelText("你好吗世界", 2)
	if len(parts) != 3 || parts[0] != "你好" || parts[1] != "吗世" || parts[2] != "界" {
		t.Fatalf("unexpected chunks: %#v", parts)
	}
	if got := strings.Join(parts, ""); got != "你好吗世界" {
		t.Fatalf("reassembled text changed: %q", got)
	}
}

func TestValidateNativeEndpointPinsHosts(t *testing.T) {
	good := map[string]string{
		"telegram": "https://api.telegram.org",
		"slack":    "https://slack.com/api/chat.postMessage",
		"discord":  "https://discord.com/api/v10",
		"whatsapp": "https://graph.facebook.com/v23.0/123/messages",
	}
	for kind, endpoint := range good {
		if err := validateNativeEndpoint(kind, endpoint); err != nil {
			t.Fatalf("%s endpoint rejected: %v", kind, err)
		}
	}
	bad := []struct{ kind, endpoint string }{
		{"telegram", "https://api.telegram.org.evil.example"},
		{"slack", "https://slack.com.evil.example/api/chat.postMessage"},
		{"discord", "https://discord.com.evil.example/api/v10"},
		{"whatsapp", "https://graph.facebook.com.evil.example/v23/messages"},
		{"telegram", "http://api.telegram.org"},
		{"telegram", "https://user:pass@api.telegram.org"},
	}
	for _, tc := range bad {
		if err := validateNativeEndpoint(tc.kind, tc.endpoint); err == nil {
			t.Fatalf("expected rejection for %s %s", tc.kind, tc.endpoint)
		}
	}
}

func TestVerifySlackSignature(t *testing.T) {
	secret := strings.Repeat("s", 32)
	body := []byte(`{"type":"event_callback"}`)
	now := time.Now().UTC().Truncate(time.Second)
	ts := strconv.FormatInt(now.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("v0:" + ts + ":"))
	_, _ = mac.Write(body)
	req := httptest.NewRequest("POST", "https://example.invalid", bytes.NewReader(body))
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", "v0="+hex.EncodeToString(mac.Sum(nil)))
	if err := verifySlackSignature(secret, req, body, now); err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Slack-Signature", "v0="+strings.Repeat("0", 64))
	if err := verifySlackSignature(secret, req, body, now); err == nil {
		t.Fatal("tampered slack signature accepted")
	}
}

func TestVerifySlackSignatureRejectsOldTimestamp(t *testing.T) {
	secret := strings.Repeat("s", 32)
	body := []byte(`{}`)
	now := time.Now().UTC().Truncate(time.Second)
	ts := strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("v0:" + ts + ":"))
	_, _ = mac.Write(body)
	req := httptest.NewRequest("POST", "https://example.invalid", bytes.NewReader(body))
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", "v0="+hex.EncodeToString(mac.Sum(nil)))
	if err := verifySlackSignature(secret, req, body, now); err == nil {
		t.Fatal("old slack request accepted")
	}
}

func TestVerifyDiscordSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"type":1}`)
	now := time.Now().UTC().Truncate(time.Second)
	ts := strconv.FormatInt(now.Unix(), 10)
	sig := ed25519.Sign(priv, append([]byte(ts), body...))
	req := httptest.NewRequest("POST", "https://example.invalid", bytes.NewReader(body))
	req.Header.Set("X-Signature-Timestamp", ts)
	req.Header.Set("X-Signature-Ed25519", hex.EncodeToString(sig))
	if err := verifyDiscordSignature(hex.EncodeToString(pub), req, body, now); err != nil {
		t.Fatal(err)
	}
	body[0] = '['
	if err := verifyDiscordSignature(hex.EncodeToString(pub), req, body, now); err == nil {
		t.Fatal("tampered discord body accepted")
	}
}

func TestVerifyMetaSignature(t *testing.T) {
	secret := strings.Repeat("m", 32)
	body := []byte(`{"object":"whatsapp_business_account"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	req := httptest.NewRequest("POST", "https://example.invalid", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	if err := verifyMetaSignature(secret, req, body); err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Hub-Signature-256", "sha256="+strings.Repeat("0", 64))
	if err := verifyMetaSignature(secret, req, body); err == nil {
		t.Fatal("tampered meta signature accepted")
	}
}

func TestPublicDeliveryRedactsSecrets(t *testing.T) {
	d := &OutboundDelivery{ID: "out_x", Status: "waiting_task", Secrets: map[string]string{"discord_interaction_token": "super-secret"}, Attempts: 1}
	view := publicDelivery(d)
	if _, ok := view["secrets"]; ok {
		t.Fatal("admin delivery view exposed secrets")
	}
	for _, v := range view {
		if strings.Contains(fmt.Sprint(v), "super-secret") {
			t.Fatal("admin delivery view leaked secret value")
		}
	}
}

func TestAmbiguousFiveHundredIsNotAutoRetry(t *testing.T) {
	err := &channelHTTPError{Status: 500, Body: "upstream"}
	if isRetryableChannelError(err) {
		t.Fatal("HTTP 500 must not be blindly retried")
	}
	if !isAmbiguousChannelError(err) {
		t.Fatal("HTTP 500 should require reconciliation")
	}
}
