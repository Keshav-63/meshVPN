# MeshVPN Analytics Dashboard - Complete Guide

This guide will help you set up and access the comprehensive analytics dashboard for your MeshVPN platform.

## Overview

The analytics system consists of:
- **Prometheus** (running in WSL) - Metrics collection and storage
- **Grafana** (running in Docker) - Visualization dashboards
- **Control-plane** - Exposes metrics on port 8080
- **Worker-agents** - Expose metrics on port 9090

## Quick Start

### 1. Start Prometheus in WSL

```bash
# In your WSL terminal
cd /mnt/c/Users/Keshav\ suthar/Desktop/MeshVPN-slef-hosting/infra/observability

# Download Prometheus (if not already installed)
wget https://github.com/prometheus/prometheus/releases/download/v2.51.0/prometheus-2.51.0.linux-amd64.tar.gz
tar xvfz prometheus-2.51.0.linux-amd64.tar.gz
cd prometheus-2.51.0.linux-amd64

# Copy the config file
cp ../../prometheus.yml .

# Start Prometheus
./prometheus --config.file=prometheus.yml
```

Prometheus will start on `http://localhost:9090`

### 2. Start Grafana (Docker)

```bash
# In WSL or Windows terminal
cd /mnt/c/Users/Keshav\ suthar/Desktop/MeshVPN-slef-hosting/infra/observability

# Start Grafana
docker-compose up -d grafana
```

Grafana will start on `http://localhost:3001`

### 3. Access the Dashboard

Open your browser and navigate to:

```
http://localhost:3001
```

**No login required** - Anonymous access is enabled for local development.

The comprehensive dashboard will load automatically as the default home dashboard.

## Dashboard Features

### 📊 Platform Overview Section
- **Total Workers** - Count of all registered workers
- **Idle/Busy/Offline Workers** - Worker status distribution
- **Total Deployments** - Number of deployments
- **Running Deployments** - Active deployments
- **Total Pods** - Kubernetes pods across all deployments
- **Total Requests** - Cumulative request count

### 🔧 Worker Capacity & Resource Utilization
- **Worker Capacity Utilization** - Shows total, used, and available capacity
- **Worker Status Distribution** - Timeline of idle, busy, and offline workers

### 📈 Request Analytics & Performance
- **Platform Request Rate** - Requests per second across the platform
- **Request Latency Percentiles** - p50, p90, p95, p99 latencies
  - Green threshold: < 0.5s
  - Yellow threshold: 0.5s - 1s
  - Red threshold: > 1s

### 🌐 Bandwidth & Network Usage
- **Platform Total Bandwidth** - Upload and download rates in bytes/sec
- **Top 10 Deployments by Bandwidth** - Pie chart showing bandwidth consumers

### 💻 Resource Consumption (CPU & Memory)
- **Platform CPU Usage** - Average and peak CPU usage with thresholds:
  - Green: < 70%
  - Yellow: 70-90%
  - Red: > 90%
- **Platform Memory Usage** - Average and peak memory consumption
- **Top 10 Deployments by CPU** - Pie chart of CPU consumers
- **Top 10 Deployments by Memory** - Pie chart of memory consumers

### 🚀 Deployment Analytics
- **Deployment Status Distribution** - Running, queued, and failed deployments
- **Platform-wide Pod Count** - Current vs desired pods
- **Top 10 Deployments by Request Rate** - Most active deployments
- **Worker Job Completion Rate** - Job success/failure rates

### 🖥️ Worker Details & System Resources
- **Worker Inventory Table** - Complete worker details:
  - Worker ID and Name
  - Current Jobs (color-coded: green/yellow/red)
  - Pods managed
  - CPU Cores
  - Memory (GB)
- **CPU Cores by Worker** - Bar chart showing CPU allocation
- **Memory by Worker** - Bar chart showing memory allocation
- **Pods Deployed per Worker** - Distribution of pods
- **Active Jobs per Worker** - Current job load per worker

