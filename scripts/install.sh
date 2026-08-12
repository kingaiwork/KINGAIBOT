#!/usr/bin/env bash
set -euo pipefail

REPO="${KINGAGENT_REPO:-${1:-kingaiwork/KINGAIBOT}}"
CHANNEL="${KINGAGENT_CHANNEL:-latest}"
[[ "$REPO" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || { echo "Invalid GitHub repository identifier." >&2; exit 2; }
REPO_OWNER="${REPO%%/*}"; REPO_NAME="${REPO#*/}"
[[ "$REPO_OWNER" != "." && "$REPO_OWNER" != ".." && "$REPO_NAME" != "." && "$REPO_NAME" != ".." ]] || { echo "Invalid GitHub repository path segment." >&2; exit 2; }
if [[ "$CHANNEL" == "latest" ]]; then
  BASE="https://github.com/${REPO}/releases/latest/download"
elif [[ "$CHANNEL" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9._-]+)?$ ]]; then
  BASE="https://github.com/${REPO}/releases/download/${CHANNEL}"
else
  echo "KINGAGENT_CHANNEL must be 'latest' or a vMAJOR.MINOR.PATCH release tag." >&2; exit 2
fi
REPO_REGEX="${REPO//./\\.}"
WORKFLOW_IDENTITY_REGEX="^https://github\.com/${REPO_REGEX}/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9._-]+)?$"
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"; ARCH_RAW="$(uname -m)"
case "$OS" in linux|darwin) ;; *) echo "Unsupported OS: $OS" >&2; exit 1;; esac
case "$ARCH_RAW" in x86_64|amd64) ARCH=amd64;; aarch64|arm64) ARCH=arm64;; *) echo "Unsupported architecture: $ARCH_RAW" >&2; exit 1;; esac
ASSET="king-agent-os_${OS}_${ARCH}.tar.gz"; TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
sha256_file() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; elif command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | awk '{print $1}'; else echo "No SHA-256 utility found." >&2; return 1; fi; }
verify_checksum() { local sums="$1" asset="$2" file="$3" expected actual; expected="$(awk -v a="$asset" '$2==a || $2=="*"a {print $1; exit}' "$sums")"; [[ -n "$expected" ]] || { echo "Checksum entry not found for $asset" >&2; return 1; }; actual="$(sha256_file "$file")"; [[ "$(printf %s "$actual" | tr '[:upper:]' '[:lower:]')" == "$(printf %s "$expected" | tr '[:upper:]' '[:lower:]')" ]] || { echo "SHA256 verification failed for $asset" >&2; return 1; }; echo "SHA256 verified."; }
gen_token() { if command -v openssl >/dev/null 2>&1; then openssl rand -hex 32; else head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'; fi; }
ensure_env_token() { local file="$1" name="$2"; if ! grep -q "^${name}=" "$file"; then printf '%s=%s\n' "$name" "$(gen_token)" >> "$file"; fi; }
echo "Downloading KINGAIBOT from $REPO..."
curl --fail --silent --show-error --location "$BASE/$ASSET" -o "$TMP/$ASSET"
curl --fail --silent --show-error --location "$BASE/SHA256SUMS" -o "$TMP/SHA256SUMS"
verify_checksum "$TMP/SHA256SUMS" "$ASSET" "$TMP/$ASSET"
if command -v cosign >/dev/null 2>&1; then
  curl --fail --silent --show-error --location "$BASE/$ASSET.sigstore.json" -o "$TMP/$ASSET.sigstore.json"
  cosign verify-blob --bundle "$TMP/$ASSET.sigstore.json" --certificate-oidc-issuer "https://token.actions.githubusercontent.com" --certificate-identity-regexp "$WORKFLOW_IDENTITY_REGEX" "$TMP/$ASSET" >/dev/null
  echo "Sigstore identity verified."
elif [[ "${KINGAGENT_REQUIRE_SIGNATURE:-0}" == "1" ]]; then echo "KINGAGENT_REQUIRE_SIGNATURE=1 but cosign is not installed." >&2; exit 3
else echo "WARNING: cosign not found; initial install is checksum-verified only. Set KINGAGENT_REQUIRE_SIGNATURE=1 to fail closed." >&2; fi
tar -xzf "$TMP/$ASSET" -C "$TMP"
if [[ "$OS" == linux ]]; then
  if [[ ${EUID:-$(id -u)} -ne 0 ]]; then echo "Linux service installation requires root. Run through sudo." >&2; exit 1; fi
  id kingagent >/dev/null 2>&1 || useradd --system --home /var/lib/kingagent --shell /usr/sbin/nologin kingagent
  install -m 0755 "$TMP/kingagentd" /usr/local/bin/kingagentd; install -m 0755 "$TMP/kingagent" /usr/local/bin/kingagent
  install -d -m 0750 -o kingagent -g kingagent /var/lib/kingagent /var/lib/kingagent/workspace; install -d -m 0750 /etc/kingagent /usr/local/lib/kingagent
  if [[ ! -f /etc/kingagent/config.json ]]; then cp "$TMP/config.example.json" /etc/kingagent/config.json; sed -i -e 's#\./data/workspace#/var/lib/kingagent/workspace#g' -e 's#\./data#/var/lib/kingagent#g' /etc/kingagent/config.json; chmod 0640 /etc/kingagent/config.json; fi
  if [[ ! -f /etc/kingagent/kingagent.env ]]; then
    cat > /etc/kingagent/kingagent.env <<ENV
KINGAGENT_ADMIN_TOKEN=$(gen_token)
KINGAGENT_MCP_TOKEN=$(gen_token)
KINGAGENT_A2A_TOKEN=$(gen_token)
OPENAI_API_KEY=
KINGAGENT_REPO=$REPO
ENV
  else
    ensure_env_token /etc/kingagent/kingagent.env KINGAGENT_ADMIN_TOKEN; ensure_env_token /etc/kingagent/kingagent.env KINGAGENT_MCP_TOKEN; ensure_env_token /etc/kingagent/kingagent.env KINGAGENT_A2A_TOKEN
    grep -q '^KINGAGENT_REPO=' /etc/kingagent/kingagent.env || printf '\nKINGAGENT_REPO=%s\n' "$REPO" >> /etc/kingagent/kingagent.env
  fi
  chmod 0600 /etc/kingagent/kingagent.env
  printf 'KINGAGENT_REPO=%s\n' "$REPO" > /etc/kingagent/update.env; chmod 0600 /etc/kingagent/update.env
  install -m 0755 "$TMP/update.sh" /usr/local/lib/kingagent/update.sh
  cp "$(dirname "$0")/../deploy/systemd/kingagent.service" /etc/systemd/system/kingagent.service 2>/dev/null || true
  cp "$(dirname "$0")/../deploy/systemd/kingagent-update.service" /etc/systemd/system/kingagent-update.service 2>/dev/null || true
  cp "$(dirname "$0")/../deploy/systemd/kingagent-update.timer" /etc/systemd/system/kingagent-update.timer 2>/dev/null || true
  if [[ ! -f /etc/systemd/system/kingagent.service ]]; then
    cat > /etc/systemd/system/kingagent.service <<'UNIT'
[Unit]
Description=KINGAIBOT Runtime
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
User=kingagent
Group=kingagent
EnvironmentFile=/etc/kingagent/kingagent.env
ExecStart=/usr/local/bin/kingagentd -config /etc/kingagent/config.json
WorkingDirectory=/var/lib/kingagent
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/kingagent
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictRealtime=true
LockPersonality=true
MemoryDenyWriteExecute=true
CapabilityBoundingSet=
AmbientCapabilities=
UMask=0077
[Install]
WantedBy=multi-user.target
UNIT
  fi
  if [[ ! -f /etc/systemd/system/kingagent-update.service ]]; then
    cat > /etc/systemd/system/kingagent-update.service <<'UNIT'
[Unit]
Description=KINGAIBOT signed update check
After=network-online.target
[Service]
Type=oneshot
EnvironmentFile=/etc/kingagent/update.env
ExecStart=/usr/local/lib/kingagent/update.sh
UNIT
  fi
  if [[ ! -f /etc/systemd/system/kingagent-update.timer ]]; then
    cat > /etc/systemd/system/kingagent-update.timer <<'UNIT'
[Unit]
Description=Check KINGAIBOT updates
[Timer]
OnBootSec=15min
OnUnitActiveSec=6h
RandomizedDelaySec=30min
Persistent=true
[Install]
WantedBy=timers.target
UNIT
  fi
  systemctl daemon-reload; systemctl enable --now kingagent.service; systemctl enable --now kingagent-update.timer
  echo "Installed. Configure your model API key in /etc/kingagent/kingagent.env and restart: sudo systemctl restart kingagent"
  echo "Health: curl http://127.0.0.1:18888/healthz"
else
  PREFIX="${HOME}/.local/kingagent"; PLIST="${HOME}/Library/LaunchAgents/com.kingai.agentos.plist"
  mkdir -p "$PREFIX/bin" "$PREFIX/data/workspace" "$HOME/Library/LaunchAgents"
  install -m 0755 "$TMP/kingagentd" "$PREFIX/bin/kingagentd"; install -m 0755 "$TMP/kingagent" "$PREFIX/bin/kingagent"; install -m 0755 "$TMP/update.sh" "$PREFIX/update.sh"
  [[ -f "$PREFIX/config.json" ]] || cp "$TMP/config.example.json" "$PREFIX/config.json"
  if [[ ! -f "$PREFIX/kingagent.env" ]]; then
    cat > "$PREFIX/kingagent.env" <<ENV
KINGAGENT_ADMIN_TOKEN=$(gen_token)
KINGAGENT_MCP_TOKEN=$(gen_token)
KINGAGENT_A2A_TOKEN=$(gen_token)
OPENAI_API_KEY=
KINGAGENT_REPO=$REPO
ENV
  else
    ensure_env_token "$PREFIX/kingagent.env" KINGAGENT_ADMIN_TOKEN; ensure_env_token "$PREFIX/kingagent.env" KINGAGENT_MCP_TOKEN; ensure_env_token "$PREFIX/kingagent.env" KINGAGENT_A2A_TOKEN
    grep -q '^KINGAGENT_REPO=' "$PREFIX/kingagent.env" || printf '\nKINGAGENT_REPO=%s\n' "$REPO" >> "$PREFIX/kingagent.env"
  fi
  chmod 0600 "$PREFIX/kingagent.env"
  cat > "$PREFIX/run.sh" <<EOF_RUN
#!/usr/bin/env bash
set -a
source "$PREFIX/kingagent.env"
set +a
exec "$PREFIX/bin/kingagentd" -config "$PREFIX/config.json"
EOF_RUN
  chmod 0755 "$PREFIX/run.sh"; printf 'KINGAGENT_REPO=%s\n' "$REPO" > "$PREFIX/update.env"; chmod 0600 "$PREFIX/update.env"
  cat > "$PREFIX/update-run.sh" <<EOF_UPDATE
#!/usr/bin/env bash
set -a
source "$PREFIX/update.env"
set +a
exec "$PREFIX/update.sh"
EOF_UPDATE
  chmod 0755 "$PREFIX/update-run.sh"
  cat > "$PLIST" <<EOF_PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>com.kingai.agentos</string>
  <key>ProgramArguments</key><array><string>$PREFIX/run.sh</string></array>
  <key>RunAtLoad</key><true/><key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
  <key>WorkingDirectory</key><string>$PREFIX</string><key>StandardOutPath</key><string>$PREFIX/agent.log</string><key>StandardErrorPath</key><string>$PREFIX/agent.err.log</string>
  <key>ProcessType</key><string>Background</string><key>ThrottleInterval</key><integer>10</integer>
</dict></plist>
EOF_PLIST
  UPDATE_PLIST="${HOME}/Library/LaunchAgents/com.kingai.agentos.update.plist"
  cat > "$UPDATE_PLIST" <<EOF_UPLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>com.kingai.agentos.update</string><key>ProgramArguments</key><array><string>$PREFIX/update-run.sh</string></array>
  <key>RunAtLoad</key><false/><key>StartInterval</key><integer>21600</integer><key>WorkingDirectory</key><string>$PREFIX</string>
  <key>StandardOutPath</key><string>$PREFIX/update.log</string><key>StandardErrorPath</key><string>$PREFIX/update.err.log</string>
</dict></plist>
EOF_UPLIST
  launchctl bootout "gui/$(id -u)/com.kingai.agentos" >/dev/null 2>&1 || true; launchctl bootout "gui/$(id -u)/com.kingai.agentos.update" >/dev/null 2>&1 || true
  launchctl bootstrap "gui/$(id -u)" "$PLIST"; launchctl bootstrap "gui/$(id -u)" "$UPDATE_PLIST"
  echo "Installed to $PREFIX and launchd service enabled."; echo "Set OPENAI_API_KEY in $PREFIX/kingagent.env, then run: launchctl kickstart -k gui/$(id -u)/com.kingai.agentos"
fi
