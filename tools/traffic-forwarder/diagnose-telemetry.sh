#!/bin/bash
# Diagnostic: Check why telemetry isn't being sent

echo "🔍 Traffic Forwarder Diagnostic"
echo "================================"
echo ""

# 1. Check Traefik pod
echo "Step 1: Checking Traefik pod..."
TRAEFIK_POD=$(kubectl get pods -n kube-system -l app.kubernetes.io/name=traefik -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
if [ -z "$TRAEFIK_POD" ]; then
    echo "❌ Traefik pod not found!"
    exit 1
fi
echo "✅ Found: $TRAEFIK_POD"
echo ""

# 2. Check if Traefik access logs exist
echo "Step 2: Checking Traefik access logs..."
LOGS=$(kubectl logs -n kube-system "$TRAEFIK_POD" --tail=5 2>/dev/null || echo "NO_LOGS")
if [ "$LOGS" = "NO_LOGS" ]; then
    echo "❌ No logs found from Traefik"
    exit 1
fi
echo "Latest 5 log entries:"
echo "---"
echo "$LOGS" | head -5
echo "---"
echo ""

# 3. Check if any logs are JSON (access logs)
echo "Step 3: Checking for JSON access logs..."
JSON_COUNT=$(kubectl logs -n kube-system "$TRAEFIK_POD" --tail=200 2>/dev/null | grep -c '{' || echo "0")
if [ "$JSON_COUNT" -eq 0 ]; then
    echo "❌ No JSON access logs found (Traefik not receiving traffic)"
    echo "   This means traffic is not routing through Traefik!"
    echo ""
    echo "Troubleshooting:"
    echo "  1. Is there a Traefik Ingress pointing to your service?"
    echo "  2. Try: curl -H 'Host: app-id.keshavstack.tech' http://localhost:8080/"
    echo "  3. Check routes: kubectl get ingress -A"
    exit 1
fi
echo "✅ Found $JSON_COUNT JSON entries in logs"
echo ""

# 4. Sample a JSON access log
echo "Step 4: Sample access log entry..."
SAMPLE=$(kubectl logs -n kube-system "$TRAEFIK_POD" --tail=200 2>/dev/null | grep '{' | head -1)
echo "Sample:"
echo "$SAMPLE" | jq '.' 2>/dev/null || echo "Could not parse: $SAMPLE"
echo ""

# 5. Check if RequestHost contains the domain
echo "Step 5: Checking RequestHost in logs..."
HOSTS=$(kubectl logs -n kube-system "$TRAEFIK_POD" --tail=200 2>/dev/null | grep '{' | jq -r '.RequestHost' 2>/dev/null | sort | uniq)
echo "RequestHosts seen in logs:"
echo "$HOSTS"
echo ""

# 6. Check traffic forwarder logs for errors
echo "Step 6: Checking traffic forwarder for errors..."
FORWARDER_LOGS=$(kubectl logs -l app=traffic-forwarder --tail=50 2>/dev/null)
ERROR_COUNT=$(echo "$FORWARDER_LOGS" | grep -i "error\|failed\|refused" | wc -l)
if [ "$ERROR_COUNT" -gt 0 ]; then
    echo "❌ Found $ERROR_COUNT errors in forwarder:"
    echo "$FORWARDER_LOGS" | grep -i "error\|failed\|refused"
else
    echo "✅ No errors in forwarder logs"
fi
echo ""

# 7. Check if control-plane is reachable from k3d
echo "Step 7: Checking control-plane reachability from k3d..."
REACHABLE=$(docker exec k3d-meshvpn-server-0 sh -c "curl -s -m 2 http://host.docker.internal:8081/health" 2>&1)
if echo "$REACHABLE" | grep -q "OK"; then
    echo "✅ Control plane reachable from k3d"
else
    echo "❌ Control plane NOT reachable from k3d"
    echo "   This is why telemetry can't be sent!"
    echo "   Error: $REACHABLE"
fi
echo ""

echo "📊 Summary:"
echo "If steps 1-5 ✅: Traffic is reaching Traefik, DNS/routing is working"
echo "If step 7 ✅: Telemetry can be delivered to control-plane"
echo "If step 6 ✅: No errors in forwarder"
echo "If all ✅: Telemetry should be flowing!"
