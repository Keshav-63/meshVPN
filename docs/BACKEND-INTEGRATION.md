# Backend Integration Draft - MeshVPN

Date: 2026-04-09

## Goal

Define a clean backend integration contract for frontend and external clients, based on the current MeshVPN control-plane implementation and inspired by proven flow patterns from Veltrix.

This document separates:
- Implemented now (safe to use immediately)
- Recommended next (small, high-value additions)

---

## 1. Current Backend Surface (Implemented)

Base URL:
- Local: http://localhost:8080

Public endpoints (no user JWT):
- GET /health
- GET /metrics
- POST /api/telemetry/deployment-request
- POST /api/telemetry/deployment-request/batch
- POST /api/workers/register
- POST /api/workers/:id/heartbeat
- GET /api/workers/:id/claim-job
- POST /api/workers/:id/job-complete
- POST /api/workers/:id/job-failed

Protected endpoints (Supabase JWT required):
- GET /auth/whoami
- POST /deploy
- GET /deployments
- GET /deployments/:id
- GET /deployments/:id/build-logs
- GET /deployments/:id/app-logs
- GET /deployments/:id/app-logs/stream
- GET /deployments/:id/analytics
- GET /deployments/:id/analytics/stream
- GET /user/analytics
- GET /platform/analytics
- GET /platform/analytics/deployments
- GET /platform/workers/:id/analytics
- GET /workers

---

## 2. Authentication Integration Contract

### 2.1 Token Source

Frontend uses Supabase Auth and sends:
- Authorization: Bearer <supabase_access_token>

Backend middleware validates:
- HS256 and ES256 Supabase JWTs
- Expiry
- Provider allowlist

Allowed providers in current backend:
- github
- email

### 2.2 Important Behavior

On first valid login, backend can auto-upsert the user record from JWT claims.

Frontend should treat token refresh as primary recovery path for 401.

### 2.3 Required Env on Control-Plane

- REQUIRE_AUTH=true
- SUPABASE_URL
- SUPABASE_ANON_KEY
- SUPABASE_JWT_SECRET
- FRONTEND_URL

---

## 3. Frontend to Backend Mapping

This section mirrors the Veltrix-style app flow but mapped to MeshVPN endpoints.

### 3.1 Authenticated Session Check

Frontend action:
- Validate user session in Supabase
- Call GET /auth/whoami for backend identity consistency

Expected result:
- Stable user identity and provider context before dashboard data fetches

### 3.2 Deployments List Page

Endpoint:
- GET /deployments

Use case:
- Populate project/deployment cards
- Show status + high-level metrics

Refresh strategy:
- Poll every 10-30s for status transitions (queued/building/running/failed)

### 3.3 Deployment Details Page

Primary endpoint:
- GET /deployments/:id

Supplementary endpoints:
- GET /deployments/:id/analytics
- GET /deployments/:id/analytics/stream (SSE)
- GET /deployments/:id/build-logs
- GET /deployments/:id/build-logs/stream (SSE)
- GET /deployments/:id/app-logs
- GET /deployments/:id/app-logs/stream (SSE)

Recommended UI model:
- Initial page load from GET /deployments/:id
- SSE streams for real-time metrics and logs
- Fallback polling when SSE disconnects

### 3.4 Create Deployment Flow

Endpoint:
- POST /deploy

Typical request body:
- repo (required)
- subdomain (optional)
- package (optional)
- env, build_args (optional)
- autoscaling fields (optional)

Post-create behavior:
- Redirect to deployment details route
- Begin periodic status refresh

### 3.5 Platform Analytics Page

Endpoints:
- GET /platform/analytics
- GET /platform/analytics/deployments
- GET /platform/workers/:id/analytics

Use case:
- Admin dashboards
- Worker health and deployment distribution

---

## 4. Veltrix-Inspired Integration Patterns (Adopted)

The following patterns are already aligned with your current architecture:
- Supabase JWT as the single auth token for API calls
- API-first deployment lifecycle (create, list, details, logs, analytics)
- SSE for live deployment metrics
- Dashboard-first experience with status-driven polling

