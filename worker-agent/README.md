# Worker Agent

Remote worker binary for distributed deployment across multiple machines using Tailscale mesh network.

## Latest Worker Changes

No new fields are required in `agent.yaml` for the recent failover/rebalance rollout.

Worker update requirements:

1. Pull latest code for `worker-agent/`
2. Rebuild worker binary
3. Restart worker process/service

Why restart is required:

1. Worker now reports `deployment_id` when sending `job-complete` and `job-failed`
2. This keeps deployment ownership/status accurate during failover and rebalance

Quick update commands:

```bash
cd worker-agent
go mod tidy
go build -o worker-agent cmd/worker-agent/main.go
./worker-agent -config agent.yaml
```

## Quick Start

### 1. Install Prerequisites on Worker Machine

```bash
# Install Tailscale
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up

# Verify Tailscale IP
tailscale ip -4
# Output: 100.64.1.2 (your worker's IP)

# Install Docker
curl -fsSL https://get.docker.com | sh

# Install K3D
curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash

# Create K8s cluster
k3d cluster create worker-cluster \
  --port "80:80@loadbalancer" \
  --agents 0 \
  --servers 1

# Verify cluster
kubectl cluster-info
```

### 2. Configure Worker

```bash
cd worker-agent

# Edit agent.yaml
nano agent.yaml
```

**Update these fields:**
- `worker.id`: Unique ID (e.g., `worker-laptop-1`)
- `worker.name`: Descriptive name (e.g., `"Keshav's Laptop"`)
- `control_plane.url`: Control-plane Tailscale IP (get from control-plane: `tailscale ip -4`)
- `runtime.kubeconfig`: Path to your kubeconfig file
- `capabilities.memory_gb`: Total RAM on this machine
- `capabilities.cpu_cores`: Total CPU cores

### 3. Build and Run Worker

```bash
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

Worker will now:
- Send heartbeat every 30 seconds
- Poll for jobs every 5 seconds
- Execute assigned deployments
- Report completion/failure to control-plane

### 4. Verify Worker Registration

On control-plane machine:

```bash
curl http://localhost:8080/workers \
  -H "Authorization: Bearer <your-jwt-token>" | jq
```

You should see your worker listed with `status: "idle"`.

## Configuration Reference

### agent.yaml Structure

```yaml
worker:
  id: string                    # Unique worker ID (required)
  name: string                  # Display name (required)
  tailscale_ip: string          # Auto-detected if empty
  max_concurrent_jobs: int      # How many jobs can run simultaneously

control_plane:
  url: string                   # Control-plane URL (required)

runtime:
  type: string                  # "kubernetes" or "docker"
  kubeconfig: string            # Path to kubeconfig
  namespace: string             # K8s namespace for deployments
  kubectl_bin: string           # Path to kubectl binary
  metrics_port: int             # Metrics HTTP port (default 9091)

capabilities:
  memory_gb: int                # Total RAM (GB)
  cpu_cores: int                # Total CPU cores
  supported_packages: []string  # ["small", "medium", "large"]
```

## How It Works

### Worker Lifecycle

1. **Startup**: Worker reads config, detects Tailscale IP
2. **Registration**: POST to `/api/workers/register` with capabilities
3. **Heartbeat Loop**: POST to `/api/workers/:id/heartbeat` every 30s
4. **Job Polling**: GET `/api/workers/:id/claim-job` every 5s
5. **Job Execution**: Clone → Build → Push → Deploy to local K8s cluster
6. **Completion**: POST to `/api/workers/:id/job-complete` or `job-failed`

### Job Execution Flow

When worker claims a job:

1. **Clone Repository**: `git clone <repo> /tmp/<deployment-id>`
2. **Build Image**: `docker build -t <image> /tmp/<deployment-id>`
3. **Push to GHCR**: `docker push <image>`
4. **Deploy to K8s**: `kubectl apply -f -` (manifest with Deployment, Service, Ingress)
5. **Report Success**: Notify control-plane job is done

## Networking

Workers connect to control-plane via **Tailscale mesh network**:

- No port forwarding required
- Encrypted peer-to-peer communication
- Works across NAT, firewalls, different networks
- Control-plane and all workers must be on same Tailscale network

**Verify Connectivity:**
```bash
# From worker machine, ping control-plane
ping 100.64.1.1  # Replace with control-plane IP

# Test HTTP connectivity
curl http://100.64.1.1:8080/health
```

### Start Cloudflare Tunnel for Public Access

Use this when deployments are reachable locally with a Host header but not publicly from the internet.

```bash
# Start tunnel
cd /mnt/c/Users/Shreeyansh/Desktop/MeshVPN-Veltrix/meshVPN/infra
ln -sf ../.env .env
docker compose up -d cloudflared

# Check status
docker compose ps

