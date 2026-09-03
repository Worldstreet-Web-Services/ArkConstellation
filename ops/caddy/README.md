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

## Applying

Replace the `UPSTREAM` / `BLOCKSCOUT_*` / `FAUCET` placeholders, then:

```bash
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
