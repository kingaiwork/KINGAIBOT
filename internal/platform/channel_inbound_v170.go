package platform

import (
	"crypto/hmac"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type nativeInbound struct {
	EventID string
	Sender  string
	Target  string
	Thread  string
	Text    string
	Secrets map[string]string
}

func readNativeBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAPIRequest)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_body"})
		return nil, false
	}
	return body, true
}

func (m *Manager) InboundHandlerV170() http.Handler {
	g := m.gatewayV170()
	mux := http.NewServeMux()
	mux.Handle("/v1/inbound/", m.InboundHandlerV161())
	mux.HandleFunc("GET /v1/adapters/{kind}/{id}", g.httpNativeVerification)
	mux.HandleFunc("POST /v1/adapters/{kind}/{id}", g.httpNativeInbound)
	return mux
}

func (g *channelGatewayV170) httpNativeVerification(w http.ResponseWriter, r *http.Request) {
	kind := normalizeChannelKind(r.PathValue("kind"))
	if kind != "whatsapp" {
		writePlatformJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "verification_not_supported"})
		return
	}
	channel, err := g.manager.channel(r.PathValue("id"))
	if err != nil || !channel.Enabled || !isWhatsAppKind(channel.Kind) {
		writePlatformJSON(w, http.StatusNotFound, map[string]any{"error": "channel_not_found"})
		return
	}
	verifyToken, err := secretFromEnv(nativeSecretEnv(channel, "_VERIFY_TOKEN"), 16)
	if err != nil {
		writePlatformJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "channel_misconfigured"})
		return
	}
	if r.URL.Query().Get("hub.mode") != "subscribe" || !hmac.Equal([]byte(r.URL.Query().Get("hub.verify_token")), []byte(verifyToken)) {
		writePlatformJSON(w, http.StatusForbidden, map[string]any{"error": "verification_failed"})
		return
	}
	challenge := r.URL.Query().Get("hub.challenge")
	if challenge == "" || len(challenge) > 1024 {
		writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_challenge"})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, challenge)
}

func (g *channelGatewayV170) httpNativeInbound(w http.ResponseWriter, r *http.Request) {
	kind := normalizeChannelKind(r.PathValue("kind"))
	channel, err := g.manager.channel(r.PathValue("id"))
	if err != nil || !channel.Enabled {
		writePlatformJSON(w, http.StatusNotFound, map[string]any{"error": "channel_not_found"})
		return
	}
	if normalizeChannelKind(channel.Kind) != kind {
		writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "channel_kind_mismatch"})
		return
	}
	body, ok := readNativeBody(w, r)
	if !ok {
		return
	}
	switch kind {
	case "telegram":
		g.handleTelegram(w, r, channel, body)
	case "slack":
		g.handleSlack(w, r, channel, body)
	case "discord":
		g.handleDiscord(w, r, channel, body)
	case "whatsapp":
		g.handleWhatsApp(w, r, channel, body)
	default:
		writePlatformJSON(w, http.StatusNotFound, map[string]any{"error": "native_adapter_not_supported"})
	}
}

func (g *channelGatewayV170) acceptNative(w http.ResponseWriter, channel *Channel, in nativeInbound, ack func()) {
	m := g.manager
	if in.EventID == "" || len(in.EventID) > 256 || in.Sender == "" || len(in.Sender) > 512 || in.Target == "" || len(in.Target) > 512 {
		writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_native_event"})
		return
	}
	text, err := cleanText(in.Text, maxPromptLen, "text")
	if err != nil {
		writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "unsupported_or_empty_message"})
		return
	}
	if !senderAllowed(channel.AllowedSenders, in.Sender) {
		writePlatformJSON(w, http.StatusForbidden, map[string]any{"error": "sender_not_allowed"})
		return
	}
	route, err := g.route(channel, in.Sender, in.Target, in.Thread)
	if err != nil {
		platformProblem(w, err)
		return
	}
	pseudo := pseudonymousSender(in.Sender, route.ID)
	receipt, duplicate, err := m.reserveInboundSafe(channel, in.EventID, pseudo, map[string]any{"native_kind": normalizeChannelKind(channel.Kind), "route_id": route.ID})
	if err != nil {
		platformProblem(w, err)
		return
	}
	if duplicate {
		if receipt.TaskID != "" {
			_, _ = g.ensurePending(channel, route, receipt.TaskID, in.Secrets)
			if receipt.Status != "accepted" {
				_ = m.finishInbound(receipt, route.SessionID, receipt.TaskID, "accepted", "")
			}
			ack()
			return
		}
		writePlatformJSON(w, http.StatusConflict, map[string]any{"error": "inbound_reconciliation_required", "receipt_id": receipt.ID})
		return
	}
	created, err := m.SendSessionV14(route.SessionID, text)
	if err != nil {
		_ = m.finishInbound(receipt, route.SessionID, "", "failed", memorySafeError(err))
		platformProblem(w, err)
		return
	}
	if err := m.finishInbound(receipt, route.SessionID, created.ID, "task_created", ""); err != nil {
		writePlatformJSON(w, http.StatusInternalServerError, map[string]any{"error": "receipt_task_link_failed"})
		return
	}
	if _, err := g.ensurePending(channel, route, created.ID, in.Secrets); err != nil {
		writePlatformJSON(w, http.StatusInternalServerError, map[string]any{"error": "outbound_queue_persistence_failed"})
		return
	}
	if err := m.audit("channel.native.inbound.accepted", map[string]any{"channel_id": channel.ID, "receipt_id": receipt.ID, "route_id": route.ID, "session_id": route.SessionID, "task_id": created.ID, "kind": normalizeChannelKind(channel.Kind), "sender_sha256_96": senderDigest(in.Sender)}); err != nil {
		writePlatformJSON(w, http.StatusInternalServerError, map[string]any{"error": "audit_failed"})
		return
	}
	if err := m.finishInbound(receipt, route.SessionID, created.ID, "accepted", ""); err != nil {
		writePlatformJSON(w, http.StatusInternalServerError, map[string]any{"error": "receipt_acceptance_persistence_failed"})
		return
	}
	ack()
}

