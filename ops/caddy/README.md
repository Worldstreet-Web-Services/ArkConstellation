# Devnet edge (Caddy)

Caddy terminates TLS and routes `*.34.60.137.196.sslip.io` to the devnet's
services. It runs as a **host systemd unit** on `ark-devnet` (us-central1-a) —
not a container — so upstreams are `localhost:<published port>`.

`Caddyfile` here is the live `/etc/caddy/Caddyfile`, captured 2026-09-04, plus
the `grpc.` route. It is a description of what runs, not a proposal.

## Endpoint status, measured 2026-09-04

| Host | Result | |
|---|---|---|
| `rpc.` | 200 | working |
| `lcd.` | 501 | working — LCD returns 501 for `GET /`, which is correct |
| `explorer.` | 200 | working |
| `explorer-api.` | 404 at `/` | working — no route at root, real API paths respond |
| `evm.` | 405 | working — JSON-RPC rejects GET |
| `faucet.` | 200 | working |
| `grpc.` | 000 | **the only genuine gap** |

Every certificate is valid and renewing normally; the ACME log shows only
`info` lines, no failures, with expiries in November.

> **Correction.** An earlier revision of this file reported `rpc.` and `lcd.` as
> broken and `explorer.` as intermittent, and proposed changes to fix them. That
> was wrong. Those probes ran with 12–15 second timeouts from a laptop and hit
> transient failures; re-tested with a longer timeout, from both a laptop and the
> VM itself, all three return correctly and always have. No route needed fixing.
> The record is kept here so the same false alarm is not raised twice.

## The one real gap: gRPC

IBC relayers need gRPC for account queries and transaction simulation. RPC and
LCD are not enough.

The node is already serving it. `entrypoint.sh` moves gRPC to `0.0.0.0:9095`,
deliberately leaving 9090 to CometBFT's Prometheus endpoint that
`ops/monitoring/prometheus.yml` scrapes as `sentry-0:9090`:

```
$ docker exec ark-sentry-0 netstat -tln | grep 909
tcp   0  0 :::9090   :::*  LISTEN
tcp   0  0 :::9095   :::*  LISTEN
```

**What is missing is the port mapping.** `docker port ark-sentry-0` lists 8100,
8545, 8546, 9090, 26656 and 26657 — no 9095 — so nothing outside the compose
network can reach it. Publishing it is the fix, added in
`ops/docker/docker-compose.devnet.yml`.

## Applied 2026-09-04

Already live on `ark-devnet`. This PR brings the repo into line with the host.

**The host is not a git checkout.** `/home/Evangel/ArkConstellation` was extracted
from `ark-ops.tar.gz`, so there is no `git pull` path — an earlier draft of this
file said there was, wrongly. Changes have to be copied to the host, and the repo
is documentation of the host rather than its source. Worth fixing separately: a
deployment nobody can `git pull` is a deployment whose drift nobody can see.

What was run, with backups taken first
(`/etc/caddy/Caddyfile.bak-20260904`, `docker-compose.devnet.yml.bak-20260904`):

```bash
# 9095/9096 added to the sentry port blocks, then
docker compose -f ops/docker/docker-compose.devnet.yml up -d sentry-0 sentry-1
# grpc. block appended to /etc/caddy/Caddyfile, then
sudo caddy validate --config /etc/caddy/Caddyfile && sudo systemctl reload caddy
```

Measured through the restart:

| | |
|---|---|
| Chain height before → after | 205028 → 205033, never paused |
| Validators | `Up 7 days` — separate containers, never restarted |
| `grpc.` after | **HTTP 415 over HTTP/2** — a real gRPC server answering |
| `rpc.` / `lcd.` / `evm.` / `explorer.` / `faucet.` | 200 / 501 / 405 / 200 / 200, unchanged |

The sentry recreate did interrupt the public RPC, LCD and EVM endpoints for a few
seconds. Consensus was unaffected, exactly as expected: the validators are
separate containers with no published ports, and blocks kept being produced.

## Verifying

```bash
grpcurl grpc.34.60.137.196.sslip.io:443 list
```

A plain `curl` is not a useful check here: gRPC is HTTP/2 and will not answer an
HTTP/1.1 request, so `000` is expected even when it is working. And note that
**any** `*.sslip.io` name resolves to this host and completes a TCP handshake, so
"the port is open" proves nothing about what is behind it.

## Note for production

`sslip.io` should not outlive the devnet. The hostnames encode the IP, so every
consumer that hard-codes one breaks the moment the host moves.
`networks/devnet/README.md` already makes this point.
