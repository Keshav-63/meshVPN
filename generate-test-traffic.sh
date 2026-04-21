#!/bin/bash
# Generate test traffic to trigger telemetry collection

echo "🚀 Generating test traffic to MeshVPN deployments..."
echo ""

# Configuration
TARGET_HOST="${1:-http://localhost:8080}"
NUM_USERS="${2:-5}"
SPAWN_RATE="${3:-1}"
DURATION="${4:-2m}"

echo "Configuration:"
echo "  Target: $TARGET_HOST"
echo "  Users: $NUM_USERS"
echo "  Spawn rate: $SPAWN_RATE users/sec"
echo "  Duration: $DURATION"
echo ""

# Check if locust is installed
if ! command -v locust &> /dev/null; then
    echo "❌ Locust not installed. Installing..."
    pip install locust
fi

echo "📊 Starting load test..."
echo ""
echo "Watch the telemetry in another terminal:"
echo "  kubectl logs -l app=traffic-forwarder -f"
echo ""
echo "Press Ctrl+C to stop"
echo ""

# Run locust with command-line options (headless mode)
locust -f locustfile.py \
    -H "$TARGET_HOST" \
    -u "$NUM_USERS" \
    -r "$SPAWN_RATE" \
    --run-time "$DURATION" \
    --headless \
    --csv=load-test-results \
    --csv-prefix=edge-traffic

echo ""
echo "✅ Load test complete!"
echo ""
echo "Results saved to edge-traffic_*.csv"