func (g *channelGatewayV170) handleTelegram(w http.ResponseWriter, r *http.Request, channel *Channel, body []byte) {
	secret, err := secretFromEnv(nativeSecretEnv(channel, "_WEBHOOK_SECRET"), 32)
	if err != nil {
		writePlatformJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "channel_misconfigured"})
		return
	}
	if !hmac.Equal([]byte(r.Header.Get("X-Telegram-Bot-Api-Secret-Token")), []byte(secret)) {
		writePlatformJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_signature"})
		return
	}
	var u struct {
		UpdateID int64 `json:"update_id"`
		Message  *struct {
			MessageThreadID int64  `json:"message_thread_id"`
			Text            string `json:"text"`
			From            *struct {
				ID    int64 `json:"id"`
				IsBot bool  `json:"is_bot"`
			} `json:"from"`
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
		CallbackQuery *struct {
			Data string `json:"data"`
			From struct {
				ID    int64 `json:"id"`
				IsBot bool  `json:"is_bot"`
			} `json:"from"`
			Message *struct {
				MessageThreadID int64 `json:"message_thread_id"`
				Chat            struct {
					ID int64 `json:"id"`
				} `json:"chat"`
			} `json:"message"`
		} `json:"callback_query"`
	}
	if json.Unmarshal(body, &u) != nil {
		writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}
	var in nativeInbound
	in.EventID = strconv.FormatInt(u.UpdateID, 10)
	if u.Message != nil && u.Message.From != nil && !u.Message.From.IsBot {
		in.Sender = strconv.FormatInt(u.Message.From.ID, 10)
		in.Target = strconv.FormatInt(u.Message.Chat.ID, 10)
		in.Text = u.Message.Text
		if u.Message.MessageThreadID != 0 {
			in.Thread = strconv.FormatInt(u.Message.MessageThreadID, 10)
		}
	} else if u.CallbackQuery != nil && u.CallbackQuery.Message != nil && !u.CallbackQuery.From.IsBot {
		in.Sender = strconv.FormatInt(u.CallbackQuery.From.ID, 10)
		in.Target = strconv.FormatInt(u.CallbackQuery.Message.Chat.ID, 10)
		in.Text = u.CallbackQuery.Data
		if u.CallbackQuery.Message.MessageThreadID != 0 {
			in.Thread = strconv.FormatInt(u.CallbackQuery.Message.MessageThreadID, 10)
		}
	} else {
		writePlatformJSON(w, http.StatusOK, map[string]any{"ok": true, "ignored": true})
		return
	}
	g.acceptNative(w, channel, in, func() { writePlatformJSON(w, http.StatusOK, map[string]any{"ok": true}) })
}

