#!/usr/bin/env bash
# Capture the devnet's live edge configuration so it can be committed.
#
# The public endpoint layer (TLS termination and hostname routing for
# *.34.60.137.196.sslip.io) was set up directly on the host and is not in this
# repo. That is why rpc./lcd. could break with no commit explaining it, why the
# grpc. route was never noticed missing, and why nobody can review a change to
# any of it without shell access.
#
# Run this ON THE DEVNET HOST, then commit the output as the baseline. Changes
# to ops/caddy/Caddyfile then become a reviewable diff against reality instead
# of a file written from guesses.
#
#   ./capture-live-config.sh > live-edge-config.txt
#
# REVIEW BEFORE COMMITTING. A Caddyfile can contain basicauth hashes, API tokens
# or upstream credentials. This script does not redact anything — read it first.
set -uo pipefail

section() { printf '\n===== %s =====\n' "$1"; }

section "captured"
date -u +'%Y-%m-%dT%H:%M:%SZ'; hostname

section "is caddy a host process or a container?"
if systemctl list-units --type=service 2>/dev/null | grep -qi caddy; then
  echo "HOST PROCESS (systemd)"
  systemctl cat caddy 2>/dev/null
  systemctl status caddy --no-pager 2>/dev/null | head -12
elif docker ps --format '{{.Names}} {{.Image}}' 2>/dev/null | grep -qi caddy; then
  echo "CONTAINER"
  docker ps --filter name=caddy --format '{{.Names}}\t{{.Image}}\t{{.Ports}}'
  CID=$(docker ps -q --filter name=caddy | head -1)
  echo "--- networks (decides whether upstreams are 127.0.0.1 or service names) ---"
  docker inspect "$CID" -f '{{range $k,$v := .NetworkSettings.Networks}}{{$k}}{{"\n"}}{{end}}' 2>/dev/null
  echo "--- mounts ---"
  docker inspect "$CID" -f '{{range .Mounts}}{{.Source}} -> {{.Destination}}{{"\n"}}{{end}}' 2>/dev/null
else
  echo "NEITHER FOUND — check for nginx/traefik, or a proxy on a different host"
  docker ps --format '{{.Names}}\t{{.Image}}' 2>/dev/null | head -20
fi

section "the live Caddyfile"
for p in /etc/caddy/Caddyfile /opt/caddy/Caddyfile ./Caddyfile "${CADDYFILE:-}"; do
  [ -n "$p" ] && [ -f "$p" ] && { echo "--- $p ---"; cat "$p"; break; }
done
CID=$(docker ps -q --filter name=caddy 2>/dev/null | head -1)
[ -n "${CID:-}" ] && { echo "--- from container ---"; docker exec "$CID" cat /etc/caddy/Caddyfile 2>/dev/null; }

section "what is actually listening"
(ss -tlnp 2>/dev/null || netstat -tlnp 2>/dev/null) | grep -E ':(80|443|1317|8545|8546|9090|9095|26657|3000|4000|8088)\b'

section "published container ports"
docker ps --format '{{.Names}}\t{{.Ports}}' 2>/dev/null

section "certificate state per hostname"
# Each hostname carries its own certificate, so one can fail while its
# neighbours are fine — the shape of the intermittent explorer. failures here
for h in rpc lcd grpc evm explorer explorer-api faucet; do
  printf '%-14s ' "$h"
  echo | timeout 12 openssl s_client -connect "$h.34.60.137.196.sslip.io:443" \
      -servername "$h.34.60.137.196.sslip.io" 2>/dev/null \
    | openssl x509 -noout -enddate -issuer 2>/dev/null | tr '\n' ' ' || echo "NO CERT"
  echo
done

section "recent TLS / certificate errors in the caddy log"
journalctl -u caddy --since '48 hours ago' --no-pager 2>/dev/null \
  | grep -iE 'error|obtain|renew|certificate|tls' | tail -30 \
  || echo "(no systemd journal for caddy)"
