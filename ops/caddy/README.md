# Devnet edge (Caddy)

## What was broken

Probed 2026-09-03 from outside:

| Host | Result | |
|---|---|---|
| `evm.` | 405 | fine — JSON-RPC rejects GET |
| `faucet.` | 200 | fine |
| `explorer-api.` | 404 at `/` | fine — real API paths work |
| `explorer.` | intermittent | succeeded and failed minutes apart |
| `rpc.` | nothing | **broken** |
| `lcd.` | nothing | **broken** |
| `grpc.` | nothing | **never configured** |

The chain itself was healthy throughout (block 197,467, and the direct-IP ports
26657/1317 answered normally), so this is edge routing, not the node.

Blockscout kept working because it has its own database and indexes through the
**EVM** JSON-RPC, which was up. It never touches the Cosmos RPC or LCD.

## The gRPC port — no conflict to resolve

An earlier draft of this change claimed 9090 was contested and proposed moving
Prometheus. That was wrong, and worth recording so nobody repeats it.

`entrypoint.sh` **already** relocates the node's gRPC server:

```sh
# Ensure gRPC is on 9095 to avoid port collision with CometBFT Prometheus on 9090
sed -i 's|^address = "0.0.0.0:9090"|address = "0.0.0.0:9095"|' app.toml
```

So gRPC listens on 9095 inside the container and 9090 belongs to CometBFT's
Prometheus endpoint, which `ops/monitoring/prometheus.yml` scrapes by name as
`sentry-0:9090`. Moving Prometheus would have broken monitoring to fix nothing.

The actual gap was that **9095 was never published to the host**. That is fixed in
`docker-compose.devnet.yml` in this same change: `9095:9095` on sentry-0 and
`9096:9095` on sentry-1.

## This file is not yet the source of truth

The live edge configuration was set up directly on the devnet host and is not in
this repo — no compose file here defines Caddy, and no document mentions
`sslip.io` or the host address. The `Caddyfile` here was written from the outside
in, by probing which hostnames answered and reading which ports the compose
publishes.

That has a consequence worth stating plainly: **do not install this file over the
running one.** There is a live Caddyfile on that host containing the routes that
currently work, and this one may be missing settings it has.

The intended order is baseline first, changes second:

```bash
# on the devnet host
./capture-live-config.sh > live-edge-config.txt   # review before committing —
                                                  # a Caddyfile can hold secrets
```

Commit that output, then re-apply this file as a diff against it. The repo then
reflects reality, the `127.0.0.1`-versus-service-names question answers itself,
and the next person to change a route can see what changed and why.

Until then this is a proposal about the edge, not a description of it.

## Applying

Upstreams are `127.0.0.1:<published port>`, on the assumption that **Caddy runs on
the host** rather than inside the `arkdevnet` compose network — no Caddy service
exists in this repo's compose files. If it is in fact containerised and shares
that network, swap the addresses for service names (`sentry-0:9095`, and so on);
Docker DNS will not resolve those from outside the network.

```bash
docker compose -f ops/docker/docker-compose.devnet.yml up -d sentry-0 sentry-1
caddy validate --config Caddyfile
caddy reload  --config Caddyfile     # zero-downtime
```

## Verifying

```bash
curl -s https://rpc.34.60.137.196.sslip.io/status | jq .result.sync_info.latest_block_height
curl -s https://lcd.34.60.137.196.sslip.io/cosmos/staking/v1beta1/params | jq .params

# gRPC: 415 means a real gRPC server answered a non-gRPC request. 000 means
# nothing is listening — note that ANY *.sslip.io name will still resolve and
# complete a TCP handshake, so "the port is open" proves nothing here.
curl -s -o /dev/null -w '%{http_code} %{http_version}\n' https://grpc.34.60.137.196.sslip.io/
```

Definitive gRPC check:

```bash
grpcurl grpc.34.60.137.196.sslip.io:443 list
```

## Diagnosing the intermittent explorer

The config adds explicit dial and response timeouts so a slow backend fails fast
instead of hanging. If it still flaps, the cause is almost certainly TLS
issuance rather than proxying — enable `debug` in the global block and look for
certificate errors for that specific hostname:

```bash
journalctl -u caddy -f | grep -iE "certificate|obtain|renew|tls"
echo | openssl s_client -connect explorer.34.60.137.196.sslip.io:443 \
  -servername explorer.34.60.137.196.sslip.io 2>/dev/null | openssl x509 -noout -dates -issuer
```

Each hostname gets its own certificate, so one can fail while its neighbours are
fine — which matches the observed pattern exactly.

## Note for production

`sslip.io` is fine for a devnet and should not outlive it. The hostnames encode
the IP, so the endpoints cannot survive the host moving, and every consumer that
has hard-coded one breaks at once. Migrate to an Ark-owned domain before anything
depends on these — the same point `networks/devnet/README.md` already makes.
