# Quick Start - Get MeshVPN Running in 15 Minutes

**Total Resource Usage:** ~800-1300 MB RAM, ~1-1.7 CPU cores

---

## Step 1: Cleanup (If Needed)

If you have any previous installation:

```bash
cd ~/MeshVPN-slef-hosting
./scripts/cleanup-all.sh
```

**Confirmation:** Type `yes` when prompted

---

## Step 2: Create K3D Cluster

```bash
k3d cluster create meshvpn \
  --port "80:80@loadbalancer" \
  --port "443:443@loadbalancer" \
  --agents 0 \
  --servers 1
```

**Note:** Traefik ingress controller is included by default (required for routing)

**Wait:** 30-60 seconds for cluster to be ready

**Verify:**
```bash
kubectl get nodes
# Should show 1 node in Ready state

# Verify Traefik is running
kubectl -n kube-system get pods | grep traefik
```

---

## Step 3: Create Namespace & GHCR Secret

```bash
# Create namespace
kubectl create namespace meshvpn-apps

# Export kubeconfig (use 'get' not 'write' to avoid corruption)
k3d kubeconfig get meshvpn > ~/k3d-kubeconfig.yaml

# Also copy to default kubectl location
cp ~/.config/k3d/kubeconfig-meshvpn.yaml ~/.kube/config

# Login to GHCR
export GITHUB_TOKEN="your_github_token_here"
echo $GITHUB_TOKEN | docker login ghcr.io -u your-github-username --password-stdin

# Create secret
kubectl create secret docker-registry ghcr-secret \
  --docker-server=ghcr.io \
  --docker-username=your-github-username \
  --docker-password=$GITHUB_TOKEN \
  -n meshvpn-apps

# Patch service account
kubectl patch serviceaccount default \
  -n meshvpn-apps \
  -p '{"imagePullSecrets": [{"name": "ghcr-secret"}]}'
```

---

## Step 4: Configure Environment

```bash
cd ~/MeshVPN-slef-hosting
cp .env.example .env
nano .env
```

**Minimum required in `.env`:**

```env
DATABASE_URL=postgresql://user:password@host:5432/dbname
SUPABASE_JWT_SECRET=your_jwt_secret
REQUIRE_AUTH=false
RUNTIME_BACKEND=k3s
ENABLE_CPU_HPA=false
K8S_NAMESPACE=meshvpn-apps
K8S_CONFIG_PATH=/home/your-username/k3d-kubeconfig.yaml
KUBECTL_BIN=kubectl
K8S_IMAGE_PREFIX=ghcr.io/your-github-username
APP_BASE_DOMAIN=keshavstack.tech
WORKER_POLL_INTERVAL=5s
CLOUDFLARE_TUNNEL_TOKEN=your_tunnel_token
```

**Replace:**
- `your-username` (run `echo $USER`)
- `your-github-username`
- Database credentials
- Cloudflare tunnel token

---

## Step 5: Start Cloudflare Tunnel

```bash
# Cloudflare Tunnel
cd ~/MeshVPN-slef-hosting/infra
docker compose up -d

# Verify tunnel is running
docker logs cloudflared-1 --tail 10
```

**Note:** Observability stack (Prometheus/Grafana) setup is optional and currently has networking limitations with WSL2. Skip for now.

---

## Step 6: Start Control-Plane

**Terminal 1 (keep running):**

```bash
cd ~/MeshVPN-slef-hosting
./start-control-plane.sh
```

**Expected output:**
```
[INFO] [store] initializing postgres repositories
deployment repository init failed, falling back to in-memory store: ...
[INFO] [main] starting router require_auth=false has_database=false analytics=false
[INFO] [worker] deployment worker started poll_interval=2s
Listening and serving HTTP on 0.0.0.0:8080
```

**Note:** Database connection failures are expected if using in-memory mode. The system will still work for testing.

---

## Step 7: Verify Everything Works

**Terminal 2 (new terminal):**

```bash
# Health check
curl http://localhost:8080/health
# Expected: {"status":"LaptopCloud running"}

# Kubernetes
kubectl get nodes
# Expected: 1 node Ready

# Docker containers
docker ps
# Expected: 6-8 containers running

# Resource usage
docker stats --no-stream
# Expected: ~700-1000 MB total
```

