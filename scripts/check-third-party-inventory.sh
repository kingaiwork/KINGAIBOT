#!/usr/bin/env bash
set -euo pipefail

inventory="THIRD_PARTY_COMPONENTS.md"
if [[ ! -f "$inventory" ]]; then
  echo "missing $inventory" >&2
  exit 1
fi

mapfile -t modules < <(go list -m -f '{{if not .Main}}{{.Path}} {{.Version}}{{end}}' all | sed '/^[[:space:]]*$/d')

if (( ${#modules[@]} == 0 )); then
  echo "third-party inventory: no external Go modules detected"
  exit 0
fi

failed=0
for item in "${modules[@]}"; do
  path="${item%% *}"
  version="${item#* }"
  if ! grep -Fq "$path" "$inventory"; then
    echo "unregistered Go dependency: $path $version" >&2
    failed=1
  fi
done

if (( failed != 0 )); then
  echo "register every external module in $inventory with version, SPDX license, purpose and required notices before merging" >&2
  exit 1
fi

echo "third-party inventory: all external Go modules are registered"
