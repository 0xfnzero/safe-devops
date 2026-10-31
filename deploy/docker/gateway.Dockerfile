FROM caddy:2.10.2-alpine
# The upstream binary carries CAP_NET_BIND_SERVICE. Linux refuses to execute
# that file when Compose drops ALL capabilities. We only bind 8080/8443, so
# remove the unneeded file capability instead of granting runtime privileges.
RUN setcap -r /usr/bin/caddy
USER 10001:10001