These keep implementation practical and avoid over-engineering.

---

## 5. GitHub Integration Design

Veltrix demonstrates two practical repo selection patterns. For MeshVPN, use this phased approach.

### Phase A (Use Immediately, No Backend Change)

Frontend-only GitHub repo fetch:
- User signs in with GitHub via Supabase OAuth
- Frontend reads provider token from Supabase session
- Frontend calls GitHub API directly to list repos
- Frontend sends selected repo URL to POST /deploy

Pros:
- Fastest path
- Zero backend work

Tradeoff:
- Provider token handling remains client-side

### Phase B (Recommended Next, Small Backend Add)

Introduce backend proxy endpoints for repo discovery:
- GET /integrations/github/repos
- POST /integrations/github/validate

Proposed behavior:
- Frontend sends regular Supabase JWT only
- Backend exchanges/stores provider token securely (server-side)
- Backend calls GitHub APIs and returns filtered repo metadata

Benefits:
- Better token hygiene
- Consistent rate-limit handling and error normalization
- Cleaner frontend code

---

## 6. Request and Response Contracts

### 6.1 Unified Error Shape (Recommended)

Adopt a stable error response envelope across all protected routes:

- error: machine-readable code
- message: user-friendly text
- request_id: trace id for support

Example:
- error: deployment_not_found
- message: Deployment does not exist or is not owned by this user
- request_id: req_7d3a1f

### 6.2 Pagination and Filters (Recommended)

For future scale, add optional query parameters:
- GET /deployments?status=running&cursor=...&limit=20

Keep backward compatibility by defaulting to current behavior when params are absent.

### 6.3 Endpoint Request and Response Formats

The following schemas reflect the current backend handlers.

Auth header for protected endpoints:

```http
Authorization: Bearer <supabase_access_token>
Content-Type: application/json
```

#### GET /auth/whoami

Response 200:

```json
{
	"sub": "user_123",
	"email": "user@example.com",
	"provider": "github"
}
```

#### POST /deploy

Request:

```json
{
	"repo": "https://github.com/org/app",
	"port": 3000,
	"subdomain": "myapp",
	"package": "small",
	"env": {
		"NODE_ENV": "production"
	},
	"build_args": {
		"NEXT_PUBLIC_API_URL": "https://api.example.com"
	},
	"min_replicas": 1,
	"max_replicas": 3,
	"cpu_target_utilization": 70,
	"node_selector": {
		"disktype": "ssd"
	}
}
```

Port guidance:
- Use the port exposed by your production container.
- For Vite apps built into static files and served by Nginx (like this Dockerfile), use `port: 80`.
- Do not use `5173` for production deploys; `5173` is Vite dev server port.

Response 202:

```json
{
	"message": "deployment queued",
	"deployment_id": "dep-123456",
	"status": "queued",
	"repo": "https://github.com/org/app",
	"subdomain": "myapp",
	"url": "https://myapp.keshavstack.tech",
	"port": 3000,
	"package": "small",
	"cpu_cores": 0.5,
	"memory_mb": 512,
	"scaling_mode": "horizontal",
	"min_replicas": 1,
	"max_replicas": 3,
	"cpu_target_utilization": 70,
	"memory_target_utilization": 75,
	"autoscaling_enabled": true
}
```

#### GET /deployments

Response 200:

```json
{
	"deployments": [
		{
			"deployment_id": "dep-123456",
			"subdomain": "myapp",
			"url": "https://myapp.keshavstack.tech",
			"status": "running",
			"package": "small",
			"current_pods": 2,
			"request_count_24h": 15420,
			"cpu_usage_percent": 45.2,
			"memory_usage_mb": 496,
			"last_updated": "2026-04-09T09:00:00Z",
			"started_at": "2026-04-09T06:00:00Z"
		}
	]
}
```

#### GET /deployments/:id

Response 200:

