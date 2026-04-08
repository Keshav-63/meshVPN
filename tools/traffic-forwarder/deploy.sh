#!/bin/bash
set -e

# Load root .env so telemetry/config values are honored without manual export.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
if [ -f "$PROJECT_ROOT/.env" ]; then
  set -a
  source "$PROJECT_ROOT/.env"
  set +a
fi

echo "🚀 Setting up Traffic Tracking for MeshVPN"
echo ""

# Step 1: Check if Traefik has access logs enabled
echo "Step 1: Checking Traefik access logs..."
TRAEFIK_NAMESPACE=${TRAEFIK_NAMESPACE:-kube-system}
TRAEFIK_DEPLOYMENT=$(kubectl get deployment -n $TRAEFIK_NAMESPACE -l app.kubernetes.io/name=traefik -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

if [ -z "$TRAEFIK_DEPLOYMENT" ]; then
    echo "❌ Could not find Traefik deployment in namespace $TRAEFIK_NAMESPACE"
    echo "   Set TRAEFIK_NAMESPACE environment variable if Traefik is in a different namespace"
    exit 1
fi

echo "✅ Found Traefik: $TRAEFIK_DEPLOYMENT in namespace $TRAEFIK_NAMESPACE"

# Check if access logs are enabled
ACCESS_LOG_ENABLED=$(kubectl get deployment -n $TRAEFIK_NAMESPACE $TRAEFIK_DEPLOYMENT -o jsonpath='{.spec.template.spec.containers[0].args}' | grep -c "accesslog=true" || echo "0")

if [ "$ACCESS_LOG_ENABLED" = "0" ]; then
    echo "⚠️  Access logs not enabled. Enabling now..."

    # Patch the deployment to add access log args
    kubectl patch deployment -n $TRAEFIK_NAMESPACE $TRAEFIK_DEPLOYMENT --type='json' -p='[
      {
        "op": "add",
        "path": "/spec/template/spec/containers/0/args/-",
        "value": "--accesslog=true"
      },
      {
        "op": "add",
        "path": "/spec/template/spec/containers/0/args/-",
        "value": "--accesslog.format=json"
      }
    ]'

    echo "✅ Access logs enabled. Waiting for Traefik to restart..."
    kubectl rollout status deployment -n $TRAEFIK_NAMESPACE $TRAEFIK_DEPLOYMENT
else
    echo "✅ Access logs already enabled"
fi

# Step 2: Build the forwarder Docker image
echo ""
echo "Step 2: Building traffic forwarder Docker image..."
docker build -t traffic-forwarder:latest .

if [ $? -eq 0 ]; then
    echo "✅ Docker image built successfully"
else
    echo "❌ Failed to build Docker image"
    exit 1
fi

# Step 3: Load image into k3d cluster (if using k3d)
if command -v k3d &> /dev/null; then
    CLUSTER_NAME=${K3D_CLUSTER_NAME:-meshvpn}
    echo ""
    echo "Step 3: Loading image into k3d cluster $CLUSTER_NAME..."
    k3d image import traffic-forwarder:latest -c $CLUSTER_NAME
    echo "✅ Image loaded into k3d cluster"
fi

# Step 4: Check bridge proxy
echo ""
echo "Step 4: Checking bridge proxy for WSL → Docker networking..."

# Compile the proxy if needed
if [ ! -f "./bridge-proxy.exe" ]; then
    echo "Compiling bridge proxy..."
    go build -o bridge-proxy.exe bridge-proxy.go
fi

# Check if proxy is accessible from k3d (the real test)
if docker exec k3d-meshvpn-server-0 sh -c "wget -qO- --timeout=2 http://host.docker.internal:8081/health" > /dev/null 2>&1; then
    echo "✅ Bridge proxy is accessible from k3d"
else
    echo "⚠️  Bridge proxy not accessible. Starting it now..."

    # Kill any existing proxy process
    pkill -f "bridge-proxy.exe" 2>/dev/null || true

    # Start proxy in background
    ./bridge-proxy.exe > bridge-proxy.log 2>&1 &
    PROXY_PID=$!
    echo "Started bridge proxy (PID: $PROXY_PID)"

    # Wait for it to be accessible from k3d
    sleep 3

    if docker exec k3d-meshvpn-server-0 sh -c "wget -qO- --timeout=2 http://host.docker.internal:8081/health" > /dev/null 2>&1; then
        echo "✅ Bridge proxy started successfully"
    else
        echo "❌ Failed to start bridge proxy. Check bridge-proxy.log"
        echo "   Manual start: ./bridge-proxy.exe &"
        exit 1
    fi
