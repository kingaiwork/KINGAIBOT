# Operations

## Release procedure

1. Merge only after CI and CodeQL are clean.
2. Update changelog/version references.
3. Tag `vX.Y.Z`.
4. The release workflow runs format, vet, govulncheck and race tests, then cross-compiles six targets.
5. It creates deterministic-oriented archives, CycloneDX SBOM and SHA-256 manifest.
6. Public repositories receive provenance and SBOM attestations.
7. GitHub OIDC + Sigstore/Cosign signs each archive, SBOM and manifest.
8. GitHub Release publishes all assets/bundles.
9. Clients verify identity/checksum, install, restart, health-check and roll back if unhealthy.

Third-party GitHub Actions are pinned to full commit SHAs, not floating major-version tags.

## Incident switch-off

Fast containment mechanisms:
- set a risky tool to `deny`,
- remove/disable a remote MCP/A2A endpoint,
- bind the service to loopback,
- rotate **Admin, MCP and A2A tokens independently**,
- rotate provider credentials,
- stop the service,
- use host firewall/egress policy to cut network access.

Do not rely on prompt instructions as an incident-control boundary.

## Audit-integrity incident

If `/readyz` reports `runtime_integrity` or tool calls report audit failure:

1. stop automation that can create side effects,
2. preserve/copy the audit file and host logs for investigation,
3. identify disk corruption, unauthorized modification or write/sync failure,
4. restore from a trusted consistent backup or reconcile the log under an approved incident process,
5. verify integrity before re-enabling tool execution.

Do not simply delete the audit history to make the service ready.

## Approval reconciliation

An approval left in `executing` after a crash is intentionally not automatically replayed. Determine from the external system whether the side effect occurred. Reconcile manually before creating/retrying a new action.
