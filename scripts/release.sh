#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ $# -gt 1 ]]; then
  echo 'Usage: scripts/release.sh [VERSION]' >&2
  exit 2
fi
if [[ $# -eq 1 ]]; then export VERSION="$1"; fi
exec python3 scripts/package.py release