```json
{
	"deployment": {
		"deployment_id": "dep-123456",
		"repo": "https://github.com/org/app",
		"subdomain": "myapp",
		"url": "https://myapp.keshavstack.tech",
		"status": "running",
		"package": "small",
		"port": 3000,
		"scaling_mode": "horizontal",
		"min_replicas": 1,
		"max_replicas": 3,
		"cpu_cores": 0.5,
		"memory_mb": 512,
		"started_at": "2026-04-09T06:00:00Z",
		"finished_at": "0001-01-01T00:00:00Z"
	},
	"metrics": {
		"requests": {
			"total": 125000,
			"last_hour": 3600,
			"last_24h": 89000,
			"per_second": 1.0
		},
		"latency": {
			"p50_ms": 45.2,
			"p90_ms": 120.5,
			"p99_ms": 350.0
		},
		"bandwidth": {
			"sent_bytes": 524288000,
			"received_bytes": 104857600
		},
		"last_updated": "2026-04-09T09:00:00Z"
	},
	"pods": [],
	"resources": {},
	"scaling": {}
}
```

#### GET /deployments/:id/build-logs

Response 200:

```json
{
	"deployment_id": "dep-123456",
	"status": "running",
	"build_logs": "Step 1/8 : FROM node:18-alpine\\nStep 2/8 : WORKDIR /app"
}
```

#### GET /deployments/:id/build-logs/stream

Response 200 uses Server-Sent Events with incremental chunks:

```text
data: {"deployment_id":"dep-123456","status":"deploying","offset":1024,"chunk":"Step 6/12 : RUN npm run build\n ...","complete":false,"timestamp":1775725200}

event: complete
data: {"deployment_id":"dep-123456","status":"running","offset":8450,"chunk":"","complete":true,"timestamp":1775725305}
```

#### GET /deployments/:id/app-logs?tail=200

Response 200:

```json
{
	"deployment_id": "dep-123456",
	"container": "app-myapp",
	"tail": 200,
	"cursor": 0,
	"next_cursor": 1240,
	"delta": false,
	"application_logs": "[2026-04-09T09:00:00Z] server started"
}
```

Incremental polling mode:
- Send `cursor` query param from previous `next_cursor`.
- Example: `GET /deployments/:id/app-logs?tail=200&cursor=1240`
- Response contains only new `application_logs` chunk and updated `next_cursor`.

#### GET /deployments/:id/app-logs/stream

Response 200 uses Server-Sent Events with incremental chunks:

```text
data: {"deployment_id":"dep-123456","container":"app-myapp","chunk":"10.42.0.183 - - [10/Apr/2026:12:01:03 +0000] \"GET / HTTP/1.1\" 304 0 ...\n","next_cursor":1365,"complete":false,"reset_full_buffer":false,"timestamp":1775812863}
```

#### GET /deployments/:id/analytics

Response 200:

```json
{
	"deployment_id": "dep-123456",
	"deployment": {
		"repo": "https://github.com/org/app",
		"subdomain": "myapp",
		"url": "https://myapp.keshavstack.tech",
		"package": "small",
		"status": "running",
		"scaling_mode": "horizontal",
		"min_replicas": 1,
		"max_replicas": 3,
		"started_at": "2026-04-09T06:00:00Z"
	},
	"metrics": {
		"requests": {
			"total": 125000,
			"last_hour": 3600,
			"last_24h": 89000,
			"per_second": 1.0
		},
		"latency": {
			"p50_ms": 45.2,
			"p90_ms": 120.5,
			"p99_ms": 350.0
		},
		"bandwidth": {
			"sent_bytes": 524288000,
			"received_bytes": 104857600
		},
		"pods": {
			"current": 2,
			"desired": 2
		},
		"resources": {
			"cpu_usage_percent": 45.0,
			"memory_usage_mb": 496
		},
		"last_updated": "2026-04-09T09:00:00Z"
	}
}
```

#### GET /deployments/:id/analytics/stream

Response 200 uses Server-Sent Events:

