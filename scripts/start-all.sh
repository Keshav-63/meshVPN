#!/bin/bash

# MeshVPN Start All Services Script
# Starts K3D cluster, Cloudflare tunnel, Observability stack, and Control-plane

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║                                                              ║${NC}"
echo -e "${BLUE}║            MeshVPN Start All Services Script                 ║${NC}"
echo -e "${BLUE}║                                                              ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════════════════════════╝${NC}\n"

# Step 1: Check if K3D cluster exists
echo -e "${YELLOW}[1/5]${NC} Checking K3D cluster..."
if k3d cluster list | grep -q "meshvpn"; then
    echo "  Cluster exists, starting..."
    k3d cluster start meshvpn
    echo -e "  ${GREEN}✓${NC} Cluster started"
else
    echo -e "  ${RED}✗${NC} Cluster not found!"
    echo ""
    echo "  Please create cluster first:"
    echo "  k3d cluster create meshvpn \\"
    echo "    --port \"80:80@loadbalancer\" \\"
    echo "    --port \"443:443@loadbalancer\" \\"
    echo "    --agents 0 \\"
    echo "    --servers 1 \\"
    echo "    --servers-memory 512M"
    echo ""
    exit 1
fi

# Wait for cluster to be ready
echo "  Waiting for cluster to be ready..."
sleep 10

# Verify cluster
kubectl get nodes >/dev/null 2>&1 || {
    echo -e "  ${YELLOW}⚠${NC}  Updating kubeconfig..."
    export KUBECONFIG=$(k3d kubeconfig write meshvpn)
}

kubectl get nodes
echo -e "  ${GREEN}✓${NC} Cluster is ready\n"

# Step 2: Start Cloudflare Tunnel
echo -e "${YELLOW}[2/5]${NC} Starting Cloudflare Tunnel..."
cd ~/MeshVPN-slef-hosting/infra 2>/dev/null || cd /mnt/c/Users/*/Desktop/MeshVPN-slef-hosting/infra

if [ ! -f ".env" ] && [ ! -f "../.env" ]; then
    echo -e "  ${RED}✗${NC} .env file not found!"
    echo "  Please create .env file with CLOUDFLARE_TUNNEL_TOKEN"
    exit 1
fi

docker compose up -d
sleep 3
docker logs infra-cloudflared-1 --tail 5
echo -e "  ${GREEN}✓${NC} Cloudflare Tunnel started\n"

# Step 3: Start Observability Stack
echo -e "${YELLOW}[3/5]${NC} Starting Observability Stack..."
cd ~/MeshVPN-slef-hosting/infra/observability 2>/dev/null || cd /mnt/c/Users/*/Desktop/MeshVPN-slef-hosting/infra/observability

docker compose up -d
sleep 3
echo -e "  ${GREEN}✓${NC} Prometheus started on http://localhost:9090"
echo -e "  ${GREEN}✓${NC} Grafana started on http://localhost:3001\n"

# Step 4: Show all running services
echo -e "${YELLOW}[4/5]${NC} Checking all services..."
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" | grep -E "k3d-meshvpn|cloudflared|observability" || true
echo ""

# Step 5: Show next steps
echo -e "${YELLOW}[5/5]${NC} Next steps...\n"

echo -e "${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║                                                              ║${NC}"
echo -e "${GREEN}║                 All Services Started! ✓                      ║${NC}"
echo -e "${GREEN}║                                                              ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}\n"

echo "Services running:"
echo "  • K3D Cluster: meshvpn"
echo "  • Cloudflare Tunnel: infra-cloudflared-1"
echo "  • Prometheus: http://localhost:9090"
echo "  • Grafana: http://localhost:3001"
echo ""
echo "To start Control-Plane:"
echo "  cd ~/MeshVPN-slef-hosting"
echo "  ./start-control-plane.sh"
echo ""
echo "Or run manually:"
echo "  cd ~/MeshVPN-slef-hosting/control-plane"
echo "  export \$(cat ../.env | grep -v '^#' | xargs)"
echo "  export K8S_CONFIG_PATH=\"\$HOME/k3d-kubeconfig.yaml\""
echo "  go run ./cmd/control-plane"
echo ""
echo "Check health:"
echo "  kubectl get nodes"
echo "  curl http://localhost:8080/health"
echo ""
