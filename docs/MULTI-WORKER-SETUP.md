# Multi-Worker Setup Guide

Complete guide for setting up distributed worker deployment system with Tailscale mesh network.

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Prerequisites](#prerequisites)
- [Step 1: Setup Control-Plane](#step-1-setup-control-plane)
- [Step 2: Setup Remote Worker](#step-2-setup-remote-worker)
- [Step 3: Test Deployment](#step-3-test-deployment)
- [How It Works](#how-it-works)
- [API Reference](#api-reference)
- [Troubleshooting](#troubleshooting)

---

## Architecture Overview

```
┌────────────────────────────────────────────────────────────────┐
│  Control-Plane Machine (Hybrid: Coordinator + Worker)         │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────────┐  │
│  │   HTTP API   │  │  Embedded    │  │  Job Distributor  │  │
│  │  :8080       │  │  Worker      │  │  (Assigns jobs)   │  │
│  └──────────────┘  └──────────────┘  └───────────────────┘  │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │  PostgreSQL: deployment_jobs, workers, deployments       │ │
│  └──────────────────────────────────────────────────────────┘ │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────────┐  │
│  │ K8s Cluster  │  │  Prometheus  │  │  Analytics        │  │
│  │ (local pods) │  │  :9090       │  │  Collector        │  │
│  └──────────────┘  └──────────────┘  └───────────────────┘  │
│  Tailscale IP: 100.64.1.1 (control-plane)                    │
└────────────────────────────────────────────────────────────────┘
                │                  │                  │
        (Tailscale)        (Tailscale)        (Tailscale)
                │                  │                  │
                ↓                  ↓                  ↓
      ┌──────────────┐   ┌──────────────┐   ┌──────────────┐
      │  Worker 1    │   │  Worker 2    │   │  Worker N    │
      │  (Laptop)    │   │  (Server)    │   │  (Cloud VM)  │
      │  Agent:8081  │   │  Agent:8081  │   │  Agent:8081  │
      ├──────────────┤   ├──────────────┤   ├──────────────┤
      │ K8s Cluster  │   │ K8s Cluster  │   │ K8s Cluster  │
      │ Pods: 2      │   │ Pods: 5      │   │ Pods: 3      │
      └──────────────┘   └──────────────┘   └──────────────┘
      100.64.1.2         100.64.1.3         100.64.1.N
```

### Job Placement Decision Logic

When a deployment request comes in:

1. **Small packages** (CPUCores ≤ 0.5):
   - Try control-plane first (if `CONTROL_PLANE_AS_WORKER=true`)
   - Fallback to remote workers if control-plane is busy

2. **Medium/Large packages** (CPUCores > 0.5):
   - Try remote workers first
   - Fallback to control-plane if no remote workers available

3. **Placement Strategies**:
   - `smart` (default): Small → local, Medium/Large → remote
   - `local-first`: Always try control-plane first
   - `remote-only`: Never use control-plane as worker

---

## Prerequisites

### Control-Plane Machine

- **OS**: Ubuntu/Debian (WSL2 on Windows)
- **Docker**: For building images
- **Kubernetes**: K3D cluster running
- **Tailscale**: Installed and connected
- **PostgreSQL**: Supabase or local instance
- **Go**: 1.23+ (for building control-plane)

### Remote Worker Machines

- **OS**: Linux (Ubuntu/Debian recommended)
- **Docker**: For building images
- **Kubernetes**: K3D or K3s cluster
- **Tailscale**: Installed and connected to same network
- **Go**: 1.23+ (for building worker-agent)
- **kubectl**: Configured with cluster access

---

## Step 1: Setup Control-Plane

### 1.1 Update Configuration

Edit `.env` file (in project root):

```env
# Enable multi-worker mode
ENABLE_MULTI_WORKER=true

# Control-plane also acts as worker
CONTROL_PLANE_AS_WORKER=true
CONTROL_PLANE_WORKER_ID=control-plane-local
CONTROL_PLANE_MAX_JOBS=2

# Job placement strategy
JOB_PLACEMENT_STRATEGY=smart

# Worker authentication
WORKER_SHARED_SECRET=my-secret-token-12345

# Database (required for multi-worker)
DATABASE_URL=postgresql://postgres:<password>@db.<project-ref>.supabase.co:5432/postgres?sslmode=require

# Runtime backend (must be kubernetes for multi-worker)
RUNTIME_BACKEND=k3s
K8S_NAMESPACE=meshvpn-apps
```

### 1.2 Run Database Migration

```bash
cd ~/MeshVPN-slef-hosting/control-plane

# Apply migration 004_multi_worker.sql
psql $DATABASE_URL -f internal/store/migrations/004_multi_worker.sql
```

**Expected output:**
```
CREATE TABLE
CREATE INDEX
CREATE INDEX
ALTER TABLE
CREATE INDEX
```

### 1.3 Start Control-Plane

```bash
cd ~/MeshVPN-slef-hosting

./start-control-plane.sh
```

**Expected output:**
```
[INFO] [store] initializing postgres repositories
[INFO] [main] multi-worker mode enabled strategy=smart control_plane_as_worker=true
[INFO] [distributor] registered control-plane as worker worker_id=control-plane-local max_jobs=2
[INFO] [distributor] job distributor started (multi-worker mode, strategy=smart)
[INFO] [analytics] analytics collector started interval=1m
[INFO] [main] starting router require_auth=true has_database=true analytics=true
[INFO] [http] worker API endpoints registered
Listening and serving HTTP on 0.0.0.0:8080
```

### 1.4 Verify Control-Plane Worker Registration

```bash
# Check if control-plane registered as worker
curl -s http://localhost:8080/workers \
  -H "Authorization: Bearer <your-jwt-token>" | jq
```

**Expected response:**
```json
{
  "workers": [
    {
      "worker_id": "control-plane-local",
      "name": "Control-Plane (Local Worker)",
      "tailscale_ip": "localhost",
      "hostname": "control-plane",
      "status": "idle",
      "capabilities": {
        "runtime": "kubernetes",
        "max_concurrent_jobs": 2,
        "supported_packages": ["small", "medium", "large"]
      },
      "max_concurrent_jobs": 2,
      "current_jobs": 0,
      "last_heartbeat": "2026-03-22T10:30:00Z",
      "created_at": "2026-03-22T10:00:00Z",
      "updated_at": "2026-03-22T10:30:00Z"
    }
  ]
}
```

---

## Step 2: Setup Remote Worker

### 2.1 Install Tailscale on Worker Machine

```bash
# On worker machine (laptop/server)
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up

# Verify Tailscale IP
tailscale ip -4
# Output: 100.64.1.2
```

### 2.2 Setup K3D Cluster

```bash
# Install K3D
curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash

# Create cluster
k3d cluster create worker-cluster \
  --port "80:80@loadbalancer" \
  --agents 0 \
  --servers 1

# Verify cluster
kubectl cluster-info
```

### 2.3 Configure Worker Agent

```bash
cd ~/MeshVPN-slef-hosting/worker-agent

# Copy example config
cp agent.yaml.example agent.yaml

# Edit config
nano agent.yaml
```

Update `agent.yaml`:

```yaml
worker:
  id: worker-laptop-1  # Unique ID for this worker
  name: "Keshav's Laptop Worker"
  tailscale_ip: ""  # Auto-detected (or set manually: 100.64.1.2)
  max_concurrent_jobs: 2

control_plane:
  url: http://100.107.233.70:8080  # Control-plane Tailscale IP
  shared_secret: "my-secret-token-12345"  # Must match .env

runtime:
  type: kubernetes
  kubeconfig: /home/keshav/.kube/config
  namespace: worker-apps
  kubectl_bin: kubectl

capabilities:
  memory_gb: 16
  cpu_cores: 8
  supported_packages:
    - small
    - medium
    - large
```

### 2.4 Build and Run Worker Agent

```bash
cd ~/MeshVPN-slef-hosting/worker-agent

# Build worker agent
go build -o worker-agent cmd/worker-agent/main.go

# Run worker
./worker-agent -config agent.yaml
```

**Expected output:**
```
Auto-detected Tailscale IP: 100.64.1.2
Starting worker agent: Keshav's Laptop Worker (worker-laptop-1)
Successfully registered with control-plane
```

### 2.5 Verify Worker Registration

On control-plane machine:

```bash
curl -s http://localhost:8080/workers \
  -H "Authorization: Bearer <your-jwt-token>" | jq
```

**Expected response (now with 2 workers):**
```json
{
  "workers": [
    {
      "worker_id": "control-plane-local",
      "name": "Control-Plane (Local Worker)",
      "tailscale_ip": "localhost",
      "status": "idle",
      "current_jobs": 0,
      "max_concurrent_jobs": 2
    },
    {
      "worker_id": "worker-laptop-1",
      "name": "Keshav's Laptop Worker",
      "tailscale_ip": "100.64.1.2",
      "hostname": "keshav-laptop",
      "status": "idle",
      "current_jobs": 0,
      "max_concurrent_jobs": 2,
      "last_heartbeat": "2026-03-22T10:32:00Z"
    }
  ]
}
```

---

## Step 3: Test Deployment

### 3.1 Deploy Small Package (Should use control-plane)

```bash
curl -X POST http://localhost:8080/deploy \
  -H "Authorization: Bearer <your-jwt-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/dockersamples/static-site",
    "package": "small",
    "port": 80
  }'
```

**Expected response:**
```json
{
  "message": "deployment queued",
  "deployment_id": "d4bc923f",
  "status": "queued",
  "repo": "https://github.com/dockersamples/static-site",
  "subdomain": "static-site",
  "url": "https://static-site.keshavstack.tech",
  "port": 80,
  "package": "small",
  "cpu_cores": 0.5,
  "memory_mb": 512,
  "scaling_mode": "none",
  "min_replicas": 1,
  "max_replicas": 1,
  "autoscaling_enabled": false
}
```

**Check job assignment:**
```bash
# Wait 3 seconds for distributor to assign job
sleep 3

# Query database
psql $DATABASE_URL -c "SELECT job_id, deployment_id, status, assigned_worker_id FROM deployment_jobs WHERE deployment_id='d4bc923f';"
```

**Expected output:**
```
         job_id         | deployment_id |  status  | assigned_worker_id
------------------------+---------------+----------+--------------------
 job-abc123             | d4bc923f      | running  | control-plane-local
```

### 3.2 Deploy Large Package (Should use remote worker)

```bash
curl -X POST http://localhost:8080/deploy \
  -H "Authorization: Bearer <your-jwt-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/vercel/next.js",
    "package": "large",
    "port": 3000,
    "subdomain": "nextjs-app"
  }'
```

**Expected response:**
```json
{
  "message": "deployment queued",
  "deployment_id": "e5ddae78",
  "package": "large",
  "cpu_cores": 2.0,
  "memory_mb": 2048
}
```

**Check job assignment (should go to remote worker):**
```bash
sleep 3

psql $DATABASE_URL -c "SELECT job_id, deployment_id, status, assigned_worker_id FROM deployment_jobs WHERE deployment_id='e5ddae78';"
```

**Expected output:**
```
         job_id         | deployment_id |  status  | assigned_worker_id
------------------------+---------------+----------+-------------------
 job-xyz789             | e5ddae78      | running  | worker-laptop-1
```

**On worker machine, you should see:**
```
Claimed job: job-xyz789
Cloning repository: https://github.com/vercel/next.js
Building image: ghcr.io/your-org/e5ddae78:latest
Pushing image: ghcr.io/your-org/e5ddae78:latest
Creating Kubernetes deployment
Job completed successfully: job-xyz789
```

### 3.3 Monitor Worker Status

```bash
# Watch worker list in real-time
watch -n 2 'curl -s http://localhost:8080/workers -H "Authorization: Bearer <token>" | jq ".workers[] | {worker_id, status, current_jobs, max_concurrent_jobs}"'
```

**Expected output:**
```
{
  "worker_id": "control-plane-local",
  "status": "busy",
  "current_jobs": 1,
  "max_concurrent_jobs": 2
}
{
  "worker_id": "worker-laptop-1",
  "status": "busy",
  "current_jobs": 1,
  "max_concurrent_jobs": 2
}
```

---

## How It Works

### Worker Registration Flow

```
Worker Machine              Control-Plane
     │                            │
     │  POST /api/workers/register│
     │  {                         │
     │    worker_id: "...",       │
     │    name: "...",            │
     │    tailscale_ip: "...",    │
     │    capabilities: {...}     │
     │  }                         │
     ├───────────────────────────>│
     │                            │ (Store in workers table)
     │                            │
     │  200 OK                    │
     │  { status: "registered" }  │
     │<───────────────────────────┤
     │                            │
```

### Heartbeat Flow

```
Worker Machine              Control-Plane
     │                            │
     │  POST /api/workers/:id/heartbeat (every 30s)
     │  {                         │
     │    status: "idle",         │
     │    current_jobs: 0         │
     │  }                         │
     ├───────────────────────────>│
     │                            │ (UPDATE workers SET last_heartbeat=NOW())
     │                            │
     │  200 OK                    │
     │<───────────────────────────┤
     │                            │
```

### Job Execution Flow

```
User                Control-Plane         Job Distributor      Worker Machine
 │                        │                      │                    │
 │ POST /deploy           │                      │                    │
 ├──────────────────────> │                      │                    │
 │                        │ INSERT job (queued)  │                    │
 │                        ├─────────────────────>│                    │
 │                        │                      │                    │
 │ 202 Accepted           │                      │ (Every 3s poll)    │
 │<────────────────────── ┤                      │                    │
 │                        │                      │ SELECT unassigned  │
 │                        │                      │ job                │
 │                        │                      │                    │
 │                        │                      │ Apply strategy:    │
 │                        │                      │ - Small → local    │
 │                        │                      │ - Large → remote   │
 │                        │                      │                    │
 │                        │                      │ UPDATE job SET     │
 │                        │                      │ assigned_worker_id │
 │                        │                      │ = 'worker-1'       │
 │                        │                      ├────────────────────┤
 │                        │                      │                    │
 │                        │                      │                    │ GET /api/workers/:id/claim-job (every 5s)
 │                        │                      │                    │<───────────────
 │                        │  ClaimForWorker()    │                    │
 │                        │<─────────────────────┼────────────────────┤
 │                        │  job details         │                    │
 │                        ├──────────────────────┼───────────────────>│
 │                        │                      │                    │ Clone repo
 │                        │                      │                    │ Build image
 │                        │                      │                    │ Push to GHCR
 │                        │                      │                    │ Apply K8s manifest
 │                        │                      │                    │
 │                        │  POST /api/workers/:id/job-complete       │
 │                        │<───────────────────────────────────────────┤
 │                        │  { job_id: "..." }   │                    │
 │                        │                      │                    │
 │                        │ UPDATE job status='done'                  │
 │                        │ UPDATE worker current_jobs--              │
 │                        │                      │                    │
```

### Smart Placement Logic

```go
func smartPlacement(job DeploymentJob) Worker {
    // Small packages → control-plane (fast, no network overhead)
    if job.CPUCores <= 0.5 && controlPlaneAvailable() {
        return controlPlaneWorker
    }

    // Medium/Large → remote worker (offload heavy work)
    if remoteWorker := findRemoteWorker(); remoteWorker != nil {
        return remoteWorker
    }

    // Fallback to control-plane
    if controlPlaneAvailable() {
        return controlPlaneWorker
    }

    // Job stays queued (no workers available)
    return nil
}
```

---

## API Reference

### Worker Registration

**Endpoint:** `POST /api/workers/register`

**Request:**
```json
{
  "worker_id": "worker-laptop-1",
  "name": "My Laptop Worker",
  "tailscale_ip": "100.64.1.2",
  "hostname": "keshav-laptop",
  "capabilities": {
    "runtime": "kubernetes",
    "k8s_version": "v1.31",
    "memory_gb": 16,
    "cpu_cores": 8,
    "max_concurrent_jobs": 2,
    "supported_packages": ["small", "medium", "large"]
  }
}
```

**Response:**
```json
{
  "status": "registered",
  "worker": {
    "worker_id": "worker-laptop-1",
    "name": "My Laptop Worker",
    "tailscale_ip": "100.64.1.2",
    "status": "idle",
    "max_concurrent_jobs": 2,
    "current_jobs": 0
  }
}
```

### Worker Heartbeat

**Endpoint:** `POST /api/workers/:id/heartbeat`

**Request:**
```json
{
  "status": "idle",
  "current_jobs": 0
}
```

**Response:**
```json
{
  "status": "ok"
}
```

### Claim Job

**Endpoint:** `GET /api/workers/:id/claim-job`

**Response (job available):**
```json
{
  "job": {
    "job_id": "job-abc123",
    "deployment_id": "d4bc923f",
    "repo": "https://github.com/user/app",
    "subdomain": "my-app",
    "port": 3000,
    "cpu_cores": 0.5,
    "memory_mb": 512,
    "env": {},
    "build_args": {},
    "queued_at": "2026-03-22T10:00:00Z"
  }
}
```

**Response (no jobs):**
```json
{
  "job": null
}
```

### Report Job Complete

**Endpoint:** `POST /api/workers/:id/job-complete`

**Request:**
```json
{
  "job_id": "job-abc123"
}
```

**Response:**
```json
{
  "status": "ok"
}
```

### Report Job Failed

**Endpoint:** `POST /api/workers/:id/job-failed`

**Request:**
```json
{
  "job_id": "job-abc123",
  "error": "docker build failed: no such image"
}
```

**Response:**
```json
{
  "status": "ok"
}
```

### List Workers (Admin)

**Endpoint:** `GET /workers` (requires auth)

**Response:**
```json
{
  "workers": [
    {
      "worker_id": "control-plane-local",
      "name": "Control-Plane (Local Worker)",
      "tailscale_ip": "localhost",
      "status": "idle",
      "current_jobs": 0,
      "max_concurrent_jobs": 2,
      "last_heartbeat": "2026-03-22T10:30:00Z"
    },
    {
      "worker_id": "worker-laptop-1",
      "name": "Keshav's Laptop Worker",
      "tailscale_ip": "100.64.1.2",
      "status": "busy",
      "current_jobs": 1,
      "max_concurrent_jobs": 2,
      "last_heartbeat": "2026-03-22T10:30:05Z"
    }
  ]
}
```

---

## Troubleshooting

### Worker Not Registering

**Symptom:** Worker fails to register with control-plane.

**Solution:**
1. Verify Tailscale is running: `tailscale status`
2. Check control-plane URL is correct in `agent.yaml`
3. Verify control-plane is accessible: `curl http://100.64.1.1:8080/health`
4. Check worker logs for connection errors

### Jobs Not Being Assigned

**Symptom:** Jobs stay in `queued` status, never assigned to workers.

**Solution:**
1. Check control-plane logs for distributor errors
2. Verify `ENABLE_MULTI_WORKER=true` in control-plane `.env`
3. Check workers are reporting `status=idle`
4. Query database: `SELECT * FROM workers;`

### Worker Marked Offline

**Symptom:** Worker shows `status=offline` despite running.

**Solution:**
1. Check worker heartbeat is working: watch worker logs
2. Verify no firewall blocking Tailscale traffic
3. Increase `WORKER_HEARTBEAT_TIMEOUT` in control-plane `.env`
4. Restart worker agent

### Job Execution Fails

**Symptom:** Worker claims job but execution fails.

**Solution:**
1. Check worker logs for error details
2. Verify Docker is running on worker: `docker ps`
3. Verify kubectl works: `kubectl get nodes`
4. Check GHCR authentication: `docker login ghcr.io`
5. Ensure worker has network access to GitHub

### Control-Plane Not Acting as Worker

**Symptom:** Small packages not assigned to control-plane.

**Solution:**
1. Verify `CONTROL_PLANE_AS_WORKER=true` in `.env`
2. Check control-plane registered as worker: `SELECT * FROM workers WHERE worker_id='control-plane-local';`
3. Restart control-plane to trigger worker registration
4. Check distributor logs

---

## Next Steps

- **Add More Workers:** Repeat Step 2 on additional machines
- **Monitor Analytics:** Setup Prometheus/Grafana dashboards
- **Worker Health Checks:** Monitor heartbeat timeouts
- **Load Balancing:** Distribute load across multiple workers
- **Worker Tags:** Add capability matching (GPU, high-memory, etc.)

---

## Support

For issues or questions:
- Check logs: control-plane and worker-agent output
- Inspect database: `deployment_jobs`, `workers` tables
- GitHub Issues: https://github.com/your-org/meshvpn/issues