```text
data: {"deployment_id":"dep-123456","timestamp":1775725200,"requests":{"total":125000,"last_hour":3600,"last_24h":89000,"per_second":1.0},"latency":{"p50_ms":45.2,"p90_ms":120.5,"p99_ms":350.0},"bandwidth":{"sent_bytes":524288000,"received_bytes":104857600},"pods":{"current":2,"desired":2},"resources":{"cpu_usage_percent":45.0,"memory_usage_mb":496}}
```

#### GET /platform/analytics

Response 200:

```json
{
	"platform": {
		"deployments": {
			"total": 25,
			"running": 20,
			"failed": 1,
			"queued": 4
		},
		"workers": {
			"total": 3,
			"idle": 1,
			"busy": 1,
			"offline": 1
		},
		"capacity": {
			"total": 24,
			"used": 10,
			"available": 14,
			"utilization_percent": 41.7
		},
		"resources": {
			"total_pods": 42
		},
		"traffic": {
			"total_requests": 985000,
			"bandwidth_sent_bytes": 987654321,
			"bandwidth_recv_bytes": 123456789,
			"avg_latency_p50_ms": 52.1
		}
	},
	"workers": []
}
```

#### GET /user/analytics

Response 200:

```json
{
	"user_id": "user_123",
	"summary": {
		"deployments_total": 4,
		"deployments_running": 3,
		"deployments_failed": 1,
		"deployments_queued": 0,
		"requests_total": 185000,
		"requests_last_hour": 4200,
		"bandwidth_sent_bytes": 845000000,
		"bandwidth_recv_bytes": 125000000,
		"pods_current": 6,
		"pods_desired": 6
	},
	"deployments": [
		{
			"deployment_id": "dep-123456",
			"subdomain": "myapp",
			"url": "https://myapp.keshavstack.tech",
			"status": "running",
			"package": "small",
			"metrics": {
				"requests_total": 125000,
				"requests_last_hour": 3600,
				"bandwidth_sent_bytes": 524288000,
				"bandwidth_recv_bytes": 104857600,
				"current_pods": 2,
				"desired_pods": 2,
				"cpu_usage_percent": 45.0,
				"memory_usage_mb": 496,
				"latency_p50_ms": 45.2,
				"latency_p90_ms": 120.5,
				"latency_p99_ms": 350.0,
				"last_updated": "2026-04-09T09:00:00Z"
			}
		}
	]
}
```

#### GET /platform/analytics/deployments

Response 200:

```json
{
	"count": 2,
	"deployments": [
		{
			"deployment_id": "dep-123456",
			"user_id": "user_123",
			"subdomain": "myapp",
			"url": "https://myapp.keshavstack.tech",
			"status": "running",
			"owner_worker_id": "worker-1",
			"package": "small",
			"requested_at": "2026-04-09T06:00:00Z",
			"metrics": {
				"requests_total": 125000,
				"requests_last_hour": 3600,
				"bandwidth_sent_bytes": 524288000,
				"bandwidth_recv_bytes": 104857600,
				"current_pods": 2,
				"desired_pods": 2,
				"cpu_usage_percent": 45.0,
				"memory_usage_mb": 496,
				"latency_p50_ms": 45.2,
				"latency_p90_ms": 120.5,
				"latency_p99_ms": 350.0,
				"last_updated": "2026-04-09T09:00:00Z"
			}
		}
	]
}
```

#### GET /platform/workers/:id/analytics

Response 200:

```json
{
	"worker": {
		"worker_id": "worker-1",
		"name": "edge-worker-1",
		"tailscale_ip": "100.64.0.10",
		"status": "idle",
		"current_jobs": 0,
		"max_concurrent_jobs": 4,
		"last_heartbeat": "2026-04-09T09:00:00Z"
	},
	"resources": {
		"total_pods": 5,
		"total_requests": 225000,
		"cpu_cores": 8,
		"memory_gb": 16
	},
	"deployments": [
		{
			"deployment_id": "dep-123456",
			"subdomain": "myapp",
			"package": "small",
			"current_pods": 2,
			"request_count": 125000,
			"cpu_percent": 45.0,
			"memory_mb": 496
		}
	]
}
```

#### GET /workers

