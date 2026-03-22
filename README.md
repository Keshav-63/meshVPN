# MeshVPN Self-Hosting Platform

**Current Architecture**: Windows → WSL2 (Debian) → K3D → Cloudflare Tunnel

A complete self-hosting platform that turns your laptop into a deployment engine with Kubernetes orchestration:

1. **Choose a package** (Small/Medium/Large) for simple resource allocation
2. **Auto-generate subdomains** from GitHub repo names with conflict detection
3. **Clone & build** Git repositories into Docker images
4. **Push to GHCR** (GitHub Container Registry) with automatic authentication
5. **Deploy to K3D** Kubernetes cluster with autoscaling (subscribers)
6. **Monitor with analytics** - real-time metrics via REST API and Server-Sent Events (SSE)
7. **Route traffic** via `<subdomain>.keshavstack.tech` through Cloudflare Tunnel

## Quick Start

### 🚀 New User? Start Here!

**15-minute setup with minimal resources (~800-1300 MB RAM):**

👉 **[QUICK-START.md](QUICK-START.md)** - Fast track to get running

### 🔄 Clean Installation from Scratch

**Have previous setup? Start fresh:**

👉 **[Fresh Installation Guide](docs/FRESH-INSTALL.md)** - Complete cleanup and reinstall

Quick cleanup:
```bash
./scripts/cleanup-all.sh
```

### 📚 Detailed Setup Guide

**Comprehensive documentation:**

👉 **[Complete Setup Guide](docs/SETUP.md)** - Full installation and configuration

## Architecture Components

- `control-plane/`: Go API that orchestrates deployments with async worker queue
- `apps/`: Local checkout area for cloned repositories
- `infra/docker-compose.yml`: Runs Cloudflare Tunnel
- `infra/observability/`: Lean Prometheus + Grafana stack (350MB limit)
- `scripts/`: Cloudflare Tunnel automation scripts

## System Requirements

### Windows Host
- Windows 10/11 with WSL2 enabled
- Docker Desktop installed and running

### WSL2 (Debian)
- Go 1.21+
- Git
- kubectl
- K3D
- Docker CLI (via Docker Desktop)

### External Services
- Cloudflare account (for tunnel and domain)
- GitHub Container Registry access
- Supabase or PostgreSQL database

**Detailed installation instructions**: [docs/SETUP.md](docs/SETUP.md)

## How It Works

### Deployment Flow

1. **User submits deploy request** → Control-plane API (via Cloudflare Tunnel)
2. **Worker picks up job** → Clone repo, build image, push to GHCR
3. **K3D deployment** → Create Kubernetes resources (Deployment, Service, Ingress, HPA)
4. **Cloudflare routes traffic** → `*.keshavstack.tech` → K3D Traefik → App pods

### Key Architectural Decisions

- **K3D instead of native K3s**: Avoids WSL2 cgroup crash loops
- **host.docker.internal routing**: Bridges Docker Desktop's WSL2 VM networking
- **3-tier package system**: Simplifies resource selection (Small/Medium/Large)
- **Subscription-based autoscaling**: HPA only for subscribers, free tier gets fixed replicas
- **Auto-subdomain generation**: Extracted from GitHub repo names with conflict detection
- **PostgreSQL analytics**: 1-minute aggregation with Server-Sent Events (SSE) streaming
- **ServiceAccount imagePullSecrets**: Universal fix for GHCR authentication
- **Auto-provisioned Grafana**: Dashboards and datasources configured automatically

## Quick Deploy Example

```bash
# Health check
curl https://self.keshavstack.tech/health

# Deploy an app with Medium package
curl -X POST https://self.keshavstack.tech/deploy \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/your-org/your-app.git",
    "package": "medium",
    "port": 3000
  }'

# Subdomain auto-generated from repo name: your-app.keshavstack.tech
# Package provides: 1 CPU core, 1GB RAM, max 5 replicas (if subscriber)

# Check deployment status
curl https://self.keshavstack.tech/deployments

# View real-time analytics
curl https://self.keshavstack.tech/deployments/{id}/analytics \
  -H "Authorization: Bearer $TOKEN"
```

## Environment Configuration

### 1. Cloudflare Tunnel Setup

Use the automated script:

```bash
cd scripts
# Edit setup-cloudflare-tunnel.go with your API credentials
go run setup-cloudflare-tunnel.go
```

The script will:
- Create tunnel "MeshVPN_SelfHosting"
- Set up `*.keshavstack.tech` → `http://host.docker.internal:80`
- Set up `self.keshavstack.tech` → `http://host.docker.internal:8080`
- Generate tunnel token for docker-compose

**Why host.docker.internal?** Docker Desktop creates a separate VM for WSL2. This bridges the networking gap.

### 2. Environment Variables

Copy `.env.example` to `.env` (at project root) and configure:

```env
CLOUDFLARE_TUNNEL_TOKEN=<from_setup_script>
APP_BASE_DOMAIN=keshavstack.tech
DATABASE_URL=<your_postgres_url>
SUPABASE_JWT_SECRET=<your_jwt_secret>
RUNTIME_BACKEND=k3s
ENABLE_CPU_HPA=true
K8S_NAMESPACE=meshvpn-apps
K8S_IMAGE_PREFIX=ghcr.io/your-github-username
```

**Complete setup guide**: [docs/SETUP.md](docs/SETUP.md)

## API Endpoints

### Deployment Management

