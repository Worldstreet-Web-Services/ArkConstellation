# Monitoring Runbook — ArkConstellation

This runbook covers the setup, operation, and maintenance of the monitoring
infrastructure for ArkConstellation nodes.

---

## Quick Start

### Start the devnet with monitoring

```bash
# Start the devnet with init container and monitoring stack
docker compose -f ops/docker/docker-compose.devnet.yml up -d --build

# Access monitoring
#    Grafana:       http://localhost:3000  (admin / arkconstellation)
#    Prometheus:    http://localhost:9092
#    AlertManager:  http://localhost:9093
```

### Connect to existing devnet (no Docker)

If the devnet is running via pystarport (local processes, not Docker),
update the Prometheus config targets to point at the host:

1. Edit `ops/monitoring/prometheus.yml`
2. Replace the Docker service names with your host IP:
   ```yaml
   static_configs:
     - targets:
         - "192.168.1.100:26660"  # sentry-0 Prometheus port
         - "192.168.1.100:26670"  # sentry-1 Prometheus port
   ```
3. Restart Prometheus:
   ```bash
   docker compose -f ops/docker/docker-compose.devnet.yml restart prometheus
   ```

---

## Architecture Overview

### Sentry Node Architecture

```
                    Public Internet
                         |
            +------------+------------+
            |                         |
      [sentry-0]              [sentry-1]      ← Public-facing relay tier
      P2P: 26656               P2P: 26676
      RPC: 26657               RPC: 26677
      LCD: 1317                LCD: 1318
      Prometheus: 9090         Prometheus: 9091
            |                         |
            | persistent_peers        | persistent_peers
            | pex=true                | pex=true
            | private_peer_ids=       | private_peer_ids=
            |   [validator-0]         |   [validator-1]
            |                         |
      [validator-0]           [validator-1]   ← Signing nodes (private)
      (no public ports)       (no public ports)
      persistent_peers=       persistent_peers=
        sentry-0 only           sentry-1 only
      pex=false                pex=false
```

### Key Ports

| Port    | Protocol | Description                     |
|---------|----------|---------------------------------|
| 26656   | TCP      | P2P peer connections            |
| 26657   | HTTP     | CometBFT RPC                    |
| 1317    | HTTP     | Cosmos LCD/API (REST)           |
| 8545    | HTTP     | EVM JSON-RPC (if enabled)       |
| 9090    | HTTP     | Prometheus metrics              |
| 9095    | gRPC/h2c | Cosmos gRPC (sentry-0, loopback) |
| 3000    | HTTP     | Grafana dashboards              |
| 9091    | HTTP     | Prometheus (devnet compose)     |
| 9092    | HTTP     | Prometheus (devnet compose)     |
| 9093    | HTTP     | AlertManager                    |

---

## Prometheus Configuration

**File:** `ops/monitoring/prometheus.yml`

### Scraped Targets

| Job Name        | Target               | Metrics                    |
|-----------------|----------------------|----------------------------|
| `cometbft`      | `<node>:9090`        | Block height, consensus, P2P |
| `evm-rpc`       | `<node>:8545`        | EVM base fee, gas, tx count |
| `node-exporter` | `<node>:9100`        | CPU, RAM, disk, network     |
| `prometheus`    | `localhost:9090`     | Prometheus self-monitoring  |

### Adding a New Node

1. Add the node's Prometheus endpoint to the appropriate `static_configs` target list
2. Use the Docker service name (Docker Compose) or IP:port (bare metal)
3. Reload Prometheus: `curl -X POST http://localhost:9092/-/reload`

---

## Grafana Dashboards

**Access:** `http://localhost:3000` (admin / arkconstellation)

### Pre-loaded Dashboards

| Dashboard              | UID                    | What it shows                         |
|------------------------|------------------------|---------------------------------------|
| Block Production       | `ark-block-production` | Block height, rate, time, rounds      |
| Validator Health       | `ark-validator-health` | Voting power, jailing, signing rate   |
| P2P Health             | `ark-p2p-health`       | Peer count, traffic, connectivity     |
| EVM Gas & Fees         | `ark-evm-gas`          | Base fee, gas utilization, tx throughput |

### Importing a Dashboard

1. Go to **Dashboards → Import**
2. Upload the JSON file from `ops/monitoring/grafana/dashboards/`
3. Select **Prometheus** as the data source
4. Click **Import**

---

## AlertManager Configuration

**File:** `ops/monitoring/alerts/alertmanager.yml`
**UI:** `http://localhost:9093`

### Alert Routing

| Severity | Receiver     | Repeat Interval |
|----------|-------------|-----------------|
| critical | `critical`  | 5 minutes       |
| warning  | `warning`   | 30 minutes      |

