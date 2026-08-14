#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-${GITHUB_REF_NAME:-1.3.0}}"
VERSION="${VERSION#v}"
MIN_GO_VERSION="${KINGAGENT_MIN_RELEASE_GO:-1.26.6}"
GO_VERSION="$(go env GOVERSION)"
python3 - "$GO_VERSION" "$MIN_GO_VERSION" <<'PYGO'
import re, sys
def parse(v: str):
    v = v.removeprefix("go")
    m = re.fullmatch(r"(\d+)\.(\d+)(?:\.(\d+))?(?:[a-z].*)?", v)
    if not m: raise SystemExit(f"unsupported Go version string: {v!r}")
    return tuple(int(x or 0) for x in m.groups())
current = parse(sys.argv[1]); minimum = parse(sys.argv[2])
if current < minimum:
    raise SystemExit(f"production release blocked: Go {sys.argv[2]}+ is required; current toolchain is {sys.argv[1]}")
PYGO
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST="$ROOT/dist"
if [[ -z "${SOURCE_DATE_EPOCH:-}" ]]; then SOURCE_DATE_EPOCH="$(git -C "$ROOT" log -1 --format=%ct 2>/dev/null || true)"; fi
SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-315532800}"
[[ "$SOURCE_DATE_EPOCH" =~ ^[0-9]+$ ]] || { echo "SOURCE_DATE_EPOCH must be an integer epoch" >&2; exit 2; }
export VERSION SOURCE_DATE_EPOCH
rm -rf "$DIST"; mkdir -p "$DIST"
normalize_mtime() {
  local dir="$1"
  python3 - "$dir" "$SOURCE_DATE_EPOCH" <<'PY'
import os, sys
root, epoch = sys.argv[1], int(sys.argv[2])
for base, dirs, files in os.walk(root):
    for name in dirs + files:
        p = os.path.join(base, name)
        try: os.utime(p, (epoch, epoch), follow_symlinks=False)
        except (FileNotFoundError, PermissionError): pass
try: os.utime(root, (epoch, epoch), follow_symlinks=False)
except (FileNotFoundError, PermissionError): pass
PY
}
build_one() {
  local goos="$1" goarch="$2" ext="" archive=""
  [[ "$goos" == windows ]] && ext=".exe"
  local dir="$DIST/stage_${goos}_${goarch}"; mkdir -p "$dir"
  echo "Building $goos/$goarch"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -buildvcs=false -ldflags="-s -w -X main.version=$VERSION" -o "$dir/kingagentd$ext" "$ROOT/cmd/kingagentd"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -buildvcs=false -ldflags='-s -w' -o "$dir/kingagent$ext" "$ROOT/cmd/kingagent"
  cp "$ROOT/configs/config.example.json" "$dir/config.example.json"
  cp "$ROOT/README.md" "$dir/README.md"
  cp "$ROOT/LICENSE-COMMERCIAL.txt" "$dir/LICENSE-COMMERCIAL.txt"
  cp "$ROOT/scripts/update.sh" "$dir/update.sh"
  cp "$ROOT/scripts/update.ps1" "$dir/update.ps1"
  chmod 0755 "$dir/update.sh"; normalize_mtime "$dir"
  if [[ "$goos" == windows ]]; then
    archive="$DIST/kingaibot_${goos}_${goarch}.zip"
    (cd "$dir" && find . -type f -print | LC_ALL=C sort | zip -q -X "$archive" -@)
  else
    archive="$DIST/kingaibot_${goos}_${goarch}.tar.gz"
    tar --sort=name --mtime="@$SOURCE_DATE_EPOCH" --owner=0 --group=0 --numeric-owner -C "$dir" -cf - . | gzip -n > "$archive"
  fi
  rm -rf "$dir"
}
pids=()
for target in "linux amd64" "linux arm64" "darwin amd64" "darwin arm64" "windows amd64" "windows arm64"; do build_one $target & pids+=("$!"); done
for pid in "${pids[@]}"; do wait "$pid"; done
"$ROOT/scripts/generate-sbom.py" "$DIST/sbom.cdx.json"
(cd "$DIST" && sha256sum kingaibot_* sbom.cdx.json > SHA256SUMS)
echo "Release artifacts written to $DIST"
