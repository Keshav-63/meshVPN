#!/bin/bash

# Fix Kubeconfig Script
# Regenerates proper kubeconfig for K3D cluster

set -e

echo "=== K3D Kubeconfig Fix ==="
echo ""

# Step 1: Check if cluster exists
echo "[1/5] Checking if K3D cluster exists..."
if ! k3d cluster list | grep -q "meshvpn"; then
    echo "ERROR: K3D cluster 'meshvpn' not found!"
    echo ""
    echo "Create the cluster first:"
    echo "  k3d cluster create meshvpn \\"
    echo "    --port \"80:80@loadbalancer\" \\"
    echo "    --port \"443:443@loadbalancer\" \\"
    echo "    --agents 0 \\"
    echo "    --servers 1 \\"
    echo "    --k3s-arg \"--disable=traefik@server:0\""
    exit 1
fi
echo "✓ Cluster exists"

# Step 2: Check cluster status
echo ""
echo "[2/5] Checking cluster status..."
cluster_status=$(k3d cluster list | grep meshvpn | awk '{print $2}')
if [ "$cluster_status" != "1/1" ]; then
    echo "WARNING: Cluster not fully ready. Starting..."
    k3d cluster start meshvpn
    sleep 10
fi
echo "✓ Cluster is running"

# Step 3: Remove old kubeconfig
echo ""
echo "[3/5] Removing old kubeconfig..."
rm -f ~/.kube/config
rm -f ~/k3d-kubeconfig.yaml
echo "✓ Old configs removed"

# Step 4: Generate new kubeconfig
echo ""
echo "[4/5] Generating new kubeconfig..."

# Method 1: Write to ~/.kube/config (default location)
mkdir -p ~/.kube
k3d kubeconfig write meshvpn > ~/.kube/config
chmod 600 ~/.kube/config

# Method 2: Also create ~/k3d-kubeconfig.yaml for control-plane
k3d kubeconfig get meshvpn > ~/k3d-kubeconfig.yaml
chmod 600 ~/k3d-kubeconfig.yaml

echo "✓ Kubeconfig generated"

# Step 5: Verify
echo ""
echo "[5/5] Verifying kubeconfig..."

echo ""
echo "File size:"
ls -lh ~/.kube/config
ls -lh ~/k3d-kubeconfig.yaml

echo ""
echo "Server URL:"
grep "server:" ~/.kube/config
grep "server:" ~/k3d-kubeconfig.yaml

echo ""
echo "Testing kubectl:"
kubectl get nodes

echo ""
echo "=== Fix Complete! ==="
echo ""
echo "Kubeconfig locations:"
echo "  ~/.kube/config (default)"
echo "  ~/k3d-kubeconfig.yaml (for control-plane)"
echo ""
echo "Server URL:"
grep "server:" ~/.kube/config | awk '{print $2}'
echo ""
