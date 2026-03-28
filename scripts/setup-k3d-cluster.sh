#!/bin/bash
# Complete K3D Cluster Setup for MeshVPN Control-Plane
# Run this from WSL: ./scripts/setup-k3d-cluster.sh

set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║     MeshVPN K3D Cluster Setup for Control-Plane             ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════════════════════════╝${NC}\n"

# Configuration
CLUSTER_NAME="meshvpn"
NAMESPACE="meshvpn-apps"
KUBECONFIG_PATH="$HOME/k3d-kubeconfig.yaml"

# Step 1: Delete existing cluster if it exists
echo -e "${YELLOW}[1/7] Checking for existing cluster...${NC}"
if k3d cluster list | grep -q "$CLUSTER_NAME"; then
    echo -e "${YELLOW}Found existing cluster '$CLUSTER_NAME'. Deleting...${NC}"
    k3d cluster delete $CLUSTER_NAME
    echo -e "${GREEN}✓ Old cluster deleted${NC}"
else
    echo -e "${GREEN}✓ No existing cluster found${NC}"
fi

# Step 2: Create fresh K3D cluster
echo -e "\n${YELLOW}[2/7] Creating fresh K3D cluster...${NC}"
k3d cluster create $CLUSTER_NAME \
  --api-port 6550 \
  --port "80:80@loadbalancer" \
  --port "443:443@loadbalancer" \
  --agents 0 \
  --servers 1 \
  --wait

echo -e "${GREEN}✓ K3D cluster created successfully${NC}"

# Step 3: Export kubeconfig
echo -e "\n${YELLOW}[3/7] Exporting kubeconfig...${NC}"
k3d kubeconfig get $CLUSTER_NAME > $KUBECONFIG_PATH
export KUBECONFIG=$KUBECONFIG_PATH
echo -e "${GREEN}✓ Kubeconfig exported to: $KUBECONFIG_PATH${NC}"

# Step 4: Wait for cluster to be ready
echo -e "\n${YELLOW}[4/7] Waiting for cluster to be ready...${NC}"
sleep 10
kubectl wait --for=condition=Ready nodes --all --timeout=120s
echo -e "${GREEN}✓ Cluster is ready${NC}"

# Step 5: Create namespace
echo -e "\n${YELLOW}[5/7] Creating namespace '$NAMESPACE'...${NC}"
kubectl create namespace $NAMESPACE || echo "Namespace already exists"
echo -e "${GREEN}✓ Namespace created${NC}"

# Step 6: Install metrics-server
echo -e "\n${YELLOW}[6/7] Installing metrics-server...${NC}"
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml

# Patch metrics-server for K3D (disable TLS verification)
echo "Patching metrics-server for K3D..."
kubectl patch deployment metrics-server -n kube-system --type='json' \
  -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--kubelet-insecure-tls"}]'

# Wait for metrics-server to be ready
echo "Waiting for metrics-server to be ready..."
kubectl rollout status deployment/metrics-server -n kube-system --timeout=120s
echo -e "${GREEN}✓ Metrics-server installed and ready${NC}"

# Step 7: Verify installation
echo -e "\n${YELLOW}[7/7] Verifying cluster setup...${NC}"

echo -e "\nCluster Info:"
kubectl cluster-info

echo -e "\nNodes:"
kubectl get nodes

echo -e "\nNamespaces:"
kubectl get namespaces

echo -e "\nMetrics-server pods:"
kubectl get pods -n kube-system -l k8s-app=metrics-server

# Test metrics-server (may take a few seconds)
echo -e "\nTesting metrics-server (waiting 15s for metrics to be available)..."
sleep 15
if kubectl top nodes &> /dev/null; then
    echo -e "${GREEN}✓ Metrics-server is working!${NC}"
    kubectl top nodes
else
    echo -e "${YELLOW}⚠ Metrics-server not ready yet (this is normal, it may take 1-2 minutes)${NC}"
fi

# Summary
echo -e "\n${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║                    Setup Complete! ✓                         ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}\n"

echo -e "${BLUE}Next Steps:${NC}"
echo ""
echo "1. Update your .env file with:"
echo "   ${YELLOW}K8S_CONFIG_PATH=$KUBECONFIG_PATH${NC}"
echo ""
echo "2. Start the control-plane:"
echo "   ${YELLOW}./start-control-plane.sh${NC}"
echo ""
echo "3. Verify metrics collection:"
echo "   ${YELLOW}kubectl top pods -n $NAMESPACE${NC}"
echo "   ${YELLOW}curl http://localhost:8080/metrics | grep platform${NC}"
echo ""
echo -e "${BLUE}Cluster Details:${NC}"
echo "  Cluster Name: $CLUSTER_NAME"
echo "  Namespace: $NAMESPACE"
echo "  Kubeconfig: $KUBECONFIG_PATH"
echo "  API Port: 6550"
echo "  HTTP Port: 80"
echo "  HTTPS Port: 443"
echo ""
echo -e "${GREEN}Happy deploying! 🚀${NC}"
