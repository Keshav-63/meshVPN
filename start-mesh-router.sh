#!/bin/bash

# MeshVPN Edge Router Startup Script

set -e

echo "=== MeshVPN Edge Router Startup ==="
echo ""

# Load environment variables
if [ ! -f .env ]; then
    echo "ERROR: .env file not found!"
    echo "Please create .env from .env.example"
    exit 1
fi

echo "Loading environment variables..."
export $(cat .env | grep -v '^#' | xargs)

# Start mesh-router
echo "Starting mesh-router on port 8082..."
cd tools/mesh-router
go run main.go
