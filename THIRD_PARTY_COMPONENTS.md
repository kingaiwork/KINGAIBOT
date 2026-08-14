# KINGAIBOT Third-Party Component Inventory

This file is the human-reviewed inventory required by `docs/ORIGINALITY_IP_POLICY.md`.

Every production third-party component must remain clearly identified as third-party. It must never be represented as KINGAIBOT-original code.

## Go modules

KINGAIBOT currently has no external Go module dependencies in `go.mod`; the production Go code uses the Go standard library.

When a module is added, record:

| Module path | Version | SPDX license | Purpose | Source reviewed | Required notices |
|---|---:|---|---|---|---|
| _none currently_ | — | — | — | — | — |

## GitHub Actions used by CI

CI uses GitHub-maintained actions and pins them to immutable commit SHAs in workflow files. These actions are build infrastructure, not KINGAIBOT product source code.

Current examples include:
- `actions/checkout`
- `actions/setup-go`

Any new action should be pinned to an immutable commit SHA and reviewed before use.

## External standards and protocols

Implementation of an open protocol does not make its reference implementation part of KINGAIBOT. Protocol specifications may be used as interoperability requirements while KINGAIBOT keeps an independent internal architecture.

Current compatibility boundaries include:
- MCP
- A2A
- HTTP/JSON
- provider API formats

## Review rule

A dependency may not be merged into production until its source, version, license/SPDX identifier, purpose and required notices are recorded here or in an equivalent generated SBOM plus reviewed provenance record.
