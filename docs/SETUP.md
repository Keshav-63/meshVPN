# MeshVPN Self-Hosting Setup Guide (K3D + WSL2 + Docker Desktop)

**Current Architecture**: Windows Host → WSL2 (Debian) → K3D → Cloudflare Tunnel

This is the complete setup guide for running MeshVPN control-plane with K3D-based Kubernetes deployment on WSL2.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Prerequisites](#prerequisites)
3. [Initial Setup](#initial-setup)
4. [Database Setup & Migrations](#database-setup--migrations)
5. [Cloudflare Tunnel Configuration](#cloudflare-tunnel-configuration)
6. [K3D Cluster Setup](#k3d-cluster-setup)
7. [GitHub Container Registry Authentication](#github-container-registry-authentication)
8. [Observability Stack](#observability-stack)
9. [Running the Control-Plane](#running-the-control-plane)
10. [Testing Your Setup](#testing-your-setup)
11. [Resource Packages & Analytics](#resource-packages--analytics)
12. [Troubleshooting](#troubleshooting)

---

## Architecture Overview

### System Stack

```
User Request
    ↓
Cloudflare Tunnel (cloudflared in docker-compose)
    ↓
    ├─→ self.keshavstack.tech → http://host.docker.internal:8080 (Go Control-Plane API)
    └─→ *.keshavstack.tech → http://host.docker.internal:80 (K3D Traefik Ingress)
                                    ↓
                                  K3D Cluster (WSL2)
                                    ↓
                        User App Pods (with strict resource limits)
```

### Why K3D (Not Native K3s)?

- **Problem**: Native K3s on WSL2 has ContainerManager cgroup crash loops
- **Solution**: K3D (~150MB RAM) runs a lightweight K3s cluster inside Docker
- **Benefit**: Stable, no cgroup issues, works perfectly with Docker Desktop on WSL2

### Why host.docker.internal?

Docker Desktop on Windows creates a separate VM for WSL2 containers. Using `host.docker.internal` allows containers in docker-compose (cloudflared) to reach services running in WSL2 (Go control-plane on port 8080, K3D Traefik on port 80).

### Resource Packages (Critical for Laptop Stability)

MeshVPN uses a 3-tier package system for resource allocation:

- **Small**: 0.5 CPU cores, 512 MB RAM, max 3 replicas
- **Medium**: 1.0 CPU core, 1024 MB RAM, max 5 replicas
- **Large**: 2.0 CPU cores, 2048 MB RAM, max 10 replicas

**Autoscaling**: Enabled only for subscribers (non-subscribers run 1 fixed replica)

See [PACKAGES.md](./PACKAGES.md) for detailed specifications.

### Observability Stack

- **Old approach**: 2GB kube-prometheus-stack Helm chart (too heavy for laptops)
- **Current approach**: Lean Prometheus + Grafana in docker-compose with 350MB hard limit
- **Scraping**: Manual Prometheus config pointing to Go control-plane `/metrics` endpoint

---

## Prerequisites

### Windows Host Requirements

- Windows 10/11 with WSL2 enabled
- Docker Desktop installed and running
- WSL2 distribution: Debian (or Ubuntu)

### WSL2 (Debian) Requirements

Install these inside your WSL2 Debian environment:

```bash
# Update package lists
sudo apt update

# Install required packages
sudo apt install -y git curl wget

# Install Go (1.21+)
wget https://go.dev/dl/go1.21.6.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.21.6.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Verify Go installation
go version

# Install kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl
kubectl version --client

# Install K3D
curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash
k3d version
```

### External Services

1. **Cloudflare Account**
   - Domain configured in Cloudflare (e.g., keshavstack.tech)
   - API Token with permissions: Zone:DNS:Edit + Account:Cloudflare Tunnel:Edit
   - Account ID and Zone ID

2. **GitHub Container Registry (GHCR)**
   - GitHub account with GHCR access
   - Personal Access Token with `write:packages` permission

3. **Supabase (or PostgreSQL)**
   - Database URL
   - JWT Secret (for Supabase auth)

---

## Initial Setup

### 1. Clone the Repository

```bash
cd ~
git clone <your-repo-url> MeshVPN-slef-hosting
cd MeshVPN-slef-hosting
```

### 2. Configure Environment Variables

```bash
cp .env.example .env
```

Edit `.env` with your values (located at project root):

```env
# Cloudflare Tunnel Token (get this from the setup script)
CLOUDFLARE_TUNNEL_TOKEN=your_token_here

# Domain
APP_BASE_DOMAIN=keshavstack.tech

# Database
DATABASE_URL=postgresql://user:password@host:5432/dbname
# OR
SUPABASE_DB_URL=postgresql://postgres:password@db.project.supabase.co:5432/postgres?sslmode=require

# Supabase JWT Secret
SUPABASE_JWT_SECRET=your_jwt_secret_here

# Auth
REQUIRE_AUTH=true

# Worker Settings
WORKER_POLL_INTERVAL=2s
WORKER_BATCH_SIZE=3

# Runtime Backend
RUNTIME_BACKEND=k3s

# CPU HPA
ENABLE_CPU_HPA=true

# Kubernetes Settings
K8S_NAMESPACE=meshvpn-apps
K8S_CONFIG_PATH=/root/.kube/config
KUBECTL_BIN=kubectl

# GitHub Container Registry (your username/org)
K8S_IMAGE_PREFIX=ghcr.io/your-github-username
```

---

## Database Setup & Migrations

MeshVPN requires PostgreSQL for user management, analytics, and deployment tracking.

### 1. Database Options

**Option A: Supabase (Recommended for Production)**
- Create free project at https://supabase.com
- Get connection string from Project Settings → Database
- Format: `postgresql://postgres:[password]@db.[project].supabase.co:5432/postgres?sslmode=require`

**Option B: Local PostgreSQL**
```bash
# Install PostgreSQL on WSL2
sudo apt install postgresql postgresql-contrib
sudo service postgresql start
```

### 2. Run Migrations

Migrations are applied automatically on control-plane startup, but you can also run them manually:

```bash
cd control-plane

# Set database URL
export DATABASE_URL="your_postgres_connection_string"

# Migrations run automatically when control-plane starts
# Or use a migration tool like golang-migrate if needed
```

### 3. Verify Migrations

Connect to your database and verify tables exist:

```sql
\dt  -- List all tables

-- Expected tables:
-- - deployments
-- - deployment_jobs
-- - users
-- - deployment_metrics
-- - deployment_requests
```

### 4. Migration Files

Migrations are located in `control-plane/internal/store/migrations/`:
- `001_initial.sql` - Deployments and jobs tables
- `002_add_deployment_fields.sql` - Additional deployment fields
- `003_users_and_analytics.sql` - Users, metrics, and analytics tables

---

## Cloudflare Tunnel Configuration

### Option 1: Automated Setup Script (Recommended)

1. **Edit the setup script** at `scripts/setup-cloudflare-tunnel.go`:

```go
const (
    CLOUDFLARE_API_TOKEN = "your_api_token_here"
    CLOUDFLARE_ACCOUNT_ID = "your_account_id_here"
    CLOUDFLARE_ZONE_ID = "your_zone_id_here"
    // ... rest stays the same
)
```

2. **Run the script**:

```bash
cd scripts
go run setup-cloudflare-tunnel.go
```

3. **Copy the tunnel token** from the output and add it to `infra/.env`:

```env
CLOUDFLARE_TUNNEL_TOKEN=<token_from_script_output>
```

### Option 2: Manual Setup via Cloudflare Dashboard

1. Go to Cloudflare Zero Trust → Networks → Tunnels
2. Create a tunnel named "MeshVPN_SelfHosting"
3. Add public hostnames:
   - `self.keshavstack.tech` → `http://host.docker.internal:8080`
   - `*.keshavstack.tech` → `http://host.docker.internal:80`
4. Copy the tunnel token and add it to `infra/.env`

---

## K3D Cluster Setup

### 1. Create K3D Cluster

```bash
k3d cluster create meshvpn \
  --port "80:80@loadbalancer" \
  --port "443:443@loadbalancer" \
  --agents 0 \
  --k3s-arg "--disable=traefik@server:0"
```

**Why these flags?**
- `--port "80:80@loadbalancer"`: Expose port 80 for Traefik ingress
- `--agents 0`: Single-node cluster (sufficient for laptop)
- `--disable=traefik`: We'll use our own Traefik configuration

### 2. Verify Cluster

```bash
kubectl cluster-info
kubectl get nodes
```

Expected output: 1 node in Ready state.

### 3. Create Namespace

```bash
kubectl create namespace meshvpn-apps
```

### 4. Export Kubeconfig (for Control-Plane to access)

```bash
k3d kubeconfig get meshvpn > ~/k3d-kubeconfig.yaml
```

Update your `infra/.env`:

```env
K8S_CONFIG_PATH=/home/your-username/k3d-kubeconfig.yaml
```

---

## GitHub Container Registry Authentication

This is **critical** to avoid ImagePullBackOff errors.

### 1. Login to GHCR from WSL2

```bash
echo $GITHUB_TOKEN | docker login ghcr.io -u your-github-username --password-stdin
```

### 2. Create Kubernetes Secret

```bash
kubectl create secret docker-registry ghcr-secret \
  --docker-server=ghcr.io \
  --docker-username=your-github-username \
  --docker-password=$GITHUB_TOKEN \
  -n meshvpn-apps
```

### 3. Patch Default ServiceAccount (Universal Fix)

```bash
kubectl patch serviceaccount default \
  -n meshvpn-apps \
  -p '{"imagePullSecrets": [{"name": "ghcr-secret"}]}'
```

**Why?** This applies the secret to ALL pods in the namespace automatically, no need to specify in manifests.

### 4. Verify

```bash
kubectl get serviceaccount default -n meshvpn-apps -o yaml
```

You should see:

```yaml
imagePullSecrets:
- name: ghcr-secret
```

---

## Observability Stack

### 1. Create Observability Docker Compose

Create `infra/observability/docker-compose.yml`:

```yaml
version: "3.8"

services:
  prometheus:
    image: prom/prometheus:latest
    container_name: prometheus
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus-data:/prometheus
    command:
      - "--config.file=/etc/prometheus/prometheus.yml"
      - "--storage.tsdb.retention.time=72h"
    mem_limit: 200m
    restart: unless-stopped

  grafana:
    image: grafana/grafana:latest
    container_name: grafana
    ports:
      - "3001:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
      - GF_USERS_ALLOW_SIGN_UP=false
    volumes:
      - grafana-data:/var/lib/grafana
    mem_limit: 150m
    restart: unless-stopped

volumes:
  prometheus-data:
  grafana-data:
```

### 2. Create Prometheus Configuration

Create `infra/observability/prometheus.yml`:

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: "control-plane"
    static_configs:
      - targets: ["host.docker.internal:8080"]
    metrics_path: /metrics
```

### 3. Start Observability Stack

```bash
cd infra/observability
docker compose up -d
```

### 4. Access Grafana

Open http://localhost:3001

**Authentication**: Disabled for local development (anonymous access with Admin role)

**Datasources**: Prometheus is auto-provisioned at `http://prometheus:9090`

**Dashboards**: MeshVPN dashboards are automatically loaded from:
- `infra/observability/grafana-dashboards/platform-overview.json`
- `infra/observability/grafana-dashboards/deployment-detail.json`

Navigate to **Dashboards → Browse → MeshVPN** to view them.

---

## Running the Control-Plane

### 1. Build and Test

```bash
cd ~/MeshVPN-slef-hosting/control-plane
go test ./...
```

### 2. Run Control-Plane

```bash
cd ~/MeshVPN-slef-hosting/control-plane

# Set environment variables
export RUNTIME_BACKEND=k3s
export ENABLE_CPU_HPA=true
export K8S_NAMESPACE=meshvpn-apps
export K8S_IMAGE_PREFIX=ghcr.io/your-github-username
export DATABASE_URL="your_database_url"
export SUPABASE_JWT_SECRET="your_jwt_secret"
export REQUIRE_AUTH=true
export K8S_CONFIG_PATH=/home/your-username/k3d-kubeconfig.yaml

# Run
go run ./cmd/control-plane
```

Expected output:

```
Control plane starting...
Listening on :8080
Worker started (poll interval: 2s)
```

### 3. Start Cloudflare Tunnel

In a new terminal:

```bash
cd ~/MeshVPN-slef-hosting/infra
docker compose up -d
```

---

## Testing Your Setup

### 1. Health Check

```bash
curl http://localhost:8080/health
```

Expected: `{"status":"ok"}`

### 2. Metrics Check

```bash
curl http://localhost:8080/metrics
```

Expected: Prometheus-formatted metrics.

### 3. Public Health Check (via Cloudflare)

```bash
curl https://self.keshavstack.tech/health
```

### 4. Deploy a Test App

**Basic Deployment (No Auth Required if REQUIRE_AUTH=false)**
```bash
curl -X POST https://self.keshavstack.tech/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/your-org/your-app.git",
    "package": "medium",
    "port": 3000
  }'
```

**With Authentication (Production)**
```bash
curl -X POST https://self.keshavstack.tech/deploy \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/your-org/your-app.git",
    "package": "medium",
    "port": 3000,
    "subdomain": "my-app"
  }'
```

Expected: `202 Accepted` with deployment details:
```json
{
  "message": "deployment queued",
  "deployment_id": "74b295d2",
  "status": "queued",
  "package": "medium",
  "cpu_cores": 1.0,
  "memory_mb": 1024,
  "scaling_mode": "none",
  "autoscaling_enabled": false,
  "url": "https://my-app.keshavstack.tech"
}
```

### 5. Monitor Deployment

```bash
# Check deployments
curl https://self.keshavstack.tech/deployments

# Get build logs
curl https://self.keshavstack.tech/deployments/<deployment_id>/build-logs

# Check K8s resources
kubectl -n meshvpn-apps get deploy,svc,ing,hpa
kubectl -n meshvpn-apps get pods -o wide
```

### 6. Access Deployed App

Open https://test.keshavstack.tech in your browser.

---

## Resource Packages & Analytics

### Package System

MeshVPN provides three resource packages for deployments:

**Small Package**
- 0.5 CPU cores, 512 MB RAM
- Max 3 replicas (subscribers only)
- Best for: Static sites, simple APIs

**Medium Package**
- 1.0 CPU core, 1024 MB RAM
- Max 5 replicas (subscribers only)
- Best for: Web applications, microservices

**Large Package**
- 2.0 CPU cores, 2048 MB RAM
- Max 10 replicas (subscribers only)
- Best for: Resource-intensive apps, databases

### Deploy with Package

```bash
curl -X POST https://self.keshavstack.tech/deploy \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/user/my-app",
    "package": "medium",
    "port": 3000
  }'
```

If package is not specified, "small" is used by default.

### Analytics API

View real-time metrics for your deployments:

**Snapshot Metrics**
```bash
curl https://self.keshavstack.tech/deployments/{id}/analytics \
  -H "Authorization: Bearer $TOKEN"
```

**Real-time Streaming (SSE)**
```bash
curl https://self.keshavstack.tech/deployments/{id}/analytics/stream \
  -H "Authorization: Bearer $TOKEN"
```

Metrics include:
- Request counts (total, last hour, last 24h)
- Latency percentiles (p50, p90, p99)
- Bandwidth usage
- Pod status
- CPU/Memory usage

See [ANALYTICS-API.md](./ANALYTICS-API.md) for complete documentation.

### Grafana Dashboards

Access platform-wide metrics at http://localhost:3001

**Pre-configured Dashboards:**
- **Platform Overview**: Total deployments, request rate, bandwidth, top deployments
- **Deployment Detail**: Per-deployment metrics with drill-down

**To import dashboards:**
1. Open Grafana at http://localhost:3001
2. Dashboards are auto-provisioned from `infra/observability/grafana-dashboards/`
3. Navigate to Dashboards → Browse → MeshVPN folder

See [GRAFANA-SETUP.md](./GRAFANA-SETUP.md) for detailed setup and customization.

### Subscription Features

**Non-Subscribers (Free)**
- All packages available
- Fixed 1 replica (no autoscaling)
- Full analytics access

**Subscribers**
- Horizontal Pod Autoscaling (HPA) based on CPU
- Dynamic scaling from 1 to package max_replicas
- Custom scaling parameters (CPU target, min/max replicas)

---

## Troubleshooting

### ImagePullBackOff Errors

**Symptom**: Pods stuck in ImagePullBackOff state.

**Fix**:
1. Verify ghcr-secret exists: `kubectl get secret ghcr-secret -n meshvpn-apps`
2. Verify ServiceAccount patch: `kubectl get sa default -n meshvpn-apps -o yaml`
3. Re-login to GHCR: `docker login ghcr.io`
4. Recreate secret if needed

### HPA Not Scaling

**Symptom**: HPA shows `<unknown>` for current CPU.

**Fix**:
1. Verify metrics-server is running (K3D includes it by default)
2. Check pod resource requests are set (control-plane does this automatically)
3. Check HPA status: `kubectl describe hpa <name> -n meshvpn-apps`

### Cloudflare Tunnel Not Working

**Symptom**: 502 errors or tunnel disconnected.

**Fix**:
1. Check cloudflared container: `docker logs cloudflared`
2. Verify token is correct in `infra/.env`
3. Verify routes use `host.docker.internal`, not `localhost`
4. Restart tunnel: `cd infra && docker compose restart cloudflared`

### Control-Plane Can't Reach K3D

**Symptom**: `unable to connect to cluster` errors.

**Fix**:
1. Verify K3D cluster is running: `k3d cluster list`
2. Verify kubeconfig path in env: `echo $K8S_CONFIG_PATH`
3. Test kubectl access: `kubectl --kubeconfig=$K8S_CONFIG_PATH get nodes`

### Out of Memory / System Lockup

**Symptom**: Laptop becomes unresponsive during builds.

**This should be fixed** by strict resource limits. If it still happens:
1. Check resource limits in deployment YAML: `kubectl get deploy <name> -n meshvpn-apps -o yaml`
2. Verify limits are: CPU=500m, Memory=512Mi
3. Update control-plane code if limits are not being applied

---

## Next Steps

1. **Explore Analytics**: See [ANALYTICS-API.md](./ANALYTICS-API.md) for real-time metrics and SSE streaming
2. **Choose Packages**: Review [PACKAGES.md](./PACKAGES.md) for resource package specifications
3. **Monitor Platform**: Check [GRAFANA-SETUP.md](./GRAFANA-SETUP.md) for dashboard customization
4. **Multi-Worker Setup**: Add more laptops as K3D agents (future enhancement - Phase 3)
5. **CI/CD Integration**: Automate deployments from GitHub Actions
6. **Custom Domains**: Configure additional domains in Cloudflare

---

## Quick Reference

### Essential Commands

```bash
# K3D Cluster
k3d cluster list
k3d cluster start meshvpn
k3d cluster stop meshvpn

# Kubernetes
kubectl get all -n meshvpn-apps
kubectl logs -f <pod-name> -n meshvpn-apps
kubectl describe pod <pod-name> -n meshvpn-apps

# Docker
docker ps
docker logs cloudflared
docker logs prometheus
docker logs grafana

# Control-Plane
cd ~/MeshVPN-slef-hosting/control-plane
go run ./cmd/control-plane
```

### Important URLs

- Control-Plane API: http://localhost:8080
- Control-Plane (Public): https://self.keshavstack.tech
- Prometheus: http://localhost:9090
- Grafana Dashboards: http://localhost:3001
- Analytics API: https://self.keshavstack.tech/deployments/{id}/analytics
- Analytics Stream (SSE): https://self.keshavstack.tech/deployments/{id}/analytics/stream
- Metrics Endpoint: http://localhost:8080/metrics
- Deployed Apps: https://<subdomain>.keshavstack.tech

---

## Architecture Decisions Summary

| Aspect | Choice | Reason |
|--------|--------|--------|
| K8s Distribution | K3D | Avoids WSL2 cgroup issues with native k3s |
| Observability | Docker Compose (Prometheus + Grafana) | Lighter than kube-prometheus-stack (350MB vs 2GB) |
| Registry | GitHub Container Registry (GHCR) | Free, integrated with GitHub repos |
| Tunnel | Cloudflare Tunnel | Free, secure, no port forwarding |
| Resource Packages | 3-tier (Small/Medium/Large) | Simple selection, predictable resource usage |
| Autoscaling | HPA for subscribers only | Subscribers get dynamic scaling, free tier gets fixed replicas |
| Analytics | PostgreSQL + SSE streaming | Real-time metrics via Server-Sent Events |
| Subdomain | Auto-generated from repo name | Simplifies deployment, conflict detection with random suffix |
| Database | PostgreSQL (Supabase) | User tracking, analytics, deployment state |
| Auth Fix | ServiceAccount imagePullSecrets patch | Universal, no per-pod configuration needed |
| Routing Fix | host.docker.internal | Bridges Docker Desktop's WSL2 VM networking |

---

**Last Updated**: 2026-03-21
**Architecture Version**: Phase 2 (K3D + Analytics + Packages + Autoscaling)
