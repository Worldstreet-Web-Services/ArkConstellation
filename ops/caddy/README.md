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

## Applying

The host runs from a checkout at `/home/Evangel/ArkConstellation`, so once this
merges:

```bash
# on ark-devnet
cd ~/ArkConstellation && git pull
docker compose -f ops/docker/docker-compose.devnet.yml up -d sentry-0   # recreates with 9095
sudo cp ops/caddy/Caddyfile /etc/caddy/Caddyfile
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy                                            # zero-downtime
```

Recreating `sentry-0` briefly interrupts the public RPC, LCD and EVM endpoints.
The validators are separate containers and keep producing blocks throughout.

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