func (g *channelGatewayV170) handleSlack(w http.ResponseWriter, r *http.Request, channel *Channel, body []byte) {
	secret, err := secretFromEnv(nativeSecretEnv(channel, "_SIGNING_SECRET"), 32)
	if err != nil {
		writePlatformJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "channel_misconfigured"})
		return
	}
	if err := verifySlackSignature(secret, r, body, now()); err != nil {
		writePlatformJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_signature"})
		return
	}
	var e struct {
		Type      string `json:"type"`
		Challenge string `json:"challenge"`
		EventID   string `json:"event_id"`
		Event     struct {
			Type    string `json:"type"`
			User    string `json:"user"`
			Text    string `json:"text"`
			Channel string `json:"channel"`
			TS      string `json:"ts"`
			Thread  string `json:"thread_ts"`
			BotID   string `json:"bot_id"`
			Subtype string `json:"subtype"`
		} `json:"event"`
	}
	if json.Unmarshal(body, &e) != nil {
		writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}
	if e.Type == "url_verification" {
		if e.Challenge == "" || len(e.Challenge) > 2048 {
			writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_challenge"})
			return
		}
		writePlatformJSON(w, http.StatusOK, map[string]any{"challenge": e.Challenge})
		return
	}
	if e.Type != "event_callback" || e.Event.Type != "message" || e.Event.User == "" || e.Event.BotID != "" || e.Event.Subtype != "" {
		writePlatformJSON(w, http.StatusOK, map[string]any{"ok": true, "ignored": true})
		return
	}
	thread := e.Event.Thread
	if thread == "" {
		thread = e.Event.TS
	}
	g.acceptNative(w, channel, nativeInbound{EventID: e.EventID, Sender: e.Event.User, Target: e.Event.Channel, Thread: thread, Text: e.Event.Text}, func() {
		writePlatformJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
}

func discordPrompt(data map[string]any) string {
	name, _ := data["name"].(string)
	if name == "" {
		return ""
	}
	if opts, ok := data["options"].([]any); ok {
		for _, key := range []string{"prompt", "text", "message", "query"} {
			for _, raw := range opts {
				if opt, ok := raw.(map[string]any); ok && opt["name"] == key {
					return strings.TrimSpace(name + ": " + strings.TrimSpace(toText(opt["value"])))
				}
			}
		}
	}
	return name
}

func toText(v any) string {
	s, _ := v.(string)
	if s != "" {
		return s
	}
	return strings.TrimSpace(strings.ReplaceAll(strings.TrimSpace(string(mustJSON(v))), "\n", " "))
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func (g *channelGatewayV170) handleDiscord(w http.ResponseWriter, r *http.Request, channel *Channel, body []byte) {
	publicKey, err := secretFromEnv(nativeSecretEnv(channel, "_PUBLIC_KEY"), 64)
	if err != nil {
		writePlatformJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "channel_misconfigured"})
		return
	}
	if err := verifyDiscordSignature(publicKey, r, body, now()); err != nil {
		writePlatformJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_signature"})
		return
	}
	var e struct {
		ID            string         `json:"id"`
		ApplicationID string         `json:"application_id"`
		Token         string         `json:"token"`
		Type          int            `json:"type"`
		ChannelID     string         `json:"channel_id"`
		Data          map[string]any `json:"data"`
		Member        *struct {
			User struct {
				ID  string `json:"id"`
				Bot bool   `json:"bot"`
			} `json:"user"`
		} `json:"member"`
		User *struct {
			ID  string `json:"id"`
			Bot bool   `json:"bot"`
		} `json:"user"`
	}
	if json.Unmarshal(body, &e) != nil {
		writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}
	if e.Type == 1 {
		writePlatformJSON(w, http.StatusOK, map[string]any{"type": 1})
		return
	}
	if e.Type != 2 {
		writePlatformJSON(w, http.StatusOK, map[string]any{"type": 4, "data": map[string]any{"content": "Unsupported interaction"}})
		return
	}
	sender := ""
	isBot := false
	if e.Member != nil {
		sender, isBot = e.Member.User.ID, e.Member.User.Bot
	} else if e.User != nil {
		sender, isBot = e.User.ID, e.User.Bot
	}
	if sender == "" || isBot || e.ChannelID == "" || e.ApplicationID == "" || e.Token == "" {
		writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_interaction"})
		return
	}
	g.acceptNative(w, channel, nativeInbound{EventID: e.ID, Sender: sender, Target: e.ChannelID, Text: discordPrompt(e.Data), Secrets: map[string]string{"discord_application_id": e.ApplicationID, "discord_interaction_token": e.Token}}, func() {
		writePlatformJSON(w, http.StatusOK, map[string]any{"type": 5})
	})
}

func (g *channelGatewayV170) handleWhatsApp(w http.ResponseWriter, r *http.Request, channel *Channel, body []byte) {
	secret, err := secretFromEnv(nativeSecretEnv(channel, "_APP_SECRET"), 32)
	if err != nil {
		writePlatformJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "channel_misconfigured"})
		return
	}
	if err := verifyMetaSignature(secret, r, body); err != nil {
		writePlatformJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_signature"})
		return
	}
	var payload struct {
		Entry []struct {
			Changes []struct {
				Value struct {
					Messages []struct {
						ID   string `json:"id"`
						From string `json:"from"`
						Type string `json:"type"`
						Text struct {
							Body string `json:"body"`
						} `json:"text"`
						Interactive struct {
							ButtonReply struct {
								Title string `json:"title"`
							} `json:"button_reply"`
							ListReply struct {
								Title string `json:"title"`
							} `json:"list_reply"`
						} `json:"interactive"`
					} `json:"messages"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}
	if json.Unmarshal(body, &payload) != nil {
		writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			for _, msg := range change.Value.Messages {
				text := ""
				switch msg.Type {
				case "text":
					text = msg.Text.Body
				case "interactive":
					text = msg.Interactive.ButtonReply.Title
					if text == "" {
						text = msg.Interactive.ListReply.Title
					}
				}
				if msg.ID != "" && msg.From != "" && strings.TrimSpace(text) != "" {
					g.acceptNative(w, channel, nativeInbound{EventID: msg.ID, Sender: msg.From, Target: msg.From, Text: text}, func() {
						writePlatformJSON(w, http.StatusOK, map[string]any{"ok": true})
					})
					return
				}
			}
		}
	}
	writePlatformJSON(w, http.StatusOK, map[string]any{"ok": true, "ignored": true})
}
