#!/usr/bin/env bash
set -euo pipefail
REPO="${1:?usage: ./scripts/publish-github.sh owner/repo [public|private]}"
VIS="${2:-private}"
[[ "$VIS" == public || "$VIS" == private ]] || { echo "visibility must be public or private" >&2; exit 2; }
[[ "$REPO" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || { echo "repository must be owner/name" >&2; exit 2; }
command -v gh >/dev/null || { echo "gh CLI is required" >&2; exit 1; }
gh auth status >/dev/null
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"; cd "$ROOT"
if [[ ! -d .git ]]; then git init -b main; fi
git add .; git diff --cached --check
if ! git diff --cached --quiet; then git commit -m "KINGAIBOT v1.1.0 hardened commercial release"; fi
if ! gh repo view "$REPO" >/dev/null 2>&1; then
  gh repo create "$REPO" --"$VIS" --source=. --remote=origin --push
else
  if git remote get-url origin >/dev/null 2>&1; then
    CURRENT="$(git remote get-url origin)"
    case "$CURRENT" in *"github.com/$REPO"*|*"github.com:$REPO"*) ;; *) echo "origin points to a different repository: $CURRENT" >&2; exit 3;; esac
  else git remote add origin "https://github.com/$REPO.git"; fi
  git push -u origin main
fi
echo "Published source to https://github.com/$REPO"
echo "Create signed release with: git tag v1.1.0 && git push origin v1.1.0"