Response 200:

```json
{
	"workers": [
		{
			"worker_id": "worker-1",
			"name": "edge-worker-1",
			"status": "idle"
		}
	]
}
```

#### POST /api/telemetry/deployment-request

Request:

```json
{
	"deployment_id": "dep-123456",
	"status_code": 200,
	"latency_ms": 42.7,
	"bytes_sent": 1024,
	"bytes_received": 256,
	"path": "/api/users",
	"method": "GET",
	"timestamp": "2026-04-09T09:00:00Z"
}
```

Response 200:

```json
{
	"status": "recorded"
}
```

#### POST /api/telemetry/deployment-request/batch

Request:

```json
{
	"requests": [
		{
			"deployment_id": "dep-123456",
			"status_code": 200,
			"latency_ms": 42.7,
			"bytes_sent": 1024,
			"bytes_received": 256,
			"path": "/",
			"method": "GET",
			"timestamp": "2026-04-09T09:00:00Z"
		}
	]
}
```

Response 200:

```json
{
	"status": "batch_processed",
	"recorded": 100,
	"failed": 0,
	"dropped": 2,
	"total": 102
}
```

---

## 7. Real-Time and Resilience Rules

### 7.1 SSE Client Rules

For /deployments/:id/analytics/stream:
- Auto-reconnect with exponential backoff
- Cap retry interval (for example 30s)
- Show stale-data indicator when stream is disconnected
- Fallback to GET /deployments/:id/analytics every 15-30s

For /deployments/:id/app-logs/stream:
- Append incoming `chunk` values in order using `next_cursor`
- If `reset_full_buffer=true`, replace local buffer with the incoming chunk
- Auto-reconnect with exponential backoff and continue with polling fallback when disconnected
- Fallback polling: GET /deployments/:id/app-logs?tail=200&cursor=<last_next_cursor>

### 7.2 Timeout and Retry

Frontend recommended defaults:
- Read timeout: 10s
- Retry safe GET requests on transient failures (429, 502, 503, 504)
- Do not blind-retry non-idempotent POST without request-level idempotency key

---

## 8. Security and Ownership Checks

Already in place:
- User identity from Supabase JWT
- User-scoped deployment access in handlers/services

Recommended hardening next:
- Worker API authentication (shared secret or mTLS)
- Per-route rate limits for public telemetry endpoints
- Optional token scope claims for admin-only platform routes

---

## 9. Implementation Checklist

### Immediate (no backend code changes)
- Standardize frontend API client around Bearer Supabase token
- Use endpoints in Section 1 as single source of truth
- Add SSE reconnect and polling fallback behavior
- Keep GitHub repo selection in frontend (Phase A)

### Next (small backend changes)
- Add GitHub proxy integration endpoints (Phase B)
- Add unified error envelope
- Add optional pagination/filters for deployments
- Add worker route authentication

---

## 10. Example Integration Flow (End-to-End)

1. User signs in using email or GitHub through Supabase.
2. Frontend stores session and calls GET /auth/whoami.
3. Dashboard loads GET /deployments.
4. User creates deployment with POST /deploy.
5. Details page loads GET /deployments/:id.
6. Live metrics start from GET /deployments/:id/analytics/stream.
7. Logs streamed via GET /deployments/:id/build-logs/stream and /app-logs/stream (fallback: /build-logs and /app-logs polling).
8. Platform/admin pages use /platform/analytics routes.

---

## 11. Notes for Current MeshVPN Stack

- Deploy payload expects repository URL and supports optional subdomain/port/package/env/build args.
- Root Dockerfile is required in target repositories for deployment builds.
- APP_BASE_DOMAIN controls generated deployment host naming.
- Telemetry ingestion now supports both single and batch request pipelines.

---

## 12. Summary

This draft gives you a backend integration path that is:
- Production-lean (works with your current routes now)
- Veltrix-inspired for UX flow quality
- Designed for incremental hardening without large rewrites

Use Phase A immediately, then implement Phase B GitHub proxy and worker-auth hardening as the next practical upgrades.
