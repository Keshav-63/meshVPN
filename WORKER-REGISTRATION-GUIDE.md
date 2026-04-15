# Complete Worker Registration Guide

Step-by-step guide to register a remote worker with the control-plane.

---


## 🎯 Overview

**What you need:**
- A separate machine (laptop/server/VM) for the worker
- Control-plane running and accessible via Tailscale
- Both machines on the same Tailscale network

**What happens during registration:**
1. Worker connects to control-plane via Tailscale
2. Worker sends registration request with capabilities
3. Control-plane stores worker in `workers` table
4. Worker starts sending heartbeats every 30 seconds
5. Worker polls for jobs every 5 seconds
6. Control-plane assigns jobs based on smart placement strategy

---

## 📋 Method 1: Automated Setup (Recommended)

### On Worker Machine:

```bash
# 1. Copy worker-agent folder to worker machine
scp -r ~/MeshVPN-slef-hosting/worker-agent/ user@worker-machine:~/

# 2. SSH into worker machine
ssh user@worker-machine

# 3. Run setup script
cd ~/worker-agent
chmod +x setup-worker.sh
./setup-worker.sh
```

**The script will:**
- ✅ Install Tailscale (if needed)
- ✅ Install Docker (if needed)
- ✅ Install K3D (if needed)
- ✅ Create K3D cluster
- ✅ Configure agent.yaml with your inputs
- ✅ Build worker-agent binary
- ✅ Start worker

**Script will ask you:**
1. **Worker ID**: Unique identifier (e.g., `worker-laptop-1`)
2. **Worker Name**: Human-readable name (e.g., `"Keshav's Laptop"`)
3. **Control-plane IP**: Tailscale IP of control-plane (default: `100.107.233.70`)
4. **Shared Secret**: From control-plane `.env` (default: `meshvpn-worker-secret-change-in-production`)

**Expected output:**
```
[INFO] Starting worker agent: Keshav's Laptop Worker (worker-laptop-1)
[INFO] Auto-detected Tailscale IP: 100.64.1.2
[INFO] Successfully registered with control-plane
```

---

## 📋 Method 2: Manual Setup

### Step 1: Install Prerequisites on Worker Machine

#### 1.1 Install Tailscale

```bash
# Install Tailscale
curl -fsSL https://tailscale.com/install.sh | sh

# Connect to Tailscale network
sudo tailscale up

# Get your Tailscale IP (save this!)
tailscale ip -4
# Output: 100.64.1.2 (example)
```

#### 1.2 Install Docker

```bash
# Install Docker
curl -fsSL https://get.docker.com | sh

# Add user to docker group
sudo usermod -aG docker $USER

# Log out and log back in for group changes to take effect
```

#### 1.3 Install K3D

```bash
# Install K3D
curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash

# Verify installation
k3d version
```

#### 1.4 Install Go (if not installed)

```bash
# Download Go 1.23
wget https://go.dev/dl/go1.23.0.linux-amd64.tar.gz

# Extract
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.23.0.linux-amd64.tar.gz

# Add to PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Verify
go version
```

---

### Step 2: Setup K3D Cluster

```bash
# Create K3D cluster for this worker
k3d cluster create worker-cluster \
  --port "80:80@loadbalancer" \
  --agents 0 \
  --servers 1

# Verify cluster is running
kubectl cluster-info

# Check nodes
kubectl get nodes
```

**Expected output:**
```
NAME                           STATUS   ROLES                  AGE   VERSION
k3d-worker-cluster-server-0    Ready    control-plane,master   30s   v1.31.0+k3s1
```

---

### Step 3: Copy Worker Agent to Worker Machine

**On control-plane machine:**

```bash
# Copy worker-agent folder to worker machine
scp -r ~/MeshVPN-slef-hosting/worker-agent/ user@worker-machine:~/worker-agent/
```

**Or clone the repository directly on worker machine:**

