# Phase 2 Observability Stack (Lightweight)

This stack is free/open-source and optimized for low operational overhead:

- metrics-server (required for HPA CPU scaling)
- kube-prometheus-stack (Prometheus + Grafana + kube-state-metrics + node-exporter)

## Install

1. Install metrics-server:

```powershell
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
```

2. Add Prometheus community Helm repo:

```powershell
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
```

3. Install kube-prometheus-stack with low-footprint values:

```powershell
helm upgrade --install observability prometheus-community/kube-prometheus-stack ^
  -n observability --create-namespace ^
  -f infra/k8s/observability/values-kube-prometheus-stack.yaml
```

4. Port-forward Grafana:

```powershell
kubectl -n observability port-forward svc/observability-grafana 3001:80
```

5. Open Grafana:

- URL: http://localhost:3001
- Username: admin
- Password: run `kubectl -n observability get secret observability-grafana -o jsonpath="{.data.admin-password}" | %{ [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($_)) }`

## Key Dashboards To Build

1. Request flow by route/pod/node(worker):
- metric examples:
  - `traefik_service_requests_total`
  - `traefik_entrypoint_requests_total`
  - `traefik_service_request_duration_seconds_bucket`
- group by labels containing route/service/pod/node.

2. Pod resource usage:
- `container_cpu_usage_seconds_total`
- `container_memory_working_set_bytes`
- `kube_pod_info` for node mapping.

3. Node(worker) resource usage:
- `node_cpu_seconds_total`
- `node_memory_MemAvailable_bytes`

4. HPA behavior:
- `kube_horizontalpodautoscaler_status_current_replicas`
- `kube_horizontalpodautoscaler_status_desired_replicas`

5. Control-plane worker metrics:
- `control_plane_worker_jobs_total`
- `control_plane_worker_job_duration_seconds`
- `control_plane_deploy_requests_total`

## Notes

- Use CPU-first autoscaling in Phase 2.1.
- Keep metric cardinality low: avoid high-cardinality labels (request IDs, user IDs).
- Default retention is set to 72h in values file and can be adjusted later.
