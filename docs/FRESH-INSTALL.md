# Fresh Installation Guide - Start from Zero


Complete guide to clean up everything and start fresh with minimal resource consumption.

## Table of Contents

1. [Cleanup Existing Setup](#cleanup-existing-setup)
2. [Prerequisites Check](#prerequisites-check)
3. [Fresh Installation](#fresh-installation)
4. [Minimal Resource Configuration](#minimal-resource-configuration)
5. [Verification](#verification)
6. [First Deployment Test](#first-deployment-test)

---

## Cleanup Existing Setup

### Step 1: Stop All Services

```bash
# Stop control-plane (Ctrl+C if running in terminal)
# Or kill the process
pkill -f "control-plane"

# Stop Cloudflare tunnel
cd ~/MeshVPN-slef-hosting/infra
docker compose down

# Stop observability stack
cd ~/MeshVPN-slef-hosting/infra/observability
docker compose down

# Remove volumes (optional - cleans all data)
docker compose down -v
```

### Step 2: Delete K3D Cluster

```bash
# List all K3D clusters
k3d cluster list

# Delete the meshvpn cluster
k3d cluster delete meshvpn

# Verify deletion
k3d cluster list
# Should show: No clusters found
```

### Step 3: Clean Docker Resources

```bash
# Remove stopped containers
docker container prune -f

# Remove unused images (careful - removes all unused images)
docker image prune -a -f

# Remove unused volumes
docker volume prune -f

# Remove unused networks
docker network prune -f

# Check Docker disk usage
docker system df
```

### Step 4: Clean Kubernetes Config

```bash
# Remove old kubeconfig
rm -f ~/.kube/config
rm -f ~/k3d-kubeconfig.yaml

# Verify
ls ~/.kube/
ls ~/k3d-kubeconfig.yaml
# Should not exist
```

### Step 5: Clean Application Data

```bash
cd ~/MeshVPN-slef-hosting

# Remove built images and checkouts (optional)
rm -rf apps/*/

# Remove any .env files
rm -f .env
rm -f infra/.env

# Keep the .env.example for reference
```

### Step 6: Verify Clean State

```bash
# No K3D clusters
k3d cluster list

# No running containers
docker ps

# Kubectl has no valid config
kubectl get nodes
# Should show: "connection refused" or "config not found"
```

**✅ Cleanup Complete!** You now have a clean slate.

---

## Prerequisites Check

### Required Tools in WSL2 Debian

```bash
# 1. Check Go
go version
# Expected: go version go1.21.x or higher
# If not installed, see installation below

# 2. Check Docker CLI
docker --version
# Expected: Docker version 20.x or higher
# (Should be available via Docker Desktop)

# 3. Check kubectl
kubectl version --client
# Expected: Client Version: v1.x
# If not installed, see installation below

# 4. Check K3D
k3d version
# Expected: k3d version v5.x
# If not installed, see installation below

# 5. Check jq (optional but helpful)
jq --version
# If not: sudo apt install jq -y
```

### Install Missing Tools

#### Install Go (if needed)

```bash
cd ~
wget https://go.dev/dl/go1.21.6.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.21.6.linux-amd64.tar.gz

# Add to PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Verify
go version
```

#### Install kubectl (if needed)

```bash
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl
rm kubectl

# Verify
kubectl version --client
```

#### Install K3D (if needed)

```bash
curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash

# Verify
k3d version
```

#### Install jq (helpful for JSON parsing)

```bash
sudo apt update
sudo apt install jq -y

# Verify
jq --version
```

---

## Fresh Installation

### Step 1: Create K3D Cluster (Minimal Resources)

```bash
# Create cluster with minimal resources
k3d cluster create meshvpn \
  --port "80:80@loadbalancer" \
  --port "443:443@loadbalancer" \
  --agents 0 \
  --servers 1 \

# Wait for cluster to be ready (may take 30-60 seconds)
echo "Waiting for cluster to be ready..."
sleep 30
```

**Why these flags?**
- `--agents 0`: Single node cluster (saves resources)
- `--servers 1`: One control plane node
- `--disable=traefik`: We'll use our own ingress configuration
- `--port 80:80`: Expose port 80 for app ingress

### Step 2: Verify Cluster

```bash
# Check cluster status
k3d cluster list
# Should show: meshvpn (1/1 nodes ready)

# Get nodes
kubectl get nodes
# Should show 1 node in Ready state

# Check system pods
kubectl get pods -n kube-system
# Should see coredns, metrics-server, local-path-provisioner pods running
```

**If kubectl fails with connection refused:**
```bash
# Export kubeconfig
export KUBECONFIG=$(k3d kubeconfig write meshvpn)

# Or permanently
k3d kubeconfig write meshvpn > ~/.kube/config
chmod 600 ~/.kube/config

# Test again
kubectl get nodes
```

### Step 3: Create Application Namespace

```bash
# Create namespace for user applications
kubectl create namespace meshvpn-apps

# Verify
kubectl get namespaces
# Should see: meshvpn-apps
```

### Step 4: Export Kubeconfig for Control-Plane

```bash
# Export to a file that control-plane can use
k3d kubeconfig get meshvpn > ~/k3d-kubeconfig.yaml
ls -lh ~/k3d-kubeconfig.yaml

# Check contents (should have server pointing to localhost)
cat ~/k3d-kubeconfig.yaml | grep server:
# Should show: server: https://0.0.0.0:<random-port>
```

### Step 5: Setup GHCR Authentication

```bash
# Set your GitHub token
export GITHUB_TOKEN="your_github_personal_access_token"

# Login to GHCR
echo $GITHUB_TOKEN | docker login ghcr.io -u your-github-username --password-stdin

# Create Kubernetes secret
kubectl create secret docker-registry ghcr-secret \
  --docker-server=ghcr.io \
  --docker-username=your-github-username \
  --docker-password=$GITHUB_TOKEN \
  -n meshvpn-apps

# Patch default service account to use the secret
kubectl patch serviceaccount default \
  -n meshvpn-apps \
  -p '{"imagePullSecrets": [{"name": "ghcr-secret"}]}'

# Verify
kubectl get secret ghcr-secret -n meshvpn-apps
kubectl get serviceaccount default -n meshvpn-apps -o yaml | grep ghcr-secret
```

### Step 6: Setup Environment Variables

```bash
cd ~/MeshVPN-slef-hosting

# Copy example env file
cp .env.example .env

# Edit with your values
nano .env
# Or use vim/vi
```

**Minimal .env Configuration:**

```env
# Database (use your Supabase or PostgreSQL URL)
DATABASE_URL=postgresql://user:password@host:5432/dbname

# Supabase JWT Secret (if using Supabase auth)
SUPABASE_JWT_SECRET=your_jwt_secret_here

# Disable auth for testing (set to false)
REQUIRE_AUTH=false

# Runtime
RUNTIME_BACKEND=k3s
ENABLE_CPU_HPA=false
HPA_MEMORY_TARGET_UTILIZATION=75
HPA_SCALE_UP_STABILIZATION_SECONDS=0
HPA_SCALE_DOWN_STABILIZATION_SECONDS=60

# Kubernetes
K8S_NAMESPACE=meshvpn-apps
K8S_CONFIG_PATH=/home/your-username/k3d-kubeconfig.yaml
KUBECTL_BIN=kubectl

# GitHub Container Registry
K8S_IMAGE_PREFIX=ghcr.io/your-github-username

# Domain
APP_BASE_DOMAIN=keshavstack.tech

# Worker (reduced for minimal resources)
WORKER_POLL_INTERVAL=5s

# Cloudflare Tunnel Token (get from setup script or dashboard)
CLOUDFLARE_TUNNEL_TOKEN=your_tunnel_token_here
```

**Important:** Replace:
- `your-username` with your actual WSL username (`echo $USER`)
- `your-github-username` with your GitHub username
- Database credentials
- Cloudflare tunnel token

---

## Minimal Resource Configuration

### Observability Stack (Lightweight)

Create `infra/observability/docker-compose.yml`:

```yaml
version: '3.8'

services:
  prometheus:
    image: prom/prometheus:latest
    container_name: observability_prometheus
    ports:
      - "9090:9090"
    command:
      - --config.file=/etc/prometheus/prometheus.yml
      - --storage.tsdb.retention.time=24h  # Reduced retention
      - --storage.tsdb.path=/prometheus
      - --log.level=warn
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus-data:/prometheus
    restart: unless-stopped
    deploy:
      resources:
        limits:
          cpus: '0.2'      # 20% of 1 CPU
          memory: 150M     # 150 MB RAM

  grafana:
    image: grafana/grafana:latest
    container_name: observability_grafana
    ports:
      - "3001:3000"
    environment:
      - GF_AUTH_ANONYMOUS_ENABLED=true
      - GF_AUTH_ANONYMOUS_ORG_ROLE=Admin
      - GF_AUTH_DISABLE_LOGIN_FORM=true
      - GF_INSTALL_PLUGINS=
      - GF_LOG_LEVEL=warn
    volumes:
      - ./grafana-provisioning/datasources:/etc/grafana/provisioning/datasources:ro
      - ./grafana-provisioning/dashboards:/etc/grafana/provisioning/dashboards:ro
      - ./grafana-dashboards:/etc/grafana/provisioning/dashboards/json:ro
      - grafana-data:/var/lib/grafana
    restart: unless-stopped
    deploy:
      resources:
        limits:
          cpus: '0.2'      # 20% of 1 CPU
          memory: 150M     # 150 MB RAM

volumes:
  prometheus-data:
  grafana-data:
```

Create `infra/observability/prometheus.yml`:

```yaml
global:
  scrape_interval: 30s       # Reduced from 15s
  evaluation_interval: 30s

scrape_configs:
  - job_name: "control-plane"
    static_configs:
      - targets: ["host.docker.internal:8080"]
    metrics_path: /metrics
```

### Start Observability Stack

```bash
cd ~/MeshVPN-slef-hosting/infra/observability

# Start services
docker compose up -d

# Check status
docker ps

# Check resource usage
docker stats --no-stream
# Should show ~200-300MB total for both containers
```

### Cloudflare Tunnel (Lightweight)

Create `infra/docker-compose.yml`:

```yaml
version: '3.8'

services:
  cloudflared:
    image: cloudflare/cloudflared:latest
    container_name: cloudflared-1
    command: tunnel --no-autoupdate run --token ${CLOUDFLARE_TUNNEL_TOKEN}
    environment:
      - CLOUDFLARE_TUNNEL_TOKEN=${CLOUDFLARE_TUNNEL_TOKEN}
    restart: unless-stopped
    deploy:
      resources:
        limits:
          cpus: '0.1'      # 10% of 1 CPU
          memory: 50M      # 50 MB RAM
    network_mode: host
```

**Note:** Make sure `.env` file is in `infra/` directory or use absolute path in docker-compose.

### Start Cloudflare Tunnel

```bash
cd ~/MeshVPN-slef-hosting/infra

# Load environment variables
export $(cat ../.env | xargs)

# Or create a symlink
ln -s ../.env .env

# Start tunnel
docker compose up -d

# Check logs
docker logs cloudflared-1

# Should see: "Connection registered" messages
```

---

## Running Control-Plane

### Step 1: Build Control-Plane

```bash
cd ~/MeshVPN-slef-hosting/control-plane

# Run tests first
go test ./...

# Build
go build -o meshvpn-control-plane ./cmd/control-plane

# Or just run directly
go run ./cmd/control-plane
```

### Step 2: Set Environment Variables

Create a startup script `~/MeshVPN-slef-hosting/start-control-plane.sh`:

```bash
#!/bin/bash

cd ~/MeshVPN-slef-hosting/control-plane

# Load environment from parent .env
export $(cat ../.env | grep -v '^#' | xargs)

# Override K8S_CONFIG_PATH with absolute path
export K8S_CONFIG_PATH="$HOME/k3d-kubeconfig.yaml"

# Start control-plane
echo "Starting MeshVPN Control-Plane..."
go run ./cmd/control-plane
```

Make it executable:

```bash
chmod +x ~/MeshVPN-slef-hosting/start-control-plane.sh
```

### Step 3: Start Control-Plane

```bash
# Run the startup script
~/MeshVPN-slef-hosting/start-control-plane.sh
```

**Expected Output:**

```
Control plane starting...
Running database migrations...
Migration 001_initial.sql: OK
Migration 002_add_deployment_fields.sql: OK
Migration 003_users_and_analytics.sql: OK
Analytics collector started interval=1m
Listening on :8080
Worker started (poll interval: 5s)
```

**Keep this terminal running!**

---

## Verification

### Step 1: Check All Services

Open a **new terminal** and run:

```bash
# K3D cluster
k3d cluster list
# Should show: meshvpn (running)

# Kubernetes nodes
kubectl get nodes
# Should show: 1 node Ready

# Docker containers
docker ps
# Should show: k3d-meshvpn-*, cloudflared-1, observability_*

# Control-plane health
curl http://localhost:8080/health
# Should return: {"status":"LaptopCloud running"}
```

### Step 2: Check Resource Usage

```bash
# Docker resource usage
docker stats --no-stream

# Expected:
# cloudflared-1:          ~20-30 MB
# observability_prometheus: ~100-150 MB
# observability_grafana:    ~100-150 MB
# k3d-meshvpn-server:       ~400-500 MB
# k3d-meshvpn-serverlb:     ~10-20 MB
# k3d-meshvpn-tools:        ~10-20 MB
# TOTAL: ~700-900 MB
```

### Step 3: Check Grafana

```bash
# Open in browser
http://localhost:3001

# Should auto-login
# Navigate to Dashboards → Browse → MeshVPN
```

### Step 4: Check Prometheus

```bash
# Open in browser
http://localhost:9090

# Go to Status → Targets
# Should show: control-plane (UP)
```

---

## First Deployment Test

### Test 1: Simple Deployment

```bash
# In a new terminal
curl -X POST http://localhost:8080/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/vercel/next.js",
    "package": "small",
    "port": 3000
  }' | jq

# Save the deployment_id from response
```

**Expected Response:**

```json
{
  "message": "deployment queued",
  "deployment_id": "abc123",
  "status": "queued",
  "package": "small",
  "cpu_cores": 0.5,
  "memory_mb": 512,
  "subdomain": "next-js",
  "url": "https://next-js.keshavstack.tech"
}
```

### Test 2: Monitor Progress

```bash
# Watch control-plane logs
# (in the terminal where control-plane is running)

# Check deployments
curl http://localhost:8080/deployments | jq

# Check Kubernetes resources
kubectl -n meshvpn-apps get deploy,svc,ing,pods

# Get build logs
curl http://localhost:8080/deployments/<deployment_id>/build-logs | jq -r '.build_logs'
```

### Test 3: Check Analytics (After Deployment Runs)

```bash
# Wait for deployment to reach "running" status
# Then wait 60 seconds for metrics collection

curl http://localhost:8080/deployments/<deployment_id>/analytics | jq
```

---

## Resource Usage Summary

**Target Total Resource Usage:**

| Service | CPU | Memory | Purpose |
|---------|-----|--------|---------|
| K3D Server | 0.2-0.5 | 500-800 MB | Kubernetes control plane |
| K3D LoadBalancer | 0.1 | 10-20 MB | Service load balancing |
| K3D Tools | 0.1 | 10-20 MB | Internal K3D utilities |
| Cloudflared | 0.1 | 20-30 MB | Cloudflare tunnel |
| Prometheus | 0.2 | 100-150 MB | Metrics collection |
| Grafana | 0.2 | 100-150 MB | Dashboards |
| Control-Plane | 0.1-0.3 | 50-100 MB | Go application |
| **TOTAL** | **~1.0-1.7 CPU** | **~800-1300 MB** | **Base platform** |

**User Applications:**
- Small package: 0.5 CPU, 512 MB per deployment
- Each deployment adds to total resource usage

**Tips for Minimal Resource Usage:**
1. Keep only 1-2 deployments running at a time for testing
2. Delete old deployments when not needed
3. Use `--servers-memory` flag when creating K3D cluster
4. Disable HPA initially (`ENABLE_CPU_HPA=false`)
5. Increase worker poll interval (`WORKER_POLL_INTERVAL=10s`)

---

## Troubleshooting

### Issue: kubectl connection refused

**Symptoms:**
```
The connection to the server 0.0.0.0:xxxxx was refused
```

**Solutions:**

```bash
# 1. Check if cluster is running
k3d cluster list

# 2. If not running, start it
k3d cluster start meshvpn

# 3. Update kubeconfig
export KUBECONFIG=$(k3d kubeconfig write meshvpn)

# 4. Test
kubectl get nodes
```

### Issue: Docker out of memory

**Symptoms:**
- Containers crash with OOM (Out of Memory)
- Docker Desktop shows high memory usage

**Solutions:**

```bash
# 1. Reduce Prometheus retention
# Edit infra/observability/docker-compose.yml
# Change: --storage.tsdb.retention.time=24h (from 72h)

# 2. Reduce concurrent deployments
# Only keep 1-2 running at a time

# 3. Increase Docker Desktop memory limit
# Docker Desktop → Settings → Resources → Memory
# Increase to 4GB or higher
```

### Issue: Control-plane can't find kubeconfig

**Symptoms:**
```
failed to create kubernetes client: unable to load config
```

**Solutions:**

```bash
# 1. Verify kubeconfig exists
ls -lh ~/k3d-kubeconfig.yaml

# 2. If missing, recreate
k3d kubeconfig write meshvpn > ~/k3d-kubeconfig.yaml

# 3. Update .env with absolute path
# K8S_CONFIG_PATH=/home/your-username/k3d-kubeconfig.yaml

# 4. Restart control-plane
```

### Issue: ImagePullBackOff errors

**Symptoms:**
```
kubectl -n meshvpn-apps get pods
# Shows: ImagePullBackOff or ErrImagePull
```

**Solutions:**

```bash
# 1. Re-login to GHCR
echo $GITHUB_TOKEN | docker login ghcr.io -u your-github-username --password-stdin

# 2. Delete and recreate secret
kubectl delete secret ghcr-secret -n meshvpn-apps

kubectl create secret docker-registry ghcr-secret \
  --docker-server=ghcr.io \
  --docker-username=your-github-username \
  --docker-password=$GITHUB_TOKEN \
  -n meshvpn-apps

# 3. Re-patch service account
kubectl patch serviceaccount default \
  -n meshvpn-apps \
  -p '{"imagePullSecrets": [{"name": "ghcr-secret"}]}'

# 4. Delete pod to retry
kubectl -n meshvpn-apps delete pod <pod-name>
```

---

## Quick Reference Commands

```bash
# Start all services
k3d cluster start meshvpn
cd ~/MeshVPN-slef-hosting/infra && docker compose up -d
cd ~/MeshVPN-slef-hosting/infra/observability && docker compose up -d
~/MeshVPN-slef-hosting/start-control-plane.sh

# Stop all services
pkill -f control-plane
cd ~/MeshVPN-slef-hosting/infra && docker compose down
cd ~/MeshVPN-slef-hosting/infra/observability && docker compose down
k3d cluster stop meshvpn

# Check status
k3d cluster list
kubectl get nodes
docker ps
curl http://localhost:8080/health

# Resource usage
docker stats --no-stream
kubectl top nodes  # Requires metrics-server
kubectl -n meshvpn-apps get pods

# Logs
docker logs cloudflared-1
docker logs observability_prometheus
docker logs observability_grafana
kubectl -n meshvpn-apps logs <pod-name>
```

---

## Next Steps

1. ✅ Run the [TESTING-QUICK-START.md](TESTING-QUICK-START.md) guide
2. ✅ Deploy a test application
3. ✅ Check analytics in Grafana
4. ✅ Read [PACKAGES.md](PACKAGES.md) for package details
5. ✅ Explore [ANALYTICS-API.md](ANALYTICS-API.md) for metrics integration

---

**Installation Complete!** 🎉

Your MeshVPN platform is now running with minimal resource consumption.

**Total Setup Time:** ~15-20 minutes
**Resource Usage:** ~800-1300 MB RAM, ~1-1.7 CPU cores
**Ready for:** Testing and development deployments

---

**Last Updated:** 2026-03-21
**Tested On:** WSL2 Debian, Docker Desktop, K3D v5.x
