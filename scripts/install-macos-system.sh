#!/usr/bin/env bash
set -euo pipefail

[[ "$(uname -s)" == "Darwin" ]] || { echo "This installer is for macOS." >&2; exit 2; }
[[ ${EUID:-$(id -u)} -eq 0 ]] || { echo "Run with sudo so KINGAIBOT can be installed as a system LaunchDaemon." >&2; exit 1; }

REPO="${KINGAGENT_REPO:-${1:-kingaiwork/KINGAIBOT}}"
CHANNEL="${KINGAGENT_CHANNEL:-latest}"
[[ "$REPO" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || { echo "Invalid GitHub repository identifier." >&2; exit 2; }
SERVICE_USER="${KINGAGENT_SERVICE_USER:-${SUDO_USER:-}}"
[[ -n "$SERVICE_USER" && "$SERVICE_USER" != "root" ]] || { echo "A non-root service user is required. Run via sudo from the intended macOS account or set KINGAGENT_SERVICE_USER." >&2; exit 2; }
id "$SERVICE_USER" >/dev/null 2>&1 || { echo "Service user does not exist: $SERVICE_USER" >&2; exit 2; }
SERVICE_GROUP="$(id -gn "$SERVICE_USER")"

case "$(uname -m)" in x86_64|amd64) ARCH=amd64;; arm64|aarch64) ARCH=arm64;; *) echo "Unsupported architecture" >&2; exit 2;; esac
ASSET="kingaibot_darwin_${ARCH}.tar.gz"
if [[ "$CHANNEL" == latest ]]; then BASE="https://github.com/${REPO}/releases/latest/download"; else BASE="https://github.com/${REPO}/releases/download/${CHANNEL}"; fi
REPO_REGEX="${REPO//./\\.}"
WORKFLOW_IDENTITY_REGEX="^https://github\.com/${REPO_REGEX}/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9._-]+)?$"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
sha256_file() { shasum -a 256 "$1" | awk '{print $1}'; }
gen_token() { openssl rand -hex 32; }
ensure_env_token() { local file="$1" name="$2"; grep -q "^${name}=" "$file" || printf '%s=%s\n' "$name" "$(gen_token)" >> "$file"; }

curl -fsSL "$BASE/$ASSET" -o "$TMP/$ASSET"
curl -fsSL "$BASE/SHA256SUMS" -o "$TMP/SHA256SUMS"
expected="$(awk -v a="$ASSET" '$2==a || $2=="*"a {print $1; exit}' "$TMP/SHA256SUMS")"
actual="$(sha256_file "$TMP/$ASSET")"
[[ -n "$expected" && "$actual" == "$expected" ]] || { echo "SHA256 verification failed" >&2; exit 3; }
if command -v cosign >/dev/null 2>&1; then
  curl -fsSL "$BASE/$ASSET.sigstore.json" -o "$TMP/$ASSET.sigstore.json"
  cosign verify-blob --bundle "$TMP/$ASSET.sigstore.json" --certificate-oidc-issuer "https://token.actions.githubusercontent.com" --certificate-identity-regexp "$WORKFLOW_IDENTITY_REGEX" "$TMP/$ASSET" >/dev/null
elif [[ "${KINGAGENT_REQUIRE_SIGNATURE:-0}" == 1 ]]; then
  echo "cosign is required because KINGAGENT_REQUIRE_SIGNATURE=1" >&2; exit 3
else
  echo "WARNING: cosign not found; install is checksum-verified only." >&2
fi

tar -xzf "$TMP/$ASSET" -C "$TMP"
for f in kingagentd kingagent kingworker kingconsole kingdesktop config.example.json update.sh; do [[ -f "$TMP/$f" ]] || { echo "Release missing $f" >&2; exit 4; }; done

ROOT="/Library/Application Support/KINGAgent"
BIN="$ROOT/bin"
DATA="$ROOT/data"
WORKSPACE="$DATA/workspace"
CONFIG="$ROOT/config.json"
ENVFILE="$ROOT/kingagent.env"
PLIST="/Library/LaunchDaemons/com.kingai.agentos.plist"
UPDATE_PLIST="/Library/LaunchDaemons/com.kingai.agentos.update.plist"
APP="/Applications/KING AI Control Center.app"

launchctl bootout system/com.kingai.agentos >/dev/null 2>&1 || true
launchctl bootout system/com.kingai.agentos.update >/dev/null 2>&1 || true
install -d -m 0755 "$ROOT" "$BIN"
install -d -m 0750 -o "$SERVICE_USER" -g "$SERVICE_GROUP" "$DATA" "$WORKSPACE"
for bin in kingagentd kingagent kingworker kingconsole kingdesktop; do install -m 0755 "$TMP/$bin" "$BIN/$bin"; done
install -m 0755 "$TMP/update.sh" "$ROOT/update.sh"
[[ -f "$CONFIG" ]] || cp "$TMP/config.example.json" "$CONFIG"
python3 - "$CONFIG" "$DATA" "$WORKSPACE" <<'PY'
import json, sys
p, data, workspace = sys.argv[1:]
with open(p, encoding='utf-8') as f: c=json.load(f)
c['runtime']['data_dir']=data
c['runtime']['workspace_dir']=workspace
c['security']['file_read_roots']=[workspace]
c['security']['file_write_roots']=[workspace]
with open(p,'w',encoding='utf-8') as f: json.dump(c,f,ensure_ascii=False,indent=2); f.write('\n')
PY
chown root:"$SERVICE_GROUP" "$CONFIG"; chmod 0640 "$CONFIG"