### Configuring Notifications

To enable Slack notifications:

1. Edit `ops/monitoring/alerts/alertmanager.yml`
2. Uncomment the `slack_configs` section under the `critical` receiver
3. Replace the `api_url` with your Slack webhook URL
4. Restart AlertManager: `docker compose restart alertmanager`

To enable PagerDuty:

1. Uncomment the `pagerduty_configs` section
2. Replace `routing_key` with your PagerDuty integration key
3. Restart AlertManager

---

## Alert Rules

**File:** `ops/monitoring/alerts/alerts.yml`

### Chain Alerts

| Alert                  | Condition                          | Severity | Action                      |
|------------------------|------------------------------------|----------|-----------------------------|
| `ChainNotProducing`    | No blocks in 30s                   | critical | Check validators, network   |
| `ConsensusRoundSlow`   | Round number > 2                   | warning  | Monitor, check connectivity |
| `MissedBlocks`         | No blocks in 1 minute              | critical | Immediate investigation     |

### Validator Alerts

| Alert                     | Condition                           | Severity | Action                         |
|---------------------------|-------------------------------------|----------|--------------------------------|
| `ValidatorJailed`         | Any validator jailed                | critical | Unjail validator               |
| `ValidatorMissingSignatures` | Signing rate < 95%              | warning  | Check validator uptime         |
| `ValidatorPowerZero`      | Voting power = 0                    | warning  | Check bonding/delegation       |

### Infrastructure Alerts

| Alert                 | Condition                        | Severity | Action                    |
|-----------------------|----------------------------------|----------|---------------------------|
| `NodeUnreachable`     | Prometheus can't scrape node     | critical | Restart node, check logs  |
| `RPCDesync`           | Node lagging behind peers        | warning  | Check disk I/O, network   |
| `BaseFeeSpike`        | Base fee > 5x 1-hour avg        | warning  | Monitor demand, config    |
| `HighDiskUsage`       | Free disk < 15%                  | warning  | Cleanup or expand disk    |
| `HighCPUUsage`        | CPU > 85% for 5 minutes          | warning  | Check process load        |

---

## Node Topology Documentation

**File:** `ops/runbooks/node-topology.md`

Document the physical deployment topology for production:

### Production Topology (Target)

```
Region 1 (e.g., US-East)         Region 2 (e.g., EU-West)
  Cloud Provider: AWS              Cloud Provider: GCP
  +-----------------------+        +-----------------------+
  |  VPC: 10.0.1.0/24     |        |  VPC: 10.0.2.0/24     |
  |                       |        |                       |
  |  sentry-0 (public)    |        |  sentry-1 (public)    |
  |    LB: rpc.us.ark..   |        |    LB: rpc.eu.ark..   |
  |                       |        |                       |
  |  validator-0 (private)|        |  validator-1 (private)|
  |    Subnet: 10.0.1.x   |        |    Subnet: 10.0.2.x   |
  +-----------------------+        +-----------------------+
```

### Requirements per Validator

- [ ] Minimum 2 different geographic regions
- [ ] Minimum 2 different cloud providers
- [ ] Each validator behind a sentry node (validator never publicly reachable)
- [ ] Remote signing via Horcrux or Tendermint KMS (no local private keys)
- [ ] Hardware signing devices (Ledger/HSM) for key material
- [ ] Firewall: validator accepts connections only from its sentry's IP
- [ ] node_exporter running for system metrics

---

## Troubleshooting

### Prometheus not scraping targets

1. Check Prometheus config: `curl http://localhost:9092/api/v1/targets`
2. Verify targets are listed and reachable
3. For Docker: ensure all containers are on the same network
4. For host: verify firewall allows Prometheus to reach target ports

### Grafana dashboards not showing data

1. Verify Prometheus datasource is configured: **Settings → Data Sources**
2. Check the datasource URL matches your Prometheus instance
3. Verify the time range in the dashboard (try "Last 15 minutes")
4. Check Prometheus has data: `curl http://localhost:9092/api/v1/query?query=cometbft_consensus_height`

### Alerts not firing

1. Check AlertManager is running: `curl http://localhost:9093/-/healthy`
2. Check Prometheus is sending alerts: `curl http://localhost:9092/api/v1/alerts`
3. Check alert rules are loaded: `curl http://localhost:9092/api/v1/rules`
4. Verify the AlertManager URL in `prometheus.yml` matches the service name

### Node not producing metrics

1. Ensure Prometheus is enabled in the node's config.toml:
   ```toml
   [prometheus]
   prometheus = true
   prometheus_listen_addr = ":9090"
   ```
2. Check the port is accessible: `curl http://<node-ip>:9090/metrics`
3. For Docker: verify the port is mapped or the node is on the same network
