# Phase 2 Beginner Guide: Tailscale + K3s + Prometheus + Grafana + Autoscaling

This guide is written for beginners and explains:

- What each component does
- Exactly what to install
- How to configure each laptop
- How scaling works across laptops
- What metrics you get (and what you do not yet get)
- Full user workflow testing

## 0) Big Picture (Simple)

Your platform has two parts:

1. Control plane (your API + worker logic)
2. Runtime cluster (k3s) that runs app pods

Traffic flow in Phase 2:

1. User calls POST /deploy on control plane
2. Control plane clones repo and builds/pushes image
3. k3s creates Deployment/Service/Ingress
4. Traefik ingress routes app domain to service
5. HPA scales pod count on CPU load
6. Kubernetes scheduler can place new pods on other laptop nodes

## 1) Does Docker still matter now?

Yes.

- In k3s mode, Docker is used to build and push images.
- k3s runs the app as pods.
- So Docker is build/push engine, k3s is runtime scheduler.

## 2) Will Traefik still work?

Yes.

- k3s usually ships with Traefik ingress controller by default.
- Your new k3s ingress resources are handled by Traefik inside k3s.
- Old docker-compose Traefik is for old docker mode/local setup.

Recommendation:

- For k3s backend, use k3s Traefik only.
- Do not mix old docker-compose Traefik routing with k3s routing for the same apps.

## 3) What metrics do you get now?

Current metrics are platform/app runtime metrics, not end-user identity analytics.

You can see:

- request rates and latency (ingress/app level)
- pod CPU/memory
- node CPU/memory
- HPA desired/current replicas
- control-plane worker job metrics

Not included by default yet:

- per-application end-user identity metrics (like user id, user plan, tenant-specific product analytics)

You can add that later by instrumenting your app and exporting custom metrics.

## 4) Will scaling run pods on other laptop?

Yes, if all of these are true:

1. Other laptop is joined as k3s worker node and is Ready
2. It has enough free CPU/memory
3. Scheduling is not restricted to control-plane node
4. Pod image can be pulled on that node

What to do on other laptop:

1. Install Tailscale
2. Join same tailnet
3. Install k3s agent and join cluster
4. Ensure node is Ready
5. Ensure registry pull access works

Then new replicas can be scheduled there.

## 5) Recommended Topology

Start simple:

- Laptop A: control-plane + k3s server
- Laptop B: k3s worker

Later add Laptop C/D workers.

## 6) What to Install (Checklist)

Control-plane laptop:

- Go
- Git
- Docker Desktop (or Docker Engine)
- kubectl
- Helm
- Tailscale
- k3s server

Worker laptop(s):

- Docker Desktop (or Docker Engine)
- Tailscale
- k3s agent

In-cluster components:

- metrics-server
- kube-prometheus-stack (Prometheus + Grafana + kube-state-metrics + node-exporter)

Optional later:

- OpenTelemetry collector
- Loki

## 7) Tailscale Setup (All Laptops)

Do this on every laptop.

1. Install Tailscale.
2. Sign in with same Tailscale account/tailnet.
3. Enable MagicDNS in Tailscale admin console.
4. Confirm each laptop can ping each other via Tailscale IP/name.

Verification:

- From Laptop A, ping Laptop B Tailscale IP.
- From Laptop B, ping Laptop A Tailscale IP.

Tips:

- Keep ACL simple initially (allow all devices in your own tailnet), then tighten later.
- Give easy names to devices in Tailscale admin.

## 8) Kubernetes (k3s) Setup

Important: k3s is Linux-native. If your laptops are Windows, use Linux environment (native Linux, VM, or WSL2).

### 8.1 On Laptop A (k3s server)

1. Install k3s server.
2. Get node token and server URL.
3. Verify:

```bash
kubectl get nodes
```

### 8.2 On Laptop B (k3s worker)

1. Install k3s agent using:
   - server URL from Laptop A
   - token from Laptop A
2. Verify on Laptop A:

```bash
kubectl get nodes -o wide
```

Expected:

- both nodes are Ready

### 8.3 Create namespace for apps

```bash
kubectl create namespace meshvpn-apps
```

(If already exists, ignore.)

## 9) Registry Setup (Required)

Because control-plane pushes images and k3s pulls them.

1. Create/use registry namespace (example: ghcr.io/your-org)
2. Login Docker on control-plane laptop:

```bash
docker login ghcr.io
```

3. Configure image pull auth for k3s namespace if needed.

Without this, pods may fail with ImagePullBackOff.

## 10) Prometheus + Grafana Setup

Use the low-overhead setup already in repo.

1. Install metrics-server:

```bash
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
```

2. Install kube-prometheus-stack:

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm upgrade --install observability prometheus-community/kube-prometheus-stack \
  -n observability --create-namespace \
  -f infra/k8s/observability/values-kube-prometheus-stack.yaml
