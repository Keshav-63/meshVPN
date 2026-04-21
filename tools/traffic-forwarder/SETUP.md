# Traffic Tracking Setup Guide

This guide shows you how to set up traffic tracking for your MeshVPN deployments.

## Prerequisites

- k3d cluster running
- Control-plane running on your host machine
- Deployments created via the control-plane

## Quick Start (Automated)

Run the automated setup script:

```bash
cd tools/traffic-forwarder
./deploy.sh
```

This script will:
1. ✅ Check if Traefik access logs are enabled
2. ✅ Enable them if not (adds `--accesslog=true --accesslog.format=json`)
3. ✅ Build the traffic-forwarder Docker image
4. ✅ Load it into your k3d cluster
5. ✅ Deploy the forwarder to Kubernetes
6. ✅ Configure it to connect to your control-plane on host

**That's it!** Traffic tracking is now active.

## Verify It's Working

### 1. Check the forwarder is running
```bash
kubectl get pods -l app=traffic-forwarder
# Should show: Running
```

### 2. Watch the forwarder logs
```bash
kubectl logs -l app=traffic-forwarder -f
```

You should see:
```
2026/03/29 12:00:00 Starting traffic forwarder...
2026/03/29 12:00:00 Control Plane: http://host.k3d.internal:8080
2026/03/29 12:00:00 Traefik Pod: traefik-xxxxx (namespace: kube-system)
2026/03/29 12:00:00 Tailing Traefik access logs...
```

### 3. Send test traffic
```bash
# Send traffic to one of your deployed apps
curl https://your-app.keshavstack.tech
curl https://your-app.keshavstack.tech
curl https://your-app.keshavstack.tech
```

### 4. Watch forwarder process requests
In the forwarder logs, you should see:
```
Processing request for: your-app
Telemetry sent successfully
```

### 5. Check metrics (after 1-2 minutes)
```bash
# Get your deployment ID
DEPLOYMENT_ID=$(curl -s http://localhost:8080/deployments | jq -r '.deployments[0].deployment_id')

# Check metrics - should show non-zero requests!
curl http://localhost:8080/deployments/$DEPLOYMENT_ID | jq '.metrics.requests'
```

Expected output:
```json
{
  "total": 3,
  "last_hour": 3,
  "last_24h": 3,
  "per_second": 0.00083
}
```

## How It Works

1. **Traefik Access Logs** - Traefik outputs JSON access logs to stdout
2. **Forwarder Tails Logs** - The forwarder uses `kubectl logs -f` to stream Traefik logs
3. **Parse & Extract** - Parses JSON, extracts subdomain, status, latency, bytes
4. **Forward to Control-Plane** - POSTs to `/api/telemetry/deployment-request`
5. **Metrics Collector** - Control-plane aggregates data every minute
6. **Available in API** - View metrics via `/deployments/:id` endpoint

## Troubleshooting

### Forwarder shows EOF errors

The forwarder lost connection to Traefik. Restart it:
```bash
kubectl rollout restart deployment traffic-forwarder
```

### No traffic data appearing

Check Traefik logs are JSON formatted:
```bash
kubectl logs -n kube-system -l app.kubernetes.io/name=traefik --tail=5
```

Should see JSON like: `{"ClientHost":"...","RequestHost":"..."`

If not, access logs aren't enabled. Re-run:
```bash
./deploy.sh
```

### Metrics show zero requests

1. Check forwarder is processing requests:
   ```bash
   kubectl logs -l app=traffic-forwarder --tail=20
   ```

2. Send test traffic and watch forwarder logs

3. Wait 1-2 minutes for aggregation (collector runs every minute)

4. Check database directly:
   ```bash
   psql $DATABASE_URL -c "SELECT COUNT(*) FROM deployment_requests;"
   ```

### Control-plane connection errors

The forwarder can't reach the control-plane. Check:

1. Control-plane is running on host:
   ```bash
   curl http://localhost:8080/health
   ```

2. Forwarder has correct URL (should be `host.k3d.internal:8080`):
   ```bash
   kubectl get deployment traffic-forwarder -o yaml | grep CONTROL_PLANE_URL
   ```

If URL is wrong, re-run:
```bash
./deploy.sh
```

## Manual Deployment

If you prefer manual deployment:

### 1. Build the image
```bash
cd tools/traffic-forwarder
docker build -t traffic-forwarder:latest .
```

### 2. Load into k3d
```bash
k3d image import traffic-forwarder:latest -c meshvpn
```

### 3. Deploy
```bash
kubectl apply -f - <<EOF
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
          value: "http://host.k3d.internal:8080"
        - name: TRAEFIK_NAMESPACE
          value: "kube-system"
EOF
```

## Configuration

Environment variables for the forwarder:

| Variable | Default | Description |
|----------|---------|-------------|
| `CONTROL_PLANE_URL` | `http://host.k3d.internal:8080` | Control-plane telemetry endpoint |
| `TRAEFIK_NAMESPACE` | `kube-system` | Namespace where Traefik is running |
| `TRAEFIK_POD` | (auto-detected) | Specific Traefik pod to tail |

## Architecture

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │ HTTPS
       ▼
┌─────────────┐
│   Traefik   │ ──► Access Logs (JSON)
└──────┬──────┘
       │
       ▼
┌─────────────────────┐
│ Traffic Forwarder   │ ──► kubectl logs -f
│ (k8s pod)           │
└──────┬──────────────┘
       │ HTTP POST
       ▼
┌─────────────────────┐
│  Control-Plane      │ ──► /api/telemetry/deployment-request
│  (host machine)     │
└──────┬──────────────┘
       │
       ▼
┌─────────────────────┐
│  PostgreSQL         │ ──► deployment_requests table
└─────────────────────┘
       │
       ▼
┌─────────────────────┐
│ Metrics Collector   │ ──► Runs every minute
└──────┬──────────────┘
       │
       ▼
┌─────────────────────┐
│ deployment_metrics  │ ──► Aggregated data
└─────────────────────┘
```

## Next Steps

- View comprehensive deployment analytics: `GET /deployments/:id`
- See all deployments with metrics: `GET /deployments`
- Stream real-time analytics: `GET /deployments/:id/analytics/stream`

See [DEPLOYMENT-ANALYTICS-API.md](../../docs/DEPLOYMENT-ANALYTICS-API.md) for full API documentation.
