# V1.1.0 Validation Record

Date: 2026-08-12  
Version: 1.1.0

This document records checks actually performed on the hardened source tree. It is not a third-party certification, formal penetration test, or a claim of zero defects.

## Local verification environment

The available local container provides Go 1.23.2. That toolchain was sufficient for source-level compatibility, race/regression tests and cross-compilation experiments, but it is **not accepted for production release artifacts**. `scripts/build-release.sh` now refuses production release builds with Go older than 1.26.5. GitHub CI and Release workflows explicitly provision Go 1.26.5.

## Checks completed locally

- `gofmt` clean.
- `go vet ./...` passed.
- `go test -race -count=1 ./...` passed, including a final rerun after entropy/auth/audit fail-closed changes.
- `go test -count=2 ./...` passed.
- Linux shell scripts pass `bash -n`; invalid repository/channel inputs are rejected before network access.
- Example JSON configuration parses successfully.
- GitHub workflow/dependabot YAML parses successfully.
- CycloneDX SBOM generator produces parseable JSON.
- Full Action `uses:` references in CI/CodeQL/Release are pinned to 40-character commit SHAs.
- GitHub CI includes native macOS compile/test, native Windows compile/test, PowerShell AST parser validation and a Docker build gate. These hosted-platform jobs are configured but cannot execute locally before the repository exists.

## Security regression coverage

Automated regression tests cover, among other cases:

- allow / ask / deny policy behavior,
- exact approval binding to task + tool + canonical argument hash,
- prevention of approval replay for different arguments,
- durable approval execution state / stale-execution reconciliation,
- terminal task cancellation not being overwritten by late completion,
- bounded queue/recovery behavior,
- lexical path traversal rejection,
- symlink escape rejection,
- shell executable-path masquerading rejection,
- HTTP private/link-local/unspecified destination policy,
- guarded DNS resolution and destination pinning,
- audit hash-chain startup verification,
- live audit tamper detection and fail-closed behavior,
- memory secret redaction,
- MCP discovery and current method validation,
- A2A 1.0 method validation,
- readiness failure when required credentials/integrity components are missing.

## End-to-end runtime smoke validation

A local mock OpenAI-compatible provider was used to validate the actual Linux amd64 runtime path:

1. start the mock provider and wait for readiness,
2. start `kingagentd`,
3. confirm `/readyz`,
4. create a durable task using the CLI/API,
5. complete the task through the provider (`SMOKE_OK`),
6. restart the runtime,
7. retrieve the same completed task after restart,
8. call MCP `server/discover`,
9. submit an A2A 1.0 `SendMessage` request,
10. verify default memory stores useful output but not the raw user prompt,
11. verify audit records contain chained hashes.

An initial smoke attempt exposed a race in the **test harness** because the mock provider had not finished listening. The harness was corrected to probe provider readiness before creating a task, and the complete smoke path then passed.

## Cross-platform compatibility validation

Using the locally available Go 1.23.2 toolchain strictly for compatibility testing, native-format binaries were successfully generated for:

- Linux amd64 (native runtime smoke-tested),
- Linux arm64 (ELF AArch64),
- macOS amd64 (Mach-O x86_64),
- macOS arm64 (Mach-O arm64),
- Windows amd64 (PE32+ x86-64),
- Windows arm64 (PE32+ ARM64).

These locally generated binaries are **test artifacts only** and are intentionally excluded from the commercial handoff package.

## Reproducible packaging check

Before the production-toolchain gate was enabled, the deterministic packaging path was run twice with the same `SOURCE_DATE_EPOCH`. All six platform archive hashes and the SBOM hash matched between both runs. The release script normalizes timestamps, sorting, ownership metadata and gzip/zip metadata where supported.

Production artifacts must be rebuilt by the GitHub Release workflow with Go 1.26.5+, then pass checksum, Sigstore and GitHub provenance verification.

## Vulnerability scanning status

A local `govulncheck` execution could not be completed because the execution environment could not resolve/download from the Go module proxy. This is recorded as an environment limitation, **not** as a clean vulnerability result. Both CI and Release workflows install and run `govulncheck@v1.6.0` as a required security gate before building/publishing.

CodeQL is configured as a separate GitHub workflow and also requires the repository/Actions environment to execute.

## Production release gate

A production release is acceptable only after the repository workflow completes all of the following:

1. Go 1.26.5 toolchain provisioned.
2. formatting check.
3. `go vet`.
4. `govulncheck`.
5. race-detector test suite.
6. CodeQL workflow.
7. deterministic six-platform build.
8. SHA-256 manifest.
9. CycloneDX SBOM.
10. GitHub provenance/SBOM attestation where supported.
11. Sigstore/Cosign signing.
12. published assets verified before installation/update.

## Remaining boundaries

See `docs/RESIDUAL-RISKS.md`. In particular, v1.1.0 does not claim every optional MCP/A2A transport/extension, formal compliance certification, an external independent penetration test, enterprise KMS/HSM integration, distributed consensus/HA, or an in-process WASM/WASI third-party plugin sandbox.
