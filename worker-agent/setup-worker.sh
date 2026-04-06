#!/bin/bash
# MeshVPN Worker Setup Script
# This script sets up a remote worker on a new machine

set -e

echo "=========================================="
echo "MeshVPN Worker Agent Setup"
echo "=========================================="
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if running on control-plane
if [ -f "../control-plane/cmd/control-plane/main.go" ]; then
    echo -e "${RED}ERROR: This script should NOT be run on the control-plane machine!${NC}"
    echo "Please run this on a separate worker machine."
    exit 1
fi

echo "This script will:"
echo "  1. Check prerequisites (Tailscale, Docker, kubectl, K3D)"
echo "  2. Setup K3D Kubernetes cluster"
echo "  3. Configure worker agent"
echo "  4. Build and start worker"
echo ""
read -p "Continue? (y/n) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    exit 1
fi

# Step 1: Check Tailscale
echo ""
echo -e "${YELLOW}[1/6] Checking Tailscale...${NC}"
if ! command -v tailscale &> /dev/null; then
    echo -e "${RED}Tailscale not found!${NC}"
    echo "Installing Tailscale..."
    curl -fsSL https://tailscale.com/install.sh | sh
    echo "Please run: sudo tailscale up"
    echo "Then run this script again."
    exit 1
fi

if ! tailscale status &> /dev/null; then
    echo -e "${RED}Tailscale not connected!${NC}"
    echo "Please run: sudo tailscale up"
    exit 1
fi

WORKER_IP=$(tailscale ip -4)
echo -e "${GREEN}✓ Tailscale connected: $WORKER_IP${NC}"

# Step 2: Check Docker
echo ""
echo -e "${YELLOW}[2/6] Checking Docker...${NC}"
if ! command -v docker &> /dev/null; then
    echo -e "${RED}Docker not found!${NC}"
    echo "Installing Docker..."
    curl -fsSL https://get.docker.com | sh
    sudo usermod -aG docker $USER
    echo -e "${YELLOW}Please log out and log back in for Docker group permissions${NC}"
    exit 1
fi

if ! docker ps &> /dev/null; then
    echo -e "${RED}Docker daemon not running or permission denied${NC}"
    echo "Please run: sudo systemctl start docker"
    echo "Or log out/in if you just installed Docker"
    exit 1
fi

echo -e "${GREEN}✓ Docker running${NC}"

# Step 3: Check K3D
echo ""
echo -e "${YELLOW}[3/6] Checking K3D...${NC}"
if ! command -v k3d &> /dev/null; then
    echo "Installing K3D..."
    curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash
fi

echo -e "${GREEN}✓ K3D installed${NC}"

# Step 4: Setup K3D cluster
echo ""
echo -e "${YELLOW}[4/6] Setting up K3D cluster...${NC}"
if k3d cluster list | grep -q "worker-cluster"; then
    echo "K3D cluster 'worker-cluster' already exists"
    read -p "Delete and recreate? (y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        k3d cluster delete worker-cluster
    else
        echo "Using existing cluster"
    fi
fi

if ! k3d cluster list | grep -q "worker-cluster"; then
    echo "Creating K3D cluster..."
    k3d cluster create worker-cluster \
        --port "80:80@loadbalancer" \
        --agents 0 \
        --servers 1

    echo -e "${GREEN}✓ K3D cluster created${NC}"
fi

# Verify kubectl works
if ! kubectl cluster-info &> /dev/null; then
    echo -e "${RED}kubectl cannot connect to cluster${NC}"
    exit 1
fi

echo -e "${GREEN}✓ kubectl working${NC}"

# Step 5: Configure worker agent
echo ""
echo -e "${YELLOW}[5/6] Configuring worker agent...${NC}"

if [ ! -f "agent.yaml" ]; then
    if [ -f "agent.yaml.example" ]; then
        cp agent.yaml.example agent.yaml
        echo "Created agent.yaml from example"
    else
        echo -e "${RED}agent.yaml.example not found!${NC}"
        exit 1
    fi
fi

