# Track 4 — Infrastructure & Observability

**Owner:** Eng 4
**Branch:** `track/4-infra-observability`
**Workspace:** `ops/`

---

## Mission

Provide full operational visibility, manage key custody, and deploy public-facing endpoints for mainnet.

---

## Prerequisites

- Download the `mantrachaind` binary from GitHub Actions CI artifacts (no Go installation needed)
- Access to cloud provider accounts across at least 2 geographic regions
- Hardware signing devices (Ledger or HSM) for the key ceremony

---

## Day 1 Tasks (00:00 – 24:00)

### Node Provisioning

- [ ] Provision validator nodes across geographically distinct cloud providers:
  - Minimum 2 different regions
  - Minimum 2 different cloud providers
  - Each validator behind a sentry node (validator signing node never publicly reachable)
- [ ] Document node topology in `ops/runbooks/node-topology.md`

### Monitoring Setup

- [ ] Deploy Prometheus configured to scrape:
  - CometBFT metrics (`/metrics` endpoint on each node)
  - EVM JSON-RPC metrics
  - System metrics (CPU, RAM, disk)
- [ ] Deploy Grafana dashboards for:
  - Block production rate and missed blocks
  - Validator voting power and uptime
  - Peer connections and P2P health
  - Base fee and gas usage
- [ ] Commit dashboard configs to `ops/monitoring/`

### Deliverable

- [ ] Live metrics ingestion confirmed for devnet nodes
- [ ] Prometheus and Grafana URLs documented in `ops/runbooks/monitoring.md`

---

## Day 2 Tasks (24:00 – 48:00)

**Waits for:** Eng 2's devnet to be live

### Blockscout Explorer

- [ ] Fork `MANTRA-Chain/mantra-explorer-evm-blockscout`
- [ ] Rebrand to ArkConstellation
- [ ] Connect to devnet EVM JSON-RPC endpoint
- [ ] Verify block and transaction indexing is working
- [ ] Deploy publicly accessible Blockscout instance

### Alert Configuration

- [ ] Configure PagerDuty (or equivalent) alerts for:
  - Missed blocks (threshold: >3 consecutive)
  - Validator stalling or jailing
  - RPC desync (block height lagging >10 blocks behind peers)
  - High base fee spike (>5x baseline)
- [ ] Test each alert fires correctly

### Runbooks

- [ ] Write `ops/runbooks/chain-halt.md` — steps for a coordinated chain halt and restart
- [ ] Write `ops/runbooks/validator-jail-unjail.md` — steps to unjail a jailed validator
- [ ] Write `ops/runbooks/state-rollback.md` — steps for emergency state rollback
- [ ] Write `ops/runbooks/key-ceremony.md` — hardware signing key ceremony procedure

### Deliverable

- [ ] Working Blockscout UI connected to devnet
- [ ] All runbooks in `ops/runbooks/` committed and reviewed by at least one other engineer

---

## Day 3 Tasks (48:00 – 72:00)

**Waits for:** `networks/mainnet/genesis.json` from Eng 2

### Hardware Multisig Key Ceremony

- [ ] Execute key ceremony per `ops/runbooks/key-ceremony.md`
- [ ] Generate multisig admin key using Ledger/HSM hardware devices
- [ ] Record multisig address
- [ ] Confirm multisig address is embedded in `genesis.json` as the upgrade authority

### Public Endpoint Configuration

- [ ] Configure load balancers in front of sentry nodes
- [ ] Set up public DNS routing for:
  - Cosmos RPC: `rpc.<chain-domain>`
  - Cosmos REST: `api.<chain-domain>`
  - EVM JSON-RPC: `evm.<chain-domain>`
  - Blockscout explorer: `explorer.<chain-domain>`
- [ ] Verify all endpoints are reachable and return correct chain ID

### Deliverable

- [ ] Admin multisig live on-chain and confirmed
- [ ] All public RPC endpoints active and verified
- [ ] Bug bounty published with contact details and scope

---

## Handoff Artifacts

| Artifact | Destination | When |
|----------|-------------|------|
| `ops/monitoring/` configs | All engineers (shared visibility) | End of Day 1 |
| Blockscout URL | Eng 3 (chaos monitoring during tests) | End of Day 2 |
| `ops/runbooks/` | All engineers (reviewed before mainnet) | End of Day 2 |
| Multisig address | Eng 2 (embed in genesis.json) | Day 3 morning |
| Public endpoint URLs | All engineers and validators | Day 3 |