```bash
# Health check
GET /health

# Prometheus metrics
GET /metrics

# Submit deployment (returns 202 Accepted with status: "queued")
POST /deploy
{
  "repo": "https://github.com/user/repo.git",
  "package": "medium",              // small, medium, or large (default: small)
  "port": 3000,
  "subdomain": "myapp",             // optional - auto-generated from repo if not provided
  "env": {
    "NODE_ENV": "production"
  },
  "build_args": {
    "NEXT_PUBLIC_API_BASE": "https://api.example.com"
  }
}

# List all deployments
GET /deployments

# Get build logs
GET /deployments/<deployment_id>/build-logs

# Get application logs (tail optional, default 200)
GET /deployments/<deployment_id>/app-logs?tail=300

# Get deployment analytics (snapshot)
GET /deployments/<deployment_id>/analytics

# Stream real-time analytics (SSE)
GET /deployments/<deployment_id>/analytics/stream
```

**Documentation**:
- [Frontend API Integration](docs/frontend-api-integration.md) - API integration guide
- [Analytics API](docs/ANALYTICS-API.md) - Real-time metrics and SSE streaming
- [Packages](docs/PACKAGES.md) - Resource package specifications

## Features

### Current Implementation

✅ **Resource Packages**: 3-tier system (Small/Medium/Large) for easy resource selection
✅ **Auto-Subdomain Generation**: Extracted from GitHub repo names with conflict detection
✅ **Subscription-Based Autoscaling**: HPA enabled for subscribers (fixed replicas for free tier)
✅ **Real-time Analytics**: REST API + Server-Sent Events (SSE) for live metrics
✅ **Deployment Metrics**: Request counts, latency percentiles (p50/p90/p99), bandwidth, pod status
✅ **User Tracking**: PostgreSQL-based user management with subscription tiers
✅ **Async Deployment Queue**: Background worker processes deployments
✅ **Kubernetes Orchestration**: K3D-based container scheduling
✅ **Build & Runtime Logs**: Complete visibility into deployment process
✅ **Cloudflare Tunnel**: Secure public access without port forwarding
✅ **GitHub Container Registry**: Automated image push/pull
✅ **Observability**: Prometheus metrics + Grafana dashboards (auto-provisioned)
✅ **Supabase Auth**: JWT-based authentication (GitHub OAuth)
✅ **Database Persistence**: PostgreSQL/Supabase for deployment state and analytics

### Resource Packages

| Package | CPU Cores | Memory | Max Replicas | Best For |
|---------|-----------|--------|--------------|----------|
| Small   | 0.5       | 512 MB | 3            | Static sites, simple APIs |
| Medium  | 1.0       | 1 GB   | 5            | Web apps, microservices |
| Large   | 2.0       | 2 GB   | 10           | Resource-intensive apps |

**Autoscaling**: Subscribers get horizontal pod autoscaling (HPA) based on CPU usage. Non-subscribers run 1 fixed replica.

See [PACKAGES.md](docs/PACKAGES.md) for complete specifications.

### Deployment Requirements

- Repository must have a `Dockerfile` in its root
- Docker build must succeed
- Application must listen on the specified port

## Documentation

### Setup & Configuration
- **[Complete Setup Guide](docs/SETUP.md)** - Full installation and configuration for K3D + WSL2
- **[Analytics API](docs/ANALYTICS-API.md)** - Real-time metrics, SSE streaming, and frontend integration
- **[Resource Packages](docs/PACKAGES.md)** - Package specifications and autoscaling behavior
- **[Grafana Setup](docs/GRAFANA-SETUP.md)** - Platform monitoring dashboards and customization
- **[Frontend API Integration](docs/frontend-api-integration.md)** - How to integrate with the control-plane API

### Testing
- **[End-to-End Testing Guide](docs/E2E-TESTING.md)** - Comprehensive testing procedures for all features
- **[Postman Collection](postman/)** - Pre-built Postman tests with environment files
- **[Automated Test Script](scripts/test-e2e.sh)** - CLI-based testing script

### Architecture Details
- **K3D Cluster**: Lightweight K3s running in Docker (avoids WSL2 cgroup issues)
- **Networking**: Cloudflare Tunnel with `host.docker.internal` routing
- **Registry**: GitHub Container Registry with ServiceAccount authentication
- **Observability**: Lean Prometheus + Grafana (350MB total) with auto-provisioned dashboards
- **Analytics**: PostgreSQL-backed metrics collection with 1-minute aggregation
- **Packages**: 3-tier resource system (Small/Medium/Large) with subscriber-based autoscaling

## Troubleshooting

### Common Issues

**ImagePullBackOff**
- Verify `ghcr-secret` exists: `kubectl get secret ghcr-secret -n meshvpn-apps`
- Check ServiceAccount has imagePullSecrets: `kubectl get sa default -n meshvpn-apps -o yaml`
- Re-authenticate with GHCR: `docker login ghcr.io`

**HPA Not Scaling**
- Check metrics-server is running (included in K3D by default)
- Verify pod resource requests are set
- Inspect HPA: `kubectl describe hpa <name> -n meshvpn-apps`

**Cloudflare 502 Errors**
- Check tunnel container: `docker logs cloudflared`
- Verify routes use `host.docker.internal` not `localhost`
- Restart tunnel: `cd infra && docker compose restart cloudflared`

**See full troubleshooting guide**: [docs/SETUP.md#troubleshooting](docs/SETUP.md#troubleshooting)