```bash
# On worker machine
git clone <your-repo-url>
cd MeshVPN-slef-hosting/worker-agent
```

---

### Step 4: Configure Worker Agent

**On worker machine:**

```bash
cd ~/worker-agent

# Copy example config
cp agent.yaml.example agent.yaml

# Edit configuration
nano agent.yaml
```

**Update `agent.yaml`:**

```yaml
worker:
  # CHANGE THIS: Unique identifier for this worker
  id: worker-laptop-1

  # CHANGE THIS: Human-readable name
  name: "Keshav's Laptop Worker"

  # Leave empty for auto-detection
  tailscale_ip: ""

  # Max concurrent jobs this worker can handle
  max_concurrent_jobs: 2

control_plane:
  # Control-plane Tailscale IP (already configured!)
  url: http://100.107.233.70:8080

  # Must match WORKER_SHARED_SECRET in control-plane .env
  shared_secret: "meshvpn-worker-secret-change-in-production"

runtime:
  type: kubernetes

  # Path to kubeconfig (K3D auto-creates this)
  kubeconfig: /home/YOUR_USERNAME/.kube/config

  # Namespace for deployments
  namespace: worker-apps

  # kubectl binary
  kubectl_bin: kubectl

capabilities:
  # Total RAM on this machine (GB)
  memory_gb: 16

  # Total CPU cores on this machine
  cpu_cores: 8

  # Package sizes this worker supports
  supported_packages:
    - small
    - medium
    - large
```

**Save and exit** (`Ctrl+X`, then `Y`, then `Enter`)

---

### Step 5: Verify Network Connectivity

**Test connection to control-plane:**

```bash
# Ping test
ping 100.107.233.70

# HTTP test
curl http://100.107.233.70:8080/health

# Expected: {"status":"LaptopCloud running"}
```

**If connection fails:**
- Verify both machines are on same Tailscale network: `tailscale status`
- Check control-plane is running: `ps aux | grep control-plane`
- Verify firewall isn't blocking: `sudo ufw status` (if using ufw)

---

### Step 6: Build Worker Agent

```bash
cd ~/worker-agent

# Initialize Go module (if not already done)
go mod init worker-agent
go mod download

# Build binary
go build -o worker-agent cmd/worker-agent/main.go

# Make executable
chmod +x worker-agent

# Verify build
./worker-agent --help
```

---

### Step 7: Start Worker Agent

```bash
# Run worker (foreground)
./worker-agent -config agent.yaml
```

**Expected output:**
```
Auto-detected Tailscale IP: 100.64.1.2
Starting worker agent: Keshav's Laptop Worker (worker-laptop-1)
Successfully registered with control-plane
```

**Worker will now:**
- ✅ Send heartbeat every 30 seconds
- ✅ Poll for jobs every 5 seconds
- ✅ Execute assigned deployments
- ✅ Report completion/failure to control-plane

---

### Step 8: Run Worker as Background Service

**Option A: Using nohup**

```bash
# Start in background
nohup ./worker-agent -config agent.yaml > worker.log 2>&1 &

# View logs
tail -f worker.log

# Stop worker
pkill -f worker-agent
```

**Option B: Using systemd (Recommended for production)**

```bash
# Create systemd service
sudo nano /etc/systemd/system/meshvpn-worker.service
```

**Add this content:**

```ini
[Unit]
Description=MeshVPN Worker Agent
After=network.target tailscaled.service

[Service]
Type=simple
User=YOUR_USERNAME
WorkingDirectory=/home/YOUR_USERNAME/worker-agent
ExecStart=/home/YOUR_USERNAME/worker-agent/worker-agent -config /home/YOUR_USERNAME/worker-agent/agent.yaml
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

**Enable and start service:**

```bash
# Reload systemd
sudo systemctl daemon-reload

# Enable service (start on boot)
sudo systemctl enable meshvpn-worker

# Start service
sudo systemctl start meshvpn-worker

# Check status
sudo systemctl status meshvpn-worker