fi

# Use env override when provided; otherwise default to bridge proxy for k3d → WSL communication.
CONTROL_PLANE_URL=${CONTROL_PLANE_URL:-http://host.docker.internal:8081}
APP_BASE_DOMAIN=${APP_BASE_DOMAIN:-keshavstack.tech}
TELEMETRY_BATCH_SIZE=${TELEMETRY_BATCH_SIZE:-100}
TELEMETRY_QUEUE_SIZE=${TELEMETRY_QUEUE_SIZE:-50000}
TELEMETRY_FLUSH_INTERVAL=${TELEMETRY_FLUSH_INTERVAL:-500ms}
TELEMETRY_HTTP_TIMEOUT=${TELEMETRY_HTTP_TIMEOUT:-10s}
echo "✅ Using bridge proxy: $CONTROL_PLANE_URL"
echo "✅ Telemetry settings: batch_size=$TELEMETRY_BATCH_SIZE queue_size=$TELEMETRY_QUEUE_SIZE flush_interval=$TELEMETRY_FLUSH_INTERVAL http_timeout=$TELEMETRY_HTTP_TIMEOUT"

# Step 5: Deploy the forwarder
echo ""
echo "Step 5: Deploying traffic forwarder to Kubernetes..."

cat > /tmp/traffic-forwarder-deploy.yaml <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: traffic-forwarder
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: traffic-forwarder
rules:
- apiGroups: [""]
  resources: ["pods", "pods/log"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: traffic-forwarder
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: traffic-forwarder
subjects:
- kind: ServiceAccount
  name: traffic-forwarder
  namespace: default
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: traffic-forwarder
  namespace: default
  labels:
    app: traffic-forwarder
spec:
  replicas: 1
  selector:
    matchLabels:
      app: traffic-forwarder
  template:
    metadata:
      labels:
        app: traffic-forwarder
    spec:
      serviceAccountName: traffic-forwarder
      containers:
      - name: forwarder
        image: traffic-forwarder:latest
        imagePullPolicy: Never
        env:
        - name: CONTROL_PLANE_URL
          value: "$CONTROL_PLANE_URL"
        - name: TRAEFIK_NAMESPACE
          value: "$TRAEFIK_NAMESPACE"
        - name: APP_BASE_DOMAIN
          value: "$APP_BASE_DOMAIN"
        - name: TELEMETRY_BATCH_SIZE
          value: "$TELEMETRY_BATCH_SIZE"
        - name: TELEMETRY_QUEUE_SIZE
          value: "$TELEMETRY_QUEUE_SIZE"
        - name: TELEMETRY_FLUSH_INTERVAL
          value: "$TELEMETRY_FLUSH_INTERVAL"
        - name: TELEMETRY_HTTP_TIMEOUT
          value: "$TELEMETRY_HTTP_TIMEOUT"
EOF

kubectl apply -f /tmp/traffic-forwarder-deploy.yaml

# Force restart so the running pod picks the latest imported local image tag.
kubectl rollout restart deployment traffic-forwarder -n default

echo "✅ Traffic forwarder deployed"

# Step 6: Wait for pod to be ready
echo ""
echo "Step 6: Waiting for traffic forwarder to start..."

# Wait on deployment rollout instead of label-based pod wait.
# Label waits can race with terminating old ReplicaSet pods and return NotFound.
kubectl rollout status deployment/traffic-forwarder -n default --timeout=120s

# Show current pod state for visibility, but do not fail after successful rollout.
echo "Current forwarder pods:"
kubectl get pods -n default -l app=traffic-forwarder -o wide || true

echo ""
echo "✅ Traffic tracking is now active!"
echo ""
echo "📊 Check the logs:"
echo "   kubectl logs -l app=traffic-forwarder -f"
echo ""
echo "🧪 Test it:"
echo "   1. Send traffic to a deployed app:"
echo "      curl https://your-app.keshavstack.tech"
echo ""
echo "   2. Watch forwarder process requests:"
echo "      kubectl logs -l app=traffic-forwarder -f"
echo ""
echo "   3. Wait 1-2 minutes for metrics to aggregate"
echo ""
echo "   4. Check metrics (should see non-zero requests):"
echo "      curl http://localhost:8080/deployments/YOUR_ID | jq '.metrics.requests'"
echo ""
echo "🎯 Control Plane URL: $CONTROL_PLANE_URL"
echo ""