## Metrics Available

### Control-Plane Metrics (`localhost:8080/metrics`)

```
# Platform-level
platform_workers_total{status="idle|busy|offline"}
platform_worker_capacity{type="total|used|available"}
platform_deployments_total{status="running|failed|queued"}
platform_pods_total
platform_requests_total
platform_bandwidth_bytes_total{direction="sent|received"}

# Deployment-level
deployment_requests_total{deployment_id, status_code}
deployment_request_latency_seconds_bucket{deployment_id}
deployment_bandwidth_bytes_total{deployment_id, direction}
deployment_pods{deployment_id, type="current|desired"}
deployment_cpu_usage_percent{deployment_id}
deployment_memory_usage_mb{deployment_id}

# Worker-level
worker_pods_total{worker_id, worker_name}
worker_current_jobs{worker_id, worker_name}
worker_cpu_cores{worker_id, worker_name}
worker_memory_gb{worker_id, worker_name}

# Control-plane operations
control_plane_deploy_requests_total{scaling_mode}
control_plane_worker_jobs_total{status}
control_plane_worker_job_duration_seconds{status}
```

### Worker-Agent Metrics (`worker-ip:9090/metrics`)

```
# Job execution
worker_agent_jobs_processed_total{status="success|failed"}
worker_agent_job_duration_seconds{status}
worker_agent_active_jobs
worker_agent_pods_managed

# Heartbeats
worker_agent_heartbeats_sent_total
worker_agent_heartbeat_failures_total

# System resources
worker_agent_system_cpu_cores
worker_agent_system_memory_gb
```

## Dashboard Controls

### Time Range Selector (Top-right)
- Last 5 minutes
- Last 15 minutes
- Last 1 hour (default)
- Last 6 hours
- Last 24 hours
- Custom range

### Auto-Refresh (Top-right)
- **5 seconds** (default) - Real-time monitoring
- 10 seconds
- 30 seconds
- 1 minute
- 5 minutes
- Off

### Panel Interactions
- **Hover** - View detailed values at specific timestamps
- **Click Legend** - Toggle series visibility
- **Drag to Zoom** - Select time range on graph
- **Panel Menu** (top-left of each panel):
  - View
  - Edit
  - Share
  - Explore
  - Inspect → Data (view raw values)

## Troubleshooting

### No Data in Dashboard

**Check Prometheus is running:**
```bash
curl http://localhost:9090/-/healthy
# Should return: Prometheus is Healthy.
```

**Check Prometheus targets:**
```bash
# Open in browser
http://localhost:9090/targets
# All targets should show "UP"
```

**Check control-plane is exposing metrics:**
```bash
curl http://localhost:8080/metrics
# Should return Prometheus format metrics
```

### Grafana Can't Connect to Prometheus

**Verify datasource connection:**
1. Go to Configuration → Data Sources in Grafana
2. Click on "Prometheus"
3. Click "Save & Test"
4. Should show "Data source is working"

**Check Docker can reach WSL:**
```bash
# From inside Grafana container
docker exec -it observability_grafana curl http://host.docker.internal:9090/-/healthy
```

### Dashboard Not Loading

**Check dashboard files exist:**
```bash
ls -la infra/observability/grafana-dashboards/
# Should see: meshvpn-comprehensive.json, platform-overview.json, deployment-detail.json
```

**Check provisioning configuration:**
```bash
cat infra/observability/grafana-provisioning/dashboards/dashboards.yml
# Should have providers configured
```

**Restart Grafana:**
```bash
cd infra/observability
docker-compose restart grafana
docker-compose logs -f grafana
```

### Metrics Are Stale

**Check analytics collector is running:**
```bash
# In control-plane logs, look for:
# "starting metrics collector interval=30s"
```

**Manually trigger collection:**
The analytics collector runs every 30 seconds automatically.

## Advanced Configuration

### Add Worker Metrics to Prometheus