# View logs
sudo journalctl -u meshvpn-worker -f
```

---

## ✅ Verify Worker Registration

### On Control-Plane Machine:

**Method 1: Database Query**

```bash
psql "postgresql://postgres.rpqlrujltxsaqixzefgb:Ghjklgfdsa@123@aws-1-ap-northeast-2.pooler.supabase.com:5432/postgres" -c "SELECT worker_id, name, tailscale_ip, status, current_jobs, max_concurrent_jobs, last_heartbeat FROM workers ORDER BY created_at DESC;"
```

**Expected output:**
```
      worker_id       |            name              | tailscale_ip   | status | current_jobs | max_concurrent_jobs |     last_heartbeat
----------------------+------------------------------+----------------+--------+--------------+--------------------+------------------------
 worker-laptop-1      | Keshav's Laptop Worker       | 100.64.1.2     | idle   |            0 |                  2 | 2026-03-22 11:35:00+00
 control-plane-local  | Control-Plane (Local Worker) | localhost      | idle   |            0 |                  2 | 2026-03-22 11:35:01+00
```

**Method 2: API Query (requires auth if REQUIRE_AUTH=true)**

```bash
# List all workers
curl http://localhost:8080/workers | jq

# Or with auth token
curl http://localhost:8080/workers \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" | jq
```

**Expected response:**
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
      "last_heartbeat": "2026-03-22T11:35:01Z"
    },
    {
      "worker_id": "worker-laptop-1",
      "name": "Keshav's Laptop Worker",
      "tailscale_ip": "100.64.1.2",
      "hostname": "keshav-laptop",
      "status": "idle",
      "current_jobs": 0,
      "max_concurrent_jobs": 2,
      "last_heartbeat": "2026-03-22T11:35:00Z"
    }
  ]
}
```

**Method 3: Control-Plane Logs**

```bash
# Check control-plane logs for worker registration
grep "worker registered" control-plane-logs.txt

# Or if running in terminal, you'll see:
# [INFO] [workers] worker registered worker_id=worker-laptop-1 name=Keshav's Laptop Worker ip=100.64.1.2
```

---

## 🧪 Test Worker Job Assignment

### Deploy Small Package (Should use control-plane)

```bash
curl -X POST http://localhost:8080/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/dockersamples/static-site",
    "package": "small",
    "subdomain": "test-small"
  }'
```

**Wait 5 seconds, then check assignment:**

```bash
psql "postgresql://postgres.rpqlrujltxsaqixzefgb:Ghjklgfdsa@123@aws-1-ap-northeast-2.pooler.supabase.com:5432/postgres" -c "SELECT deployment_id, subdomain, status, assigned_worker_id FROM deployment_jobs ORDER BY queued_at DESC LIMIT 1;"
```

**Expected:** `assigned_worker_id = control-plane-local` (small packages prefer control-plane)

### Deploy Large Package (Should use remote worker)

```bash
curl -X POST http://localhost:8080/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/vercel/next.js",
    "package": "large",
    "subdomain": "test-large"
  }'
```

**Wait 5 seconds, then check assignment:**

```bash
psql "postgresql://postgres.rpqlrujltxsaqixzefgb:Ghjklgfdsa@123@aws-1-ap-northeast-2.pooler.supabase.com:5432/postgres" -c "SELECT deployment_id, subdomain, status, assigned_worker_id FROM deployment_jobs ORDER BY queued_at DESC LIMIT 1;"
```

**Expected:** `assigned_worker_id = worker-laptop-1` (large packages prefer remote workers)

**On worker machine, check logs:**

```bash
tail -f worker.log
```

**You should see:**
```
Claimed job: job-abc123
Cloning repository: https://github.com/vercel/next.js
Building image: ghcr.io/keshav-63/deployment-id:latest
Pushing image: ghcr.io/keshav-63/deployment-id:latest
Creating Kubernetes deployment
Job completed successfully: job-abc123
```

