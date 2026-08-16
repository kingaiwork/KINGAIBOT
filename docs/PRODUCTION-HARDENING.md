# KINGAIBOT v1.6.1 Production Hardening

This release hardens the long-running system daemon without expanding autonomous authority. The cognitive runtime remains an operational self-model, not a claim of subjective consciousness, and Evolution remains proposal-only.

## Security boundaries

- The daemon keeps a global client-IP request quota and common defensive HTTP headers across core, platform, cognition, orchestration, authority and inbound-channel routes.
- `/healthz` and `/readyz` are quota-exempt so frequent supervisor probes cannot self-induce a 429/restart loop.
- Runtime queue saturation returns `503 Service Unavailable` with `Retry-After: 1` so clients can distinguish backpressure from invalid input.
- Session task metadata stores a truncated SHA-256 sender digest instead of copying the raw external sender identifier into every task record.
- Channel ingress keeps bearer-token authentication and durable event idempotency. An optional second HMAC boundary can be enabled for Internet-facing normalized gateways.

## Signed normalized Channel ingress

Existing bearer-only channels remain compatible. To require HMAC integrity for a channel whose `bearer_token_env` is:

```text
TELEGRAM_GATEWAY_TOKEN
```

provision a second secret of at least 32 bytes:

```text
TELEGRAM_GATEWAY_TOKEN_SIGNING_SECRET
```

Once that environment variable is non-empty, each request to `POST /v1/inbound/{channel_id}` must include:

```text
X-KINGAI-Timestamp: <Unix seconds>
X-KINGAI-Signature: v1=<hex HMAC-SHA256>
Authorization: Bearer <channel bearer token>
```

The canonical signing bytes are exactly:

```text
<timestamp>\n<channel_id>\n<raw HTTP request body bytes>
```

Requests outside a five-minute clock-skew window are rejected. Replaying a captured valid request inside that window still resolves to the same durable inbound receipt because `channel_id + event_id` is idempotent.

Gateway pseudocode:

```text
body = exact bytes to POST
timestamp = current Unix seconds
canonical = timestamp + "\n" + channel_id + "\n" + body
signature = "v1=" + hex(HMAC_SHA256(signing_secret, canonical))
```

Do not reuse the bearer token as the signing secret. Rotate either secret independently at the gateway and daemon boundary.

## Cognitive-runtime stability

The cognition scanner no longer decodes the entire historical task store every poll. It uses a persisted update cursor, a small timestamp-overlap window and bounded batches. This keeps steady-state learning cost tied to recently changed tasks rather than total system lifetime while preserving restart-safe fingerprints.

The learning rules remain unchanged:

- learn only from durable terminal task states;
- never replay a task to learn from it;
- never turn learned principles into authority;
- never automatically release self-modified production code;
- repeated failure patterns may create review-only Evolution proposals.

## Container consistency

Server, Worker and Console images read the repository `VERSION` file during build and inject that version into their binaries. Container validation asserts all three images report exactly the same version as `VERSION`.

`docker-compose.yml` no longer hard-requires an OpenAI key. It passes optional OpenAI, Anthropic, Gemini, OpenRouter, Groq and generic OpenAI-compatible secrets, and supports a custom provider configuration file:

```bash
KINGAGENT_CONFIG_FILE=./my-config.json docker compose up -d --build
```

The bundled Docker configuration still enables its documented default provider. To use only another cloud provider or a local/private endpoint, supply a custom config with that provider enabled.

## Production checklist

Before exposing a deployment outside loopback:

1. Put TLS termination/reverse proxying in front of the daemon; do not expose plain HTTP publicly.
2. Use distinct admin, MCP, A2A and Channel credentials.
3. Enable HMAC signing for normalized Internet-facing Channel gateways.
4. Keep `security.default_tool_policy` at `deny` and explicitly allow/ask only the tools required by the deployment.
5. Keep file roots restricted to the intended workspace and keep private-network access disabled unless a trusted local-model/private-service endpoint requires it.
6. Keep Evolution in `proposal-only` mode.
7. Monitor `/readyz`, reconciliation-required tasks, pending approvals, audit health and provider failure rates.
8. Keep signed updater verification enabled in production installations where `cosign` is available.
9. Back up the data directory with filesystem permissions preserved; it contains operational history and user/session data.
10. Treat Channel sender identifiers and conversation history as personal data and define retention appropriate to the deployment.