Edit `infra/observability/prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'worker-agents'
    scrape_interval: 10s
    static_configs:
      - targets:
          - 'worker1-tailscale-ip:9090'
          - 'worker2-tailscale-ip:9090'
        labels:
          service: 'worker-agent'
```

Restart Prometheus:
```bash
# Kill Prometheus (Ctrl+C) and restart
./prometheus --config.file=prometheus.yml
```

### Enable Grafana Authentication

Edit `infra/observability/docker-compose.yml`:

```yaml
grafana:
  environment:
    - GF_AUTH_ANONYMOUS_ENABLED=false
    - GF_AUTH_DISABLE_LOGIN_FORM=false
    - GF_SECURITY_ADMIN_USER=admin
    - GF_SECURITY_ADMIN_PASSWORD=your-secure-password
```

Restart Grafana:
```bash
docker-compose restart grafana
```

### Increase Prometheus Retention

Edit `prometheus.yml`:

```yaml
global:
  scrape_interval: 5s
  evaluation_interval: 5s

# Add storage configuration
storage:
  tsdb:
    retention.time: 30d  # Keep data for 30 days
    retention.size: 50GB  # Or max 50GB
```

### Export Dashboard

1. Open the dashboard
2. Click **Dashboard settings** (gear icon)
3. Click **JSON Model**
4. Copy JSON or click **Save to file**

### Import Dashboard

1. Click **+** → **Import**
2. Paste JSON or upload file
3. Select Prometheus datasource
4. Click **Import**

## Dashboard URLs

- **Comprehensive Dashboard**: http://localhost:3001/d/meshvpn-comprehensive
- **Platform Overview**: http://localhost:3001/d/meshvpn-platform-overview
- **Deployment Detail**: http://localhost:3001/d/meshvpn-deployment-detail
- **Prometheus UI**: http://localhost:9090
- **Prometheus Targets**: http://localhost:9090/targets
- **Prometheus Metrics**: http://localhost:8080/metrics

## Key Metrics Queries

### Top 5 Deployments by Request Rate
```promql
topk(5, sum(rate(deployment_requests_total[5m])) by (deployment_id))
```

### Average Response Time (Last Hour)
```promql
histogram_quantile(0.50, sum(rate(deployment_request_latency_seconds_bucket[1h])) by (le))
```

### Worker Utilization Percentage
```promql
(platform_worker_capacity{type="used"} / platform_worker_capacity{type="total"}) * 100
```

### Total Platform Bandwidth (MB/s)
```promql
sum(rate(deployment_bandwidth_bytes_total[5m])) / 1024 / 1024
```

### Deployment Count by Status
```promql
sum(platform_deployments_total) by (status)
```

## Best Practices

1. **Monitor Regularly** - Check dashboard daily for anomalies
2. **Set Up Alerts** - Configure Grafana alerts for critical metrics
3. **Track Trends** - Use longer time ranges to identify patterns
4. **Compare Periods** - Use time shift to compare current vs previous periods
5. **Export Reports** - Use Grafana's share/export features for reports
6. **Backup Dashboards** - Export dashboard JSON regularly

## What Metrics Tell You

### High CPU Usage (>90%)
- Scale deployments horizontally
- Optimize application code
- Consider larger package size

### High Memory Usage (>80%)
- Check for memory leaks
- Increase package memory limit
- Add more workers

### High Request Latency (p99 > 1s)
- Investigate slow endpoints
- Add caching
- Scale up resources

### Low Worker Utilization (<30%)
- Reduce worker count
- Consolidate workloads
- Review job placement strategy

### Failed Deployments Increasing
- Check worker health
- Review logs for errors
- Verify resource availability

## Support

For issues or questions:
- Check Grafana logs: `docker-compose logs grafana`
- Check Prometheus logs in WSL terminal
- Verify all services are running
- Review this guide's troubleshooting section

---

**Dashboard created**: Phase 3 Analytics Implementation
**Last updated**: 2026-03-25
**Version**: 1.0
