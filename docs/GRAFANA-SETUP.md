# Grafana Setup & Monitoring Guide

This guide explains how to set up and use Grafana dashboards for monitoring the MeshVPN platform.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Accessing Grafana](#accessing-grafana)
- [Pre-configured Dashboards](#pre-configured-dashboards)
- [Dashboard Features](#dashboard-features)
- [Creating Custom Dashboards](#creating-custom-dashboards)
- [Prometheus Queries](#prometheus-queries)
- [Troubleshooting](#troubleshooting)
- [Best Practices](#best-practices)

---

## Overview

MeshVPN includes Grafana for platform-wide observability and monitoring. The setup includes:

- **Prometheus**: Metrics collection and storage
- **Grafana**: Visualization and dashboarding
- **Auto-provisioning**: Dashboards and datasources configured automatically
- **Real-time updates**: 5-second refresh rate

---

## Quick Start

### 1. Start Observability Stack

```bash
cd infra/observability
docker-compose up -d
```

### 2. Access Grafana

Open your browser and navigate to:

```
http://localhost:3001
```

**No login required** - Anonymous access is enabled with Admin role for local development.

### 3. View Dashboards

Click **Dashboards** → **Browse** → **MeshVPN** folder to see:
- Platform Overview
- Deployment Detail

---

## Accessing Grafana

### Local Development

**URL**: `http://localhost:3001`

**Authentication**: Disabled (auto-login as Admin)

**Configuration**:
```yaml
environment:
  - GF_AUTH_ANONYMOUS_ENABLED=true
  - GF_AUTH_ANONYMOUS_ORG_ROLE=Admin
  - GF_AUTH_DISABLE_LOGIN_FORM=true
```

### Production Setup

For production, enable authentication:

```yaml
# infra/observability/docker-compose.yml
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

---

## Pre-configured Dashboards

### 1. Platform Overview

**Purpose**: High-level view of entire MeshVPN platform

**Location**: Dashboards → MeshVPN → Platform Overview

**Panels**:
- **Total Active Deployments** (Gauge): Count of running deployments
- **Platform Request Rate** (Time Series): Requests per second across all deployments
- **Platform Bandwidth Usage** (Time Series): Total sent/received bandwidth
- **Platform-wide Latency Percentiles** (Time Series): p50, p90, p99 latencies
- **Top 10 Deployments by Request Count** (Pie Chart): Busiest deployments
- **Top 10 Deployments by Bandwidth** (Pie Chart): Highest bandwidth consumers
- **Average CPU Usage** (Time Series): CPU utilization across all deployments
- **Average Memory Usage** (Time Series): Memory consumption
- **Platform Pod Distribution** (Time Series): Current vs desired pod counts

**Refresh Rate**: 5 seconds

**Time Range**: Last 6 hours (adjustable)

---

### 2. Deployment Detail

**Purpose**: Deep dive into individual deployment metrics

**Location**: Dashboards → MeshVPN → Deployment Detail

**Panels**:
- **Request Rate Over Time** (Time Series): Requests per second for selected deployment
- **Latency Percentiles** (Time Series): p50, p90, p99 latencies with thresholds
- **Status Code Distribution** (Donut Chart): HTTP status codes (2xx, 4xx, 5xx)
- **Error Rate** (Time Series): Percentage of 5xx responses
- **Pod Count** (Gauge): Current and desired replicas
- **Bandwidth Over Time** (Time Series): Sent and received bytes
- **CPU Usage** (Time Series): Percentage with warning/danger thresholds
- **Memory Usage** (Time Series): MB consumed with package limits shown

**Variables**:
- **Deployment ID** (Dropdown): Select specific deployment or "All"

**Refresh Rate**: 5 seconds

**Time Range**: Last 1 hour (adjustable)

---

## Dashboard Features

### Time Range Selector

Top-right corner allows selection of:
- Last 5 minutes
- Last 15 minutes
- Last 1 hour
- Last 6 hours
- Last 24 hours
- Custom range

### Refresh Rate

Auto-refresh dropdown (top-right):
- Off
- 5s (default)
- 10s
- 30s
- 1m
- 5m

### Panel Interactions

**Hover**: See detailed values at specific timestamps

**Click Legend**: Toggle series visibility

**Zoom**: Click and drag to zoom into time range

**Inspect**: Click panel title → Inspect → Data to see raw values

### Alerts (Optional)

Grafana supports alerting on panel queries. Example:

1. Edit panel
2. Click **Alert** tab
3. Configure conditions (e.g., "CPU > 90% for 5 minutes")
4. Set notification channel (Slack, email, etc.)

---

## Creating Custom Dashboards

### Using Prometheus Datasource

1. Click **+** → **Dashboard**
2. Click **Add visualization**
3. Select **Prometheus** datasource
4. Write PromQL query (see examples below)
5. Choose visualization type
6. Click **Save**

### Example: Custom Request Count Panel

**Query**:
```promql
sum(rate(deployment_requests_total{deployment_id="74b295d2"}[5m]))
```

**Visualization**: Time series

**Legend**: `{{deployment_id}}`

**Unit**: requests/sec (reqps)

---

## Prometheus Queries

### Deployment Metrics

#### Request Rate
```promql
# Total request rate across all deployments
sum(rate(deployment_requests_total[5m]))

# Request rate for specific deployment
sum(rate(deployment_requests_total{deployment_id="74b295d2"}[5m]))

# Request rate by status code
sum(rate(deployment_requests_total[5m])) by (status_code)
```

#### Latency Percentiles
```promql
# p50 latency (all deployments)
histogram_quantile(0.50, sum(rate(deployment_request_latency_seconds_bucket[5m])) by (le))

# p90 latency for specific deployment
histogram_quantile(0.90, sum(rate(deployment_request_latency_seconds_bucket{deployment_id="74b295d2"}[5m])) by (le))

# p99 latency
histogram_quantile(0.99, sum(rate(deployment_request_latency_seconds_bucket[5m])) by (le))
```

#### Bandwidth
```promql
# Total bandwidth sent (bytes/sec)
sum(rate(deployment_bandwidth_bytes_total{direction="sent"}[5m]))

# Bandwidth for specific deployment
sum(rate(deployment_bandwidth_bytes_total{deployment_id="74b295d2",direction="sent"}[5m]))
```

#### Pod Counts
```promql
# Current pods
deployment_pods{type="current"}

# Desired pods
deployment_pods{type="desired"}

# Pod count by deployment
deployment_pods{deployment_id="74b295d2",type="current"}
```

#### Resource Usage
```promql
# CPU usage percentage
deployment_cpu_usage_percent

# Memory usage in MB
deployment_memory_usage_mb

# Average CPU across all deployments
avg(deployment_cpu_usage_percent)
```

#### Error Rate
```promql
# Error rate (5xx / total)
sum(rate(deployment_requests_total{status_code=~"5.."}[5m])) / sum(rate(deployment_requests_total[5m])) * 100
```

### Control Plane Metrics

#### Queue Length
```promql
# Deployment queue length
deployment_queue_length

# Queue processing rate
rate(deployment_queue_processed_total[5m])
```

#### Worker Status
```promql
# Worker runs
rate(deployment_worker_runs_total[5m])

# Worker errors
rate(deployment_worker_errors_total[5m])
```

---

## Troubleshooting

### Grafana Not Starting

**Check logs**:
```bash
docker logs observability_grafana
```

**Common issues**:
- Port 3001 already in use → Change port in `docker-compose.yml`
- Permission issues → Check volume mount permissions
- Memory limits → Increase in `deploy.resources.limits.memory`

### "No data" in Panels

**Check Prometheus connection**:
1. Go to **Configuration** → **Data Sources**
2. Click **Prometheus**
3. Click **Test** button
4. Should show "Data source is working"

**If test fails**:
```bash
# Check Prometheus is running
docker ps | grep prometheus

# Test Prometheus endpoint
curl http://localhost:9090/-/healthy
```

### Dashboards Not Loading

**Check provisioning**:
```bash
# Verify dashboard files exist
ls -la infra/observability/grafana-dashboards/

# Check Grafana logs for provisioning errors
docker logs observability_grafana | grep -i provision
```

**Re-provision dashboards**:
```bash
docker-compose restart grafana
```

### Metrics Missing

**Verify Prometheus scraping**:
1. Open Prometheus: `http://localhost:9090`
2. Go to **Status** → **Targets**
3. Check control-plane target is "UP"

**Check control-plane metrics**:
```bash
curl http://localhost:8080/metrics
```

Should return Prometheus format metrics.

### High CPU/Memory in Grafana

**Reduce refresh rate**: Change from 5s to 30s or 1m

**Limit time range**: Use shorter ranges (1h instead of 24h)

**Optimize queries**: Use longer scrape intervals in queries (`[5m]` instead of `[30s]`)

---

## Best Practices

### Dashboard Organization

1. **Use Folders**: Organize related dashboards in folders
2. **Naming Convention**: Use prefix for environment (e.g., "PROD - Platform Overview")
3. **Tags**: Add tags like "meshvpn", "platform", "deployment" for easy search
4. **Star Favorites**: Star frequently-used dashboards for quick access

### Query Optimization

1. **Use Recording Rules**: Pre-aggregate expensive queries in Prometheus
2. **Limit Cardinality**: Avoid high-cardinality labels (use `deployment_id`, not `request_path`)
3. **Scrape Intervals**: Match query range to scrape interval (`[5m]` for 1m scrape)
4. **Resolution**: Use lower resolution for long time ranges

### Alerting Guidelines

1. **Define SLOs**: Set Service Level Objectives (e.g., "p99 < 500ms")
2. **Avoid Alert Fatigue**: Only alert on actionable issues
3. **Group Alerts**: Use `group_wait` to batch related alerts
4. **Escalation**: Configure multiple notification channels

### Resource Management

```yaml
# Adjust Grafana resources based on usage
grafana:
  deploy:
    resources:
      limits:
        cpus: '0.5'      # Increase if dashboards are slow
        memory: 400M      # Increase if OOM errors occur
```

### Data Retention

**Prometheus** (configured in `prometheus.yml`):
```yaml
global:
  storage.tsdb.retention.time: 15d  # Keep metrics for 15 days
  storage.tsdb.retention.size: 10GB # Or max 10GB
```

**Grafana** (dashboards persist in Docker volume):
```bash
# Backup Grafana data
docker run --rm -v observability_grafana-data:/data -v $(pwd):/backup busybox tar czf /backup/grafana-backup.tar.gz /data
```

---

## Exporting Dashboards

### Export as JSON

1. Open dashboard
2. Click **Dashboard settings** (gear icon)
3. Click **JSON Model**
4. Copy JSON or click **Save to file**

### Import Dashboard

1. Click **+** → **Import**
2. Paste JSON or upload file
3. Select Prometheus datasource
4. Click **Import**

### Share Dashboard Link

1. Click **Share** icon (next to star)
2. Select **Link** tab
3. Configure options:
   - Lock time range
   - Shorten URL
   - Theme (light/dark)
4. Copy link

---

## Advanced Configuration

### Enable Authentication

```yaml
# infra/observability/docker-compose.yml
grafana:
  environment:
    - GF_AUTH_ANONYMOUS_ENABLED=false
    - GF_SECURITY_ADMIN_USER=admin
    - GF_SECURITY_ADMIN_PASSWORD=${GRAFANA_ADMIN_PASSWORD}
    - GF_USERS_ALLOW_SIGN_UP=false
```

### Configure SMTP for Alerts

```yaml
grafana:
  environment:
    - GF_SMTP_ENABLED=true
    - GF_SMTP_HOST=smtp.gmail.com:587
    - GF_SMTP_USER=your-email@gmail.com
    - GF_SMTP_PASSWORD=${SMTP_PASSWORD}
    - GF_SMTP_FROM_ADDRESS=your-email@gmail.com
```

### Install Plugins

```yaml
grafana:
  environment:
    - GF_INSTALL_PLUGINS=grafana-piechart-panel,grafana-worldmap-panel
```

---

## Related Documentation

- [Analytics API](./ANALYTICS-API.md) - User-facing analytics endpoints
- [Packages](./PACKAGES.md) - Resource package specifications
- [Setup Guide](./SETUP.md) - Platform setup instructions

---

## Support

For Grafana-related questions or dashboard contributions, please open an issue at: https://github.com/anthropics/claude-code/issues
