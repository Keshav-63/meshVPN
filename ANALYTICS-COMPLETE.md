# Complete Analytics Guide - Platform + Deployment Level

## Overview

MeshVPN has **TWO levels of analytics**:

1. **Platform-Level Analytics** → Prometheus metrics + Grafana dashboards
   - Shows aggregate stats across ALL workers and deployments
   - Worker capacity, total pods, total requests, latency, bandwidth
   - Per-worker breakdown (CPU, memory, current jobs, pods)

2. **Deployment-Level Analytics** → REST API + Prometheus + Grafana
   - Individual deployment metrics
   - Request count, latency percentiles, bandwidth, pod status
   - CPU/memory usage per deployment

---

## 1. Platform-Level Analytics (Prometheus + Grafana)

### Start Control-Plane

```bash
cd ~/MeshVPN-slef-hosting
./start-control-plane.sh
```

**Control-plane will automatically:**
- Collect platform metrics every 1 minute
- Export metrics to Prometheus at `:8080/metrics`
- Update worker stats, deployment counts, capacity utilization

### Access Grafana Dashboard

```bash
# Open Grafana
http://localhost:3001

# Login (default credentials)
Username: admin
Password: admin
```

**Dashboard Location:**
- Go to **Dashboards** → **MeshVPN Platform Overview**
- Auto-refreshes every 5 seconds

### Platform Metrics Available

#### Worker Metrics

| Metric | Description | Type |
|--------|-------------|------|
| `platform_workers_total{status="idle"}` | Number of idle workers | Gauge |
| `platform_workers_total{status="busy"}` | Number of busy workers | Gauge |
| `platform_workers_total{status="offline"}` | Number of offline workers | Gauge |
| `platform_worker_capacity{type="total"}` | Total worker job capacity | Gauge |
| `platform_worker_capacity{type="used"}` | Currently used capacity | Gauge |
| `platform_worker_capacity{type="available"}` | Available capacity | Gauge |

#### Deployment Metrics

| Metric | Description | Type |
|--------|-------------|------|
| `platform_deployments_total{status="running"}` | Running deployments | Gauge |
| `platform_deployments_total{status="failed"}` | Failed deployments | Gauge |
| `platform_deployments_total{status="queued"}` | Queued deployments | Gauge |
| `platform_pods_total` | Total pods across all deployments | Gauge |

#### Traffic Metrics

| Metric | Description | Type |
|--------|-------------|------|
| `platform_requests_total` | Total requests across all deployments | Counter |
| `platform_bandwidth_bytes_total{direction="sent"}` | Total bandwidth sent | Counter |
| `platform_bandwidth_bytes_total{direction="received"}` | Total bandwidth received | Counter |

#### Per-Worker Metrics

| Metric | Description | Labels | Type |
|--------|-------------|--------|------|
| `worker_current_jobs` | Current jobs per worker | worker_id, worker_name | Gauge |
| `worker_pods_total` | Total pods per worker's cluster | worker_id, worker_name | Gauge |
| `worker_cpu_cores` | CPU cores available | worker_id, worker_name | Gauge |
| `worker_memory_gb` | Memory in GB available | worker_id, worker_name | Gauge |

### Example Prometheus Queries

```promql
# Total workers
sum(platform_workers_total)

# Worker capacity utilization percentage
(platform_worker_capacity{type="used"} / platform_worker_capacity{type="total"}) * 100

# Total running deployments
platform_deployments_total{status="running"}

# Total pods
platform_pods_total

# Platform request rate (requests/sec)
rate(platform_requests_total[5m])

# Platform bandwidth (bytes/sec)
rate(platform_bandwidth_bytes_total{direction="sent"}[5m])

# Workers by status
platform_workers_total{status="idle"}
platform_workers_total{status="busy"}

# Per-worker current jobs
worker_current_jobs

# Workers with most pods
topk(5, worker_pods_total)

# Total CPU cores across all workers
sum(worker_cpu_cores)

# Total memory across all workers
sum(worker_memory_gb)
```

### Grafana Dashboard Panels

**Platform Overview Dashboard** includes:

1. **Top Row Stats:**
   - Total Workers
   - Idle Workers
   - Busy Workers
   - Running Deployments
   - Total Pods
   - Total Requests

2. **Worker Capacity Utilization Chart:**
   - Total capacity vs used capacity over time

3. **Platform Request Rate:**
   - Requests/sec across all deployments

4. **Platform Bandwidth:**
   - Sent/received bytes over time

5. **Latency Percentiles:**
   - p50, p90, p99 latency across all deployments

6. **Top Deployments:**
   - By request count
   - By bandwidth usage

7. **Worker Details Table:**
   - Worker ID, Name, Current Jobs, Pods, CPU Cores, Memory (GB)

---

## 2. Deployment-Level Analytics

### REST API Endpoints

#### Get Deployment Analytics (Snapshot)

```bash
GET /deployments/:id/analytics
Authorization: Bearer <token>
```

