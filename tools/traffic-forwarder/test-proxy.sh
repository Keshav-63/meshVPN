#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "🧪 Testing bridge proxy..."
echo ""

# Kill any existing proxy
pkill -f "bridge-proxy.exe" 2>/dev/null || true
sleep 1

# Start proxy in background
echo "1️⃣ Starting proxy..."
./bridge-proxy.exe > test-proxy.log 2>&1 &
PROXY_PID=$!
echo "   PID: $PROXY_PID"
sleep 2

# Test 1: Local health check
echo ""
echo "2️⃣ Testing local health endpoint..."
if curl -s -f http://localhost:8081/health; then
    echo ""
    echo "   ✅ Health endpoint works!"
else
    echo ""
    echo "   ❌ Health endpoint FAILED"
    echo "   Logs:"
    tail -10 test-proxy.log
    kill $PROXY_PID
    exit 1
fi

# Test 2: Check if proxy is listening
echo ""
echo "3️⃣ Checking if proxy is listening on all interfaces..."
if ss -tlnp 2>/dev/null | grep -q "8081" || netstat -tln 2>/dev/null | grep -q "8081"; then
    echo "   ✅ Proxy listening on port 8081"
else
    echo "   ❌ Proxy not listening"
fi

# Test 3: Check if k3d cluster exists and test from within
echo ""
echo "4️⃣ Testing from k3d container..."
if command -v docker &> /dev/null && docker ps 2>/dev/null | grep -q "k3d-meshvpn-server"; then
    echo "   Found k3d container, testing..."
    RESULT=$(docker exec k3d-meshvpn-server-0 sh -c "wget --timeout=2 -qO- http://host.docker.internal:8081/health 2>&1" || echo "FAILED")
    if echo "$RESULT" | grep -q "OK"; then
        echo "   ✅ Successfully reached proxy from k3d container"
    else
        echo "   ❌ Failed to reach proxy from k3d: $RESULT"
        echo "   Trying with curl instead..."
        RESULT=$(docker exec k3d-meshvpn-server-0 sh -c "curl -v http://host.docker.internal:8081/health 2>&1 | head -20" || echo "FAILED")
        echo "   $RESULT"
    fi
else
    echo "   ⚠️  k3d cluster not running, skipping container test"
fi

# Cleanup
echo ""
echo "🧹 Cleaning up..."
kill $PROXY_PID 2>/dev/null || true
echo "✅ Proxy test complete!"
