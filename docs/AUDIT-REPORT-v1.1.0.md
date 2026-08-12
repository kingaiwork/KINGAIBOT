# KINGAIBOT v1.1.0 Hardening Audit Report

Date: 2026-08-12
Scope: source/runtime, task recovery, approvals, tools, network, memory, protocols, persistence, installers, updater, container/systemd and GitHub supply chain.

This is an engineering self-audit, not an independent penetration-test certificate.

## High-impact findings fixed

1. **A2A protocol drift** — legacy pre-v1 method names were rejected and v1 `SendMessage`/`GetTask`/`ListTasks`/`CancelTask` semantics were implemented.
2. **Cancellation race** — a canceled in-flight task can no longer later overwrite state as completed.
3. **Approval replay scope** — approvals now bind exact canonical arguments, not only a tool name.
4. **Crash-ambiguous side effects** — durable execution state prevents blind replay; ambiguous `executing` actions require reconciliation.
5. **Queue recovery amplification** — removed unbounded per-task enqueue goroutines; queue capacity applies real backpressure.
6. **DNS/SSRF TOCTOU** — DNS answers are validated and the connection dials the approved IP directly; link-local/metadata addresses remain denied even in private-network mode.
7. **Shell path masquerading** — allowlisted shell tools must be bare command names; path aliases cannot impersonate an allowlisted basename.
8. **Audit integrity** — added SHA-256 forward hash chain, startup + periodic verification, durable sync and fail-closed tool execution on audit failure.
9. **Agent Card Host injection** — public card no longer reflects request Host; remote deployments require explicit trusted base URL.
10. **Credential domain separation** — Admin/MCP/A2A use independent generated 256-bit tokens.
11. **Memory secret retention** — long-term raw prompts default off; stored records are bounded/expirable and redact common secret/token/private-key patterns.
12. **Cross-platform persistence** — atomic replacement semantics hardened for Unix/Windows stores.
13. **Remote CLI token leakage** — remote CLI requires HTTPS, disables redirects and caps responses.
14. **Update authenticity parity** — unattended update signature verification is fail-closed by default on Linux/macOS/Windows unless explicitly weakened.
15. **Supply-chain drift** — GitHub Actions pinned to full commit SHAs; govulncheck, CodeQL, SBOM, provenance and Sigstore release evidence added.
16. **Runtime integrity readiness** — `/readyz` now incorporates audit-subsystem health.
17. **Creation/audit split-brain** — if creation audit persistence fails, the durable task is failed and will not later execute during recovery.
18. **Entropy failure crash** — secure ID generation now returns errors instead of panicking the whole runtime when the OS CSPRNG is unavailable.
19. **Updater downgrade/readiness regression** — unattended updates reject non-increasing versions by default and preserve readiness continuity with automatic rollback.
20. **Updater credential exposure** — Linux/macOS update jobs use a dedicated non-secret update environment; Windows stores the model key in an ACL-protected file injected only into the LocalService runtime.
21. **Installer release-channel bug** — fixed immutable tag download URL construction and validates repository/channel syntax before network use.
22. **Native platform CI gap** — macOS and Windows native compile/test jobs plus PowerShell parser validation and a Docker build gate were added.
23. **Interpreter escape-by-configuration** — the shipped shell allowlist is now empty; enabling shell requires explicitly choosing executables instead of inheriting general-purpose interpreters.
24. **Audit dependency fail-open** — tool execution now fails closed if no audit log is attached; the constructor no longer silently permits an unaudited registry.
25. **Weak/reused protocol secrets** — readiness and authentication require at least 32-character bearer secrets, and readiness rejects reuse across Admin/MCP/A2A trust domains.

## Defense-in-depth retained

- default-deny tool policy,
- shell disabled by default,
- bounded workers/queue/steps/task duration/body/output,
- provider retries + fallback + circuit breaker,
- strict file roots and atomic writes,
- HTTPS-only generic HTTP tool,
- remote endpoint allow-insecure/private flags explicit,
- model/memory/remote tool output treated as untrusted,
- non-root Linux/macOS/Windows runtime identities,
- container capability drop/read-only root,
- signed health-checked updater with rollback,
- controlled evolution remains proposal/review/release, never unreviewed core self-modification.

## Residual risks

See `RESIDUAL-RISKS.md`. The most important next enterprise layers are a real local plugin sandbox (WASI/VM/container), OIDC/RBAC/multi-tenancy, KMS-backed credentials, distributed durable store/leases, OpenTelemetry/SIEM, stronger hybrid memory, full optional MCP/A2A features and independent external security testing.

## Production toolchain gate

Production release artifacts are blocked unless `go env GOVERSION` is Go 1.26.5 or newer. Local Go 1.23.2 artifacts were used only for compatibility/smoke validation and are excluded from the commercial handoff. The repository Release workflow provisions Go 1.26.5 and requires vulnerability/race gates before packaging.

