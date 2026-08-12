# KINGAIBOT v1.1.0 Commercial Hardened Handoff

This handoff contains the complete hardened source, deployment assets, CI/security gates, installers, update/rollback logic and documentation.

## Important production-release rule

The local execution environment available during this audit contains Go 1.23.2. Local binaries built with that toolchain were used only for compatibility, cross-platform format and smoke testing and are deliberately excluded from this package.

`scripts/build-release.sh` fails closed when the release Go toolchain is older than Go 1.26.5. The GitHub CI/Release workflows provision Go 1.26.5, run formatting/vet/govulncheck/race gates, build six platform archives, create a CycloneDX SBOM and SHA-256 manifest, produce GitHub attestations where supported, sign release assets with Sigstore/Cosign, and then publish the release.

Do not represent a release as production-validated until the hosted GitHub gates complete successfully.

See `docs/VALIDATION.md`, `docs/AUDIT-REPORT-v1.1.0.md`, `docs/SECURITY.md`, and `docs/RESIDUAL-RISKS.md`.
