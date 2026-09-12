# Remote signing — TMKMS, single-instance, integration-tested

**Choice: TMKMS over Horcrux.** Both are legitimate; TMKMS's softsign
backend gets a real, working, single-instance remote signer connected to a
live validator inside a single session, which is what this phase asks for.
Horcrux's whole value proposition is threshold (multi-cosigner) signing —
standing one up in single-cosigner mode would exercise almost none of what
makes Horcrux Horcrux, while still taking real setup time (shard
generation, cosigner peer config). Deferred to `GAPS.md` as the intended
upgrade path once real mainnet needs the threshold/HA property.

## What actually happened here (not just documented — run and captured)

1. Started the local devnet (`make devnet-init`), giving `validator-0`
   (`node1`) a real `priv_validator_key.json` with address
   `FC3FF8635F285BAFFA1397E677996FB7E5C2FE38`.
2. Imported that exact key into TMKMS's softsign keystore:
   `tmkms softsign import -f json node1/config/priv_validator_key.json secrets/arkdevnet_9000-1-consensus.key`
   — same key material, now living in TMKMS instead of on the validator's
   own disk.
3. Removed `node1/config/priv_validator_key.json` entirely and set
   `priv_validator_laddr` in its `config.toml`, so the node can no longer
   sign locally — it can only sign by asking whatever's listening on that
   address.
4. Started `tmkms start -c tmkms.toml`, then restarted the devnet.
5. Confirmed in `proof/live-signing-evidence.log`: TMKMS receiving real
   `SignVote` requests from the running node, signing them, and — the part
   that actually proves this end-to-end — **validator-0's signature
   (address `FC3FF8635F285BAFFA1397E677996FB7E5C2FE38`) present with
   `block_id_flag: 2` (voted, not absent) in the `last_commit` of live
   blocks** queried straight from the devnet's RPC. The chain reached
   block 14+ with validator-0 remote-signing the entire way.

Reproduce:

```bash
export PATH="$HOME/.cargo/bin:$PATH"   # or wherever cargo installed tmkms
cargo install tmkms --features=softsign --locked

make devnet-init   # fresh 4-node init, do NOT run devnet-up (see below)

NODE1=networks/devnet/data/arkdevnet_9000-1/node1
mkdir -p networks/devnet/remote-signing/secrets networks/devnet/remote-signing/state
tmkms softsign import -f json \
  "$NODE1/config/priv_validator_key.json" \
  networks/devnet/remote-signing/secrets/arkdevnet_9000-1-consensus.key
rm "$NODE1/config/priv_validator_key.json"
# point the node at the unix socket instead of local signing (see
# tmkms.toml's comment for why unix, not tcp)
sed -i.bak 's#priv_validator_laddr = ""#priv_validator_laddr = "unix://'"$(pwd)"'/networks/devnet/remote-signing/privval.sock"#' \
  "$NODE1/config/config.toml" && rm "$NODE1/config/config.toml.bak"
# expand the socket-path placeholder into an untracked local tmkms config
sed 's#__TMKMS_SOCKET_PATH__#'"$(pwd)"'/networks/devnet/remote-signing/privval.sock#' \
  networks/devnet/remote-signing/tmkms.toml \
  > networks/devnet/remote-signing/tmkms.local.toml

cd networks/devnet/remote-signing && tmkms start -c tmkms.local.toml -v & cd -
./networks/devnet/.venv-pystarport/bin/pystarport start --data networks/devnet/data --quiet &

sleep 8
curl -s http://127.0.0.1:26657/status | jq '.result.sync_info.latest_block_height'
```

(Note: the committed `tmkms.toml` carries the placeholder
`__TMKMS_SOCKET_PATH__`; the reproduce step above expands it to an
absolute path with `$(pwd)` before tmkms starts. The placeholder is used
so the file is not tied to any one checkout directory, while the Unix
socket `addr` still needs no node ID — see "Known gap" below for why.)

## Known gap discovered, not just assumed: TCP transport doesn't work as configured

The first attempt used TCP (`priv_validator_laddr = "tcp://127.0.0.1:26669"`,
`tmkms.toml`'s `addr = "tcp://<validator-0's node ID>@127.0.0.1:26669"` —
same addressing convention as p2p `persistent_peers`). It failed:
TMKMS logged `verification failed: ... validator peer ID mismatch!` with a
*different* unexpected ID on every retry.

Traced to CometBFT's own source, not a config mistake on this end:
`privval/utils.go:42` (originally observed against the pseudo-version
`v0.38.23-0.20260422215035-4928b26fd5ba`; this repo now vendors the
released tag `v0.38.23`, a strict fast-forward from that pseudo-version
that does not touch this file — find the exact path with
`go list -m github.com/cometbft/cometbft` and look under
`$(go env GOMODCACHE)`):

```go
case "tcp":
    // TODO: persist this key so external signer can actually authenticate us
    listener = NewTCPListener(ln, ed25519.GenPrivKey())
```

Every node restart generates a **fresh random** SecretConnection identity
key for the TCP privval listener — it is never persisted and has no
relationship to `node_key.json` or the consensus key. There is no way to
predict or pin the ID a `tcp://` remote-signer connection should expect,
because CometBFT itself doesn't know it in advance either. This is an
upstream, acknowledged (see the TODO) limitation of CometBFT 0.38's TCP
privval listener, not something specific to this chain's fork.

**Unix socket transport sidesteps it entirely** — `NewSignerListener`'s
`case "unix"` path (`socket_listeners.go`) does no SecretConnection
handshake at all; it's authenticated by filesystem permissions on the
socket path instead. That's what this rehearsal actually uses, and it's
genuinely representative of the common "signer co-located with validator
on the same host, different process/container" deployment pattern — not
just a workaround chosen to dodge the bug.

## What's simulated vs. what changes for real validator hardware

| Here | Real mainnet |
|---|---|
| Single TMKMS instance, softsign (key material in a plain file, encrypted at rest by the OS only) | Multi-cosigner threshold signing (Horcrux, or TMKMS with a YubiHSM2/Ledger backend) so no single machine holds a complete usable key |
| TMKMS and the validator on the same host, connected via a local Unix socket | TMKMS/signer typically on separate, more tightly-controlled hardware from the validator process, reachable only over a private network |
| Unix socket sidesteps the TCP transport gap above entirely | A genuinely separate-host signer setup hits CometBFT's unpersisted-key TCP issue head-on and needs one of: a network layer that's already trusted regardless of the ephemeral key (e.g. a private link/VPN with its own auth), a CometBFT fork/patch that persists the listener key, or confirming whether TMKMS has grown a "don't verify peer ID" escape hatch in a version newer than 0.15.0 (not checked in this session — flagged in `GAPS.md`) |
| Double-sign protection state (`state/arkdevnet_9000-1-consensus.json`) lives on the same disk as everything else in this rehearsal | Should be on durable, ideally replicated storage — losing this file and restarting is exactly the scenario that causes double-signing/slashing |
| Key imported from a devnet-only throwaway `priv_validator_key.json` that never existed outside this local run | Real mainnet key ceremony — generated directly inside the signing environment, never as a portable JSON file that touches a general-purpose disk at all |

## Files

- `tmkms.toml` — the config actually used (committed; no secrets in it)
- `secrets/`, `state/` — gitignored, regenerated by the reproduce steps above
- `proof/live-signing-evidence.log` — captured evidence: real SignVote
  traffic, plus validator-0's signature present in an actual committed
  block's `last_commit`, queried live from the running devnet's RPC
