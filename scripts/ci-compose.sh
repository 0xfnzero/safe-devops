#!/usr/bin/env bash
# Disposable CI-only identities. Never use these generated keys in production.
set -euo pipefail
cd "$(dirname "$0")/.."
driver=${1:-sqlite}
case "$driver" in sqlite|postgres) ;; *) exit 2;; esac
make build
config_dir="$PWD/.safeops/ci-$driver"
mkdir -p .safeops
public=$(./bin/safe-devops keygen --out ".safeops/ci-$driver-human.key")
./bin/safe-devops keygen --out ".safeops/ci-$driver-device.key" >/dev/null
device_public=$(./bin/safe-devops device-public-key --key ".safeops/ci-$driver-device.key")
./bin/safe-devops init-compose --dir "$config_dir" --workspace "ci-$driver" --project core \
  --broker-url https://127.0.0.1:18443 --web-url https://127.0.0.1:14443 \
  --approver-public-key "$public" --device-public-key "$device_public" --database "$driver"
# Use the Compose DNS name from the connector container; the generated server
# certificate includes this SAN. Human signatures still originate outside it.
python3 - "$config_dir" <<'PY'
import json,sys
from pathlib import Path
p=Path(sys.argv[1])/"connector/connector.json"
c=json.loads(p.read_text());c["broker_url"]="https://broker:8443";p.write_text(json.dumps(c))
PY
files=(-f deploy/compose.yaml)
[[ "$driver" != postgres ]] || files+=(-f deploy/compose.postgres.yaml)
files+=(-f deploy/compose.connector.yaml)
compose=(docker compose --env-file "$config_dir/.env" "${files[@]}")
cleanup(){
  status=$?
  if (( status != 0 )); then "${compose[@]}" logs --no-color --tail 100; fi
  "${compose[@]}" down -v --remove-orphans
  return "$status"
}
trap cleanup EXIT
"${compose[@]}" config --quiet
"${compose[@]}" up -d --build --wait --wait-timeout 240
python3 scripts/verify-deployment.py "$config_dir" ".safeops/ci-$driver-human.key" ".safeops/ci-$driver-device.key"
make deploy-check CONFIG_DIR="$config_dir"
make deploy-health CONFIG_DIR="$config_dir"
make deploy-backup CONFIG_DIR="$config_dir" WORKSPACE="ci-$driver" BACKUP_FILE=verified.json
"${compose[@]}" restart broker
"${compose[@]}" up -d --wait --wait-timeout 120
make deploy-health CONFIG_DIR="$config_dir"