# Check logs
docker compose logs -f cloudflared
```

## Troubleshooting

### Worker Fails to Register

**Error:** `Failed to register with control-plane: connection refused`

**Solutions:**
1. Verify Tailscale is running: `tailscale status`
2. Check control-plane URL in `agent.yaml` is correct
3. Ping control-plane: `ping <control-plane-ip>`
4. Verify control-plane is running: `curl http://<control-plane-ip>:8080/health`
5. Check firewall isn't blocking Tailscale traffic

### Worker Shows as Offline

**Error:** Worker shows `status: "offline"` in `/workers` list

**Solutions:**
1. Check worker is still running: `ps aux | grep worker-agent`
2. Verify heartbeat is being sent (check worker logs)
3. Increase `WORKER_HEARTBEAT_TIMEOUT` on control-plane
4. Restart worker agent

### Jobs Not Assigned to Worker

**Error:** Worker stays idle, jobs queued but never assigned

**Solutions:**
1. Verify `ENABLE_MULTI_WORKER=true` in control-plane `.env`
2. Check worker status is `"idle"` not `"busy"` or `"offline"`
3. Verify job placement strategy matches package size
4. Check distributor logs on control-plane

### Job Execution Fails

**Error:** Worker claims job but execution fails

**Solutions:**
1. Check Docker is running: `docker ps`
2. Verify kubectl works: `kubectl get nodes`
3. Check GHCR authentication: `docker login ghcr.io`
4. Ensure worker has internet access (to clone from GitHub)
5. Review worker logs for specific error

### Metrics Port Already in Use

**Error:** `Metrics server error: listen tcp :9090: bind: address already in use`

**Cause:** Another process is already using the metrics port (for example local Prometheus on 9090).

**Solutions:**
1. Stop all worker processes: `pkill -f worker-agent`
2. Confirm none are running: `pgrep -af worker-agent`
3. Check who owns the port: `ss -ltnp | grep :9090`
4. Set a different metrics port in `agent.yaml` (example `runtime.metrics_port: 9091`)
5. Start worker: `./worker-agent -config agent.yaml`

### Deployment URL Not Publicly Reachable

**Symptom:** Deployment works with local Host header, but public URL returns 404 or is unreachable.

**Solutions:**
1. Verify ingress host exists: `kubectl get ingress -n worker-apps`
2. Verify backend endpoints exist: `kubectl get endpoints -n worker-apps`
3. Start Cloudflare tunnel from `infra/` and verify logs
4. Confirm URL matches deployed subdomain exactly (for example `final-test.keshavstack.tech` vs `final-worker.keshavstack.tech`)

### Worker Heartbeat / Claim Timeouts

**Error:** `context deadline exceeded` on `/heartbeat` or `/claim-job`

**Solutions:**
1. Confirm control-plane is up: `curl http://<control-plane-ip>:8080/health`
2. Verify Tailscale connectivity both ways: `tailscale status`
3. Check for network flaps or firewall rules affecting Tailscale traffic
4. Keep worker running; it auto-recovers and resumes job polling when control-plane is reachable

## Running as Service

### systemd (Linux)

Create `/etc/systemd/system/worker-agent.service`:

```ini
[Unit]
Description=MeshVPN Worker Agent
After=network.target tailscaled.service

[Service]
Type=simple
User=youruser
WorkingDirectory=/home/youruser/worker-agent
ExecStart=/home/youruser/worker-agent/worker-agent -config /home/youruser/worker-agent/agent.yaml
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Enable and start:
```bash
sudo systemctl daemon-reload
sudo systemctl enable worker-agent
sudo systemctl start worker-agent
sudo systemctl status worker-agent
```

View logs:
```bash
sudo journalctl -u worker-agent -f
```

## Scaling Workers

To add more workers:

1. **Setup new machine** with Tailscale + K3D
2. **Copy `agent.yaml`** to new machine
3. **Change `worker.id`** to unique value (e.g., `worker-server-2`)
4. **Update capabilities** (memory_gb, cpu_cores)
5. **Build and run** worker agent
6. **Verify registration** via `/workers` API

All workers will automatically receive jobs based on placement strategy!

## Security

- **Worker Auth**: Shared-secret auth is not enforced in current code path; keep Tailscale ACLs strict.
- **Tailscale ACLs**: Restrict which machines can communicate
- **Network Isolation**: Workers should only access control-plane, not each other
- **Container Registry**: Ensure workers have GHCR authentication
- **Kubernetes RBAC**: Limit worker service account permissions

## Monitoring

View worker status:
```bash
# List all workers
curl http://<control-plane>:8080/workers -H "Authorization: Bearer <token>"

# Watch worker activity
watch -n 2 'curl -s http://localhost:8080/workers -H "Authorization: Bearer <token>" | jq ".workers[] | {worker_id, status, current_jobs}"'
```

## Next Steps

- Monitor worker performance via Prometheus/Grafana
- Add worker tags for capability matching (GPU, high-memory)
- Implement worker auto-scaling based on queue depth
- Add geographic routing (deploy to closest worker)

---

**Full Documentation:** [MULTI-WORKER-SETUP.md](../docs/MULTI-WORKER-SETUP.md)
