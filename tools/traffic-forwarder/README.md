# Traffic Forwarder

This tool tails Traefik access logs and forwards traffic metrics to the control-plane telemetry endpoint.

## Quick Start

### 1. Enable Traefik Access Logs

```bash
# Edit your Traefik deployment
kubectl edit deployment traefik -n kube-system

# Add these args to the container:
- --accesslog=true
- --accesslog.format=json
```

### 2. Build and Run the Forwarder

```bash
cd tools/traffic-forwarder
go build -o traffic-forwarder

# Run it (it will auto-detect Traefik pod)
./traffic-forwarder
```

Or run directly:
```bash
go run main.go
```

### 3. Configure (Optional)

Set environment variables to customize:

```bash
# Control plane URL (default: http://host.k3d.internal:8080 for k3d)
export CONTROL_PLANE_URL=http://host.k3d.internal:8080

# Traefik namespace (default: kube-system)
export TRAEFIK_NAMESPACE=kube-system

# Traefik pod name (auto-detected if not set)
export TRAEFIK_POD=traefik-abc123-xyz

# Run
./traffic-forwarder
```

**Note:** For k3d clusters, use `host.k3d.internal` to access services running on your host machine.

## How It Works

1. **Tails Traefik Logs** - Uses `kubectl logs -f` to stream Traefik access logs
2. **Parses JSON** - Extracts request details (status, latency, bytes)
3. **Extracts Deployment ID** - Gets subdomain from host header
4. **Forwards to Control Plane** - POSTs to `/api/telemetry/deployment-request`

## Deployment ID Mapping

Currently, the forwarder extracts the subdomain from the host (e.g., `myapp` from `myapp.keshavstack.tech`).

**Note:** This works if your subdomain matches the deployment subdomain. If you need to map subdomain to deployment_id, you can:

1. Query the control-plane API to get the mapping
2. Cache it in memory
3. Update periodically

## Running as a Kubernetes Deployment

Create a deployment to run this continuously:

```yaml
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
        image: your-registry/traffic-forwarder:latest
        env:
        - name: CONTROL_PLANE_URL
          value: "http://control-plane:8080"
        - name: TRAEFIK_NAMESPACE
          value: "kube-system"
---
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
```

## Alternative: Subdomain to Deployment ID Mapping

If you need accurate deployment_id mapping, enhance the forwarder:

```go
// Add this function to fetch mapping from control-plane
func getDeploymentMapping(controlPlaneURL string) (map[string]string, error) {
    resp, err := http.Get(controlPlaneURL + "/deployments")
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var result struct {
        Deployments []struct {
            DeploymentID string `json:"deployment_id"`
            Subdomain    string `json:"subdomain"`
        } `json:"deployments"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }

    mapping := make(map[string]string)
    for _, d := range result.Deployments {
        mapping[d.Subdomain] = d.DeploymentID
    }

    return mapping, nil
}
```

## Troubleshooting

### No logs appearing
```bash
# Check Traefik is producing access logs
kubectl logs -n kube-system -l app.kubernetes.io/name=traefik | head

# Should see JSON formatted logs
```

### Telemetry endpoint errors
```bash
# Check control-plane is accessible
curl http://localhost:8080/health

# Test telemetry endpoint
curl -X POST http://localhost:8080/api/telemetry/deployment-request \
  -H "Content-Type: application/json" \
  -d '{"deployment_id":"test","status_code":200,"latency_ms":50,"bytes_sent":1024,"bytes_received":512,"path":"/test"}'
```

### Wrong Traefik pod detected
```bash
# Set explicitly
export TRAEFIK_POD=$(kubectl get pods -n kube-system -l app.kubernetes.io/name=traefik -o jsonpath='{.items[0].metadata.name}')
./traffic-forwarder
```
