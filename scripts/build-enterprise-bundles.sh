#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-1.3.0}"
MIN_GO_VERSION="${KINGAGENT_MIN_RELEASE_GO:-1.26.6}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${KINGAI_ENTERPRISE_DIST:-$ROOT/dist/enterprise}"
GO_VERSION="$(go env GOVERSION)"

python3 - "$GO_VERSION" "$MIN_GO_VERSION" <<'PY'
import re,sys
def p(v):
    v=v.removeprefix('go')
    m=re.fullmatch(r'(\d+)\.(\d+)(?:\.(\d+))?(?:[a-z].*)?',v)
    if not m: raise SystemExit(f'unsupported Go version: {v}')
    return tuple(int(x or 0) for x in m.groups())
if p(sys.argv[1]) < p(sys.argv[2]):
    raise SystemExit(f'Go {sys.argv[2]}+ required; current {sys.argv[1]}')
PY

command -v zip >/dev/null || { echo "zip is required" >&2; exit 2; }
rm -rf "$OUT"
mkdir -p "$OUT"

build_bundle() {
  local goos="$1" goarch="$2" platform="$3" ext=""
  [[ "$goos" == "windows" ]] && ext=".exe"
  local stage="$OUT/.stage-$platform"
  mkdir -p "$stage"
  echo "Building enterprise bundle $platform"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -buildvcs=false -ldflags="-s -w -X main.version=$VERSION" -o "$stage/kingagentd$ext" "$ROOT/cmd/kingagentd"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -buildvcs=false -ldflags='-s -w' -o "$stage/kingagent$ext" "$ROOT/cmd/kingagent"
  cp "$ROOT/configs/config.example.json" "$stage/config.example.json"
  cp "$ROOT/deploy/systemd/kingagent.env.example" "$stage/kingagent.env.example"
  cp "$ROOT/deploy/enterprise/README.md" "$stage/README-ENTERPRISE.md"
  cp "$ROOT/LICENSE-COMMERCIAL.txt" "$stage/LICENSE-COMMERCIAL.txt"
  cp "$ROOT/scripts/update.sh" "$stage/update.sh"
  cp "$ROOT/scripts/update.ps1" "$stage/update.ps1"
  chmod 0755 "$stage/update.sh" 2>/dev/null || true
  (
    cd "$stage"
    find . -type f -print | LC_ALL=C sort | zip -q -X "$OUT/kingai-enterprise-$platform.zip" -@
  )
  rm -rf "$stage"
}

build_bundle linux amd64 linux-amd64
build_bundle linux arm64 linux-arm64
build_bundle windows amd64 windows-amd64
build_bundle darwin arm64 darwin-arm64
build_bundle darwin amd64 darwin-amd64

(
  cd "$OUT"
  sha256sum kingai-enterprise-*.zip > SHA256SUMS
)
echo "Enterprise bundles ready in $OUT"
