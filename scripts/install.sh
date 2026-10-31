#!/usr/bin/env bash
# Installs locally built/reviewed artifacts. Does not fetch software or alter
# firewalls, SSH keys, certificates, or production authorization policy.
set -euo pipefail
component='' binary='' config='' environment='' web_bundle='' checksum=''
dry_run=false start=false upgrade=false
usage() { cat <<'EOF'
Usage: install.sh --component broker|connector|reviewer|change-runner|web [options]
  --binary PATH --sha256 HASH   Required for Go components; verifies artifact
  --config PATH                Component JSON (Go components)
  --env PATH                   systemd environment file (broker/reviewer/web)
  --web-bundle DIRECTORY       Next.js standalone release directory (web)
  --upgrade                    Back up and replace existing component config
  --start                      Enable and start/restart after staging
  --dry-run                    Validate inputs and print plan without writes
Run as root on a Linux systemd host. Install TLS/approval keys independently.
EOF
}
while (($#)); do
  case "$1" in
    --component|--binary|--config|--env|--web-bundle|--sha256)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      case "$1" in
        --component) component=$2;; --binary) binary=$2;; --config) config=$2;;
        --env) environment=$2;; --web-bundle) web_bundle=$2;; --sha256) checksum=$2;;
      esac; shift 2;;
    --dry-run) dry_run=true; shift;; --start) start=true; shift;; --upgrade) upgrade=true; shift;;
    --help) usage; exit 0;; *) usage; exit 2;;
  esac
done
[[ "$component" != agent ]] || component=connector
case "$component" in broker|connector|reviewer|change-runner|web) ;; *) usage; exit 2;; esac
if [[ "$component" == web ]]; then
  [[ -f "$web_bundle/server.js" && -d "$web_bundle/.next/static" && -f "$environment" ]] || { echo 'Standalone web bundle and environment file required.' >&2; exit 2; }
else
  [[ -f "$binary" && -f "$config" && "$checksum" =~ ^[[:xdigit:]]{64}$ ]] || { echo 'Binary, SHA-256 and JSON config required.' >&2; exit 2; }
  if command -v sha256sum >/dev/null; then actual=$(sha256sum -- "$binary"); else actual=$(shasum -a 256 -- "$binary"); fi
  actual=${actual%% *}
  checksum=$(printf '%s' "$checksum" | tr 'A-F' 'a-f')
  [[ "$actual" == "$checksum" ]] || { echo 'Binary checksum mismatch.' >&2; exit 1; }
  if [[ "$component" == broker || "$component" == reviewer ]]; then
    [[ -f "$environment" ]] || { echo 'Component environment file required (may be empty for SQLite Broker).' >&2; exit 2; }
  fi
fi
script_root=$(cd "$(dirname "$0")/.." && pwd)
[[ -f "$script_root/deploy/safe-devops-$component.service" ]] || { echo 'Service template missing.' >&2; exit 1; }
echo "Component: $component; account: safe-devops-$component; configuration: /etc/safe-devops; start: $start"
if [[ "$component" == web ]]; then
  echo "Web bundle: $web_bundle -> /opt/safe-devops/releases/web-TIMESTAMP"
else
  echo "Verified binary: $binary -> /opt/safe-devops/releases/$checksum/safe-devops"
  echo "Configuration: $config -> /etc/safe-devops/$component.json"
fi
[[ -z "$environment" ]] || echo "Environment file: $environment -> /etc/safe-devops/$component.env"
echo "Service: safe-devops-$component.service; preserve existing config backups: $upgrade"
if $dry_run; then echo 'Dry run complete. No files, accounts or services changed; target-host permissions and runtime TLS checks still apply.'; exit 0; fi
[[ $(uname -s) == Linux && $EUID == 0 ]] || { echo 'Installation requires root on Linux.' >&2; exit 1; }
command -v systemctl >/dev/null
if [[ "$component" == web ]]; then
  command -v node >/dev/null
  node -e 'const [a,b]=process.versions.node.split(".").map(Number);if(a<22||(a===22&&b<18)){console.error("Node >=22.18 required");process.exit(1)}'
  [[ ! -e /opt/safe-devops/web || -L /opt/safe-devops/web ]] || { echo '/opt/safe-devops/web must be a release symlink, not an existing directory.' >&2; exit 1; }
fi
if ! $upgrade && { [[ -e "/etc/safe-devops/$component.json" ]] || [[ -e "/etc/safe-devops/$component.env" ]]; }; then
  echo 'Existing configuration found. Use --upgrade to preserve a backup before replacement.' >&2; exit 1
fi
account="safe-devops-$component"
if [[ "$component" == change-runner ]]; then
  getent group safe-devops-connector >/dev/null || groupadd --system safe-devops-connector
  account=root
fi
id "$account" >/dev/null 2>&1 || useradd --system --user-group --no-create-home --shell /usr/sbin/nologin "$account"
install -d -m 0755 /etc/safe-devops /opt/safe-devops /opt/safe-devops/releases /usr/local/bin
stamp=$(date -u +%Y%m%dT%H%M%SZ)
for file in "/etc/safe-devops/$component.json" "/etc/safe-devops/$component.env"; do
  if [[ -f "$file" ]]; then cp -p -- "$file" "$file.previous.$stamp"; fi
done
if [[ "$component" == web ]]; then
  destination="/opt/safe-devops/releases/web-$stamp"
  [[ ! -e "$destination" ]] || { echo 'Release path already exists.' >&2; exit 1; }
  cp -a -- "$web_bundle" "$destination"
  chown -R root:root "$destination"
  chmod -R go-w "$destination"
  ln -sfn -- "$destination" /opt/safe-devops/web
  node_binary=$(command -v node)
  sed "s|/usr/bin/node|$node_binary|" "$script_root/deploy/safe-devops-web.service" > /etc/systemd/system/safe-devops-web.service
else
  release_dir="/opt/safe-devops/releases/$checksum"
  install -d -m 0755 "$release_dir"
  install -m 0755 "$binary" "$release_dir/safe-devops"
  ln -sfn -- "$release_dir/safe-devops" /usr/local/bin/safe-devops
  config_mode=0640
  if [[ "$component" == change-runner ]]; then config_mode=0600; fi
  install -o root -g "$account" -m "$config_mode" "$config" "/etc/safe-devops/$component.json"
  install -m 0644 "$script_root/deploy/safe-devops-$component.service" "/etc/systemd/system/safe-devops-$component.service"
fi
if [[ -n "$environment" ]]; then install -o root -g root -m 0600 "$environment" "/etc/safe-devops/$component.env"; fi
systemctl daemon-reload
if $start; then
  systemctl enable "safe-devops-$component.service"
  systemctl restart "safe-devops-$component.service"
  systemctl --no-pager --full status "safe-devops-$component.service"
else
  echo "Staged. Install protected keys/certificates, validate configuration, then: systemctl enable --now safe-devops-$component"
fi