---

## Step 8: Test Deployment

```bash
# Deploy test app (use smaller app for faster testing)
curl -X POST http://localhost:8080/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/dockersamples/static-site",
    "package": "small",
    "port": 80
  }' | jq

# List deployments
curl http://localhost:8080/deployments | jq

# Watch deployment progress
kubectl -n meshvpn-apps get pods -w

# Check ingress once pod is running
kubectl -n meshvpn-apps get ingress
```

**Note:** Deployment timeout is set to 600 seconds (10 minutes) to handle large images. Watch the control-plane logs for progress.

**Access your app:**
Once status shows "running", access at: `https://<subdomain>.keshavstack.tech`
(subdomain is auto-generated from repo name, e.g., "static-site")

---

## URLs

- **Control-Plane API:** http://localhost:8080
- **Health Check:** http://localhost:8080/health
- **Metrics:** http://localhost:8080/metrics
- **Deployed Apps:** https://<subdomain>.keshavstack.tech

---

## Troubleshooting

### kubectl connection refused

```bash
# Fix kubeconfig
cp ~/.config/k3d/kubeconfig-meshvpn.yaml ~/.kube/config
kubectl get nodes
```

### Control-plane can't connect to cluster

```bash
# Verify kubeconfig path
ls -lh ~/k3d-kubeconfig.yaml

# Update .env
nano .env
# Set: K8S_CONFIG_PATH=/home/your-username/k3d-kubeconfig.yaml

# Restart control-plane
```

### Docker out of memory

```bash
# Reduce retention in prometheus
# Edit: infra/observability/docker-compose.yml
# Change: --storage.tsdb.retention.time=24h
docker compose restart prometheus
```

---

## Complete Documentation

📖 **[Fresh Installation Guide](docs/FRESH-INSTALL.md)** - Detailed step-by-step

📖 **[Setup Guide](docs/SETUP.md)** - Complete configuration reference

📖 **[Testing Guide](TESTING-QUICK-START.md)** - How to test everything

📖 **[Analytics API](docs/ANALYTICS-API.md)** - Metrics and monitoring

📖 **[Packages](docs/PACKAGES.md)** - Resource specifications

---

## Daily Usage

### Start All Services

```bash
cd ~/MeshVPN-slef-hosting
./scripts/start-all.sh
./start-control-plane.sh  # In separate terminal
```

### Stop All Services

```bash
# Stop control-plane (Ctrl+C in terminal)
cd ~/MeshVPN-slef-hosting/infra && docker compose down
k3d cluster stop meshvpn
```

### Complete Cleanup

```bash
cd ~/MeshVPN-slef-hosting
./scripts/cleanup-all.sh
```

---

**Setup Time:** 15-20 minutes

**Resource Usage:** ~800-1300 MB RAM

**Ready for:** Testing and development

---

## Recent Changes (2026-03-22)

### ✅ Fixed Issues
- **Traefik Ingress:** Enabled by default (removed `--disable=traefik` flag)
- **Kubeconfig Generation:** Changed from `k3d kubeconfig write` to `k3d kubeconfig get` to avoid file corruption
- **Deployment Timeout:** Increased from 180s to 600s (10 minutes) to handle large Docker images
- **Control-Plane Binding:** Changed from `:8080` to `0.0.0.0:8080` to allow external access
- **kubectl Configuration:** Auto-copy kubeconfig to both `~/.kube/config` and `~/k3d-kubeconfig.yaml`

### ⚠️ Known Issues
- **Observability Stack:** Prometheus/Grafana have networking limitations with WSL2 + Docker Desktop. Metrics collection deferred.
- **Database Connection:** In-memory mode used by default. PostgreSQL analytics not yet fully implemented.
- **Large Images:** Next.js and similar large frameworks may take 5-10 minutes for first deployment due to image pull time.

### 📝 Configuration Changes
- Control-plane now listens on `0.0.0.0:8080` instead of `localhost:8080`
- Deployment timeout increased to 600 seconds in [kubernetes_driver.go:70](control-plane/internal/runtime/kubernetes_driver.go#L70)
- Traefik ingress controller included by default for routing

---

**Need Help?** See [FRESH-INSTALL.md](docs/FRESH-INSTALL.md) for detailed troubleshooting
