# KINGAIBOT Unified Channel Gateway v1.7

KINGAIBOT v1.7 adds a durable, security-bounded communications layer for native chat channels while preserving the normalized signed webhook API from v1.6.1.

## Supported channel kinds

| Kind | Inbound | Outbound | Authentication |
|---|---|---|---|
| `telegram` | Native Bot API webhook | `sendMessage` | Telegram webhook secret token + bot token |
| `slack` | Events API | `chat.postMessage` | Slack HMAC signing secret + bot OAuth token |
| `discord` | HTTP interactions | interaction response / channel message | Ed25519 public key + bot token |
| `whatsapp` / `whatsapp_cloud` | WhatsApp Cloud webhook | Cloud API text message | app-secret HMAC + access token + verify token |
| `webhook` and other custom kinds | normalized KING inbound | generic JSON POST | existing Bearer/HMAC/netguard behavior |

No third-party SDK is embedded. Native adapters use Go standard-library cryptography and the existing KINGAIBOT network guard.

## Native inbound URLs

After creating a Channel, configure the provider webhook to:

```text
https://YOUR-KINGAIBOT/v1/adapters/telegram/CHANNEL_ID
https://YOUR-KINGAIBOT/v1/adapters/slack/CHANNEL_ID
https://YOUR-KINGAIBOT/v1/adapters/discord/CHANNEL_ID
https://YOUR-KINGAIBOT/v1/adapters/whatsapp/CHANNEL_ID
```

The normalized compatibility gateway remains:

```text
POST /v1/inbound/CHANNEL_ID
```

The native adapter kind in the URL must match the stored Channel kind.

## Channel configuration

A Channel continues to use the existing platform object:

```json
{
  "name": "Support Telegram",
  "kind": "telegram",
  "endpoint": "https://api.telegram.org",
  "bearer_token_env": "KING_TELEGRAM_BOT_TOKEN",
  "allowed_senders": ["123456789"]
}
```

Known native channel endpoints are pinned to their official service host. A lookalike hostname such as `api.telegram.org.evil.example` is rejected.

Recommended endpoints:

```text
Telegram: https://api.telegram.org
Slack:    https://slack.com/api/chat.postMessage
Discord:  https://discord.com/api/v10
WhatsApp: https://graph.facebook.com/<GRAPH_VERSION>/<PHONE_NUMBER_ID>/messages
```

For native public services, private-network routing and insecure HTTP are not used even when a Channel object enables those generic-webhook options.

## Secret environment variables

`bearer_token_env` names the primary outbound credential. Native adapters derive additional secret names from it.

### Telegram

```text
KING_TELEGRAM_BOT_TOKEN=<bot token>
KING_TELEGRAM_BOT_TOKEN_WEBHOOK_SECRET=<random secret, 32+ chars>
```

Set the same webhook secret when registering the Telegram webhook.

### Slack

```text
KING_SLACK_BOT_TOKEN=<xoxb bot token>
KING_SLACK_BOT_TOKEN_SIGNING_SECRET=<Slack signing secret>
```

The gateway verifies the Slack `v0` request signature and rejects timestamps outside the replay window.

### Discord

```text
KING_DISCORD_BOT_TOKEN=<bot token>
KING_DISCORD_BOT_TOKEN_PUBLIC_KEY=<Discord application Ed25519 public key in hex>
```

Discord PING interactions are answered directly. Application-command interactions are deferred while the KINGAIBOT task executes, then the durable outbound worker updates the original response. If an interaction token is no longer usable, the gateway can fall back to a normal bot message when a channel target and bot token are available.

### WhatsApp Cloud

```text
KING_WHATSAPP_TOKEN=<Cloud API access token>
KING_WHATSAPP_TOKEN_APP_SECRET=<Meta app secret>
KING_WHATSAPP_TOKEN_VERIFY_TOKEN=<operator-chosen webhook verify token>
```

The GET verification request is answered only when the verify token matches. POST bodies are authenticated before messages are accepted.

## Privacy boundary

Native provider identifiers are treated as routing secrets, not general agent memory.

The raw platform sender/phone/chat reply target is stored only in a restricted `channel-routes` record. Generic Session and Task metadata receive a pseudonymous sender plus a truncated SHA-256 digest. Audit records contain route/session/task IDs and sender digests, not raw platform user IDs.

Discord interaction tokens are stored only in the restricted pending-delivery record needed to finish that interaction. The admin API deliberately redacts the `Secrets` field.

## Durable inbound semantics

Native inbound events reuse the v1.6.1 crash-safe receipt system:

```text
provider signature/auth
  -> deterministic event receipt
  -> pseudonymous route/session
  -> durable Runtime Task
  -> task_created receipt
  -> durable outbound delivery
  -> accepted receipt
```

Provider retries with the same event ID do not create a second Task. If a Task identity is already known, the gateway repairs/ensures the deterministic outbound-delivery record instead of submitting the user message again.

Native adapters never accept a caller-provided KINGAIBOT Session ID. Session ownership is derived from the trusted Channel route, preventing a provider payload from selecting another user's session.

## Durable outbound semantics

The outbound worker scans only `outbound-pending`, not the full historic Task set.

```text
waiting_task
  -> sending (persisted before external request)
  -> delivered
  -> retry_wait       only for an explicit rate-limit response
  -> reconciliation   ambiguous transport / 5xx / crash-after-send
  -> failed           definite terminal rejection
```

A restart that finds a delivery already in `sending` changes it to `reconciliation`; KINGAIBOT does not blindly resend an external side effect whose outcome is unknown.

HTTP 429 may use bounded exponential retry. HTTP 5xx is intentionally treated as ambiguous because a remote platform could have accepted the message before returning an error.

Completed delivery receipts are retained for a bounded period; pending work is kept separately so the steady-state scanner cost depends on unresolved deliveries rather than all historical messages.

## Admin operations

All Channel Gateway administration is mounted behind the existing KINGAIBOT admin-auth boundary:

```text
GET  /v1/platform/channel-gateway/status
GET  /v1/platform/channel-gateway/pending
GET  /v1/platform/channel-gateway/receipts
POST /v1/platform/channel-gateway/pending/{id}/retry
POST /v1/platform/channel-gateway/pending/{id}/resolve
```

`retry` is for an operator who has decided replay is appropriate after reconciliation. `resolve` records the operator's external reconciliation result as `delivered` or `failed` without re-sending.

## Message limits

Outbound text is split by Unicode code points rather than bytes so UTF-8 text is never cut in the middle of a character. Conservative channel-specific limits are applied before delivery.

## Security model

The Channel Gateway does not grant new KING authority. A provider message becomes ordinary untrusted user input after transport authentication. It still passes through Session -> Runtime -> policy -> approval -> tool/capability boundaries.

Channel transport credentials do not become model-visible context. Native provider secrets are never returned by the Channel Gateway admin status endpoints.

For internet-facing operation, terminate TLS at a trusted HTTPS endpoint and keep KINGAIBOT's existing HTTP guard, request-size bounds, sender allowlists and admin authorization enabled.
