# MeshVPN Platform - System Architecture

Complete system architecture, data flow, and component interactions.

---

## Table of Contents

1. [System Overview](#system-overview)
2. [Architecture Diagram](#architecture-diagram)
3. [Core Components](#core-components)
4. [Data Flow](#data-flow)
5. [Traffic Tracking Flow](#traffic-tracking-flow)
6. [Database Schema](#database-schema)
7. [Deployment Lifecycle](#deployment-lifecycle)
8. [Scaling & Monitoring](#scaling--monitoring)

---

## System Overview

MeshVPN is a self-hosted PaaS platform for deploying web applications with:
- ✅ **Automated deployments** from Git repositories
- ✅ **Horizontal pod autoscaling** based on CPU utilization
- ✅ **Real-time analytics** with request tracking and metrics
- ✅ **Multi-worker architecture** for distributed builds
- ✅ **Traffic monitoring** via Traefik access logs
- ✅ **Kubernetes-native** runtime on k3d/k3s

---

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                         User's Browser                               │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │
│  │  Dashboard   │  │  Deployment  │  │   Logs &     │              │
│  │   (React)    │  │   Details    │  │  Analytics   │              │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘              │
│         │                  │                  │                      │
│         └──────────────────┴──────────────────┘                      │
│                            │                                         │
│                    HTTPS (Supabase JWT)                              │
└─────────────────────────────┬───────────────────────────────────────┘
                              │
┌─────────────────────────────▼───────────────────────────────────────┐
│                      Control-Plane (Go)                              │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │  HTTP API (Gin Router)                                         │ │
│  │  - /deploy, /deployments, /analytics                           │ │
│  │  - /api/telemetry/deployment-request                           │ │
│  │  - SSE streaming for real-time metrics                         │ │
│  └────────────┬───────────────────────────────┬───────────────────┘ │
│               │                               │                     │
│  ┌────────────▼───────────┐     ┌─────────────▼──────────────────┐ │
│  │  Job Distributor       │     │  Metrics Collector             │ │
│  │  - Local-first strategy│     │  - Runs every 1 minute         │ │
│  │  - Queues deployments  │     │  - Queries Kubernetes          │ │
│  └────────────┬───────────┘     │  - Aggregates request data     │ │
│               │                 └─────────────┬──────────────────┘ │
└───────────────┼───────────────────────────────┼────────────────────┘
                │                               │
                │                               │
┌───────────────▼───────────────────────────────▼────────────────────┐
│                      PostgreSQL Database                            │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────────┐ │
│  │ deployments  │  │  deployment_ │  │  deployment_metrics      │ │
│  │              │  │  requests    │  │                          │ │
│  └──────────────┘  └──────────────┘  └──────────────────────────┘ │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────────┐ │
│  │  workers     │  │    jobs      │  │      users               │ │
│  │              │  │              │  │                          │ │
│  └──────────────┘  └──────────────┘  └──────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                  Worker Agents (Local/Remote)                        │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │  1. Poll for jobs                                              │ │
│  │  2. Clone Git repo                                             │ │
│  │  3. Build Docker image (Buildpacks/Dockerfile)                 │ │
│  │  4. Push to registry                                           │ │
│  │  5. Report completion                                          │ │
│  └────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│             Kubernetes Cluster (k3d/k3s)                             │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │  Traefik Ingress (Routes traffic)                             │ │
│  │  - Access logs in JSON format                                  │ │
│  │  - Middleware for telemetry                                    │ │
│  └────────────┬───────────────────────────────────────────────────┘ │
│               │                                                     │
│  ┌────────────▼───────────────────────────────────────────────────┐ │
│  │  Deployed Applications (Pods)                                  │ │
│  │  - app-{deployment_id}-xxxxx                                   │ │
│  │  - Horizontal Pod Autoscaler                                   │ │
│  │  - Services with LoadBalancer                                  │ │
│  └─────────────────────────────────────────────────────────────────┘ │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────────┐ │
│  │  Traffic Forwarder (Pod)                                        │ │
│  │  - Tails Traefik access logs                                    │ │
│  │  - Parses JSON, extracts metrics                                │ │
│  │  - POSTs to /api/telemetry/deployment-request                   │ │
│  └─────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                   Bridge Proxy (WSL Only)                            │
│  - Forwards k3d → WSL localhost                                     │
│  - host.docker.internal:8081 → localhost:8080                       │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                   External Services                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────────┐  │
│  │  Supabase    │  │  GitHub      │  │  Docker Registry         │  │
│  │  (Auth)      │  │  (Repos)     │  │  (Images)                │  │
│  └──────────────┘  └──────────────┘  └──────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Core Components

### 1. Control-Plane (Go)

**Location**: `control-plane/`

**Responsibilities:**
- HTTP API server (Gin framework)
- Job distribution and scheduling
- Metrics collection and aggregation
- SSE streaming for real-time updates
- Database management (PostgreSQL)

**Key Services:**
- `DeploymentService` - Manages deployments
- `JobDistributor` - Assigns jobs to workers
- `MetricsCollector` - Aggregates analytics (runs every minute)
- `DeploymentDetailsService` - Comprehensive deployment data
- `AnalyticsHandler` - Serves analytics endpoints

**Config:**
- Port: `8080`
- Database: PostgreSQL
- Kubernetes: kubectl via shell execution
- Auth: Supabase JWT validation

---

### 2. Worker Agents

**Location**: `worker-agent/`

**Responsibilities:**
- Poll control-plane for pending jobs
- Clone Git repositories
- Build Docker images (Buildpacks/Dockerfile)
- Push to container registry
- Deploy to Kubernetes
- Report job status

**Types:**
- **Local Worker** (embedded in control-plane)
- **Remote Workers** (separate processes)

**Worker States:**
- `idle` - Ready for jobs
- `busy` - Processing job
- `offline` - Not responding to heartbeats

---

### 3. Traffic Forwarder

**Location**: `tools/traffic-forwarder/`

**Responsibilities:**
- Tail Traefik access logs via `kubectl logs -f`
- Parse JSON log entries
- Extract deployment metrics (subdomain, status, latency, bytes)
- POST to `/api/telemetry/deployment-request`

**Deployment:**
- Runs as Kubernetes pod
- RBAC permissions for log access
- Connects via bridge proxy (WSL) or direct (production)

---

### 4. Bridge Proxy (WSL Only)

**Location**: `tools/traffic-forwarder/bridge-proxy.go`

**Purpose:**
- Solves k3d → WSL localhost networking
- Listens on `0.0.0.0:8081`
- Forwards to `localhost:8080` (control-plane)

**Why needed:**
- Docker Desktop WSL2 isolation
- k3d pods can't reach WSL localhost directly
- `host.docker.internal:8081` → proxy → control-plane

---

### 5. Metrics Collector

**Runs in**: Control-Plane (background goroutine)

**Schedule**: Every 60 seconds

**Process:**
1. Get all active deployments from database
2. For each deployment:
   - Query Kubernetes for pod status (kubectl)
   - Get CPU/memory usage from `kubectl top`
   - Calculate request counts from `deployment_requests` table
   - Calculate latency percentiles (p50, p90, p99)
   - Calculate bandwidth (sent/received bytes)
3. Update `deployment_metrics` table
4. Update Prometheus metrics

**Performance:**
- Kubernetes queries cached for 12 seconds
- Parallel processing for multiple deployments
- Graceful error handling (failed queries don't stop collector)

---

### 6. Kubernetes Runtime

**Components:**
- **Traefik**: Ingress controller, access logs
- **Deployments**: User applications (`app-{id}`)
- **Services**: LoadBalancer for each app (`svc-{id}`)
- **HPA**: Horizontal Pod Autoscaler (if enabled)
- **Traffic Forwarder**: Telemetry collection pod

**Namespace**: `meshvpn-apps`

---

## Data Flow

### Deployment Creation Flow

```
1. User submits deployment form
   ↓
2. Frontend → POST /deploy
   ↓
3. Control-Plane validates request
   ↓
4. Creates deployment record (status: queued)
   ↓
5. Creates job record
   ↓
6. Job Distributor assigns to worker
   ↓
7. Worker claims job → status: building
   ↓
8. Worker clones repo, builds image, pushes to registry
   ↓
9. Worker creates Kubernetes resources
   ↓
10. Deployment status → running
    ↓
11. Frontend polls GET /deployments/:id for status
```

---

### Traffic Tracking Flow

```
1. User requests https://myapp.keshavstack.tech
   ↓
2. Traefik routes request to app pod
   ↓
3. Traefik writes JSON access log:
   {
     "ClientHost": "1.2.3.4",
     "RequestHost": "myapp.keshavstack.tech",
     "OriginStatus": 200,
     "Duration": 45000,  // microseconds
     "OriginContentSize": 1024,
     "RequestContentSize": 512
   }
   ↓
4. Traffic Forwarder tails logs via kubectl
   ↓
5. Parses JSON, extracts subdomain ("myapp")
   ↓
6. POSTs to control-plane:
   POST /api/telemetry/deployment-request
   {
     "deployment_id": "myapp",  // subdomain
     "status_code": 200,
     "latency_ms": 45.0,
     "bytes_sent": 1024,
     "bytes_received": 512,
     "path": "/",
     "timestamp": "2026-03-29T12:30:45Z"
   }
   ↓
7. Control-Plane resolves subdomain → deployment_id
   ↓
8. Inserts into deployment_requests table
   ↓
9. Metrics Collector aggregates (every minute)
   ↓
10. Updates deployment_metrics table
    ↓
11. Frontend queries GET /deployments/:id/analytics
    ↓
12. Shows real-time request counts, latency, bandwidth
```

---

### Real-Time Metrics Streaming Flow

```
1. Frontend opens SSE connection
   GET /deployments/:id/analytics/stream
   ↓
2. Control-Plane creates EventSource handler
   ↓
3. Every 5 seconds:
   a. Query deployment_metrics table
   b. Format as JSON
   c. Send SSE event:
      event: message
      data: {"requests": {...}, "latency": {...}}
   ↓
4. Frontend receives event
   ↓
5. Updates charts/gauges in real-time
```

---

## Database Schema

### deployments

| Column | Type | Description |
|--------|------|-------------|
| deployment_id | TEXT PRIMARY KEY | UUID |
| user_id | TEXT | Owner (Supabase user ID) |
| repo | TEXT | Git repository URL |
| subdomain | TEXT UNIQUE | Subdomain (e.g., "myapp") |
| url | TEXT | Full URL (https://myapp.keshavstack.tech) |
| status | TEXT | queued, building, running, failed, stopped |
| package | TEXT | nano, small, medium, large |
| scaling_mode | TEXT | none, horizontal |
| min_replicas | INT | HPA min (default: 1) |
| max_replicas | INT | HPA max (default: 3) |
| cpu_target_utilization | INT | HPA target CPU% (default: 70) |
| cpu_cores | REAL | Allocated CPU |
| memory_mb | INT | Allocated memory |
| env | JSONB | Environment variables |
| started_at | TIMESTAMP | Deployment start time |
| finished_at | TIMESTAMP | Deployment end time |

---

### deployment_requests

**Purpose**: Raw request logs (TTL: 7 days)

| Column | Type | Description |
|--------|------|-------------|
| id | SERIAL PRIMARY KEY | Auto-increment |
| deployment_id | TEXT | References deployments |
| timestamp | TIMESTAMP | Request time |
| status_code | INT | HTTP status |
| latency_ms | REAL | Response time |
| bytes_sent | BIGINT | Response size |
| bytes_received | BIGINT | Request size |
| path | TEXT | Request path |

**Indexes:**
- `deployment_id, timestamp DESC` - For percentile queries
- `timestamp DESC` - For cleanup

---

### deployment_metrics

**Purpose**: Aggregated metrics (updated every minute)

| Column | Type | Description |
|--------|------|-------------|
| deployment_id | TEXT PRIMARY KEY | References deployments |
| request_count_total | BIGINT | All-time requests |
| request_count_1h | BIGINT | Last hour |
| request_count_24h | BIGINT | Last 24 hours |
| requests_per_second | REAL | Current rate |
| latency_p50_ms | REAL | Median latency |
| latency_p90_ms | REAL | 90th percentile |
| latency_p99_ms | REAL | 99th percentile |
| bandwidth_sent_bytes | BIGINT | Total sent |
| bandwidth_recv_bytes | BIGINT | Total received |
| current_pods | INT | Running pods |
| desired_pods | INT | HPA desired |
| cpu_usage_percent | REAL | Avg CPU% |
| memory_usage_mb | REAL | Avg memory |
| last_updated | TIMESTAMP | Last aggregation |

---

### workers

| Column | Type | Description |
|--------|------|-------------|
| worker_id | TEXT PRIMARY KEY | UUID |
| name | TEXT | Worker name |
| status | TEXT | idle, busy, offline |
| max_concurrent_jobs | INT | Job capacity |
| cpu_cores | INT | Worker CPU |
| memory_gb | INT | Worker memory |
| last_heartbeat | TIMESTAMP | Last ping |
| created_at | TIMESTAMP | Registration time |

---

### jobs

| Column | Type | Description |
|--------|------|-------------|
| job_id | TEXT PRIMARY KEY | UUID |
| deployment_id | TEXT | References deployments |
| worker_id | TEXT | Assigned worker |
| status | TEXT | queued, building, completed, failed |
| started_at | TIMESTAMP | Job start |
| finished_at | TIMESTAMP | Job completion |
| error | TEXT | Error message (if failed) |

---

## Deployment Lifecycle

### States

```
queued → building → running
           ↓
         failed
```

### State Transitions

| From | To | Trigger |
|------|----|----|
| - | queued | User creates deployment |
| queued | building | Worker claims job |
| building | running | Kubernetes deployment succeeds |
| building | failed | Build/deploy error |
| running | stopped | User stops deployment |

---

## Scaling & Monitoring

### Horizontal Pod Autoscaling (HPA)

**Configuration** (per deployment):
- `scaling_mode: "horizontal"`
- `min_replicas: 1`
- `max_replicas: 3`
- `cpu_target_utilization: 70`

**How it works:**
1. Kubernetes HPA monitors pod CPU usage
2. If avg CPU > 70%, scale up (add pods)
3. If avg CPU < 70%, scale down (remove pods)
4. Min/max bounds enforced

**Frontend display:**
- Current pods: 2
- Desired pods: 2
- HPA enabled: ✅

---

### Metrics Collection

**Sources:**
1. **Kubernetes API** (via kubectl):
   - Pod status, restarts, age
   - CPU/memory usage (`kubectl top`)
   - Resource requests/limits

2. **Database** (deployment_requests table):
   - Request counts (total, 1h, 24h)
   - Latency percentiles (p50, p90, p99)
   - Bandwidth (sent/received bytes)

3. **Prometheus** (optional):
   - `/metrics` endpoint
   - Deployment request counters
   - Latency histograms

---

### Performance Optimizations

1. **Caching**:
   - Kubernetes queries: 12-second cache
   - Deployment metrics: Updated every minute

2. **Parallel Processing**:
   - Multiple deployments aggregated concurrently
   - Goroutines for independent queries

3. **Database Indexes**:
   - `(deployment_id, timestamp)` for request queries
   - `timestamp DESC` for cleanup

4. **Data Retention**:
   - Raw requests: 7 days
   - Aggregated metrics: Forever

---

## File Structure

```
MeshVPN-slef-hosting/
├── control-plane/          # Main control plane
│   ├── cmd/control-plane/  # Entry point
│   ├── internal/
│   │   ├── httpapi/        # HTTP handlers & routing
│   │   ├── service/        # Business logic
│   │   ├── store/          # Database repositories
│   │   ├── analytics/      # Metrics collector, K8s client
│   │   ├── runtime/        # Kubernetes driver
│   │   ├── auth/           # Supabase middleware
│   │   └── domain/         # Data models
│   └── migrations/         # SQL migrations
│
├── worker-agent/           # Deployment worker
│   ├── main.go
│   ├── worker.go
│   └── builder.go
│
├── tools/traffic-forwarder/# Traffic metrics collection
│   ├── main.go             # Forwarder code
│   ├── bridge-proxy.go     # WSL networking bridge
│   └── deploy.sh           # Automated deployment
│
├── docs/                   # Documentation
│   ├── FRONTEND-INTEGRATION.md     # Frontend guide (NEW)
│   ├── API-QUICK-REFERENCE.md      # API reference (NEW)
│   ├── SYSTEM-ARCHITECTURE.md      # This file (NEW)
│   ├── DEPLOYMENT-ANALYTICS-API.md # Analytics details
│   └── ...
│
├── scripts/                # Utilities
│   ├── setup-k3d-cluster.sh
│   └── check-workers.sh
│
└── start-control-plane.sh  # Main startup script
```

---

## Security

### Authentication

- **Supabase JWT** for all protected endpoints
- **Row-Level Security** (deployments owned by user)
- **Worker API** (no auth - internal only)
- **Telemetry endpoint** (no auth - called by forwarder)

### Network Security

- Control-plane binds to `0.0.0.0:8080` (exposed)
- Kubernetes API access via kubeconfig
- Registry credentials in environment variables
- Secrets stored in Kubernetes Secrets

---

## Monitoring & Observability

### Metrics

- **Prometheus** endpoint: `/metrics`
- **Grafana** dashboards (optional)
- **kubectl top** for resource usage
- **Traefik access logs** for traffic

### Logs

- **Build logs**: Stored in database
- **Application logs**: `kubectl logs`
- **Control-plane logs**: stdout (structured JSON)
- **Worker logs**: stdout

---

## Next Steps for Frontend

1. **Dashboard Page**:
   - List deployments with summary metrics
   - Status badges, quick actions

2. **Deployment Details Page**:
   - Use `/deployments/:id` for complete data
   - Show metrics charts (requests, latency, bandwidth)
   - Pod status table
   - Resource utilization bars
   - Scaling configuration

3. **Create Deployment Form**:
   - Repo URL, subdomain, package selector
   - Environment variables input
   - Build args (optional)

4. **Logs Viewer**:
   - Build logs during deployment
   - Application logs for debugging
   - Terminal-style UI with auto-scroll

5. **Real-Time Metrics**:
   - SSE connection for live updates
   - Animated charts/gauges
   - Live badge (connected indicator)

6. **Platform Analytics** (Admin):
   - Worker grid (status, capacity)
   - Platform-wide resource usage
   - Deployment distribution pie chart

---

## Summary

**Key Features:**
- ✅ Comprehensive deployment analytics (requests, latency, bandwidth, pods)
- ✅ Real-time metrics streaming via SSE
- ✅ Per-pod resource monitoring
- ✅ Traffic tracking via Traefik logs
- ✅ Multi-worker distributed builds
- ✅ Horizontal pod autoscaling
- ✅ Complete frontend API

**Frontend Integration:**
- See [FRONTEND-INTEGRATION.md](./FRONTEND-INTEGRATION.md)
- See [API-QUICK-REFERENCE.md](./API-QUICK-REFERENCE.md)

**Traffic Tracking:**
- Automated via traffic-forwarder
- Runs in Kubernetes
- Subdomain → deployment_id mapping
- Metrics aggregated every minute

---

**Happy building!** 🚀