```

3. Open Grafana:

```bash
kubectl -n observability port-forward svc/observability-grafana 3001:80
```

Open http://localhost:3001

## 11) Control-Plane Config (k3s mode)

Set environment variables:

- RUNTIME_BACKEND=k3s
- ENABLE_CPU_HPA=true
- K8S_NAMESPACE=meshvpn-apps
- K8S_IMAGE_PREFIX=ghcr.io/your-org
- DATABASE_URL (or SUPABASE_DB_URL)
- SUPABASE_JWT_SECRET
- REQUIRE_AUTH=true

Then run control-plane.

## 12) Full User Workflow Testing (Beginner Friendly)

### Step A: Health and metrics

1. GET /health
2. GET /metrics

Expected:

- both return success

### Step B: Submit first deploy

Send POST /deploy with CPU-first autoscaling fields:

- scaling_mode=horizontal
- min_replicas=2
- max_replicas=10
- cpu_target_utilization=65
- cpu_request_milli=500

Expected:

- 202 Accepted
- status=queued

### Step C: Watch deployment progress

1. GET /deployments
2. GET /deployments/{id}/build-logs

Expected transition:

- queued -> deploying -> running

### Step D: Verify k3s resources

```bash
kubectl -n meshvpn-apps get deploy,svc,ing,hpa
kubectl -n meshvpn-apps describe hpa
```

Expected:

- Deployment, Service, Ingress, HPA exist

### Step E: Open app URL

- Verify app is reachable through ingress host

### Step F: Trigger load and observe scaling

1. Generate load for 3-5 minutes.
2. Watch HPA:

```bash
kubectl -n meshvpn-apps get hpa -w
kubectl -n meshvpn-apps get deploy -w
```

Expected:

- replicas increase during load
- replicas reduce later (not instantly) after load drops

### Step G: Confirm pods can run on worker laptop

```bash
kubectl -n meshvpn-apps get pods -o wide
```

Expected:

- some pods may be scheduled on worker node

If all stay on server node:

- check worker node resources
- check worker node is Ready
- check taints/cordon states

### Step H: Grafana validation

Verify dashboards/panels for:

- request rate and latency
- pod CPU/memory
- node CPU/memory
- HPA desired/current replicas
- control-plane worker metrics

### Step I: Failure test

1. Keep traffic running.
2. Stop/disable worker node.
3. Verify app keeps serving from remaining node.

### Step J: Rollback test

1. Set RUNTIME_BACKEND=docker
2. ENABLE_CPU_HPA=false
3. Restart control-plane and deploy again

Expected:

- docker mode still works

## 13) Common Issues and Fixes

1. ImagePullBackOff:
- registry auth missing on cluster
- wrong K8S_IMAGE_PREFIX

2. HPA not scaling:
- metrics-server not healthy
- CPU requests not set
- check kubectl describe hpa

3. App not reachable:
- ingress host mismatch
- DNS/host routing issue
- ingress controller not running

4. No pods on worker laptop:
- worker not Ready
- worker has no resources
- scheduling constraints/taints

## 14) What to do next after this guide

1. Add node selector and affinity policy for stronger cross-laptop spread control.
2. Add dashboard JSON provisioning files so panels are pre-created.
3. Add scripted smoke test runner for one-command verification.

## 15) Copy-Paste Command Paths (First Run)

Use this section if you want exact commands in order.

### 15.1 Control-plane laptop (PowerShell + Linux shell for k3s)

1. Clone and open project:

```powershell
cd "C:\Users\Keshav suthar\Desktop\MeshVPN-slef-hosting"
```

2. Prepare environment file:

```powershell
Copy-Item infra/.env.example infra/.env -Force
```

3. Edit infra/.env and set:

- DATABASE_URL or SUPABASE_DB_URL
- SUPABASE_JWT_SECRET
- RUNTIME_BACKEND=k3s
- ENABLE_CPU_HPA=true
- K8S_NAMESPACE=meshvpn-apps
- K8S_IMAGE_PREFIX=ghcr.io/your-org

4. Verify control-plane code compiles:

```powershell
cd control-plane
go test ./...
cd ..
```

5. Install metrics stack after cluster is up:

```bash
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm upgrade --install observability prometheus-community/kube-prometheus-stack \
  -n observability --create-namespace \
  -f infra/k8s/observability/values-kube-prometheus-stack.yaml
```

6. Run control-plane (host mode recommended for k3s):

```powershell
cd control-plane
$env:RUNTIME_BACKEND="k3s"
$env:ENABLE_CPU_HPA="true"
$env:K8S_NAMESPACE="meshvpn-apps"
$env:K8S_IMAGE_PREFIX="ghcr.io/your-org"
go run ./cmd/control-plane
```

### 15.2 Worker laptop

1. Join same Tailscale network.
2. Install and start k3s agent joining server node.
3. Verify from server laptop:

```bash
kubectl get nodes -o wide
```

4. Confirm worker is Ready before testing deploy.

### 15.3 First deploy and verify

1. Health and metrics:

```powershell
curl http://localhost:8080/health
curl http://localhost:8080/metrics
```

2. Deploy request:

```powershell
curl -X POST http://localhost:8080/deploy ^
  -H "Content-Type: application/json" ^
  -d "{\"repo\":\"https://github.com/your-org/your-app.git\",\"port\":3000,\"subdomain\":\"phase2demo\",\"scaling_mode\":\"horizontal\",\"min_replicas\":2,\"max_replicas\":10,\"cpu_target_utilization\":65,\"cpu_request_milli\":500,\"cpu_limit_milli\":1000}"
```

3. Verify resources:

```bash
kubectl -n meshvpn-apps get deploy,svc,ing,hpa
kubectl -n meshvpn-apps get pods -o wide
```

4. Watch autoscaling under load:

```bash
kubectl -n meshvpn-apps get hpa -w
kubectl -n meshvpn-apps get deploy -w
```
