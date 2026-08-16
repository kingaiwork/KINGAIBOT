package platform

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const channelSignatureMaxSkew = 5 * time.Minute

func nativeSecretEnv(c *Channel, suffix string) string {
	if c == nil || strings.TrimSpace(c.BearerTokenEnv) == "" {
		return ""
	}
	return strings.TrimSpace(c.BearerTokenEnv) + suffix
}

func secretFromEnv(name string, min int) (string, error) {
	if name == "" {
		return "", errors.New("required channel secret environment is not configured")
	}
	v := os.Getenv(name)
	if len(v) < min {
		return "", errors.New("required channel secret is missing or too short")
	}
	return v, nil
}

func validFreshUnixHeader(v string, at time.Time) error {
	sec, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil {
		return errors.New("invalid signature timestamp")
	}
	delta := at.Sub(time.Unix(sec, 0))
	if delta < 0 {
		delta = -delta
	}
	if delta > channelSignatureMaxSkew {
		return errors.New("signature timestamp outside replay window")
	}
	return nil
}

func verifySlackSignature(secret string, r *http.Request, body []byte, at time.Time) error {
	ts := r.Header.Get("X-Slack-Request-Timestamp")
	if err := validFreshUnixHeader(ts, at); err != nil {
		return err
	}
	provided := strings.TrimSpace(r.Header.Get("X-Slack-Signature"))
	if !strings.HasPrefix(provided, "v0=") {
		return errors.New("invalid slack signature")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("v0:" + strings.TrimSpace(ts) + ":"))
	_, _ = mac.Write(body)
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(provided), []byte(expected)) {
		return errors.New("invalid slack signature")
	}
	return nil
}

func verifyDiscordSignature(publicKeyHex string, r *http.Request, body []byte, at time.Time) error {
	key, err := hex.DecodeString(strings.TrimSpace(publicKeyHex))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return errors.New("discord public key is invalid")
	}
	ts := strings.TrimSpace(r.Header.Get("X-Signature-Timestamp"))
	if err := validFreshUnixHeader(ts, at); err != nil {
		return err
	}
	sig, err := hex.DecodeString(strings.TrimSpace(r.Header.Get("X-Signature-Ed25519")))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return errors.New("discord signature is invalid")
	}
	msg := append([]byte(ts), body...)
	if !ed25519.Verify(ed25519.PublicKey(key), msg, sig) {
		return errors.New("discord signature is invalid")
	}
	return nil
}

func verifyMetaSignature(secret string, r *http.Request, body []byte) error {
	provided := strings.TrimSpace(r.Header.Get("X-Hub-Signature-256"))
	if !strings.HasPrefix(provided, "sha256=") {
		return errors.New("invalid meta signature")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(provided), []byte(expected)) {
		return errors.New("invalid meta signature")
	}
	return nil
}

func normalizeChannelKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "telegram":
		return "telegram"
	case "slack":
		return "slack"
	case "discord":
		return "discord"
	case "whatsapp", "whatsapp_cloud", "meta_whatsapp":
		return "whatsapp"
	default:
		return strings.ToLower(strings.TrimSpace(kind))
	}
}

func isWhatsAppKind(kind string) bool { return normalizeChannelKind(kind) == "whatsapp" }

func validateNativeEndpoint(kind, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" {
		return errors.New("native channel endpoint must be credential-free https")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	switch normalizeChannelKind(kind) {
	case "telegram":
		if host != "api.telegram.org" {
			return errors.New("telegram endpoint must use api.telegram.org")
		}
	case "slack":
		if host != "slack.com" {
			return errors.New("slack endpoint must use slack.com")
		}
	case "discord":
		if host != "discord.com" {
			return errors.New("discord endpoint must use discord.com")
		}
	case "whatsapp":
		if host != "graph.facebook.com" {
			return errors.New("whatsapp endpoint must use graph.facebook.com")
		}
	}
	return nil
}

func splitChannelText(s string, max int) []string {
	r := []rune(strings.TrimSpace(s))
	if len(r) == 0 || max <= 0 {
		return nil
	}
	out := make([]string, 0, (len(r)+max-1)/max)
	for len(r) > 0 {
		n := max
		if len(r) < n {
			n = len(r)
		}
		out = append(out, string(r[:n]))
		r = r[n:]
	}
	return out
}
