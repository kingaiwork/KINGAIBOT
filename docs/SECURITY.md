# Security Model

## Trust boundaries

Untrusted by default:
- user/model text,
- retrieved memory,
- remote HTTP content,
- MCP tool output,
- A2A peer output,
- file contents read from the workspace.

Trusted only by explicit operator configuration:
- executable binary and release identity,
- local config,
- environment/secret injection,
- tool policy,
- configured filesystem roots,
- configured remote MCP/A2A endpoints.

The model is not part of the security kernel.

## Tool policy and approvals

Every tool resolves to `allow`, `ask` or `deny`. Unknown tools inherit `default_tool_policy`, which should remain `deny` in production.

`ask` creates a durable approval bound to **task ID + tool name + canonical argument hash**. The approval cannot authorize a different argument set. Before an approved side effect starts, the runtime durably marks the approval execution state. A completed action returns the recorded result instead of running again. A crash-ambiguous `executing` state is not automatically retried; an operator must reconcile it.

## Audit integrity

Events are fsynced JSONL records linked by a SHA-256 forward hash chain. The chain is verified at startup and periodically while the runtime is live. A verification/write/sync failure marks the audit subsystem unhealthy. Side-effecting tool execution checks this state and fails closed until an operator repairs the log and a verification succeeds.

Audit tool records contain the tool name, canonical argument hash, approval ID and outcome state rather than raw arguments/results, reducing accidental secret persistence.

## Shell hardening

`shell_exec`:
- is denied in the example configuration,
- accepts only **bare executable names** from an explicit allowlist; paths such as `/tmp/git` are rejected even if `git` is allowlisted,
- uses `exec.CommandContext` directly,
- accepts argv rather than a shell command string,
- does not invoke `sh`, `bash`, `cmd.exe` or PowerShell implicitly,
- runs in the configured workspace,
- receives a minimal child environment without model API keys or API bearer tokens,
- enforces timeout and output limits.

This is process-level hardening, not a kernel sandbox. If shell is enabled for hostile workloads, add a VM/container/WASM sandbox appropriate to the threat model.

## Filesystem hardening

Reads and writes are restricted to configured roots. Existing paths are canonicalized through symlink resolution. New write paths resolve the nearest existing ancestor, reconstruct the target and validate the canonical root again before atomic replacement. Traversal and common symlink-escape paths are rejected.

A hostile local process that can concurrently mutate the same writable directory remains outside this application-level guarantee; use OS ownership/isolation for stronger race resistance.

## Network hardening

Generic `http_get` is HTTPS-only and accepts only configured hosts. Redirect destinations are revalidated. DNS answers are checked before connection and the dial is pinned to an allowed resolved IP, avoiding a second unguarded DNS resolution. Private and loopback networks are blocked by default. Even when private networking is explicitly enabled for a configured integration, link-local, multicast and unspecified addresses remain denied.

Response size and redirect depth are bounded, URL credentials are rejected, and the transport does not inherit environment proxy settings.

For production, also apply OS/network egress policy. Application-level SSRF controls are defense in depth, not a substitute for network segmentation.

## API hardening

- separate Admin/MCP/A2A bearer tokens,
- constant-time token comparison,
- body size limits and strict JSON decoding,
- security response headers,
- CORS allowlist,
- per-IP request limiting,
- server read/write/header timeouts,
- remote CLI URLs require HTTPS; loopback may use HTTP,
- CLI redirects are disabled so an Admin bearer is not forwarded to a different host.

Put internet-facing deployments behind TLS plus an identity-aware gateway/reverse proxy. Bind directly to loopback unless remote access is explicitly needed.

## Memory and secret handling

Long-term memory can be disabled. Raw task inputs are **not** stored into long-term memory by default. Stored learning records are bounded, can expire, are compacted, and redact common API keys, GitHub tokens, JWT/Bearer values, passwords/secrets and private-key blocks. Retrieved memory is injected into the model as explicitly untrusted data rather than as system authority.

Durable task snapshots still contain the mission text required to resume a task. Protect the data directory with OS permissions and disk/volume encryption when mission text is sensitive.

## Updates and supply chain

Unattended Linux/macOS/Windows updates require Cosign identity verification by default. A checksum-only update requires an explicit policy override. Initial installation always verifies SHA-256 and can be forced to require Sigstore with `KINGAGENT_REQUIRE_SIGNATURE=1`.

The GitHub workflows pin third-party Actions to full commit SHAs, run formatting/vet/race tests and govulncheck, run CodeQL, produce a CycloneDX SBOM, generate provenance/SBOM attestations for public repositories, and sign release assets with GitHub OIDC + Sigstore/Cosign.

### Shell escape boundary

`shell_exec` is denied by default and the shipped `shell_allowlist` is empty. Do not pre-populate it with interpreters such as Python/Node or general-purpose shells unless you intentionally accept that an approved interpreter invocation can perform broad side effects outside higher-level file/HTTP tool constraints. Prefer narrowly scoped wrapper binaries or MCP tools with explicit schemas.
