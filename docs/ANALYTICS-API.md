# Analytics API Documentation

This document describes the analytics endpoints available for monitoring deployment metrics in real-time.

## Table of Contents

- [Overview](#overview)
- [Authentication](#authentication)
- [Endpoints](#endpoints)
  - [GET /deployments/:id/analytics](#get-deploymentsidanalytics)
  - [GET /deployments/:id/analytics/stream](#get-deploymentsidanalyticsstream)
- [Response Schema](#response-schema)
- [Frontend Integration](#frontend-integration)
- [Metrics Explained](#metrics-explained)

---

## Overview

The Analytics API provides two endpoints for accessing deployment metrics:

1. **Snapshot endpoint**: Returns current metrics at the time of request
2. **Streaming endpoint**: Real-time metrics via Server-Sent Events (SSE), updated every 5 seconds

Metrics are collected every minute by a background worker and include:
- Request counts (total, last hour, last 24 hours)
- Latency percentiles (p50, p90, p99)
- Bandwidth usage (sent/received)
- Pod status (current/desired replicas)
- Resource usage (CPU/Memory)

---

## Authentication

All analytics endpoints require authentication via Supabase JWT token.

Include the token in the `Authorization` header:

```
Authorization: Bearer <your-jwt-token>
```

Users can only access analytics for their own deployments. Attempting to access another user's deployment will return a `403 Forbidden` error.

---

## Endpoints

### GET /deployments/:id/analytics

Returns a snapshot of current metrics for the specified deployment.

#### Parameters

- `:id` (path parameter) - The deployment ID

#### Response

**Status: 200 OK**

```json
{
  "deployment_id": "74b295d2",
  "deployment": {
    "repo": "https://github.com/user/my-app",
    "subdomain": "my-app",
    "url": "https://my-app.keshavstack.tech",
    "package": "medium",
    "status": "running",
    "scaling_mode": "horizontal",
    "min_replicas": 1,
    "max_replicas": 5,
    "started_at": "2026-03-20T10:30:00Z"
  },
  "metrics": {
    "requests": {
      "total": 15234,
      "last_hour": 450,
      "last_24h": 12400,
      "per_second": 0.125
    },
    "latency": {
      "p50_ms": 125.4,
      "p90_ms": 280.6,
      "p99_ms": 520.3
    },
    "bandwidth": {
      "sent_bytes": 52428800,
      "received_bytes": 10485760
    },
    "pods": {
      "current": 3,
      "desired": 3
    },
    "resources": {
      "cpu_usage_percent": 45.2,
      "memory_usage_mb": 384.5
    },
    "last_updated": "2026-03-21T14:23:00Z"
  }
}
```

#### Error Responses

**404 Not Found** - Deployment does not exist
```json
{
  "error": "deployment not found"
}
```

**403 Forbidden** - User does not own this deployment
```json
{
  "error": "forbidden"
}
```

**500 Internal Server Error** - Failed to load metrics
```json
{
  "error": "failed to load metrics"
}
```

---

### GET /deployments/:id/analytics/stream

Streams real-time metrics via Server-Sent Events (SSE). Metrics are sent every 5 seconds.

#### Parameters

- `:id` (path parameter) - The deployment ID

#### Response

**Status: 200 OK**

**Content-Type**: `text/event-stream`

The server sends periodic updates in SSE format:

```
data: {"deployment_id":"74b295d2","timestamp":1711028580,"requests":{"total":15234,"last_hour":450,"last_24h":12400,"per_second":0.125},"latency":{"p50_ms":125.4,"p90_ms":280.6,"p99_ms":520.3},"bandwidth":{"sent_bytes":52428800,"received_bytes":10485760},"pods":{"current":3,"desired":3},"resources":{"cpu_usage_percent":45.2,"memory_usage_mb":384.5}}

data: {"deployment_id":"74b295d2","timestamp":1711028585,"requests":{"total":15237,"last_hour":452,"last_24h":12403,"per_second":0.126},"latency":{"p50_ms":126.1,"p90_ms":281.2,"p99_ms":518.7},"bandwidth":{"sent_bytes":52430080,"received_bytes":10486272},"pods":{"current":3,"desired":3},"resources":{"cpu_usage_percent":46.1,"memory_usage_mb":385.2}}
```

#### Error Events

If metrics fetching fails, an error event is sent:

```
event: error
data: {"error": "failed to fetch metrics"}
```

#### Connection Management

- The connection remains open until the client closes it or the server stops
- Metrics are sent every 5 seconds
- The stream automatically closes when the client disconnects
- Reconnection is the client's responsibility

---

## Response Schema

### DeploymentMetrics

| Field | Type | Description |
|-------|------|-------------|
| `deployment_id` | string | Unique deployment identifier |
| `timestamp` | int64 | Unix timestamp of metrics snapshot (SSE only) |
| `requests.total` | int64 | Total requests since deployment started |
| `requests.last_hour` | int64 | Requests in the last hour |
| `requests.last_24h` | int64 | Requests in the last 24 hours |
| `requests.per_second` | float64 | Current request rate (based on last hour) |
| `latency.p50_ms` | float64 | 50th percentile latency in milliseconds |
| `latency.p90_ms` | float64 | 90th percentile latency in milliseconds |
| `latency.p99_ms` | float64 | 99th percentile latency in milliseconds |
| `bandwidth.sent_bytes` | int64 | Total bytes sent since deployment started |
| `bandwidth.received_bytes` | int64 | Total bytes received since deployment started |
| `pods.current` | int | Currently running pods |
| `pods.desired` | int | Desired number of pods |
| `resources.cpu_usage_percent` | float64 | Current CPU usage percentage |
| `resources.memory_usage_mb` | float64 | Current memory usage in MB |

---

## Frontend Integration

### Fetching Snapshot Metrics (React Example)

```typescript
import { useEffect, useState } from 'react';

interface Metrics {
  requests: {
    total: number;
    last_hour: number;
    last_24h: number;
    per_second: number;
  };
  latency: {
    p50_ms: number;
    p90_ms: number;
    p99_ms: number;
  };
  bandwidth: {
    sent_bytes: number;
    received_bytes: number;
  };
  pods: {
    current: number;
    desired: number;
  };
  resources: {
    cpu_usage_percent: number;
    memory_usage_mb: number;
  };
  last_updated: string;
}

function DeploymentAnalytics({ deploymentId, token }: { deploymentId: string; token: string }) {
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function fetchMetrics() {
      try {
        const response = await fetch(`/deployments/${deploymentId}/analytics`, {
          headers: {
            'Authorization': `Bearer ${token}`,
          },
        });

        if (!response.ok) {
          throw new Error('Failed to fetch metrics');
        }

        const data = await response.json();
        setMetrics(data.metrics);
        setLoading(false);
      } catch (err) {
        setError(err.message);
        setLoading(false);
      }
    }

    fetchMetrics();
  }, [deploymentId, token]);

  if (loading) return <div>Loading metrics...</div>;
  if (error) return <div>Error: {error}</div>;
  if (!metrics) return null;

  return (
    <div>
      <h2>Analytics</h2>
      <div>
        <h3>Requests</h3>
        <p>Total: {metrics.requests.total}</p>
        <p>Last Hour: {metrics.requests.last_hour}</p>
        <p>Rate: {metrics.requests.per_second.toFixed(2)} req/s</p>
      </div>
      <div>
        <h3>Latency</h3>
        <p>p50: {metrics.latency.p50_ms.toFixed(1)} ms</p>
        <p>p90: {metrics.latency.p90_ms.toFixed(1)} ms</p>
        <p>p99: {metrics.latency.p99_ms.toFixed(1)} ms</p>
      </div>
      <div>
        <h3>Pods</h3>
        <p>Current: {metrics.pods.current} / Desired: {metrics.pods.desired}</p>
      </div>
    </div>
  );
}
```

### Real-time Streaming with SSE (React Example)

```typescript
import { useEffect, useState } from 'react';

interface StreamMetrics {
  deployment_id: string;
  timestamp: number;
  requests: {
    total: number;
    last_hour: number;
    per_second: number;
  };
  latency: {
    p50_ms: number;
    p90_ms: number;
    p99_ms: number;
  };
  pods: {
    current: number;
    desired: number;
  };
}

function RealtimeAnalytics({ deploymentId, token }: { deploymentId: string; token: string }) {
  const [metrics, setMetrics] = useState<StreamMetrics | null>(null);
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const url = `/deployments/${deploymentId}/analytics/stream`;
    const eventSource = new EventSource(url, {
      headers: {
        'Authorization': `Bearer ${token}`,
      },
    } as any);

    eventSource.onopen = () => {
      console.log('SSE connection opened');
      setConnected(true);
      setError(null);
    };

    eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        setMetrics(data);
      } catch (err) {
        console.error('Failed to parse SSE data:', err);
      }
    };

    eventSource.addEventListener('error', (event: any) => {
      try {
        const data = JSON.parse(event.data);
        setError(data.error);
      } catch {
        // Connection error
        setError('Connection lost');
        setConnected(false);
      }
    });

    eventSource.onerror = (err) => {
      console.error('SSE error:', err);
      setConnected(false);
      setError('Connection error');
    };

    // Cleanup on unmount
    return () => {
      eventSource.close();
      console.log('SSE connection closed');
    };
  }, [deploymentId, token]);

  return (
    <div>
      <div>
        Status: {connected ? '🟢 Connected' : '🔴 Disconnected'}
        {error && <span style={{ color: 'red' }}> - {error}</span>}
      </div>

      {metrics && (
        <div>
          <h2>Real-time Analytics</h2>
          <p>Last Updated: {new Date(metrics.timestamp * 1000).toLocaleTimeString()}</p>

          <div>
            <h3>Requests</h3>
            <p>Total: {metrics.requests.total}</p>
            <p>Last Hour: {metrics.requests.last_hour}</p>
            <p>Rate: {metrics.requests.per_second.toFixed(2)} req/s</p>
          </div>

          <div>
            <h3>Latency</h3>
            <p>p50: {metrics.latency.p50_ms.toFixed(1)} ms</p>
            <p>p90: {metrics.latency.p90_ms.toFixed(1)} ms</p>
            <p>p99: {metrics.latency.p99_ms.toFixed(1)} ms</p>
          </div>

          <div>
            <h3>Pods</h3>
            <p>Current: {metrics.pods.current} / Desired: {metrics.pods.desired}</p>
          </div>
        </div>
      )}
    </div>
  );
}
```

### Vanilla JavaScript SSE Example

```javascript
const deploymentId = '74b295d2';
const token = 'your-jwt-token';

const eventSource = new EventSource(
  `/deployments/${deploymentId}/analytics/stream`,
  {
    headers: {
      'Authorization': `Bearer ${token}`
    }
  }
);

eventSource.onmessage = (event) => {
  const metrics = JSON.parse(event.data);
  console.log('Received metrics:', metrics);

  // Update your UI
  document.getElementById('requests-total').textContent = metrics.requests.total;
  document.getElementById('latency-p50').textContent = metrics.latency.p50_ms.toFixed(1) + ' ms';
  document.getElementById('pods-current').textContent = metrics.pods.current;
};

eventSource.addEventListener('error', (event) => {
  const error = JSON.parse(event.data);
  console.error('SSE error:', error);
});

eventSource.onerror = (error) => {
  console.error('Connection error:', error);
  eventSource.close();
};

// Close connection when done
// eventSource.close();
```

---

## Metrics Explained

### Request Metrics

- **Total**: Cumulative count since deployment started
- **Last Hour**: Rolling count of requests in the past 60 minutes
- **Last 24h**: Rolling count of requests in the past 24 hours
- **Per Second**: Calculated as `last_hour / 3600` - average requests per second based on the last hour

### Latency Percentiles

Latency is measured from when the request hits the ingress to when the response is sent.

- **p50 (median)**: 50% of requests complete faster than this
- **p90**: 90% of requests complete faster than this
- **p99**: 99% of requests complete faster than this

Higher percentiles (p90, p99) help identify outliers and worst-case latency.

### Bandwidth

- **Sent Bytes**: Total bytes sent in HTTP responses (response body)
- **Received Bytes**: Total bytes received in HTTP requests (request body)

This does not include HTTP headers, only payload data.

### Pods

- **Current**: Number of pods currently running and ready
- **Desired**: Number of pods the deployment should have
  - For non-subscribers: Fixed at 1 replica
  - For subscribers: Managed by Horizontal Pod Autoscaler (HPA) based on CPU usage

When `current < desired`, pods are starting up or experiencing issues.

### Resource Usage

- **CPU Usage**: Percentage of allocated CPU cores being used
- **Memory Usage**: Memory consumption in MB

**Note**: Resource metrics require Kubernetes metrics-server. If not available, these fields will be `0`.

---

## Data Retention

- **Request logs**: Kept for 7 days
- **Aggregated metrics**: Retained as long as the deployment exists
- **Real-time data**: Updated every 60 seconds by background collector

---

## Rate Limiting

Currently, there are no rate limits on analytics endpoints. SSE connections are limited to one per deployment per user.

---

## Troubleshooting

### "deployment not found"

- Verify the deployment ID is correct
- Check that the deployment exists and is in "running" status

### "forbidden"

- Ensure you're using the correct authentication token
- Verify you own this deployment (check UserID matches)

### SSE Connection Drops

- Check network connectivity
- Implement reconnection logic in your client
- Monitor browser console for connection errors

### Metrics Show Zero

- Metrics are collected every minute - wait up to 60 seconds for first update
- Ensure deployment is receiving traffic
- Check that Kubernetes metrics-server is installed (for resource metrics)

### Percentiles Are Empty

- Percentiles require at least one request in the measurement window (last hour)
- Generate some traffic to your deployment

---

## Related Documentation

- [Package Documentation](./PACKAGES.md) - Resource package specifications
- [Grafana Setup](./GRAFANA-SETUP.md) - Platform-wide analytics dashboards
- [Setup Guide](./SETUP.md) - Initial platform setup

---

## Support

For issues or questions, please open an issue at: https://github.com/anthropics/claude-code/issues
