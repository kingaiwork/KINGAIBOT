# Changelog

## 1.1.0 - 2026-08-12

Security/reliability hardening release over the initial v1.0 commercial baseline:

- Correct A2A v1 method/state/role profile and MCP 2026-07-28 result/header behavior.
- Exact argument-bound approvals with durable at-most-once execution state and crash reconciliation.
- Fixed in-flight cancellation overwrite and queue-recovery goroutine amplification.
- Added DNS-to-dial pinning, permanent link-local denial, HTTPS-only generic HTTP and stricter redirect/size controls.
- Fixed shell allowlist path masquerading; bare command names only.
- Added SHA-256 hash-chained audit events, periodic verification, readiness integration and fail-closed side effects.
- Split Admin/MCP/A2A credentials and hardened Agent Card/base URL handling.
- Added bounded/expiring secret-redacted memory with raw-input learning disabled by default.
- Hardened atomic storage replacement across Unix/Windows.
- Hardened remote CLI HTTPS/redirect/response behavior.
- Hardened update verification parity across Linux/macOS/Windows.
- Pinned GitHub Actions to immutable commit SHAs and added govulncheck, CycloneDX SBOM, provenance/SBOM attestations and Sigstore release signing.
- Added explicit residual-risk and engineering-audit documentation.

## 1.0.0 - 2026-08-12

- Initial cross-platform commercial baseline.