**Response:**
```json
{
  "deployment_id": "d48b634c",
  "metrics": {
    "deployment_id": "d48b634c",
    "request_count_total": 15423,
    "request_count_1h": 340,
    "request_count_24h": 8932,
    "requests_per_second": 0.094,
    "bandwidth_sent_bytes": 45892345,
    "bandwidth_recv_bytes": 8934567,
    "latency_p50_ms": 45.3,
    "latency_p90_ms": 123.7,
    "latency_p99_ms": 456.2,
    "current_pods": 3,
    "desired_pods": 3,
    "cpu_usage_percent": 42.5,
    "memory_usage_mb": 512.3,
    "last_updated": "2026-03-22T10:45:00Z"
  },
  "deployment": {
    "deployment_id": "d48b634c",
    "subdomain": "my-app",
    "status": "running",
    "package": "medium",
    "scaling_mode": "horizontal",
    "min_replicas": 1,
    "max_replicas": 5
  }
}
```

#### Stream Analytics (Server-Sent Events)

```bash
GET /deployments/:id/analytics/stream
Authorization: Bearer <token>
```

**Receives updates every 5 seconds:**
```
data: {"deployment_id":"d48b634c","request_count_total":15423,...}

data: {"deployment_id":"d48b634c","request_count_total":15450,...}
```

**Frontend Example:**
```javascript
const eventSource = new EventSource(
  'http://localhost:8080/deployments/d48b634c/analytics/stream',
  {
    headers: { Authorization: `Bearer ${token}` }
  }
);

eventSource.onmessage = (event) => {
  const metrics = JSON.parse(event.data);
  console.log('Real-time metrics:', metrics);
  updateDashboard(metrics);
};
```

### Deployment Metrics in Prometheus

| Metric | Description | Labels | Type |
|--------|-------------|--------|------|
| `deployment_requests_total` | Total requests per deployment | deployment_id, status_code | Counter |
| `deployment_request_latency_seconds` | Request latency histogram | deployment_id | Histogram |
| `deployment_bandwidth_bytes_total` | Bandwidth transferred | deployment_id, direction | Counter |
| `deployment_pods` | Pod counts | deployment_id, type (current/desired) | Gauge |
| `deployment_cpu_usage_percent` | CPU usage | deployment_id | Gauge |
| `deployment_memory_usage_mb` | Memory usage | deployment_id | Gauge |

### Example Prometheus Queries (Per-Deployment)

```promql
# Request rate for specific deployment
rate(deployment_requests_total{deployment_id="d48b634c"}[5m])

# Latency p50 for specific deployment
histogram_quantile(0.50, rate(deployment_request_latency_seconds_bucket{deployment_id="d48b634c"}[5m]))

# Bandwidth sent for specific deployment
rate(deployment_bandwidth_bytes_total{deployment_id="d48b634c",direction="sent"}[5m])

# Current pods
deployment_pods{deployment_id="d48b634c",type="current"}

# CPU usage
deployment_cpu_usage_percent{deployment_id="d48b634c"}

# All deployments with high latency (p99 > 500ms)
deployment_request_latency_seconds{quantile="0.99"} > 0.5

# Top 5 deployments by request rate
topk(5, sum(rate(deployment_requests_total[5m])) by (deployment_id))
```

### Grafana Dashboard (Deployment Detail)

**Access:**
- Dashboard: **MeshVPN Deployment Detail**
- Filter by deployment_id variable

**Panels:**
- Request rate over time
- Latency percentiles (p50, p90, p99)
- Status code distribution
- Bandwidth (sent/received)
- Pod count (current vs desired)
- CPU usage per pod
- Memory usage per pod

---

## 3. Platform Analytics API (NEW)

### Get Complete Platform Analytics

```bash
GET /platform/analytics
Authorization: Bearer <token>
```

**Response:**
```json
{
  "platform": {
    "deployments": {
      "total": 12,
      "running": 10,
      "failed": 1,
      "queued": 1
    },
    "workers": {
      "total": 3,
      "idle": 1,
      "busy": 2,
      "offline": 0
    },
    "capacity": {
      "total": 6,
      "used": 3,
      "available": 3,
      "utilization_percent": 50.0
    },
    "resources": {
      "total_pods": 23
    },
    "traffic": {
      "total_requests": 145234,
      "bandwidth_sent_bytes": 458923456,
      "bandwidth_recv_bytes": 89345678,
      "avg_latency_p50_ms": 52.3
    }
  },
  "workers": [
    {
      "worker_id": "control-plane-local",
      "name": "Control-Plane (Local Worker)",
      "status": "busy",
      "current_jobs": 1,
      "max_concurrent_jobs": 2,
      "deployment_count": 0,
      "cpu_cores": 0,
      "memory_gb": 0,
      "last_heartbeat": "2026-03-22T10:45:00Z"
    },
    {
      "worker_id": "worker-laptop-1",
      "name": "My Laptop",
      "status": "idle",
      "current_jobs": 0,
      "max_concurrent_jobs": 2,
      "deployment_count": 0,
      "cpu_cores": 8,
      "memory_gb": 16,
      "last_heartbeat": "2026-03-22T10:44:58Z"
    }
  ]
}
```

