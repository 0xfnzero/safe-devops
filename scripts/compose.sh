#!/usr/bin/env bash
set -euo pipefail
if [[ $# -lt 2 ]]; then echo 'Usage: scripts/compose.sh CONFIG_DIRECTORY <Docker Compose arguments>'; exit 2; fi
config_dir=$(cd "$1" && pwd); shift
root=$(cd "$(dirname "$0")/.." && pwd)
[[ -f "$config_dir/.env" && -f "$config_dir/.database" ]] || { echo 'Run safe-devops init-compose first.' >&2; exit 1; }
command -v docker >/dev/null || { echo 'Docker CLI not found. Install Docker Engine and Compose v2.' >&2; exit 1; }
docker compose version >/dev/null || { echo 'Docker Compose v2 is required.' >&2; exit 1; }
# A copied deployment must mount this directory, never the old absolute path.
export SAFEOPS_CONFIG_DIR="$config_dir"
files=(-f "$root/deploy/compose.yaml")
case "$(cat "$config_dir/.database")" in
  sqlite) ;; postgres) files+=(-f "$root/deploy/compose.postgres.yaml");;
  *) echo 'Unknown database selection.' >&2; exit 1;;
esac
exec docker compose --env-file "$config_dir/.env" "${files[@]}" "$@"