---

## 🔍 Troubleshooting

### Worker Not Registering

**Symptom:** Worker fails to register, shows connection error

**Check:**
```bash
# From worker machine
ping 100.107.233.70
curl http://100.107.233.70:8080/health
tailscale status
```

**Solutions:**
- Verify Tailscale is running: `sudo systemctl status tailscaled`
- Check both machines on same network: `tailscale status` (look for same network name)
- Verify control-plane is accessible: `curl http://CONTROL_PLANE_IP:8080/health`
- Check shared secret matches between worker `agent.yaml` and control-plane `.env`

### Worker Shows Offline

**Symptom:** Worker shows `status=offline` in database

**Check heartbeat:**
```bash
psql "CONNECTION_STRING" -c "SELECT worker_id, status, last_heartbeat, NOW() - last_heartbeat AS time_since FROM workers WHERE worker_id='worker-laptop-1';"
```

**Solutions:**
- Restart worker agent
- Check worker logs for errors
- Verify network connectivity hasn't dropped
- Increase `WORKER_HEARTBEAT_TIMEOUT` in control-plane `.env` if network is slow

### Jobs Not Assigned to Worker

**Symptom:** Worker stays idle, jobs go to control-plane

**Check worker status:**
```bash
psql "CONNECTION_STRING" -c "SELECT worker_id, status, current_jobs, max_concurrent_jobs FROM workers;"
```

**Solutions:**
- Verify worker is `idle` not `busy` or `offline`
- Deploy large packages (small ones prefer control-plane with `smart` strategy)
- Change `JOB_PLACEMENT_STRATEGY=local-first` to test
- Check distributor logs on control-plane

### Build Fails on Worker

**Symptom:** Worker claims job but build fails

**Check:**
```bash
# On worker machine
docker ps
kubectl get nodes
docker login ghcr.io
```

**Solutions:**
- Verify Docker is running: `sudo systemctl start docker`
- Check kubectl works: `kubectl get nodes`
- Login to GHCR: `echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin`
- Ensure worker has internet access to clone from GitHub

---

## 📊 Monitor Worker Activity

### Real-Time Worker Monitoring

```bash
# Watch worker status
watch -n 2 'curl -s http://localhost:8080/workers | jq ".workers[] | {id: .worker_id, status, jobs: .current_jobs, heartbeat: .last_heartbeat}"'
```

### Job Queue Status

```bash
# Check queued jobs
psql "CONNECTION_STRING" -c "SELECT status, COUNT(*) FROM deployment_jobs GROUP BY status;"
```

### Worker Job History

```bash
# Jobs completed by specific worker
psql "CONNECTION_STRING" -c "SELECT deployment_id, status, assigned_worker_id, started_at, finished_at FROM deployment_jobs WHERE assigned_worker_id='worker-laptop-1' ORDER BY finished_at DESC LIMIT 10;"
```

---

## 🎯 Success Criteria

**Worker registration is successful when:**

1. ✅ Worker shows in `SELECT * FROM workers;`
2. ✅ Worker status is `idle`
3. ✅ `last_heartbeat` updates every ~30 seconds
4. ✅ Worker appears in `GET /workers` API response
5. ✅ Worker claims and executes jobs
6. ✅ Jobs assigned to worker show in `deployment_jobs` table
7. ✅ Deployments accessible via `https://subdomain.keshavstack.tech`

---

## 📚 Related Documentation

- [QUICK-MULTI-WORKER.md](QUICK-MULTI-WORKER.md) - Quick reference with your IPs
- [MULTI-WORKER-SETUP.md](docs/MULTI-WORKER-SETUP.md) - Complete architecture guide
- [MULTI-WORKER-TESTING.md](MULTI-WORKER-TESTING.md) - Test scenarios
- [worker-agent/README.md](worker-agent/README.md) - Worker agent reference

---

**Need help?** Check control-plane logs and worker logs for detailed error messages.
