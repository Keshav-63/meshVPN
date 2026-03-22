#!/bin/bash

# MeshVPN Complete Cleanup Script
# This script removes ALL MeshVPN resources and resets to a clean state

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║                                                              ║${NC}"
echo -e "${BLUE}║         MeshVPN Complete Cleanup & Reset Script              ║${NC}"
echo -e "${BLUE}║                                                              ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════════════════════════╝${NC}\n"

echo -e "${YELLOW}⚠️  WARNING: This will delete ALL MeshVPN resources!${NC}\n"
echo "This includes:"
echo "  - K3D cluster (meshvpn)"
echo "  - Docker containers (cloudflared, prometheus, grafana)"
echo "  - Kubernetes configurations"
echo "  - Application checkouts (optional)"
echo ""
read -p "Are you sure you want to continue? (yes/no): " confirm

if [ "$confirm" != "yes" ]; then
    echo -e "${RED}Cleanup cancelled.${NC}"
    exit 0
fi

echo -e "\n${BLUE}Starting cleanup...${NC}\n"

# Step 1: Stop control-plane
echo -e "${YELLOW}[1/10]${NC} Stopping control-plane..."
pkill -f "control-plane" 2>/dev/null || echo "  No control-plane process found"
sleep 2

# Step 2: Stop Cloudflare tunnel
echo -e "${YELLOW}[2/10]${NC} Stopping Cloudflare tunnel..."
cd ~/MeshVPN-slef-hosting/infra 2>/dev/null || cd /mnt/c/Users/*/Desktop/MeshVPN-slef-hosting/infra
docker compose down 2>/dev/null || echo "  Already stopped"

# Step 3: Stop observability stack
echo -e "${YELLOW}[3/10]${NC} Stopping observability stack..."
cd ~/MeshVPN-slef-hosting/infra/observability 2>/dev/null || cd /mnt/c/Users/*/Desktop/MeshVPN-slef-hosting/infra/observability
docker compose down -v 2>/dev/null || echo "  Already stopped"

# Step 4: Delete K3D cluster
echo -e "${YELLOW}[4/10]${NC} Deleting K3D cluster..."
k3d cluster delete meshvpn 2>/dev/null || echo "  No cluster found"

# Step 5: Clean Docker resources
echo -e "${YELLOW}[5/10]${NC} Cleaning Docker resources..."
docker container prune -f >/dev/null 2>&1 || true
echo "  Containers cleaned"
docker image prune -f >/dev/null 2>&1 || true
echo "  Images cleaned"
docker volume prune -f >/dev/null 2>&1 || true
echo "  Volumes cleaned"
docker network prune -f >/dev/null 2>&1 || true
echo "  Networks cleaned"

# Step 6: Clean Kubernetes configs
echo -e "${YELLOW}[6/10]${NC} Cleaning Kubernetes configurations..."
rm -f ~/.kube/config
rm -f ~/k3d-kubeconfig.yaml
echo "  Kubeconfig files removed"

# Step 7: Clean application data (optional)
echo -e "${YELLOW}[7/10]${NC} Cleaning application data..."
read -p "  Remove application checkouts in apps/? (yes/no): " remove_apps
if [ "$remove_apps" = "yes" ]; then
    cd ~/MeshVPN-slef-hosting 2>/dev/null || cd /mnt/c/Users/*/Desktop/MeshVPN-slef-hosting
    rm -rf apps/*/
    echo "  Application checkouts removed"
else
    echo "  Skipping application checkouts"
fi

# Step 8: Clean .env files
echo -e "${YELLOW}[8/10]${NC} Cleaning environment files..."
read -p "  Remove .env files? (yes/no): " remove_env
if [ "$remove_env" = "yes" ]; then
    cd ~/MeshVPN-slef-hosting 2>/dev/null || cd /mnt/c/Users/*/Desktop/MeshVPN-slef-hosting
    rm -f .env
    rm -f infra/.env
    echo "  .env files removed (keeping .env.example)"
else
    echo "  Skipping .env files"
fi

# Step 9: Verify clean state
echo -e "${YELLOW}[9/10]${NC} Verifying clean state..."

# Check K3D
k3d_clusters=$(k3d cluster list 2>/dev/null | grep -c "meshvpn" || echo "0")
if [ "$k3d_clusters" = "0" ]; then
    echo -e "  ${GREEN}✓${NC} No K3D clusters"
else
    echo -e "  ${RED}✗${NC} K3D cluster still exists"
fi

# Check Docker
running_containers=$(docker ps -q | wc -l)
if [ "$running_containers" = "0" ]; then
    echo -e "  ${GREEN}✓${NC} No Docker containers running"
else
    echo -e "  ${YELLOW}⚠${NC}  $running_containers Docker containers still running"
fi

# Check kubectl
kubectl_test=$(kubectl get nodes 2>&1 || true)
if echo "$kubectl_test" | grep -q "refused\|not found\|Unable to connect"; then
    echo -e "  ${GREEN}✓${NC} Kubectl has no valid config"
else
    echo -e "  ${YELLOW}⚠${NC}  Kubectl still has valid config"
fi

# Step 10: Show resource usage
echo -e "${YELLOW}[10/10]${NC} Checking resource usage..."
docker system df --format "table {{.Type}}\t{{.TotalCount}}\t{{.Size}}"

echo -e "\n${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║                                                              ║${NC}"
echo -e "${GREEN}║                   Cleanup Complete! ✓                        ║${NC}"
echo -e "${GREEN}║                                                              ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}\n"

echo "Next steps:"
echo "  1. Follow docs/FRESH-INSTALL.md for fresh installation"
echo "  2. Run: k3d cluster create meshvpn ..."
echo "  3. Setup environment variables in .env"
echo "  4. Start services"
echo ""
echo "Quick start:"
echo "  cat docs/FRESH-INSTALL.md | less"
echo ""
