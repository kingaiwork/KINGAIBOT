# Residual Risks and Non-Claims

V1.1.0 is a hardened commercial single-node baseline. It is **not** a claim of zero vulnerabilities, formal verification, regulatory certification or universal containment against a hostile local administrator/kernel.

Known boundaries:

- Local `shell_exec`, when enabled, is process-level allowlisting rather than a VM/kernel/WASM sandbox.
- Filesystem root/symlink checks assume the service owns or controls its writable roots; a hostile concurrent local process with write access to the same directory can create race conditions beyond portable path validation.
- Secrets use OS environment/files. Enterprise KMS/HSM/secret-broker integration is not yet native.
- Authentication is single-deployment opaque bearer tokens. Native OIDC/SAML/SCIM, RBAC/ABAC, tenant isolation and per-tool OAuth delegation are not yet included.
- The durable store is single-node local storage. It is not distributed HA/consensus storage.
- Memory retrieval is bounded lexical relevance, not vector/graph hybrid retrieval with provenance ranking.
- MCP support is a focused 2026-07-28 tools profile, not every extension.
- A2A support implements the declared v1 JSON-RPC operations but not streaming/push/extended-card optional capabilities.
- MCP input-required output exposes approval requirements, but external protocol clients do not gain authority to self-approve local side effects.
- OpenTelemetry exporters, external SIEM integration and enterprise metrics/SLO dashboards are not yet built into the core.
- Release provenance/Sigstore evidence is produced by GitHub only after the repository/tag workflow exists; locally produced handoff archives cannot carry GitHub OIDC provenance.
- Docker build stages use explicit version/distribution tags, but the base-image references are not yet pinned to immutable multi-architecture digests; production container publishing should lock verified digests and refresh them through reviewed dependency updates.
- No independent external penetration-test or formal certification has been performed on this handoff build.

Production deployment should combine the runtime controls with OS least privilege, disk encryption, network egress controls, reverse-proxy TLS/identity, backups, centralized monitoring and an environment-specific security review.
