package platform

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const inboundSignatureMaxSkew = 5 * time.Minute

var errInboundSigningMisconfigured = errors.New("channel signing secret is misconfigured")

// Channel signing is opt-in and backward compatible. When
// <BEARER_TOKEN_ENV>_SIGNING_SECRET is present, every normalized gateway
// request must carry a fresh HMAC signature over its exact raw request body.
// This gives the shared bearer credential one job (gateway authentication) and
// the signing secret another (message integrity/replay-window protection).
func inboundSigningSecretEnv(c *Channel) string {
	if c == nil || strings.TrimSpace(c.BearerTokenEnv) == "" {
		return ""
	}
	return strings.TrimSpace(c.BearerTokenEnv) + "_SIGNING_SECRET"
}

func verifyInboundSignatureV161(r *http.Request, c *Channel, rawBody []byte, now time.Time) error {
	envName := inboundSigningSecretEnv(c)
	if envName == "" {
		return nil
	}
	secret := os.Getenv(envName)
	if secret == "" {
		// Compatibility mode: a channel that has not provisioned the optional
		// signing secret continues to use the existing bearer-token boundary.
		return nil
	}
	if len(secret) < 32 {
		return errInboundSigningMisconfigured
	}

	tsText := strings.TrimSpace(r.Header.Get("X-KINGAI-Timestamp"))
	sigText := strings.TrimSpace(r.Header.Get("X-KINGAI-Signature"))
	if tsText == "" || sigText == "" {
		return errors.New("signed channel request requires timestamp and signature")
	}
	tsUnix, err := strconv.ParseInt(tsText, 10, 64)
	if err != nil {
		return errors.New("invalid channel signature timestamp")
	}
	ts := time.Unix(tsUnix, 0)
	delta := now.Sub(ts)
	if delta < 0 {
		delta = -delta
	}
	if delta > inboundSignatureMaxSkew {
		return errors.New("channel signature timestamp outside replay window")
	}

	const prefix = "v1="
	if !strings.HasPrefix(sigText, prefix) {
		return errors.New("unsupported channel signature version")
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(sigText, prefix))
	if err != nil || len(provided) != sha256.Size {
		return errors.New("invalid channel signature")
	}
	canonical := []byte(tsText + "\n" + c.ID + "\n")
	canonical = append(canonical, rawBody...)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(canonical)
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return errors.New("invalid channel signature")
	}
	return nil
}

// SignInboundBody is intentionally exported for trusted gateway adapters and
// tests. The signing secret itself remains outside channel configuration and
// is never written to durable state.
func SignInboundBody(secret, channelID string, unixSeconds int64, rawBody []byte) (string, error) {
	if len(secret) < 32 {
		return "", fmt.Errorf("signing secret must be at least 32 bytes")
	}
	tsText := strconv.FormatInt(unixSeconds, 10)
	canonical := []byte(tsText + "\n" + channelID + "\n")
	canonical = append(canonical, rawBody...)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(canonical)
	return "v1=" + hex.EncodeToString(mac.Sum(nil)), nil
}
