#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-${GITHUB_REF_NAME:-1.4.0}}"
VERSION="${VERSION#v}"
MIN_GO_VERSION="${KINGAGENT_MIN_RELEASE_GO:-1.26.6}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST="${DIST:-$ROOT/dist-v14}"
GO_VERSION="$(go env GOVERSION)"

python3 - "$GO_VERSION" "$MIN_GO_VERSION" <<'PY'
import re, sys

def parse(v: str):
    v = v.removeprefix("go")
    m = re.fullmatch(r"(\d+)\.(\d+)(?:\.(\d+))?(?:[a-z].*)?", v)
    if not m:
        raise SystemExit(f"unsupported Go version: {v}")
    return tuple(int(x or 0) for x in m.groups())

if parse(sys.argv[1]) < parse(sys.argv[2]):
    raise SystemExit(f"Go {sys.argv[2]}+ required; current {sys.argv[1]}")
PY

if [[ -z "${SOURCE_DATE_EPOCH:-}" ]]; then
  SOURCE_DATE_EPOCH="$(git -C "$ROOT" log -1 --format=%ct 2>/dev/null || true)"
fi
SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-315532800}"
[[ "$SOURCE_DATE_EPOCH" =~ ^[0-9]+$ ]] || { echo "SOURCE_DATE_EPOCH must be integer" >&2; exit 2; }

rm -rf "$DIST"
mkdir -p "$DIST"

normalize_mtime() {
  python3 - "$1" "$SOURCE_DATE_EPOCH" <<'PY'
import os, sys
root, epoch = sys.argv[1], int(sys.argv[2])
for base, dirs, files in os.walk(root):
    for name in dirs + files:
        path = os.path.join(base, name)
        try:
            os.utime(path, (epoch, epoch), follow_symlinks=False)
        except (FileNotFoundError, PermissionError):
            pass
try:
    os.utime(root, (epoch, epoch), follow_symlinks=False)
except (FileNotFoundError, PermissionError):
    pass
PY
}

build_target() {
  local goos="$1"
  local goarch="$2"
  local ext=""
  local stage="$DIST/.stage_${goos}_${goarch}"
  [[ "$goos" == "windows" ]] && ext=".exe"
  mkdir -p "$stage"

  for cmd in kingagentd kingagent kingworker kingconsole; do
    local ldflags="-s -w"
    [[ "$cmd" != "kingagent" ]] && ldflags="-s -w -X main.version=$VERSION"
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
      go build -trimpath -buildvcs=false -ldflags="$ldflags" \
      -o "$stage/${cmd}${ext}" "$ROOT/cmd/$cmd"
  done

  cp "$ROOT/configs/config.example.json" "$stage/config.example.json"
  cp "$ROOT/README.md" "$stage/README.md"
  cp "$ROOT/LICENSE-COMMERCIAL.txt" "$stage/LICENSE-COMMERCIAL.txt"
  cp "$ROOT/scripts/update.sh" "$stage/update.sh"
  cp "$ROOT/scripts/update.ps1" "$stage/update.ps1"
  [[ -f "$ROOT/docs/PLATFORM.md" ]] && cp "$ROOT/docs/PLATFORM.md" "$stage/PLATFORM.md"
  chmod 0755 "$stage/update.sh"
  normalize_mtime "$stage"

  if [[ "$goos" == "windows" ]]; then
    local archive="$DIST/kingaibot_${VERSION}_${goos}_${goarch}.zip"
    (cd "$stage" && find . -type f -print | LC_ALL=C sort | zip -q -X "$archive" -@)
  else
    local archive="$DIST/kingaibot_${VERSION}_${goos}_${goarch}.tar.gz"
    tar --sort=name --mtime="@$SOURCE_DATE_EPOCH" --owner=0 --group=0 --numeric-owner \
      -C "$stage" -cf - . | gzip -n > "$archive"
  fi
  rm -rf "$stage"
}

pids=()
for target in \
  "linux amd64" \
  "linux arm64" \
  "darwin amd64" \
  "darwin arm64" \
  "windows amd64" \
  "windows arm64"; do
  build_target $target &
  pids+=("$!")
done
for pid in "${pids[@]}"; do wait "$pid"; done

"$ROOT/scripts/generate-sbom.py" "$DIST/sbom.cdx.json"
(
  cd "$DIST"
  sha256sum kingaibot_* sbom.cdx.json > SHA256SUMS
)

cat > "$DIST/RELEASE-MANIFEST.json" <<EOF
{
  "product": "KINGAIBOT",
  "version": "$VERSION",
  "go": "$GO_VERSION",
  "binaries": ["kingagentd", "kingagent", "kingworker", "kingconsole"],
  "targets": ["linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64", "windows/amd64", "windows/arm64"]
}
EOF
normalize_mtime "$DIST"

echo "Full KINGAIBOT v1.4 release written to $DIST"
