# MeshVPN End-to-End Validation (Swagger + Prometheus + Grafana)

This runbook validates the full platform path:

1. API and worker flow (Swagger UI)
2. Deployment analytics endpoints
3. Platform analytics endpoints
4. Prometheus scrape and metric availability
5. Grafana dashboards and panels

Use this after any major platform change.

---

## 1. Preflight Checklist

## Services

1. Control-plane running at http://localhost:8080
2. Prometheus running at http://localhost:9090
3. Grafana running at http://localhost:3000
4. At least one worker registered (control-plane worker or remote worker)

## Dashboard files expected

1. infra/observability/grafana-dashboards/platform-overview.json
2. infra/observability/grafana-dashboards/deployment-detail.json
3. infra/observability/grafana-dashboards/meshvpn-comprehensive.json

## Quick status commands

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080/health
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080/swagger/index.html
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:9090/-/healthy
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:3000/api/health
```

Expected: all return 200.

---

## 2. Swagger UI End-to-End API Validation

Open Swagger UI:

http://localhost:8080/swagger/index.html

## Authentication setup

1. Click Authorize
2. Add Bearer token: Bearer <SUPABASE_JWT>
3. Confirm protected endpoints are unlocked

## Core API checks (in this order)

1. GET /health
Expected: 200

2. GET /auth/whoami
Expected: 200 with sub/email/provider

3. POST /deploy
Use payload:

```json
{
  "repo": "https://github.com/<org>/<repo>.git",
  "package": "small",
  "port": 3000
}
```

Expected:
1. 202 accepted
2. deployment_id present
3. subdomain and url present
4. status queued

4. GET /deployments
Expected:
1. new deployment appears
2. includes status and summary metrics fields

5. GET /deployments/{id}
Expected:
1. deployment block
2. metrics block
3. pods array
4. resources block
5. scaling block

6. GET /deployments/{id}/build-logs
Expected: non-empty logs during or after deployment

7. GET /deployments/{id}/app-logs
Expected: runtime logs when container is running

8. GET /deployments/{id}/analytics
Expected: deployment-level request, latency, bandwidth metrics object

9. GET /platform/analytics
Expected:
1. workers totals
2. deployments totals
3. resources utilization
4. pods totals

10. GET /platform/workers/{worker_id}/analytics
Expected: per-worker analytics response

---

## 3. Prometheus Validation

## Target health

Check target page:

http://localhost:9090/targets

Expected:
1. control-plane target exists
2. state UP
3. last scrape updates every few seconds

## Metric presence checks

Run:

```bash
curl -s http://localhost:8080/metrics | grep -E '^(platform_|deployment_|worker_|control_plane_)' | head -n 80
```

Expected series families present:

1. control_plane_worker_jobs_total
2. control_plane_worker_job_duration_seconds
3. deployment_pods
4. deployment_cpu_usage_percent
5. deployment_memory_usage_mb
6. platform_workers_total
7. platform_worker_capacity
8. platform_deployments_total
9. platform_pods_total
10. worker_current_jobs
11. worker_cpu_cores
12. worker_memory_gb

## Optional PromQL quick checks

In Prometheus expression browser:

1. platform_workers_total
2. platform_deployments_total
3. sum(deployment_pods{type="current"})
4. sum(deployment_cpu_usage_percent)

Expected: non-empty values for active system.

---

## 4. Grafana Validation

Open Grafana:

http://localhost:3000

## Data source check

1. Go to Connections -> Data Sources -> Prometheus
2. URL must be http://localhost:9090
3. Click Save & Test
4. Expected: Data source is working

## Dashboard import check

Import or verify these dashboards:

1. infra/observability/grafana-dashboards/platform-overview.json
2. infra/observability/grafana-dashboards/deployment-detail.json
3. infra/observability/grafana-dashboards/meshvpn-comprehensive.json

## Panel checks (must all be non-error)

For Platform Overview:

1. Worker status panel
2. Deployment status panel
3. Pod totals panel
4. Worker capacity panel

For Deployment Detail:

1. Deployment pods current vs desired
2. CPU usage panel
3. Memory usage panel
4. Request and latency panels

Expected:
1. No "No data" for active deployments/workers
2. No query errors
3. Panel values move after new deploy or traffic

---

## 5. Platform Analytics Completeness Validation

Run this functional sequence:

1. Trigger one new deployment via Swagger
2. Confirm deployment transitions queued -> running
3. Open /platform/analytics and verify totals changed
4. Open Prometheus and verify deployment_* metrics for new deployment_id
5. Open Grafana deployment dashboard for that deployment
6. Send traffic to deployment URL (browser or curl loop)
7. Verify request/latency/bandwidth metrics begin updating

Traffic generator example:

```bash
for i in {1..50}; do curl -s -o /dev/null https://<subdomain>.<domain>; done
```

If traffic telemetry is not yet wired, deployment request counters may remain low until telemetry forwarder path is active.

---

## 6. Failover and Rebalance Validation (Platform Side)

After deploying an app on a remote worker:

1. Stop remote worker agent
2. Wait beyond WORKER_HEARTBEAT_TIMEOUT
3. Verify worker becomes offline in /workers and /platform/analytics
4. Verify deployment is re-queued and re-assigned
5. Verify deployment returns to running
6. Restart worker agent
7. Verify conservative rebalance behavior (subject to cooldown and score threshold)

Expected:

1. No manual DB edits needed
2. Ownership and status remain consistent
3. Metrics continue to reflect worker and deployment state

---

## 7. Pass/Fail Criteria

System is PASS only if all are true:

1. Swagger protected and unprotected endpoints behave as expected
2. /platform/analytics and /deployments/{id}/analytics return valid payloads
3. Prometheus target control-plane is UP
4. Core platform_* and deployment_* metrics are present
5. Grafana datasource is healthy and dashboards render without query errors
6. End-to-end deploy action shows up across API, metrics, and dashboards

System is FAIL if any of these happen:

1. Prometheus target DOWN
2. Platform analytics endpoint missing fields or 5xx
3. Grafana panels all No data while metrics exist in /metrics
4. Deployment state inconsistent between API and observed cluster state

---

## 8. Troubleshooting Quick Map

1. Swagger works, analytics empty:
Check telemetry ingestion and metrics collector logs.

2. Metrics endpoint has data, Grafana empty:
Check datasource URL and dashboard query variables/time range.

3. Prometheus healthy but target DOWN:
Check control-plane process and scrape target in infra/observability/prometheus.yml.

4. Deployment analytics missing for specific IDs:
Validate deployment exists in Kubernetes and status in deployments table is accurate.
