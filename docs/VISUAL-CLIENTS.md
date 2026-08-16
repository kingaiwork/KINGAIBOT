# KING AI Visual Clients

KINGAIBOT ships one governed runtime with a cross-platform visual client instead of separate agent implementations per operating system.

## Components

- `kingagentd` — governed runtime daemon.
- `kingagent` — operator CLI.
- `kingworker` — capability-scoped remote worker for server, edge and device nodes.
- `kingconsole` — local-only HTTP Control Center on `127.0.0.1:18889`.
- `kingdesktop` — Windows/macOS/Linux visual launcher. It starts `kingconsole` on demand and opens the Control Center in the system browser.

The visual client never exposes the runtime directly to the public network. Local HTTP is limited to loopback; remote runtime/API access continues to require HTTPS.

## Desktop installation

### Linux x86-64 / ARM64

```bash
curl -fsSL https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/install.sh | sudo KINGAGENT_REQUIRE_SIGNATURE=1 bash -s -- kingaiwork/KINGAIBOT
```

The installer adds `KING AI Control Center` to desktop application menus when `/usr/share/applications` is available. It can also be launched with:

```bash
kingdesktop
```

### macOS Intel / Apple Silicon

```bash
curl -fsSL https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/install.sh | KINGAGENT_REQUIRE_SIGNATURE=1 bash -s -- kingaiwork/KINGAIBOT
```

The installer creates `~/Applications/KING AI Control Center.app` and installs the signed-release binaries under `~/.local/kingagent/bin`.

### Windows x64 / ARM64

Run PowerShell as Administrator:

```powershell
$env:KINGAGENT_REPO='kingaiwork/KINGAIBOT'; $env:KINGAGENT_REQUIRE_SIGNATURE='1'; irm https://raw.githubusercontent.com/kingaiwork/KINGAIBOT/main/scripts/install.ps1 | iex
```

The installer adds `KING AI Control Center` shortcuts to the Desktop and Start Menu.

## Server / Edge / IoT

Server, edge and IoT deployments use the same runtime binaries. Headless systems do not need a desktop shell: operators can run `kingconsole` on loopback and access it through an approved secure administration path, or use `kingagent` and the KING AI cloud control plane.

For constrained devices, deploy `kingworker` rather than the full coordinator whenever the device only needs bounded capabilities. This keeps authority, approvals, budgets and reconciliation centralized while device execution stays narrow.

## Security model

The visual layer does not gain authority merely because it is local. Requests still pass through KINGAIBOT identity, capability, policy, approval, audit and reconciliation controls. Tokens are not persisted by the browser UI. High-risk actions remain approval-gated.

## Release contract

GitHub Release assets use stable canonical names so one-command installers and unattended updates resolve the same artifacts:

- `kingaibot_linux_amd64.tar.gz`
- `kingaibot_linux_arm64.tar.gz`
- `kingaibot_darwin_amd64.tar.gz`
- `kingaibot_darwin_arm64.tar.gz`
- `kingaibot_windows_amd64.zip`
- `kingaibot_windows_arm64.zip`

Every archive includes the visual launcher and console. Release publication continues to produce SHA-256 checksums, CycloneDX SBOM, provenance attestations and Sigstore bundles.
