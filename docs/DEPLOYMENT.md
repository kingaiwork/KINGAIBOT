# Deployment

## Linux production

Recommended topology:

```text
Internet
  -> TLS reverse proxy / WAF / identity gateway
  -> 127.0.0.1:18888 KINGAIBOT
  -> explicit model / MCP / A2A egress only
```

The supplied systemd unit runs under an unprivileged `kingagent` account, drops capabilities, makes the OS filesystem read-only to the service except its data path, and enables additional systemd hardening. Keep the API on loopback unless a trusted gateway is in front of it.

Configure runtime credentials in `/etc/kingagent/kingagent.env`; the installer generates independent Admin/MCP/A2A tokens. The signed updater uses a separate root-only `/etc/kingagent/update.env` and therefore does not inherit the runtime tokens or model API key. Add the model key, then restart:

```bash
sudoedit /etc/kingagent/kingagent.env
sudo systemctl restart kingagent
curl -fsS http://127.0.0.1:18888/readyz
```

Logs:

```bash
journalctl -u kingagent -f
```

## Docker

```bash
cd deploy/docker
export KINGAGENT_ADMIN_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_MCP_TOKEN="$(openssl rand -hex 32)"
export KINGAGENT_A2A_TOKEN="$(openssl rand -hex 32)"
export OPENAI_API_KEY="..."
docker compose up -d --build
```

The image uses a distroless non-root runtime. Compose drops capabilities, enables `no-new-privileges`, uses a read-only root filesystem, gives only `/data` persistent write access and publishes the service to loopback by default.

## macOS

The installer places files under `~/.local/kingagent`, creates a per-user launchd agent and a six-hour updater. It generates three independent bearer tokens. Runtime secrets remain in `kingagent.env`; the updater sources a separate `update.env` containing only update configuration. Unattended updates require Cosign unless checksum-only verification is explicitly opted into.

Set the model key in `~/.local/kingagent/kingagent.env`, then:

```bash
launchctl kickstart -k gui/$UID/com.kingai.agentos
```

## Windows

Run `scripts/install.ps1` as Administrator. It installs under `%ProgramData%\KINGAgent`, generates separate Admin/MCP/A2A tokens, and runs the runtime as **NT AUTHORITY\LOCAL SERVICE**, not Administrator/SYSTEM. The model API key is stored in the ACL-protected `%ProgramData%\KINGAgent\model-api-key.txt` instead of a machine-wide environment variable; the LocalService launch wrapper injects it only into the runtime process. Program/config/token paths are read/execute-only to the runtime identity; the dedicated data path grants modify rights. A separate SYSTEM maintenance task performs signed updates every six hours without reading the runtime credential files.

Unattended updates require Cosign by default. Set `KINGAGENT_ALLOW_CHECKSUM_ONLY=1` only when accepting the weaker checksum-only policy. Initial install can be forced to fail without signature verification by setting `KINGAGENT_REQUIRE_SIGNATURE=1`. Automatic updates reject downgrades by default and preserve readiness continuity: if the previous runtime was ready, the new runtime must be ready after restart or the updater rolls back.

## Backups

Back up at minimum:
- configuration (secrets separately if externally managed),
- `data/tasks`,
- `data/approvals`,
- `data/memory`,
- `data/events`,
- `data/evolution`,
- workspace state required by workloads.

Use volume snapshots or stop/quiesce the single-node service for a consistent local-file backup. Encrypt backups that contain mission/task data.
