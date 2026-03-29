#!/bin/bash

echo "🔍 Traffic Tracking Diagnostic Tool"
echo "===================================="
echo ""

# 1. Check if traffic forwarder is running
echo "1️⃣ Checking traffic forwarder status..."
FORWARDER_PODS=$(kubectl get pods -l app=traffic-forwarder -o jsonpath='{.items[*].metadata.name}' 2>/dev/null)
if [ -z "$FORWARDER_PODS" ]; then
    echo "❌ Traffic forwarder is not running"
    echo "   Run: cd tools/traffic-forwarder && ./deploy.sh"
    exit 1
else
    echo "✅ Traffic forwarder is running: $FORWARDER_PODS"
fi

# 2. Check Traefik access log configuration
echo ""
echo "2️⃣ Checking Traefik access log configuration..."
TRAEFIK_NAMESPACE=${TRAEFIK_NAMESPACE:-kube-system}
ACCESSLOG_ENABLED=$(kubectl get deployment traefik -n $TRAEFIK_NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].args}' 2>/dev/null | grep -c "accesslog=true" || echo "0")

if [ "$ACCESSLOG_ENABLED" -gt "0" ]; then
    echo "✅ Access logs are enabled"

    # Check format
    FORMAT_JSON=$(kubectl get deployment traefik -n $TRAEFIK_NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].args}' 2>/dev/null | grep -c "accesslog.format=json" || echo "0")
    if [ "$FORMAT_JSON" -gt "0" ]; then
        echo "✅ Access logs are in JSON format"
    else
        echo "⚠️  Access logs are NOT in JSON format"
        echo "   Fix: kubectl patch deployment traefik -n $TRAEFIK_NAMESPACE --type='json' -p='[{\"op\":\"add\",\"path\":\"/spec/template/spec/containers/0/args/-\",\"value\":\"--accesslog.format=json\"}]'"
    fi
else
    echo "❌ Access logs are NOT enabled"
    echo "   Fix: Run ./deploy.sh again, or manually patch:"
    echo "   kubectl patch deployment traefik -n $TRAEFIK_NAMESPACE --type='json' -p='[{\"op\":\"add\",\"path\":\"/spec/template/spec/containers/0/args/-\",\"value\":\"--accesslog=true\"},{\"op\":\"add\",\"path\":\"/spec/template/spec/containers/0/args/-\",\"value\":\"--accesslog.format=json\"}]'"
fi

# 3. Check for deployments
echo ""
echo "3️⃣ Checking for active deployments..."
DEPLOYMENT_COUNT=$(kubectl get ingress -n meshvpn-apps 2>/dev/null | grep -c "app-" || echo "0")
if [ "$DEPLOYMENT_COUNT" -gt "0" ]; then
    echo "✅ Found $DEPLOYMENT_COUNT deployment(s)"
    kubectl get ingress -n meshvpn-apps -o custom-columns=NAME:.metadata.name,HOST:.spec.rules[0].host --no-headers
else
    echo "⚠️  No deployments found"
    echo "   Deploy a test app to generate traffic"
fi

# 4. Generate test traffic
echo ""
echo "4️⃣ Testing with sample traffic..."
FIRST_HOST=$(kubectl get ingress -n meshvpn-apps -o jsonpath='{.items[0].spec.rules[0].host}' 2>/dev/null)
if [ ! -z "$FIRST_HOST" ]; then
    echo "   Sending request to: https://$FIRST_HOST"
    RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" "https://$FIRST_HOST" 2>/dev/null || echo "000")
    if [ "$RESPONSE" != "000" ]; then
        echo "✅ Got response: HTTP $RESPONSE"
        echo "   Now check forwarder logs: kubectl logs -l app=traffic-forwarder --tail=20"
    else
        echo "⚠️  Could not connect to deployment"
    fi
else
    echo "⚠️  No deployment hosts found to test"
fi

# 5. Check database for request data
echo ""
echo "5️⃣ Checking database for request records..."
if [ ! -z "$DATABASE_URL" ]; then
    REQUEST_COUNT=$(psql "$DATABASE_URL" -t -c "SELECT COUNT(*) FROM deployment_requests;" 2>/dev/null | tr -d ' ')
    if [ ! -z "$REQUEST_COUNT" ]; then
        echo "✅ Found $REQUEST_COUNT request(s) in database"
        if [ "$REQUEST_COUNT" -gt "0" ]; then
            echo "   Recent requests:"
            psql "$DATABASE_URL" -c "SELECT deployment_id, status_code, latency_ms, timestamp FROM deployment_requests ORDER BY timestamp DESC LIMIT 5;" 2>/dev/null
        fi
    else
        echo "⚠️  Could not query database"
    fi
else
    echo "⚠️  DATABASE_URL not set, skipping database check"
fi

# 6. Summary and next steps
echo ""
echo "📋 Summary"
echo "=========="
echo ""
echo "✅ What's working:"
echo "   - Traffic forwarder is deployed and running"
echo ""
echo "🧪 To test traffic tracking:"
echo "   1. Send traffic to your app:"
echo "      curl https://YOUR-APP.keshavstack.tech"
echo ""
echo "   2. Watch forwarder logs (should see processing messages):"
echo "      kubectl logs -l app=traffic-forwarder -f"
echo ""
echo "   3. Wait 1-2 minutes for metrics to aggregate"
echo ""
echo "   4. Check metrics endpoint:"
echo "      curl http://localhost:8080/deployments/YOUR_ID | jq '.metrics.requests'"
echo ""
echo "📊 Check forwarder logs right now:"
echo "   kubectl logs -l app=traffic-forwarder --tail=50"
echo ""
