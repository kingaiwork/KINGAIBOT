package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/memory"
	"github.com/kingaiwork/KINGAIBOT/internal/netguard"
)

const channelRemoteBodyLimit = 1 << 20

func (g *channelGatewayV170) send(ctx context.Context, channel *Channel, route *ChannelRoute, text string, secrets map[string]string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", errors.New("channel message is empty")
	}
	kind := normalizeChannelKind(channel.Kind)
	if kind == "telegram" || kind == "slack" || kind == "discord" || kind == "whatsapp" {
		if err := validateNativeEndpoint(kind, channel.Endpoint); err != nil {
			return "", err
		}
	}
	switch kind {
	case "telegram":
		return g.sendTelegram(ctx, channel, route, text)
	case "slack":
		return g.sendSlack(ctx, channel, route, text)
	case "discord":
		return g.sendDiscord(ctx, channel, route, text, secrets)
	case "whatsapp":
		return g.sendWhatsApp(ctx, channel, route, text)
	default:
		return g.manager.remotePOST(ctx, channel.Endpoint, channel.BearerTokenEnv, channel.AllowPrivateNetwork, map[string]any{"recipient": route.ReplyTarget, "text": text, "channel_id": channel.ID, "kind": channel.Kind})
	}
}

// SendChannelV170 is the governed model/operator-facing send path. Native
// adapters use platform-specific authentication and official endpoint pinning;
// generic webhook channels retain the existing netguard-backed POST behavior.
func (m *Manager) SendChannelV170(ctx context.Context, channel *Channel, recipient, text string) (string, error) {
	if channel == nil || !channel.Enabled {
		return "", errors.New("channel disabled or unavailable")
	}
	recipient, err := cleanText(recipient, 512, "recipient")
	if err != nil {
		return "", err
	}
	text, err = cleanText(text, maxPromptLen, "text")
	if err != nil {
		return "", err
	}
	if len(channel.AllowedSenders) > 0 && !senderAllowed(channel.AllowedSenders, recipient) {
		return "", errors.New("recipient not allowed")
	}
	t := now()
	route := &ChannelRoute{ID: "manual", ChannelID: channel.ID, ReplyTarget: recipient, CreatedAt: t, UpdatedAt: t}
	return m.gatewayV170().send(ctx, channel, route, text, nil)
}

func (g *channelGatewayV170) doJSON(ctx context.Context, method, endpoint, auth string, payload any) (string, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "KINGAIBOT-ChannelGateway/"+channelGatewayVersion)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	client := netguard.Client(30*time.Second, false)
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return "", &ambiguousChannelError{err: errors.New("channel transport failed")}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, channelRemoteBodyLimit+1))
	if err != nil {
		return "", &ambiguousChannelError{err: errors.New("channel response read failed")}
	}
	if len(body) > channelRemoteBodyLimit {
		return "", errors.New("channel response exceeds limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &channelHTTPError{Status: resp.StatusCode, Body: safeChannelError(errors.New(string(body)))}
	}
	return string(body), nil
}

func channelToken(c *Channel) (string, error) {
	return secretFromEnv(c.BearerTokenEnv, 8)
}

func (g *channelGatewayV170) sendTelegram(ctx context.Context, c *Channel, route *ChannelRoute, text string) (string, error) {
	token, err := channelToken(c)
	if err != nil {
		return "", err
	}
	base := strings.TrimRight(c.Endpoint, "/") + "/bot" + url.PathEscape(token) + "/sendMessage"
	var last string
	for _, chunk := range splitChannelText(text, 4096) {
		payload := map[string]any{"chat_id": route.ReplyTarget, "text": chunk}
		if route.Thread != "" {
			if thread, err := strconv.ParseInt(route.Thread, 10, 64); err == nil {
				payload["message_thread_id"] = thread
			}
		}
		last, err = g.doJSON(ctx, http.MethodPost, base, "", payload)
		if err != nil {
			return "", err
		}
		var result struct {
			OK          bool   `json:"ok"`
			Description string `json:"description"`
		}
		if json.Unmarshal([]byte(last), &result) == nil && !result.OK {
			return "", errors.New("telegram API rejected message: " + memory.SanitizeContent(result.Description))
		}
	}
	return last, nil
}

func (g *channelGatewayV170) sendSlack(ctx context.Context, c *Channel, route *ChannelRoute, text string) (string, error) {
	token, err := channelToken(c)
	if err != nil {
		return "", err
	}
	var last string
	for _, chunk := range splitChannelText(text, 35000) {
		payload := map[string]any{"channel": route.ReplyTarget, "text": chunk}
		if route.Thread != "" {
			payload["thread_ts"] = route.Thread
		}
		last, err = g.doJSON(ctx, http.MethodPost, c.Endpoint, "Bearer "+token, payload)
		if err != nil {
			return "", err
		}
		var result struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if json.Unmarshal([]byte(last), &result) == nil && !result.OK {
			return "", errors.New("slack API rejected message: " + memory.SanitizeContent(result.Error))
		}
	}
	return last, nil
}

func (g *channelGatewayV170) sendDiscord(ctx context.Context, c *Channel, route *ChannelRoute, text string, secrets map[string]string) (string, error) {
	chunks := splitChannelText(text, 2000)
	if len(chunks) == 0 {
		return "", errors.New("discord message is empty")
	}
	applicationID := secrets["discord_application_id"]
	interactionToken := secrets["discord_interaction_token"]
	var last string
	var err error
	if applicationID != "" && interactionToken != "" {
		endpoint := strings.TrimRight(c.Endpoint, "/") + "/webhooks/" + url.PathEscape(applicationID) + "/" + url.PathEscape(interactionToken) + "/messages/@original"
		last, err = g.doJSON(ctx, http.MethodPatch, endpoint, "", map[string]any{"content": chunks[0]})
		if err == nil {
			chunks = chunks[1:]
		} else {
			var he *channelHTTPError
			if !errors.As(err, &he) || (he.Status != http.StatusUnauthorized && he.Status != http.StatusNotFound) {
				return "", err
			}
		}
	}
	if len(chunks) == 0 {
		return last, nil
	}
	token, err := channelToken(c)
	if err != nil {
		return "", err
	}
	for _, chunk := range chunks {
		endpoint := strings.TrimRight(c.Endpoint, "/") + "/channels/" + url.PathEscape(route.ReplyTarget) + "/messages"
		last, err = g.doJSON(ctx, http.MethodPost, endpoint, "Bot "+token, map[string]any{"content": chunk})
		if err != nil {
			return "", err
		}
	}
	return last, nil
}

func (g *channelGatewayV170) sendWhatsApp(ctx context.Context, c *Channel, route *ChannelRoute, text string) (string, error) {
	token, err := channelToken(c)
	if err != nil {
		return "", err
	}
	var last string
	for _, chunk := range splitChannelText(text, 4096) {
		payload := map[string]any{
			"messaging_product": "whatsapp",
			"recipient_type":    "individual",
			"to":                route.ReplyTarget,
			"type":              "text",
			"text":              map[string]any{"preview_url": false, "body": chunk},
		}
		last, err = g.doJSON(ctx, http.MethodPost, c.Endpoint, "Bearer "+token, payload)
		if err != nil {
			return "", err
		}
	}
	return last, nil
}
