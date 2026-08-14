# KINGAIBOT Validation

This document defines release-validation requirements for the current security-first platform. A passing test suite reduces known risk; it is not a claim of absolute bug freedom.

## V1.3 release baseline

A V1.3 release candidate must use Go **1.26.6 or newer within the supported 1.26 line** and pass the following gates on the exact commit/tag being released.

## Required CI gates

### Linux

```bash
test -z "$(gofmt -l .)"
go vet ./...
go test -race -count=1 ./...
govulncheck ./...
```

The full validation workflow additionally builds:

```text
kingagentd
kingagent
kingworker
kingconsole
```

### Native macOS

```bash
go test -count=1 ./...
go build ./cmd/kingagentd
go build ./cmd/kingagent
go build ./cmd/kingworker
go build ./cmd/kingconsole
```

### Native Windows

The same four binaries must compile and all tests must run on a GitHub-hosted Windows runner.

### Docker

The validation suite builds independent images for:

- server (`deploy/docker/Dockerfile`)
- worker (`deploy/docker/Dockerfile.worker`)
- console (`deploy/docker/Dockerfile.console`)

### Code scanning

CodeQL must run against the release candidate. A green CodeQL workflow does not replace manual threat review, especially around new network, authentication or parsing boundaries.

## Full release bundle gate

`scripts/build-release-v13.sh` builds six target combinations:

```text
linux/amd64
linux/arm64
darwin/amd64
darwin/arm64
windows/amd64
windows/arm64
```

Each archive must contain:

- `kingagentd`
- `kingagent`
- `kingworker`
- `kingconsole`
- example configuration
- README
- commercial license notice
- update scripts

The release output also contains:

- CycloneDX SBOM
- `RELEASE-MANIFEST.json`
- `SHA256SUMS`
- Sigstore bundles for tag releases
- GitHub artifact provenance when supported by the repository visibility/plan

## Security regression areas

### Exact approval

Tests must preserve:

- canonical argument hashing
- task + tool + arguments binding
- no replay with different arguments
- cached result semantics for already-completed approved actions
- reconciliation behavior when an external side effect may have executed but durable completion is uncertain

### Filesystem sandbox

Tests cover:

- traversal rejection
- safe file operations inside approved roots
- symlink/path escape resistance through the Go `os.Root` design
- no recursive-delete agent primitive
- worker file sandbox traversal rejection

### Network guard

Tests cover or exercise:

- insecure public endpoint rejection
- private-network/loopback policy
- DNS/IP restrictions
- redirect restrictions
- bounded response sizes
- credential separation

### Native providers

Provider tests use mock HTTP servers to verify protocol conversion without sending real credentials:

- OpenAI-compatible request/response behavior
- Anthropic Messages tool-use/tool-result round trip
- Gemini function-call/function-response round trip
- provider type validation at config startup

### Platform identity

Tests verify:

- viewer cannot gain write authority
- admin role can satisfy admin permission
- raw platform API keys are not persisted
- revocation blocks future authentication

### Inbound channels

Tests verify:

- missing/invalid channel bearer token is rejected
- sender allowlist is enforced
- repeated `event_id` does not create a second task

### Reviewed knowledge

Tests verify:

- proposed knowledge does not appear in trusted search
- approved knowledge becomes searchable
- rejected knowledge stays hidden
- agent knowledge tool can propose but cannot self-approve
- secrets are redacted before persistence

### Cluster leases

Tests verify:

- Worker raw tokens are not persisted
- capability mismatch cannot lease a job
- default `manual` replay policy moves expired ambiguous work to reconciliation
- explicitly `safe` work can be requeued
- one lease cannot complete the same job twice
- worker sandbox and HTTPS allowlist behavior

### Controlled evolution

Tests verify:

- proposal cannot be approved before evaluation
- a failed evaluation does not silently promote trust
- release requires an approved + staged proposal
- release digest must match the staged SHA-256
- signature verification and passed health status are required
- rollback becomes a durable/audited state
- agent tools can propose but do not expose approve/stage/release authority

### Runtime task creation classification

Tests verify:

- invalid input is classified as a client error
- queue saturation is classified separately from invalid input
- queue-full tasks are persisted as terminal failed rather than stranded queued work
- HTTP layer maps transient runtime saturation to `503 Service Unavailable`

## Manual security review checklist

Before a production tag:

1. Review any newly added tool or extension capability.
2. Confirm its default tool policy is `deny` or `ask` if it can cause side effects.
3. Confirm secrets are referenced by environment variable and are not serialized into model-visible list APIs.
4. Review new HTTP clients for redirect, SSRF and response-size behavior.
5. Review new persistent trust transitions for audit-before-trust ordering.
6. Confirm ambiguous external side effects are not automatically replayed.
7. Confirm temporary branch-mutating workflows have been removed.
8. Confirm the release commit is immutable and the release artifact digest/signature/provenance can be verified.

## Residual validation limits

The current automated suite does not prove:

- correctness of every third-party model endpoint beyond the protocol adapters tested with mocks;
- security of future vendor-specific Telegram/Slack/WhatsApp/browser/mobile adapters that are not bundled in the current trust root;
- multi-controller distributed database correctness, because V1.3 is a single durable coordinator + multi-worker baseline;
- absence of all prompt-injection or model-behavior failures;
- absence of unknown vulnerabilities in operating systems, external APIs or future dependencies.

Production deployment should retain least privilege, network segmentation, external monitoring, backups and human takeover.
