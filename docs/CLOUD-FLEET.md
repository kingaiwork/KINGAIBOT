# KINGAIBOT v1.8 Cloud Identity, Fleet and Encrypted Continuity

KINGAIBOT remains a local authority-first Runtime. v1.8 adds an optional KING AI Cloud control plane for device enrollment, fleet health, restriction-only policy and end-to-end encrypted continuity.

## Security invariant

```text
KING AI Cloud policy
        |
        | may only contract
        v
Local config -> Authority Envelope -> Policy -> Exact Approval -> Audit -> Execution
```

Cloud policy cannot create a Capability Envelope, turn `deny` into `allow`, enable a disabled provider, create/enable a channel, approve a high-risk action, bypass reconciliation, or read local provider credentials.

The default failure mode is **local-first**: a cloud outage does not stop already-authorized local work. Set `KINGAI_CLOUD_REQUIRE_POLICY=1` only when an enterprise deployment intentionally requires fail-closed cloud-policy availability at startup.

## Device identity and enrollment

Each enrolled node owns a local Ed25519 private key. The private key is generated on the node and stored under the Runtime data directory with mode `0600` on Unix-like systems. Only the SPKI public key is sent to the control plane.

Enrollment uses the existing KINGAIASE OPS one-time enrollment contract:

1. An authorized OPS administrator creates a one-time enrollment token.
2. The token is provisioned to the target node through `KINGAI_ENROLLMENT_TOKEN`.
3. KINGAIBOT generates/loads its Ed25519 device key and signs the canonical `KINGAI-OPS-ENROLL-V2` request.
4. The control plane consumes the token and returns `node_id`, `key_id`, `organization_id` and `workspace_id`.
5. The raw one-time enrollment token is never written into KINGAIBOT state.
6. Subsequent cloud requests are signed with the node key.

The local Cloud & Fleet page is available through the visual client at:

```text
http://127.0.0.1:18889/ui/cloud/
```

It shows enrollment state, Node ID, Workspace, heartbeat, policy version, E2EE sync status and the latest cloud error. The page keeps the local Admin Token only in page memory.

## Heartbeat and fleet health

Heartbeat requests are signed with the node Ed25519 key and use a monotonic sequence. The cloud fleet can summarize availability and health without receiving local model prompts, memory content or provider API keys.

Default heartbeat interval:

```text
60 seconds
```

Configure with:

```text
KINGAI_CLOUD_HEARTBEAT_SECONDS=60
```

## Restriction-only cloud policy

The v1.8 policy surface deliberately supports only contraction:

- disable an existing provider by name;
- disable an existing channel by ID, name or kind;
- lower Runtime `max_steps`;
- lower Runtime worker count;
- lower task timeout;
- tighten a tool from `allow -> ask -> deny`;
- disable encrypted continuity sync.

Cloud policy cannot carry `allow` as a remote expansion. Local configuration and local Authority remain the maximum envelope.

## End-to-end encrypted continuity

Continuity sync is opt-in:

```text
KINGAI_MEMORY_SYNC=1
KINGAI_SYNC_KEY=<base64 of exactly 32 random bytes>
```

The sync key is provisioned directly to devices that are intentionally allowed to share continuity. It is **not** uploaded to KING AI Cloud.

Before upload, KINGAIBOT creates a bounded continuity snapshot and encrypts it locally with AES-256-GCM. The authenticated associated data binds:

```text
workspace + stream + key_id
```

The cloud stores only:

- `node_id`
- `stream`
- monotonic `sequence`
- `key_id` (short hash-derived identifier, not the key)
- `nonce_b64`
- `ciphertext_b64`
- `ciphertext_sha256`
- timestamps and tenant scope

There is no plaintext-memory or sync-key database column.

The server keeps only the most recent five encrypted envelopes per node/stream. Sequence claims are monotonic and transactionally coupled to envelope persistence to reject stale/replayed writes.

## What is synchronized

The continuity snapshot contains:

- a bounded set of durable Memory records;
- an inspectable Cognition snapshot for recovery/continuity evidence.

Peer Cognition state is **not** automatically merged into another node's self-model. Only Memory records are imported, sanitized, deduplicated and capped to a confidence of `0.95`. This prevents one peer's operational self-model from silently becoming another node's identity.

## Local admin API

All cloud management endpoints below remain behind the existing local admin authentication boundary:

```text
GET  /v1/cloud/status
POST /v1/cloud/policy/pull
POST /v1/cloud/sync
```

No cloud credential or Admin Token is returned by these endpoints.

## Environment template

Release bundles include `cloud.env.example`. Important values are:

```text
KINGAI_CLOUD_ENABLED=1
KINGAI_CLOUD_BASE_URL=https://api.kingai.work
KINGAI_ENROLLMENT_TOKEN=
KINGAI_CLOUD_ENVIRONMENT=production
KINGAI_NODE_CLASS=server
KINGAI_NODE_PROVIDER=
KINGAI_NODE_REGION=
KINGAI_CLOUD_HEARTBEAT_SECONDS=60
KINGAI_CLOUD_REQUIRE_POLICY=0
KINGAI_MEMORY_SYNC=0
KINGAI_MEMORY_SYNC_SECONDS=900
KINGAI_SYNC_KEY=
KINGAI_CLOUD_ALLOW_CUSTOM_ENDPOINT=0
```

Production pins `api.kingai.work` and HTTPS unless a local operator explicitly enables a custom endpoint. Custom endpoints still require HTTPS and pass KINGAIBOT network guards.

## Operational meaning

Cloud management does not turn KINGAIBOT into a remote shell. The fleet layer is for identity, health, policy contraction and encrypted continuity. Execution continues to flow through the local Runtime's durable Task, Authority, Policy, Approval, Audit, Evidence and Reconciliation machinery.