### Get Worker-Specific Analytics

```bash
GET /platform/workers/:worker_id/analytics
Authorization: Bearer <token>
```

**Response:**
```json
{
  "worker": {
    "worker_id": "worker-laptop-1",
    "name": "My Laptop",
    "tailscale_ip": "100.64.1.2",
    "status": "idle",
    "current_jobs": 0,
    "max_concurrent_jobs": 2,
    "last_heartbeat": "2026-03-22T10:45:00Z"
  },
  "resources": {
    "total_pods": 5,
    "total_requests": 23456,
    "cpu_cores": 8,
    "memory_gb": 16
  },
  "deployments": [
    {
      "deployment_id": "d48b634c",
      "subdomain": "my-app",
      "package": "medium",
      "current_pods": 3,
      "request_count": 15423,
      "cpu_percent": 42.5,
      "memory_mb": 512.3
    },
    {
      "deployment_id": "e5ddae78",
      "subdomain": "another-app",
      "package": "small",
      "current_pods": 2,
      "request_count": 8033,
      "cpu_percent": 18.2,
      "memory_mb": 256.7
    }
  ]
}
```

---

## 4. Complete Setup Steps

### Step 1: Start Control-Plane

```bash
cd ~/MeshVPN-slef-hosting
./start-control-plane.sh
```

**Verify it's running:**
```bash
curl http://localhost:8080/health
```

**Expected:**
```json
{"status":"LaptopCloud running"}
```

### Step 2: Start Observability Stack

```bash
cd infra/observability
docker compose up -d
```

**Services started:**
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3001

### Step 3: Verify Prometheus is Scraping

```bash
# Open Prometheus
http://localhost:9090

# Go to Status → Targets
# Verify "control-plane" target is UP
```

**Expected:**
- Endpoint: `control-plane-proxy:8080/metrics`
- State: UP
- Last Scrape: < 5s ago

### Step 4: Access Grafana Dashboard

```bash
# Open Grafana
http://localhost:3001

# Login
Username: admin
Password: admin

# Go to Dashboards → MeshVPN Platform Overview
```

**You should see:**
- Total workers, idle/busy counts
- Running deployments, total pods
- Platform request rate chart
- Worker capacity utilization
- Worker details table

### Step 5: Test Platform Analytics API

```bash
# Get platform analytics
curl http://localhost:8080/platform/analytics

# Get worker-specific analytics
curl http://localhost:8080/platform/workers/control-plane-local/analytics
```

---

## 5. Metrics Collection Timeline

| Interval | What Happens |
|----------|--------------|
| Every 1 minute | Analytics collector aggregates metrics from all deployments and workers |
| Every 5 seconds | Prometheus scrapes /metrics endpoint from control-plane |
| Every 5 seconds | Grafana dashboard auto-refreshes |
| Every 5 seconds | SSE streams send updates to connected clients |

---

## 6. Troubleshooting

### Prometheus shows no data

**Check:**
```bash
# 1. Control-plane is running
curl http://localhost:8080/metrics

# 2. Should see metrics like:
# platform_workers_total{status="idle"} 1
# platform_deployments_total{status="running"} 2
# platform_pods_total 5

# 3. Check Prometheus targets
http://localhost:9090/targets
# control-plane target should be UP
```

### Grafana dashboard is empty

**Fix:**
```bash
# 1. Check Prometheus data source in Grafana
# Settings → Data Sources → Prometheus
# URL should be: http://prometheus:9090

# 2. Test connection (click "Save & Test")

# 3. Go to Explore tab, run query:
platform_workers_total

# Should return data
```

### Analytics collector not running

**Check control-plane logs:**
```bash
# Should see:
[INFO] [main] analytics collector started interval=1m

# Every minute:
[INFO] [analytics-collector] processing 5 active deployments
```

**If not running, check .env:**
```env
DATABASE_URL=postgresql://...  # Must be set
```

---

## 7. Summary

### Platform-Level Analytics (Aggregated)
✅ **Access:** Grafana Dashboard (http://localhost:3001)
✅ **Metrics:** All in Prometheus
✅ **Shows:** Workers, deployments, capacity, total requests, total bandwidth, total pods
✅ **Per-Worker:** Current jobs, pods, CPU, memory

### Deployment-Level Analytics (Individual)
✅ **Access:** REST API + Grafana Dashboard
✅ **Metrics:** All in Prometheus + Database
✅ **Shows:** Requests, latency, bandwidth, pods, CPU/memory per deployment
✅ **Real-time:** SSE streaming endpoint

### All Metrics Exported to Prometheus
✅ Platform metrics: `platform_*`
✅ Worker metrics: `worker_*`
✅ Deployment metrics: `deployment_*`
✅ Control-plane metrics: `control_plane_*`

**Everything is in Prometheus and Grafana - NO HTML dashboards needed!**
