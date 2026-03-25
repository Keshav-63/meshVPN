#!/bin/bash
# MeshVPN Analytics Stack Startup Script

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
OBSERVABILITY_DIR="$PROJECT_ROOT/infra/observability"

echo "=== MeshVPN Analytics Stack Startup ==="
echo ""

# Check if Prometheus is already running
if pgrep -x "prometheus" > /dev/null; then
    echo "✓ Prometheus is already running"
else
    echo "Starting Prometheus..."

    # Check if Prometheus is installed
    if ! command -v prometheus &> /dev/null; then
        echo "⚠ Prometheus not found. Installing..."

        cd /tmp
        wget https://github.com/prometheus/prometheus/releases/download/v2.51.0/prometheus-2.51.0.linux-amd64.tar.gz
        tar xvfz prometheus-2.51.0.linux-amd64.tar.gz
        sudo mv prometheus-2.51.0.linux-amd64/prometheus /usr/local/bin/
        sudo mv prometheus-2.51.0.linux-amd64/promtool /usr/local/bin/
        rm -rf prometheus-2.51.0.linux-amd64*

        echo "✓ Prometheus installed"
    fi

    # Start Prometheus in background
    cd "$OBSERVABILITY_DIR"
    nohup prometheus --config.file=prometheus.yml \
        --storage.tsdb.path=/tmp/prometheus-data \
        --web.listen-address=:9090 \
        > /tmp/prometheus.log 2>&1 &

    echo "✓ Prometheus started on http://localhost:9090"
    echo "  Logs: tail -f /tmp/prometheus.log"
fi

# Wait for Prometheus to be ready
echo ""
echo "Waiting for Prometheus to be ready..."
for i in {1..10}; do
    if curl -s http://localhost:9090/-/healthy > /dev/null 2>&1; then
        echo "✓ Prometheus is healthy"
        break
    fi
    sleep 1
    echo -n "."
done

# Start Grafana
echo ""
echo "Starting Grafana..."
cd "$OBSERVABILITY_DIR"

if docker-compose ps grafana | grep -q "Up"; then
    echo "✓ Grafana is already running"
else
    docker-compose up -d grafana
    echo "✓ Grafana started on http://localhost:3001"
fi

# Wait for Grafana to be ready
echo ""
echo "Waiting for Grafana to be ready..."
for i in {1..30}; do
    if curl -s http://localhost:3001/api/health > /dev/null 2>&1; then
        echo "✓ Grafana is healthy"
        break
    fi
    sleep 1
    echo -n "."
done

echo ""
echo "=== Analytics Stack Ready ==="
echo ""
echo "📊 Access Points:"
echo "  • Grafana Dashboard: http://localhost:3001"
echo "  • Prometheus UI:     http://localhost:9090"
echo "  • Metrics Endpoint:  http://localhost:8080/metrics"
echo ""
echo "🎯 Default Dashboard: MeshVPN Comprehensive Analytics"
echo "   (Auto-loads when you open Grafana)"
echo ""
echo "📖 Complete Guide: $PROJECT_ROOT/GRAFANA-DASHBOARD-GUIDE.md"
echo ""
echo "🔧 To stop analytics:"
echo "  • Prometheus: pkill prometheus"
echo "  • Grafana:    docker-compose -f $OBSERVABILITY_DIR/docker-compose.yml stop grafana"
echo ""
