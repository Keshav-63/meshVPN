#!/bin/bash
# Comprehensive diagnostic for bridge-proxy issues

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "🔍 Bridge Proxy Diagnostic Report"
echo "=================================="
echo ""

# Step 1: Verify Go code compiles
echo "Step 1️⃣ : Checking Go code for compilation errors..."
if go build -o /tmp/test-build bridge-proxy.go 2>&1 | head -20; then
    echo "✅ Code compiles successfully"
else
    echo "❌ Compilation failed, showing errors above"
    exit 1
fi
rm -f /tmp/test-build

# Step 2: Check current binary
echo ""
echo "Step 2️⃣ : Checking current binary..."
if [ -f "./bridge-proxy.exe" ]; then
    echo "✅ bridge-proxy.exe exists"
    ls -lh bridge-proxy.exe
else
    echo "⚠️  bridge-proxy.exe not found, building..."
    go build -o bridge-proxy.exe bridge-proxy.go
fi

# Step 3: Kill any existing proxies
echo ""
echo "Step 3️⃣ : Cleaning up old processes..."
pkill -f "bridge-proxy.exe" 2>/dev/null || true
sleep 1

# Step 4: Start proxy and check it
echo ""
echo "Step 4️⃣ : Starting proxy..."
rm -f test-diagnostic.log
./bridge-proxy.exe > test-diagnostic.log 2>&1 &
PROXY_PID=$!
echo "PID: $PROXY_PID"
sleep 2

# Step 5: Check if process is still running
echo ""
echo "Step 5️⃣ : Checking if proxy is running..."
if ps -p $PROXY_PID > /dev/null 2>&1; then
    echo "✅ Process running"
else
    echo "❌ Process crashed!"
    echo "Last logs:"
    cat test-diagnostic.log
    exit 1
fi

# Step 6: Check logs for startup errors
echo ""
echo "Step 6️⃣ : Checking startup logs..."
sleep 1
tail -5 test-diagnostic.log
if grep -i "error\|fatal" test-diagnostic.log; then
    echo "⚠️  Error found in logs!"
fi

# Step 7: Test health endpoint locally
echo ""
echo "Step 7️⃣ : Testing /health endpoint..."
for i in {1..3}; do
    echo "  Attempt $i..."
    if curl -s -m 2 http://localhost:8081/health; then
        echo ""
        echo "  ✅ Health endpoint responded!"
        break
    else
        if [ $i -lt 3 ]; then
            sleep 1
        fi
    fi
done

# Step 8: Check listening ports
echo ""
echo "Step 8️⃣ : Checking listening ports..."
echo "Proxy should be on 0.0.0.0:8081"
ss -tlnp 2>/dev/null | grep 8081 || echo "No process found on port 8081"

# Step 9: Test from WSL perspective
echo ""
echo "Step 9️⃣ : Testing connectivity..."
echo "  Testing http://localhost:8081/health"
curl -v http://localhost:8081/health 2>&1 | head -20

# Step 10: Check k3d connectivity (if available)
echo ""
echo "Step 🔟: Checking k3d connectivity..."
if docker ps 2>/dev/null | grep -q "k3d-meshvpn-server"; then
    echo "  k3d cluster found, testing from container..."
    docker exec k3d-meshvpn-server-0 sh -c "curl -v http://host.docker.internal:8081/health 2>&1 | head -20"
else
    echo "  k3d cluster not running, skipping"
fi

# Cleanup
echo ""
echo "🧹 Cleaning up..."
kill $PROXY_PID 2>/dev/null || true
echo "✅ Diagnostic complete!"
