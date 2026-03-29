# API Quick Reference

Fast lookup for all MeshVPN API endpoints.

---

## Base URL

```
http://localhost:8080
```

---

## Authentication

All protected endpoints require:
```http
Authorization: Bearer <supabase_jwt_token>
```

---

## Endpoints

### System

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/health` | GET | No | Health check |
| `/metrics` | GET | No | Prometheus metrics |

### Authentication

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/auth/whoami` | GET | Yes | Get current user info |

### Deployments

| Endpoint | Method | Auth | Description | Response |
|----------|--------|------|-------------|----------|
| `/deploy` | POST | Yes | Create deployment | `{deployment_id, subdomain, url, status}` |
| `/deployments` | GET | Yes | List all user deployments | `{deployments: [{id, subdomain, url, status, metrics}]}` |
| `/deployments/:id` | GET | Yes | Get complete deployment details | `{deployment, metrics, pods, resources, scaling}` |
| `/deployments/:id/build-logs` | GET | Yes | Get build logs | `{deployment_id, logs, status}` |
| `/deployments/:id/app-logs` | GET | Yes | Get application logs | `{deployment_id, logs, status}` |

### Analytics

| Endpoint | Method | Auth | Description | Response |
|----------|--------|------|-------------|----------|
| `/deployments/:id/analytics` | GET | Yes | Get deployment metrics | `{deployment_id, deployment, metrics}` |
| `/deployments/:id/analytics/stream` | GET (SSE) | Yes | Real-time metrics stream | Server-Sent Events |

### Platform (Admin)

| Endpoint | Method | Auth | Description | Response |
|----------|--------|------|-------------|----------|
| `/platform/analytics` | GET | Yes | Platform-wide metrics | `{workers, deployments, resources, pods}` |
| `/platform/workers/:id/analytics` | GET | Yes | Worker-specific metrics | `{worker_id, metrics, jobs}` |

### Workers (Internal)

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/workers` | GET | Yes | List all workers |
| `/api/workers/register` | POST | No | Register new worker |
| `/api/workers/:id/heartbeat` | POST | No | Worker heartbeat |
| `/api/workers/:id/claim-job` | GET | No | Claim pending job |
| `/api/workers/:id/job-complete` | POST | No | Mark job complete |
| `/api/workers/:id/job-failed` | POST | No | Mark job failed |

### Telemetry (Internal)

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/api/telemetry/deployment-request` | POST | No | Record request metrics |
| `/api/telemetry/deployment-request/batch` | POST | No | Batch record requests |

---

## Common Request Bodies

### Create Deployment

```json
POST /deploy

{
  "repo": "https://github.com/user/repo",
  "subdomain": "myapp",
  "package": "small",
  "env": {
    "NODE_ENV": "production",
    "API_KEY": "secret"
  },
  "build_args": {
    "NEXT_PUBLIC_API_URL": "https://api.example.com"
  }
}
```

**Packages**: `nano`, `small`, `medium`, `large`

---

## Common Response Structures

### Deployment List

```json
{
  "deployments": [
    {
      "deployment_id": "abc123",
      "subdomain": "myapp",
      "url": "https://myapp.keshavstack.tech",
      "status": "running",
      "package": "small",
      "current_pods": 2,
      "request_count_24h": 15420,
      "cpu_usage_percent": 45.2,
      "memory_usage_mb": 496,
      "last_updated": "2026-03-29T12:30:00Z",
      "started_at": "2026-03-29T10:00:00Z"
    }
  ]
}
```

### Deployment Details

```json
{
  "deployment": {
    "deployment_id": "abc123",
    "subdomain": "myapp",
    "url": "https://myapp.keshavstack.tech",
    "status": "running",
    "package": "small",
    ...
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
    }
  },
  "pods": [
    {
      "pod_name": "app-abc123-xk2p9",
      "status": "Running",
      "ready": true,
      "cpu_usage_milli": 120,
      "memory_usage_mb": 256,
      "age": "2h15m"
    }
  ],
  "resources": {
    "cpu_requested_milli": 500,
    "cpu_limit_milli": 1000,
    "cpu_usage_milli": 225,
    "cpu_usage_percent": 45.0,
    "memory_requested_mb": 512,
    "memory_limit_mb": 1024,
    "memory_usage_mb": 496,
    "memory_usage_percent": 96.9
  },
  "scaling": {
    "mode": "horizontal",
    "current_pods": 2,
    "desired_pods": 2,
    "min_replicas": 1,
    "max_replicas": 3,
    "hpa_enabled": true
  }
}
```

### Platform Analytics

```json
{
  "workers": {
    "total": 3,
    "online": 2,
    "offline": 1,
    "busy": 1,
    "idle": 1
  },
  "deployments": {
    "total": 25,
    "running": 20,
    "building": 2,
    "failed": 1,
    "stopped": 2
  },
  "resources": {
    "total_cpu_cores": 12,
    "total_memory_gb": 24,
    "used_cpu_cores": 8.5,
    "used_memory_gb": 16.2,
    "cpu_utilization_percent": 70.8,
    "memory_utilization_percent": 67.5
  }
}
```

---

## Status Codes

| Code | Meaning | Action |
|------|---------|--------|
| 200 | OK | Success |
| 201 | Created | Resource created |
| 400 | Bad Request | Check request body |
| 401 | Unauthorized | Refresh token |
| 403 | Forbidden | No access |
| 404 | Not Found | Resource doesn't exist |
| 500 | Server Error | Retry |

---

## Query Parameters

### Logs Endpoints

- `tail=100` - Number of recent lines
- `follow=true` - Stream logs in real-time

---

## Server-Sent Events (SSE)

### `/deployments/:id/analytics/stream`

Real-time metrics stream (updates every 5 seconds).

**EventSource Example:**
```typescript
const eventSource = new EventSource(
  `http://localhost:8080/deployments/${id}/analytics/stream?token=${token}`
);

eventSource.onmessage = (event) => {
  const metrics = JSON.parse(event.data);
  console.log(metrics);
};
```

---

## Error Responses

```json
{
  "error": "deployment not found"
}
```

```json
{
  "error": "unauthorized"
}
```

```json
{
  "error": "invalid payload"
}
```

---

## Frontend Integration

See [FRONTEND-INTEGRATION.md](./FRONTEND-INTEGRATION.md) for:
- Complete code examples
- UI component patterns
- Real-time features
- Error handling
- Data visualization

---

## Traffic Tracking

Traffic metrics are automatically collected via:
1. Traefik access logs (JSON format)
2. Traffic forwarder (tails logs, sends to control-plane)
3. Control-plane telemetry endpoint
4. Metrics collector (aggregates every minute)

**Telemetry Flow:**
```
User Request → Traefik → Access Log → Traffic Forwarder →
Control-Plane (/api/telemetry/deployment-request) → Database →
Metrics Collector → Aggregated Metrics → Analytics API
```

---

## Testing Endpoints

```bash
# Health check
curl http://localhost:8080/health

# Get deployments (requires auth)
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/deployments

# Get deployment details
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/deployments/abc123

# Stream live metrics
curl -H "Authorization: Bearer $TOKEN" \
  -N http://localhost:8080/deployments/abc123/analytics/stream
```

---

**For complete documentation, see:**
- [FRONTEND-INTEGRATION.md](./FRONTEND-INTEGRATION.md) - Frontend integration guide
- [DEPLOYMENT-ANALYTICS-API.md](./DEPLOYMENT-ANALYTICS-API.md) - Analytics API details
