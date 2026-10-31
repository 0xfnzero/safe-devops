#!/usr/bin/env bash
set -euo pipefail
# Official image starts as root, then docker-entrypoint.sh drops to postgres.
install -d -m 0700 -o postgres -g postgres /run/postgresql/tls
install -m 0600 -o postgres -g postgres /run/safe-devops/server-tls.key /run/postgresql/tls/server.key
install -m 0644 -o postgres -g postgres /run/safe-devops/server.crt /run/postgresql/tls/server.crt
exec /usr/local/bin/docker-entrypoint.sh postgres -c ssl=on -c ssl_cert_file=/run/postgresql/tls/server.crt -c ssl_key_file=/run/postgresql/tls/server.key