# Get worker ID
echo ""
echo "Enter a unique worker ID (e.g., worker-laptop-1, worker-server-1):"
read -p "Worker ID: " WORKER_ID

if [ -z "$WORKER_ID" ]; then
    echo -e "${RED}Worker ID cannot be empty!${NC}"
    exit 1
fi

# Get worker name
echo "Enter a descriptive name for this worker:"
read -p "Worker Name: " WORKER_NAME

if [ -z "$WORKER_NAME" ]; then
    WORKER_NAME="Worker $WORKER_ID"
fi

# Get control-plane IP
echo ""
echo "Enter the control-plane Tailscale IP address:"
read -p "Control-plane IP: " CONTROL_PLANE_IP

if [ -z "$CONTROL_PLANE_IP" ]; then
    echo -e "${RED}Control-plane IP cannot be empty!${NC}"
    exit 1
fi

# Test connection to control-plane
echo ""
echo "Testing connection to control-plane..."
if ! curl -s --max-time 5 "http://$CONTROL_PLANE_IP:8080/health" > /dev/null; then
    echo -e "${RED}WARNING: Cannot connect to control-plane at $CONTROL_PLANE_IP:8080${NC}"
    echo "Please verify:"
    echo "  1. Control-plane is running"
    echo "  2. Both machines are on same Tailscale network"
    echo "  3. IP address is correct"
    read -p "Continue anyway? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
else
    echo -e "${GREEN}✓ Control-plane reachable${NC}"
fi

# Get shared secret
echo ""
echo "Enter the worker shared secret (from control-plane .env):"
echo "(Default: meshvpn-worker-secret-change-in-production)"
read -p "Shared secret: " SHARED_SECRET

if [ -z "$SHARED_SECRET" ]; then
    SHARED_SECRET="meshvpn-worker-secret-change-in-production"
fi

# Update agent.yaml
cat > agent.yaml <<EOF
# Worker Agent Configuration
# Auto-generated by setup-worker.sh

worker:
  id: $WORKER_ID
  name: "$WORKER_NAME"
  tailscale_ip: "$WORKER_IP"
  max_concurrent_jobs: 2

control_plane:
  url: http://$CONTROL_PLANE_IP:8080
  shared_secret: "$SHARED_SECRET"

runtime:
  type: kubernetes
  kubeconfig: $HOME/.kube/config
  namespace: worker-apps
  kubectl_bin: kubectl

capabilities:
  memory_gb: 16
  cpu_cores: 8
  supported_packages:
    - small
    - medium
    - large
EOF

echo -e "${GREEN}✓ agent.yaml configured${NC}"

# Step 6: Build worker agent
echo ""
echo -e "${YELLOW}[6/6] Building worker agent...${NC}"

if [ ! -f "go.mod" ]; then
    echo "Initializing Go module..."
    go mod init worker-agent
fi

echo "Syncing Go module dependencies..."
go mod tidy

echo "Building worker agent binary..."
go build -o worker-agent cmd/worker-agent/main.go

if [ ! -f "worker-agent" ]; then
    echo -e "${RED}Build failed!${NC}"
    exit 1
fi

chmod +x worker-agent

echo -e "${GREEN}✓ Worker agent built successfully${NC}"

# Summary
echo ""
echo "=========================================="
echo -e "${GREEN}Setup Complete!${NC}"
echo "=========================================="
echo ""
echo "Worker Configuration:"
echo "  Worker ID:        $WORKER_ID"
echo "  Worker Name:      $WORKER_NAME"
echo "  Tailscale IP:     $WORKER_IP"
echo "  Control-plane:    http://$CONTROL_PLANE_IP:8080"
echo ""
echo "To start the worker:"
echo "  ./worker-agent -config agent.yaml"
echo ""
echo "To run as background service:"
echo "  nohup ./worker-agent -config agent.yaml > worker.log 2>&1 &"
echo ""
echo "To view logs:"
echo "  tail -f worker.log"
echo ""
echo "To stop worker:"
echo "  pkill -f worker-agent"
echo ""
echo "=========================================="
echo ""
read -p "Start worker now? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Starting worker agent..."
    ./worker-agent -config agent.yaml
fi
