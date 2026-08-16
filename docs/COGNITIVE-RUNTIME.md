# KINGAIBOT Cognitive Runtime

KINGAIBOT v1.6 adds a persistent cognitive runtime for operational continuity, memory, reflection, experience learning and controlled evolution.

## What it is

The cognitive runtime maintains an auditable operational self-model. It is not a claim of subjective consciousness. Its purpose is to make long-running agents remember prior outcomes, identify repeated failure patterns, retain learned operational principles and create review-only evolution proposals when evidence crosses a configured safety threshold.

The learning path is:

1. A normal KINGAIBOT task is durably committed as `completed` or `failed`.
2. The cognitive scanner observes only that committed record. It never replays the task.
3. The task outcome is converted into a sanitized `experience` memory record.
4. Provider reliability and failure-pattern statistics are updated in `data/cognition/self-model.json`.
5. Reflection periodically consolidates repeated evidence into bounded learned principles.
6. Repeated failures can create a controlled Evolution proposal.
7. Evolution remains `proposal-only`: evaluation, approval, staging, signature verification, health verification, release and rollback remain separate governed transitions.

## Safety boundary

Cognition never receives administrator credentials and never grants itself new authority. It cannot bypass tool policy, approvals, Capability Envelopes, audit, WorkGraph state, Cluster authority, release signatures or reconciliation requirements.

A repeated production failure can produce a proposal, but that proposal has no direct source-edit or deployment permission.

The self-model API is admin-only:

- `GET /v1/cognition/status`
- `POST /v1/cognition/reflect`

## Persistent data

The runtime stores:

- `data/memory/memory.jsonl` — episodic and learned experience memory
- `data/knowledge/` — reviewed knowledge
- `data/cognition/self-model.json` — bounded operational self-model
- `data/cognition/processed-tasks.json` — restart-safe learning deduplication index
- `data/evolution/` — review-only improvement proposals and controlled lifecycle records
- `data/events/` — append-only audit evidence

Secrets are sanitized before cognitive memory is written. Raw API keys should live only in environment variables or the platform secret store used by the service wrapper.

## Cloud and local model gateway

KINGAIBOT uses one prioritized provider list with circuit breaking and fallback. Provider configuration is independent from cognition.

Native protocol adapters:

- OpenAI-compatible Chat Completions
- Anthropic Messages API
- Google Gemini API

Because many hosted and local servers expose OpenAI-compatible endpoints, the same adapter can also connect to compatible services without adding a vendor-specific SDK. `configs/providers.catalog.json` includes reference blocks for OpenAI, Anthropic, Gemini, OpenRouter, Groq, Ollama, LM Studio, vLLM, LocalAI and a generic OpenAI-compatible endpoint.

For API-key providers, set `api_key_env` to the environment-variable name. Never put the secret value in `config.json`.

For same-machine local models, `api_key_env` may be empty. Loopback HTTP requires `allow_insecure_http: true`; local/private-network access must also be explicitly permitted by provider and network policy.

### Ollama example

```json
{
  "name": "ollama-local",
  "type": "openai-compatible",
  "base_url": "http://127.0.0.1:11434/v1",
  "api_key_env": "",
  "model": "your-installed-model",
  "priority": 5,
  "enabled": true,
  "allow_private_network": true,
  "allow_insecure_http": true
}
```

### LM Studio example

```json
{
  "name": "lmstudio-local",
  "type": "openai-compatible",
  "base_url": "http://127.0.0.1:1234/v1",
  "api_key_env": "",
  "model": "your-loaded-model",
  "priority": 5,
  "enabled": true,
  "allow_private_network": true,
  "allow_insecure_http": true
}
```

## System-service operation

The core process is `kingagentd`. It is designed to run continuously as an operating-system service and to recover durable state after restart.

For security, "system service" does not mean unrestricted root authority. Linux uses the dedicated `kingagent` service account with systemd hardening. Windows uses a boot-time service-account task while the signed updater runs separately with SYSTEM authority. The visual client is not the privileged runtime.

This separation is intentional: persistent cognition needs continuity, not unlimited OS privileges.
