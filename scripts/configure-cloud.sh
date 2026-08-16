#!/usr/bin/env bash
set -euo pipefail

upsert_env() {
  local file="$1" name="$2" value="$3" tmp found=0 line
  tmp="$(mktemp)"
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" == "$name="* ]]; then
      printf '%s=%s\n' "$name" "$value" >> "$tmp"
      found=1
    else
      printf '%s\n' "$line" >> "$tmp"
    fi
  done < "$file"
  if [[ "$found" == 0 ]]; then printf '%s=%s\n' "$name" "$value" >> "$tmp"; fi
  cat "$tmp" > "$file"
  rm -f "$tmp"
}

generate_sync_key() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 32 | tr -d '\n'
  else
    python3 - <<'PY'
import base64, os
print(base64.b64encode(os.urandom(32)).decode(), end='')
PY
  fi
}

OS="$(uname -s)"
ENVFILE=""
STATEFILE=""
RESTART_KIND=""

if [[ "$OS" == "Linux" && -f /etc/kingagent/kingagent.env ]]; then
  [[ ${EUID:-$(id -u)} -eq 0 ]] || { echo "Linux system service configuration requires sudo/root." >&2; exit 1; }
  ENVFILE=/etc/kingagent/kingagent.env
  STATEFILE=/var/lib/kingagent/cloud/state.json
  RESTART_KIND=systemd
elif [[ "$OS" == "Darwin" && -f "/Library/Application Support/KINGAgent/kingagent.env" ]]; then
  [[ ${EUID:-$(id -u)} -eq 0 ]] || { echo "macOS system LaunchDaemon configuration requires sudo/root." >&2; exit 1; }
  ENVFILE="/Library/Application Support/KINGAgent/kingagent.env"
  STATEFILE="/Library/Application Support/KINGAgent/data/cloud/state.json"
  RESTART_KIND=launchd-system
elif [[ "$OS" == "Darwin" && -f "${HOME}/.local/kingagent/kingagent.env" ]]; then
  ENVFILE="${HOME}/.local/kingagent/kingagent.env"
  STATEFILE="${HOME}/.local/kingagent/data/cloud/state.json"
  RESTART_KIND=launchd-user
else
  echo "KINGAIBOT installation not found. Install the Runtime before cloud enrollment." >&2
  exit 2
fi

TOKEN="${KINGAI_ENROLLMENT_TOKEN:-}"
if [[ -z "$TOKEN" ]]; then
  printf 'KING AI one-time enrollment token: ' >/dev/tty
  IFS= read -r -s TOKEN </dev/tty
  printf '\n' >/dev/tty
fi
[[ "$TOKEN" == kop_enroll_* ]] || { echo "Enrollment token must use the kop_enroll_ prefix." >&2; exit 2; }

BASE_URL="${KINGAI_CLOUD_BASE_URL:-https://api.kingai.work}"
[[ "$BASE_URL" == https://* ]] || { echo "KINGAI_CLOUD_BASE_URL must use HTTPS." >&2; exit 2; }
MEMORY_SYNC="${KINGAI_MEMORY_SYNC:-0}"
SYNC_KEY="${KINGAI_SYNC_KEY:-}"
if [[ "$MEMORY_SYNC" == 1 && -z "$SYNC_KEY" ]]; then SYNC_KEY="$(generate_sync_key)"; fi

upsert_env "$ENVFILE" KINGAI_CLOUD_ENABLED 1
upsert_env "$ENVFILE" KINGAI_CLOUD_BASE_URL "$BASE_URL"
upsert_env "$ENVFILE" KINGAI_ENROLLMENT_TOKEN "$TOKEN"
upsert_env "$ENVFILE" KINGAI_CLOUD_ENVIRONMENT "${KINGAI_CLOUD_ENVIRONMENT:-production}"
upsert_env "$ENVFILE" KINGAI_NODE_CLASS "${KINGAI_NODE_CLASS:-server}"
upsert_env "$ENVFILE" KINGAI_NODE_PROVIDER "${KINGAI_NODE_PROVIDER:-}"
upsert_env "$ENVFILE" KINGAI_NODE_REGION "${KINGAI_NODE_REGION:-}"
upsert_env "$ENVFILE" KINGAI_CLOUD_HEARTBEAT_SECONDS "${KINGAI_CLOUD_HEARTBEAT_SECONDS:-60}"
upsert_env "$ENVFILE" KINGAI_CLOUD_REQUIRE_POLICY "${KINGAI_CLOUD_REQUIRE_POLICY:-0}"
upsert_env "$ENVFILE" KINGAI_MEMORY_SYNC "$MEMORY_SYNC"
upsert_env "$ENVFILE" KINGAI_MEMORY_SYNC_SECONDS "${KINGAI_MEMORY_SYNC_SECONDS:-900}"
upsert_env "$ENVFILE" KINGAI_SYNC_KEY "$SYNC_KEY"
upsert_env "$ENVFILE" KINGAI_CLOUD_ALLOW_CUSTOM_ENDPOINT "${KINGAI_CLOUD_ALLOW_CUSTOM_ENDPOINT:-0}"

case "$RESTART_KIND" in
  systemd)
    systemctl restart kingagent.service
    ;;
  launchd-system)
    launchctl kickstart -k system/com.kingai.agentos
    ;;
  launchd-user)
    launchctl kickstart -k "gui/$(id -u)/com.kingai.agentos"
    ;;
esac

# The token is one-time. Once the durable node identity appears locally, remove
# the raw token from the service environment so later restarts never carry it.
for _ in $(seq 1 20); do
  if [[ -f "$STATEFILE" ]] && grep -Eq '"enrolled"[[:space:]]*:[[:space:]]*true' "$STATEFILE"; then
    upsert_env "$ENVFILE" KINGAI_ENROLLMENT_TOKEN ""
    echo "KING AI Cloud enrollment complete. One-time token removed from service environment."
    echo "Cloud & Fleet: http://127.0.0.1:18889/ui/cloud/"
    exit 0
  fi
  sleep 0.5
done

echo "Cloud enrollment was configured, but a durable enrolled state was not observed yet." >&2
echo "Inspect the local Cloud & Fleet page or service logs before reusing/replacing the one-time token." >&2
exit 3