if [[ ! -f "$ENVFILE" ]]; then
  cat > "$ENVFILE" <<ENV
KINGAGENT_ADMIN_TOKEN=$(gen_token)
KINGAGENT_MCP_TOKEN=$(gen_token)
KINGAGENT_A2A_TOKEN=$(gen_token)
OPENAI_API_KEY=
ANTHROPIC_API_KEY=
GEMINI_API_KEY=
OPENROUTER_API_KEY=
GROQ_API_KEY=
KINGAGENT_REPO=$REPO
ENV
else
  ensure_env_token "$ENVFILE" KINGAGENT_ADMIN_TOKEN
  ensure_env_token "$ENVFILE" KINGAGENT_MCP_TOKEN
  ensure_env_token "$ENVFILE" KINGAGENT_A2A_TOKEN
  grep -q '^KINGAGENT_REPO=' "$ENVFILE" || printf 'KINGAGENT_REPO=%s\n' "$REPO" >> "$ENVFILE"
fi
chown root:"$SERVICE_GROUP" "$ENVFILE"; chmod 0640 "$ENVFILE"

cat > "$ROOT/run.sh" <<EOF_RUN
#!/usr/bin/env bash
set -a
source "$ENVFILE"
set +a
exec "$BIN/kingagentd" -config "$CONFIG"
EOF_RUN
chmod 0755 "$ROOT/run.sh"
cat > "$ROOT/update.env" <<EOF_UPDATE_ENV
KINGAGENT_REPO=$REPO
KINGAGENT_INSTALL_ROOT=$ROOT
KINGAGENT_LAUNCHD_DOMAIN=system/com.kingai.agentos
EOF_UPDATE_ENV
chmod 0600 "$ROOT/update.env"
cat > "$ROOT/update-run.sh" <<EOF_UPDATE_RUN
#!/usr/bin/env bash
set -a
source "$ROOT/update.env"
set +a
exec "$ROOT/update.sh"
EOF_UPDATE_RUN
chmod 0755 "$ROOT/update-run.sh"

cat > "$PLIST" <<EOF_PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>com.kingai.agentos</string>
  <key>ProgramArguments</key><array><string>$ROOT/run.sh</string></array>
  <key>UserName</key><string>$SERVICE_USER</string>
  <key>GroupName</key><string>$SERVICE_GROUP</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
  <key>WorkingDirectory</key><string>$ROOT</string>
  <key>StandardOutPath</key><string>$DATA/agent.log</string>
  <key>StandardErrorPath</key><string>$DATA/agent.err.log</string>
  <key>ProcessType</key><string>Background</string>
  <key>ThrottleInterval</key><integer>10</integer>
</dict></plist>
EOF_PLIST
chmod 0644 "$PLIST"

cat > "$UPDATE_PLIST" <<EOF_UPDATE_PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>com.kingai.agentos.update</string>
  <key>ProgramArguments</key><array><string>$ROOT/update-run.sh</string></array>
  <key>RunAtLoad</key><false/>
  <key>StartInterval</key><integer>21600</integer>
  <key>WorkingDirectory</key><string>$ROOT</string>
  <key>StandardOutPath</key><string>$DATA/update.log</string>
  <key>StandardErrorPath</key><string>$DATA/update.err.log</string>
</dict></plist>
EOF_UPDATE_PLIST
chmod 0644 "$UPDATE_PLIST"

mkdir -p "$APP/Contents/MacOS"
cat > "$APP/Contents/MacOS/kingai-control-center" <<EOF_APP
#!/usr/bin/env bash
exec "$BIN/kingdesktop"
EOF_APP
chmod 0755 "$APP/Contents/MacOS/kingai-control-center"
cat > "$APP/Contents/Info.plist" <<'EOF_INFO'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>KING AI Control Center</string>
  <key>CFBundleDisplayName</key><string>KING AI Control Center</string>
  <key>CFBundleIdentifier</key><string>work.kingai.controlcenter</string>
  <key>CFBundleVersion</key><string>1.6</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleExecutable</key><string>kingai-control-center</string>
</dict></plist>
EOF_INFO

launchctl bootstrap system "$PLIST"
launchctl bootstrap system "$UPDATE_PLIST"

echo "KINGAIBOT installed as macOS system LaunchDaemon."
echo "Runtime service user: $SERVICE_USER (not root)."
echo "Configuration: $CONFIG"
echo "Model API keys: $ENVFILE"
echo "Visual client: $APP"
echo "Cognitive status after configuring an admin token in your shell: $BIN/kingagent cognition"
