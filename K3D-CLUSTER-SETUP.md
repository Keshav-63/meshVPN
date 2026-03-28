# K3D Cluster Setup - Quick Start

## ⚡ TL;DR - Run This Now

**From WSL (Debian/Ubuntu):**

```bash
# Navigate to project directory
cd ~/MeshVPN-slef-hosting  # Adjust path if needed

# Make script executable
chmod +x scripts/setup-k3d-cluster.sh

# Run the automated setup
./scripts/setup-k3d-cluster.sh
```

That's it! The script handles everything automatically.

---

## 📋 What the Script Does

1. **Deletes old cluster** - Removes existing `meshvpn` cluster if present
2. **Creates fresh cluster** - Sets up K3D with proper ports and configuration
3. **Installs metrics-server** - Enables CPU and memory metrics collection
4. **Creates namespace** - Sets up `meshvpn-apps` namespace
5. **Exports kubeconfig** - Saves config to `~/k3d-kubeconfig.yaml`
6. **Verifies setup** - Tests that everything is working

---

## 🔧 Manual Commands (If Script Fails)

If you prefer manual setup or the script fails:

```bash
# 1. Delete existing cluster
k3d cluster delete meshvpn

# 2. Create fresh cluster
k3d cluster create meshvpn \
  --api-port 6550 \
  --port "80:80@loadbalancer" \
  --port "443:443@loadbalancer" \
  --agents 0 \
  --servers 1 \
  --wait

# 3. Export kubeconfig
k3d kubeconfig get meshvpn > ~/k3d-kubeconfig.yaml
export KUBECONFIG=~/k3d-kubeconfig.yaml

# 4. Create namespace
kubectl create namespace meshvpn-apps

# 5. Install metrics-server
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml

# 6. Patch metrics-server for K3D
kubectl patch deployment metrics-server -n kube-system --type='json' \
  -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--kubelet-insecure-tls"}]'

# 7. Wait for metrics-server
kubectl rollout status deployment/metrics-server -n kube-system --timeout=120s

# 8. Verify
kubectl cluster-info
kubectl get nodes
kubectl top nodes
```

---

## ✅ Verification Commands

After setup, verify everything is working:

```bash
# Check cluster status
kubectl cluster-info

# Check nodes are ready
kubectl get nodes

# Check metrics-server is working
kubectl top nodes
kubectl top pods -n kube-system

# Check namespace exists
kubectl get namespace meshvpn-apps

# List all pods
kubectl get pods -A
```

---

## 🔄 Troubleshooting

### Metrics-server not working

Wait 1-2 minutes and try again. Metrics-server needs time to collect initial data:

```bash
# Wait and retry
sleep 60
kubectl top nodes
```

### Cluster not starting

Check Docker is running and has enough resources:

```bash
# Check Docker
docker ps

# Restart Docker Desktop if needed
```

### "command not found: k3d"

Install k3d first:

```bash
curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash
```

### Port already in use

Kill processes using ports 80, 443, or 6550:

```bash
# On WSL
sudo lsof -i :80
sudo lsof -i :443
sudo lsof -i :6550

# Kill if needed
sudo kill -9 <PID>
```

---

## 📝 After Cluster Setup

### 1. Update .env file

Make sure your `.env` has:

```env
K8S_CONFIG_PATH=/home/Keshav/k3d-kubeconfig.yaml  # Use your actual username
K8S_NAMESPACE=meshvpn-apps
RUNTIME_BACKEND=k3s
KUBECTL_BIN=kubectl
```

### 2. Start Control-Plane

```bash
./start-control-plane.sh
```

### 3. Check Logs

Control-plane should log:
- ✅ "registered control-plane as worker"
- ✅ "analytics collector started"
- ✅ "telemetry endpoints registered"

### 4. Test Metrics

```bash
# Check Prometheus metrics
curl http://localhost:8080/metrics | grep platform

# Check K8s metrics
kubectl top pods -n meshvpn-apps
```

---

## 🎯 Expected Output

After successful setup:

```
╔══════════════════════════════════════════════════════════════╗
║                    Setup Complete! ✓                         ║
╚══════════════════════════════════════════════════════════════╝

Next Steps:

1. Update your .env file with:
   K8S_CONFIG_PATH=/home/Keshav/k3d-kubeconfig.yaml

2. Start the control-plane:
   ./start-control-plane.sh

3. Verify metrics collection:
   kubectl top pods -n meshvpn-apps
   curl http://localhost:8080/metrics | grep platform

Cluster Details:
  Cluster Name: meshvpn
  Namespace: meshvpn-apps
  Kubeconfig: /home/Keshav/k3d-kubeconfig.yaml
  API Port: 6550
  HTTP Port: 80
  HTTPS Port: 443

Happy deploying! 🚀
```

---

## 📚 Related Documentation

- Full setup guide: [docs/SETUP.md](docs/SETUP.md)
- Observability fixes: [OBSERVABILITY-FIX-SUMMARY.md](OBSERVABILITY-FIX-SUMMARY.md)
- Multi-worker setup: [docs/MULTI-WORKER-SETUP.md](docs/MULTI-WORKER-SETUP.md)

---

## ⚙️ Cluster Configuration

The cluster is configured with:
- **Name**: `meshvpn`
- **API Port**: 6550
- **HTTP Port**: 80 (for Traefik ingress)
- **HTTPS Port**: 443 (for SSL)
- **Nodes**: 1 server, 0 agents
- **Namespace**: `meshvpn-apps`
- **Metrics**: metrics-server installed and configured
- **Kubeconfig**: `~/k3d-kubeconfig.yaml`
