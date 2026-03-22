#!/bin/bash

# MeshVPN Control-Plane Startup Script

set -e

echo "=== MeshVPN Control-Plane Startup ==="
echo ""

# Load environment variables
if [ ! -f .env ]; then
    echo "ERROR: .env file not found!"
    echo "Please create .env from .env.example"
    exit 1
fi

echo "Loading environment variables..."
export $(cat .env | grep -v '^#' | xargs)

# Verify kubeconfig exists
if [ ! -f "$K8S_CONFIG_PATH" ]; then
    echo "ERROR: Kubeconfig not found at: $K8S_CONFIG_PATH"
    echo "Please run: k3d kubeconfig get meshvpn > ~/k3d-kubeconfig.yaml"
    exit 1
fi

echo "Kubeconfig: $K8S_CONFIG_PATH"
echo "Namespace: $K8S_NAMESPACE"
echo "Registry: $K8S_IMAGE_PREFIX"
echo ""

# Start control-plane
echo "Starting control-plane..."
cd control-plane
go run ./cmd/control-plane
