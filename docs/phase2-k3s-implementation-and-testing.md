# Phase 2: K3s + Tailscale + CPU-First Autoscaling

This runbook covers implementation usage and complete testing for Phase 2.

For full installation sequence, Docker vs k3s runtime role explanation, and complete end-to-end user workflow testing, see:

- `docs/phase2-installation-and-user-workflow.md`

If you are new to this stack and want beginner-friendly step-by-step instructions, use:

- `docs/phase2-beginner-full-setup.md`

## Scope Implemented

- Modular runtime backend driver:
  - Docker backend (existing behavior)
  - Kubernetes backend (k3s via kubectl)
- CPU-first autoscaling policy:
  - `scaling_mode=horizontal`
  - `min_replicas`, `max_replicas`, `cpu_target_utilization`
- Worker-applied HPA (optional feature flag)
- Prometheus metrics endpoint on control-plane: `GET /metrics`
- Observability stack manifests and low-footprint Helm values

## New Environment Variables

Set in your shell or infra env:

- `RUNTIME_BACKEND=docker|k3s|kubernetes`
- `ENABLE_CPU_HPA=true|false`
- `K8S_NAMESPACE=meshvpn-apps` (default `default`)
- `K8S_CONFIG_PATH=<optional kubeconfig path>`
- `KUBECTL_BIN=kubectl` (optional)
- `K8S_IMAGE_PREFIX=<registry/org>` required for kubernetes backend image push

Existing vars still apply:

- `DATABASE_URL` or `SUPABASE_DB_URL`
- `SUPABASE_JWT_SECRET`
- `REQUIRE_AUTH`
- `WORKER_POLL_INTERVAL`

## Deploy API (CPU-first fields)

Example payload:

```json
{
  "repo": "https://github.com/your-org/your-app.git",
  "port": 3000,
  "subdomain": "phase2demo",
  "scaling_mode": "horizontal",
  "min_replicas": 2,
  "max_replicas": 10,
  "cpu_target_utilization": 65,
  "cpu_request_milli": 500,
  "cpu_limit_milli": 1000,
  "node_selector": {
    "meshvpn.worker": "true"
  }
}
```

Notes:

- CPU-first policy is enabled in validation logic.
- Memory and custom metric scaling are future add-ons.

## Control-plane Metrics

Scrape:

- `GET /metrics`

Key metrics:

- `control_plane_deploy_requests_total{scaling_mode=...}`
- `control_plane_worker_jobs_total{status=...}`
- `control_plane_worker_job_duration_seconds`

## Observability Stack Setup

See:

- `infra/k8s/observability/README.md`
- `infra/k8s/observability/values-kube-prometheus-stack.yaml`

This gives:

- metrics-server
- Prometheus
- Grafana
- kube-state-metrics
- node-exporter

Retention default: 72h.

## Complete Testing Plan

## A) Local Build Validation

1. Go build and test:

```powershell
cd control-plane
go test ./...
```

2. Start control-plane in docker backend mode:

```powershell
$env:RUNTIME_BACKEND="docker"
$env:ENABLE_CPU_HPA="false"
go run ./cmd/control-plane
```

3. Verify:

```powershell
curl http://localhost:8080/health
curl http://localhost:8080/metrics
```

## B) K3s Backend Validation

1. Ensure k3s cluster reachable:

```powershell
kubectl get nodes
kubectl get ns
```

2. Set backend env:

```powershell
$env:RUNTIME_BACKEND="k3s"
$env:ENABLE_CPU_HPA="true"
$env:K8S_NAMESPACE="meshvpn-apps"
$env:K8S_IMAGE_PREFIX="ghcr.io/<your-org>"
```

3. Start control-plane:

```powershell
go run ./cmd/control-plane
```

4. Trigger deploy with horizontal mode.

5. Verify k8s objects:

```powershell
kubectl -n meshvpn-apps get deploy,svc,ing,hpa
kubectl -n meshvpn-apps describe hpa
```

Expected:

- Deployment/Service/Ingress created.
- HPA present when `ENABLE_CPU_HPA=true` and `scaling_mode=horizontal`.

## C) Autoscaling Behavior Test

1. Generate load to app endpoint for 3-5 minutes.
2. Watch HPA and replicas:

```powershell
kubectl -n meshvpn-apps get hpa -w
kubectl -n meshvpn-apps get deploy -w
```

Expected:

- Scale up when average CPU crosses target.
- Gradual scale down due to 300s stabilization.

## D) Request Flow + Worker Visibility

In Grafana, verify dashboards include:

1. Request flow route -> service/pod -> node(worker)
2. Pod CPU and memory
3. Node(worker) CPU and memory
4. HPA current vs desired replicas
5. Control-plane worker metrics

## E) Reliability Test

1. Start traffic.
2. Stop worker node (or cordon/drain for controlled test).
3. Verify app continues serving traffic from remaining node.
4. Verify p95 remains in 250-350ms target band under normal load.

## F) Rollback Test

1. Switch backend:

```powershell
$env:RUNTIME_BACKEND="docker"
$env:ENABLE_CPU_HPA="false"
```

2. Restart control-plane and trigger deploy.
3. Confirm legacy docker flow still works.

## Troubleshooting

- If k8s deploy fails with image pull errors:
  - check `K8S_IMAGE_PREFIX`
  - verify registry auth and image push permissions
- If HPA does not scale:
  - verify metrics-server installed and healthy
  - verify pod CPU requests are set
  - check `kubectl describe hpa <name>`
- If request flow metrics are missing:
  - verify ingress metrics are enabled
  - verify Prometheus targets are up

## Suggested Next Iteration

1. Add memory/custom metric policies as optional modules.
2. Add per-node affinity policy module.
3. Add OpenTelemetry tracing as Phase 2.2 optional add-on.
