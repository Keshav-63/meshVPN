# Grafana Local Setup (WSL Debian, No Docker)

This guide runs both Prometheus and Grafana inside WSL to avoid Docker-to-WSL networking issues.

## Why local setup

- Prometheus can scrape control-plane metrics directly from `localhost:8080`
- Grafana reads from `localhost:9090`
- No `host.docker.internal` bridge dependency

## Prerequisites

- WSL Debian
- Control-plane running on port `8080`
- Prometheus installed in WSL (`prometheus --version`)

## Step 1: Stop Docker observability (if running)

```bash
cd /mnt/c/Users/Shreeyansh/Desktop/Veltrix/meshVPN/infra/observability
docker compose down
```

## Step 2: Start Prometheus locally

```bash
cd /mnt/c/Users/Shreeyansh/Desktop/Veltrix/meshVPN/infra/observability
cd /mnt/c/Users/Shreeyansh/Desktop/Veltrix/meshVPN/infra/observability
prometheus \
  --config.file=/mnt/c/Users/Keshav\ suthar/Desktop/MeshVPN-slef-hosting/infra/observability/prometheus.yml \
  --web.listen-address=0.0.0.0:9090 \
  --storage.tsdb.path=/tmp/prometheus-data
  
In another terminal, verify:

```bash
curl http://localhost:9090/-/healthy
curl http://localhost:9090/targets
```

The `control-plane` target should be `UP`.

## Step 3: Install Grafana in WSL (one time)

```bash
cd /tmp
wget https://dl.grafana.com/oss/release/grafana_10.4.3_amd64.deb
sudo dpkg -i grafana_10.4.3_amd64.deb || sudo apt-get -f install -y
```

## Step 4: Start Grafana locally

```bash
sudo service grafana-server start
sudo service grafana-server status
```

Grafana UI:

```text
http://localhost:3000
```

Default login (first run):

- Username: `admin`
- Password: `admin`

## Step 5: Configure Prometheus datasource in Grafana

In Grafana:

1. Go to `Connections -> Data sources`
2. Add/Select `Prometheus`
3. Set URL to `http://localhost:9090`
4. Click `Save & Test`

Expected result: `Data source is working`

## Step 6: Import MeshVPN dashboards

1. Go to `Dashboards -> New -> Import`
2. Import these JSON files:
   - `/mnt/c/Users/Shreeyansh/Desktop/Veltrix/meshVPN/infra/observability/grafana-dashboards/platform-overview.json`
   - `/mnt/c/Users/Shreeyansh/Desktop/Veltrix/meshVPN/infra/observability/grafana-dashboards/deployment-detail.json`
3. Select the `Prometheus` datasource when prompted

## Useful commands

### Stop services

```bash
pkill prometheus
sudo service grafana-server stop
```

### Restart services

```bash
pkill prometheus
cd /mnt/c/Users/Shreeyansh/Desktop/Veltrix/meshVPN/infra/observability
prometheus --config.file=prometheus.yml --web.listen-address=0.0.0.0:9090
sudo service grafana-server restart
```

### Logs and health

```bash
curl http://localhost:8080/metrics | head -n 20
curl http://localhost:9090/-/healthy
curl http://localhost:3000/api/health
sudo journalctl -u grafana-server -n 100 --no-pager
```

## Common issues

### Grafana not reachable on 3000

```bash
sudo service grafana-server status
sudo journalctl -u grafana-server -n 100 --no-pager
```

### Prometheus target is DOWN

- Ensure control-plane is running and serving metrics:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/metrics
```

- Confirm target in `infra/observability/prometheus.yml` is `localhost:8080`.

### No data in dashboard

- Verify datasource URL is exactly `http://localhost:9090`
- Check time range in Grafana (top-right)
- Wait 10-20 seconds for first scrape cycle
