#!/usr/bin/env bash
set -euo pipefail
: "${KINGAGENT_REPO:?KINGAGENT_REPO must be set, e.g. owner/repo}"
[[ "$KINGAGENT_REPO" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || { echo "Invalid GitHub repository identifier." >&2; exit 2; }
REPO_OWNER="${KINGAGENT_REPO%%/*}"; REPO_NAME="${KINGAGENT_REPO#*/}"
[[ "$REPO_OWNER" != "." && "$REPO_OWNER" != ".." && "$REPO_NAME" != "." && "$REPO_NAME" != ".." ]] || { echo "Invalid GitHub repository path segment." >&2; exit 2; }
REPO_REGEX="${KINGAGENT_REPO//./\\.}"; WORKFLOW_IDENTITY_REGEX="^https://github\.com/${REPO_REGEX}/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9._-]+)?$"
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"; ARCH_RAW="$(uname -m)"; case "$ARCH_RAW" in x86_64|amd64) ARCH=amd64;; aarch64|arm64) ARCH=arm64;; *) exit 2;; esac
[[ "$OS" == linux || "$OS" == darwin ]] || exit 2
ASSET="kingaibot_${OS}_${ARCH}.tar.gz"; BASE="https://github.com/${KINGAGENT_REPO}/releases/latest/download"; TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
sha256_file() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; elif command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | awk '{print $1}'; else echo "No SHA-256 utility found." >&2; return 1; fi; }
verify_checksum() { local expected actual; expected="$(awk -v a="$ASSET" '$2==a || $2=="*"a {print $1; exit}' "$TMP/SHA256SUMS")"; [[ -n "$expected" ]] || { echo "Checksum entry not found" >&2; return 1; }; actual="$(sha256_file "$TMP/$ASSET")"; [[ "$(printf %s "$actual" | tr '[:upper:]' '[:lower:]')" == "$(printf %s "$expected" | tr '[:upper:]' '[:lower:]')" ]] || { echo "SHA256 verification failed" >&2; return 1; }; }
semver_gt() { local a="$1" b="$2" ama ami apa bma bmi bpa; [[ "$a" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)([-+].*)?$ ]] || return 2; ama=${BASH_REMATCH[1]}; ami=${BASH_REMATCH[2]}; apa=${BASH_REMATCH[3]}; [[ "$b" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)([-+].*)?$ ]] || return 2; bma=${BASH_REMATCH[1]}; bmi=${BASH_REMATCH[2]}; bpa=${BASH_REMATCH[3]}; (( ama > bma )) && return 0; (( ama < bma )) && return 1; (( ami > bmi )) && return 0; (( ami < bmi )) && return 1; (( apa > bpa )) && return 0; return 1; }

curl -fsSL "$BASE/$ASSET" -o "$TMP/$ASSET"
curl -fsSL "$BASE/SHA256SUMS" -o "$TMP/SHA256SUMS"
verify_checksum
if command -v cosign >/dev/null 2>&1; then
  curl -fsSL "$BASE/$ASSET.sigstore.json" -o "$TMP/$ASSET.sigstore.json"
  cosign verify-blob --bundle "$TMP/$ASSET.sigstore.json" --certificate-oidc-issuer "https://token.actions.githubusercontent.com" --certificate-identity-regexp "$WORKFLOW_IDENTITY_REGEX" "$TMP/$ASSET" >/dev/null
elif [[ "${KINGAGENT_ALLOW_CHECKSUM_ONLY:-0}" != "1" ]]; then
  echo "cosign is required for unattended updates. Set KINGAGENT_ALLOW_CHECKSUM_ONLY=1 only if you accept checksum-only verification." >&2; exit 3
fi

tar -xzf "$TMP/$ASSET" -C "$TMP"
for required in kingagentd kingagent kingworker kingconsole kingdesktop; do [[ -f "$TMP/$required" ]] || { echo "Release is missing $required" >&2; exit 4; }; done
if [[ "$OS" == linux ]]; then BIN_DIR=/usr/local/bin; RESTART_CMD=(systemctl restart kingagent); else PREFIX="${HOME}/.local/kingagent"; BIN_DIR="$PREFIX/bin"; RESTART_CMD=(launchctl kickstart -k "gui/$(id -u)/com.kingai.agentos"); fi
NEW="$($TMP/kingagentd -version)"; OLD="$($BIN_DIR/kingagentd -version 2>/dev/null || echo none)"; [[ "$NEW" != "$OLD" ]] || exit 0
if [[ "$OLD" != "none" && "${KINGAGENT_ALLOW_DOWNGRADE:-0}" != "1" ]]; then set +e; semver_gt "$NEW" "$OLD"; cmp_rc=$?; set -e; if [[ $cmp_rc -eq 2 ]]; then echo "Cannot safely compare versions ($OLD -> $NEW); refusing unattended update." >&2; exit 5; elif [[ $cmp_rc -ne 0 ]]; then echo "Refusing downgrade or non-increasing version: $OLD -> $NEW" >&2; exit 5; fi; fi
WAS_READY=0; if curl -fsS --max-time 5 http://127.0.0.1:18888/readyz >/dev/null 2>&1; then WAS_READY=1; fi

bins=(kingagentd kingagent kingworker kingconsole kingdesktop)
for bin in "${bins[@]}"; do
  install -m 0755 "$TMP/$bin" "$BIN_DIR/$bin.new"
  [[ -f "$BIN_DIR/$bin" ]] && mv "$BIN_DIR/$bin" "$BIN_DIR/$bin.prev"
  mv "$BIN_DIR/$bin.new" "$BIN_DIR/$bin"
done
"${RESTART_CMD[@]}" || true
sleep 2
POSTCHECK_OK=1
curl -fsS --max-time 5 http://127.0.0.1:18888/healthz >/dev/null || POSTCHECK_OK=0
if [[ $WAS_READY -eq 1 ]]; then curl -fsS --max-time 5 http://127.0.0.1:18888/readyz >/dev/null || POSTCHECK_OK=0; fi
if [[ $POSTCHECK_OK -ne 1 ]]; then
  for bin in "${bins[@]}"; do [[ -f "$BIN_DIR/$bin.prev" ]] && mv "$BIN_DIR/$bin.prev" "$BIN_DIR/$bin"; done
  "${RESTART_CMD[@]}" || true
  echo "Update failed post-update health/readiness continuity check; rolled back." >&2
  exit 4
fi
for bin in "${bins[@]}"; do rm -f "$BIN_DIR/$bin.prev"; done
echo "Updated KINGAIBOT runtime + visual client: $OLD -> $NEW"
