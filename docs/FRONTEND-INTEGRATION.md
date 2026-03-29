# Frontend Integration Guide - MeshVPN Platform

Complete API reference and integration guide for building the MeshVPN frontend dashboard.

---

## Table of Contents

1. [Authentication](#authentication)
2. [API Endpoints Overview](#api-endpoints-overview)
3. [User Dashboard](#user-dashboard)
4. [Deployment Management](#deployment-management)
5. [Analytics & Monitoring](#analytics--monitoring)
6. [Real-Time Features](#real-time-features)
7. [Frontend Code Examples](#frontend-code-examples)
8. [Error Handling](#error-handling)

---

## Authentication

All protected endpoints require Supabase JWT authentication.

### Headers

```http
Authorization: Bearer <supabase_jwt_token>
Content-Type: application/json
```

### Get Current User

**GET** `/auth/whoami`

Returns the authenticated user's information.

**Response:**
```json
{
  "user_id": "auth0|abc123",
  "email": "user@example.com",
  "name": "John Doe",
  "created_at": "2026-03-15T10:00:00Z"
}
```

---

## API Endpoints Overview

### Base URL
```
http://localhost:8080
```

### Endpoint Categories

| Category | Endpoints | Authentication |
|----------|-----------|----------------|
| System | `/health`, `/metrics` | No |
| Auth | `/auth/whoami` | Yes |
| Deployments | `/deploy`, `/deployments`, `/deployments/:id` | Yes |
| Analytics | `/deployments/:id/analytics`, `/deployments/:id/analytics/stream` | Yes |
| Platform | `/platform/analytics`, `/platform/workers/:id/analytics` | Yes |
| Logs | `/deployments/:id/build-logs`, `/deployments/:id/app-logs` | Yes |
| Workers | `/workers` | Yes |
| Telemetry | `/api/telemetry/deployment-request` | No (internal) |

---

## User Dashboard

### 1. Dashboard Overview

Display user's deployments with key metrics.

**GET** `/deployments`

**Query Parameters:**
- None (returns all user's deployments)

**Response:**
```json
{
  "deployments": [
    {
      "deployment_id": "abc123-def456",
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

**UI Components:**
- **Deployment Cards**: Show subdomain, status badge, package tier
- **Quick Stats**: Request count (24h), CPU%, Memory usage
- **Status Indicators**:
  - 🟢 Running
  - 🟡 Building
  - 🔴 Failed
  - ⚪ Stopped

---

## Deployment Management

### 2. Create New Deployment

**POST** `/deploy`

**Request Body:**
```json
{
  "repo": "https://github.com/user/my-app",
  "subdomain": "myapp",
  "package": "small",
  "env": {
    "NODE_ENV": "production",
    "API_KEY": "secret123"
  },
  "build_args": {
    "NEXT_PUBLIC_API_URL": "https://api.example.com"
  }
}
```

**Response:**
```json
{
  "deployment_id": "abc123-def456",
  "subdomain": "myapp",
  "url": "https://myapp.keshavstack.tech",
  "status": "queued",
  "message": "Deployment queued for processing"
}
```

**Available Packages:**
- `nano`: 0.25 CPU, 256MB RAM
- `small`: 0.5 CPU, 512MB RAM
- `medium`: 1 CPU, 1GB RAM
- `large`: 2 CPU, 2GB RAM

**Frontend Flow:**
1. User fills deployment form
2. Submit POST request
3. Show loading state
4. Redirect to deployment details page
5. Poll `/deployments/:id` for status updates

---

### 3. Get Deployment Details (Comprehensive)

**GET** `/deployments/:id`

Returns **complete** deployment information including config, metrics, pod details, and resource allocation.

**Response:**
```json
{
  "deployment": {
    "deployment_id": "abc123-def456",
    "repo": "https://github.com/user/my-app",
    "subdomain": "myapp",
    "url": "https://myapp.keshavstack.tech",
    "status": "running",
    "package": "small",
    "scaling_mode": "horizontal",
    "cpu_cores": 0.5,
    "memory_mb": 512,
    "min_replicas": 1,
    "max_replicas": 3,
    "cpu_target_utilization": 70,
    "started_at": "2026-03-29T10:00:00Z",
    "env": {
      "NODE_ENV": "production"
    }
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
    "last_updated": "2026-03-29T12:30:00Z"
  },
  "pods": [
    {
      "pod_name": "app-abc123-7d9f8b-xk2p9",
      "status": "Running",
      "ready": true,
      "restarts": 0,
      "cpu_usage_milli": 120,
      "memory_usage_mb": 256,
      "age": "2h15m",
      "created_at": "2026-03-29T10:15:00Z"
    },
    {
      "pod_name": "app-abc123-7d9f8b-m4n8q",
      "status": "Running",
      "ready": true,
      "restarts": 0,
      "cpu_usage_milli": 105,
      "memory_usage_mb": 240,
      "age": "45m",
      "created_at": "2026-03-29T11:45:00Z"
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
    "cpu_target_utilization": 70,
    "hpa_enabled": true
  }
}
```

**UI Components:**

#### Deployment Header
- Subdomain badge
- Status indicator
- Live URL link
- Package tier badge

#### Metrics Dashboard
- **Request Graph**: Line chart showing requests over time
- **Latency Chart**: Bar chart for p50/p90/p99
- **Bandwidth**: Sent vs Received pie chart

#### Pod Status Table
| Pod Name | Status | CPU | Memory | Restarts | Age |
|----------|--------|-----|--------|----------|-----|
| app-abc123-xk2p9 | 🟢 Running | 120m | 256 MB | 0 | 2h15m |
| app-abc123-m4n8q | 🟢 Running | 105m | 240 MB | 0 | 45m |

#### Resource Allocation
- **CPU Progress Bar**: Usage vs Requested vs Limit
  ```
  [████████░░] 45% (225m / 500m requested, 1000m limit)
  ```
- **Memory Progress Bar**: Usage vs Requested vs Limit
  ```
  [████████████████████] 96.9% (496 MB / 512 MB requested, 1024 MB limit)
  ```

#### Scaling Configuration
- Mode: Horizontal / None
- Current Pods: 2/2 (desired)
- Range: 1-3 replicas
- HPA Target: 70% CPU

---

### 4. Get Build Logs

**GET** `/deployments/:id/build-logs`

**Query Parameters:**
- `follow=true` (optional): Stream logs in real-time

**Response:**
```json
{
  "deployment_id": "abc123-def456",
  "logs": "Step 1/8 : FROM node:18-alpine\nStep 2/8 : WORKDIR /app\n...",
  "status": "running"
}
```

**UI Component:**
- Terminal-style log viewer with auto-scroll
- Syntax highlighting for Docker/build commands
- Follow mode toggle

---

### 5. Get Application Logs

**GET** `/deployments/:id/app-logs`

**Query Parameters:**
- `tail=100` (optional): Number of recent lines
- `follow=true` (optional): Stream logs

**Response:**
```json
{
  "deployment_id": "abc123-def456",
  "logs": "[2026-03-29 12:30:45] Server listening on port 3000\n[2026-03-29 12:30:46] Connected to database\n...",
  "status": "running"
}
```

**UI Component:**
- Terminal-style viewer
- Search/filter functionality
- Download logs button
- Auto-refresh toggle

---

## Analytics & Monitoring

### 6. Get Deployment Analytics (Backward Compatible)

**GET** `/deployments/:id/analytics`

Returns aggregated metrics (kept for backward compatibility).

**Response:**
```json
{
  "deployment_id": "abc123-def456",
  "deployment": {
    "repo": "https://github.com/user/my-app",
    "subdomain": "myapp",
    "url": "https://myapp.keshavstack.tech",
    "package": "small",
    "status": "running",
    "scaling_mode": "horizontal",
    "min_replicas": 1,
    "max_replicas": 3,
    "started_at": "2026-03-29T10:00:00Z"
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
    "last_updated": "2026-03-29T12:30:00Z"
  }
}
```

---

### 7. Platform Analytics

**GET** `/platform/analytics`

System-wide metrics for admin/monitoring dashboard.

**Response:**
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
  },
  "pods": {
    "total": 42,
    "running": 40,
    "pending": 1,
    "failed": 1
  },
  "last_updated": "2026-03-29T12:30:00Z"
}
```

**UI Component:**
- Platform overview cards
- Resource utilization gauges
- Worker status grid
- Deployment status pie chart

---

## Real-Time Features

### 8. Server-Sent Events (SSE) - Live Metrics

**GET** `/deployments/:id/analytics/stream`

Streams real-time metrics updates every 5 seconds.

**Response Format:**
```
event: message
data: {"deployment_id":"abc123","timestamp":1711713012,"requests":{"total":125000,"last_hour":3600,"last_24h":89000,"per_second":1.0},"latency":{"p50_ms":45.2,"p90_ms":120.5,"p99_ms":350.0},"bandwidth":{"sent_bytes":524288000,"received_bytes":104857600},"pods":{"current":2,"desired":2},"resources":{"cpu_usage_percent":45.0,"memory_usage_mb":496}}

event: message
data: {"deployment_id":"abc123","timestamp":1711713017,"requests":{"total":125005,"last_hour":3605,"last_24h":89005,"per_second":1.01},"latency":{"p50_ms":45.3,"p90_ms":120.4,"p99_ms":349.8},"bandwidth":{"sent_bytes":524289024,"received_bytes":104858112},"pods":{"current":2,"desired":2},"resources":{"cpu_usage_percent":45.1,"memory_usage_mb":497}}
```

**Error Events:**
```
event: error
data: {"error": "failed to fetch metrics"}
```

---

## Frontend Code Examples

### React: Fetch Deployments List

```typescript
import { useState, useEffect } from 'react';
import { useSupabase } from '@/hooks/useSupabase';

interface Deployment {
  deployment_id: string;
  subdomain: string;
  url: string;
  status: string;
  package: string;
  current_pods: number;
  request_count_24h: number;
  cpu_usage_percent: number;
  memory_usage_mb: number;
  last_updated: string;
  started_at: string;
}

function DeploymentsList() {
  const { session } = useSupabase();
  const [deployments, setDeployments] = useState<Deployment[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function fetchDeployments() {
      try {
        const response = await fetch('http://localhost:8080/deployments', {
          headers: {
            'Authorization': `Bearer ${session.access_token}`,
            'Content-Type': 'application/json',
          },
        });

        if (!response.ok) {
          throw new Error(`HTTP error! status: ${response.status}`);
        }

        const data = await response.json();
        setDeployments(data.deployments || []);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch deployments');
      } finally {
        setLoading(false);
      }
    }

    if (session) {
      fetchDeployments();
    }
  }, [session]);

  if (loading) return <div>Loading deployments...</div>;
  if (error) return <div>Error: {error}</div>;

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
      {deployments.map((deployment) => (
        <DeploymentCard key={deployment.deployment_id} deployment={deployment} />
      ))}
    </div>
  );
}
```

### React: Deployment Details with Real-Time Updates

```typescript
import { useState, useEffect } from 'react';
import { useParams } from 'react-router-dom';
import { useSupabase } from '@/hooks/useSupabase';

interface DeploymentDetails {
  deployment: {
    deployment_id: string;
    subdomain: string;
    url: string;
    status: string;
    package: string;
    // ... other fields
  };
  metrics: {
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
    last_updated: string;
  };
  pods: Array<{
    pod_name: string;
    status: string;
    ready: boolean;
    restarts: number;
    cpu_usage_milli: number;
    memory_usage_mb: number;
    age: string;
  }>;
  resources: {
    cpu_requested_milli: number;
    cpu_limit_milli: number;
    cpu_usage_milli: number;
    cpu_usage_percent: number;
    memory_requested_mb: number;
    memory_limit_mb: number;
    memory_usage_mb: number;
    memory_usage_percent: number;
  };
  scaling: {
    mode: string;
    current_pods: number;
    desired_pods: number;
    min_replicas: number;
    max_replicas: number;
    cpu_target_utilization: number;
    hpa_enabled: boolean;
  };
}

function DeploymentDetailsPage() {
  const { id } = useParams<{ id: string }>();
  const { session } = useSupabase();
  const [details, setDetails] = useState<DeploymentDetails | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function fetchDetails() {
      try {
        const response = await fetch(`http://localhost:8080/deployments/${id}`, {
          headers: {
            'Authorization': `Bearer ${session.access_token}`,
          },
        });

        const data = await response.json();
        setDetails(data);
      } catch (err) {
        console.error('Failed to fetch deployment details:', err);
      } finally {
        setLoading(false);
      }
    }

    if (session && id) {
      fetchDetails();
      // Poll every 10 seconds for updates
      const interval = setInterval(fetchDetails, 10000);
      return () => clearInterval(interval);
    }
  }, [session, id]);

  if (loading) return <div>Loading...</div>;
  if (!details) return <div>Deployment not found</div>;

  return (
    <div className="space-y-6">
      {/* Header */}
      <DeploymentHeader deployment={details.deployment} />

      {/* Metrics Cards */}
      <MetricsGrid metrics={details.metrics} />

      {/* Pod Status Table */}
      <PodStatusTable pods={details.pods} />

      {/* Resource Allocation */}
      <ResourceAllocation resources={details.resources} />

      {/* Scaling Configuration */}
      <ScalingConfig scaling={details.scaling} />
    </div>
  );
}
```

### React: Real-Time SSE Metrics Stream

```typescript
import { useState, useEffect, useRef } from 'react';
import { useSupabase } from '@/hooks/useSupabase';

interface LiveMetrics {
  deployment_id: string;
  timestamp: number;
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
}

function LiveMetricsChart({ deploymentId }: { deploymentId: string }) {
  const { session } = useSupabase();
  const [metrics, setMetrics] = useState<LiveMetrics | null>(null);
  const [isConnected, setIsConnected] = useState(false);
  const eventSourceRef = useRef<EventSource | null>(null);

  useEffect(() => {
    if (!session || !deploymentId) return;

    // Create EventSource with auth token in URL (SSE doesn't support custom headers)
    const url = `http://localhost:8080/deployments/${deploymentId}/analytics/stream?token=${session.access_token}`;
    const eventSource = new EventSource(url);

    eventSource.onopen = () => {
      console.log('SSE connection opened');
      setIsConnected(true);
    };

    eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        setMetrics(data);
      } catch (err) {
        console.error('Failed to parse SSE data:', err);
      }
    };

    eventSource.onerror = (error) => {
      console.error('SSE error:', error);
      setIsConnected(false);
      eventSource.close();
    };

    eventSourceRef.current = eventSource;

    return () => {
      eventSource.close();
      setIsConnected(false);
    };
  }, [session, deploymentId]);

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <div className={`w-2 h-2 rounded-full ${isConnected ? 'bg-green-500' : 'bg-red-500'}`} />
        <span className="text-sm text-gray-600">
          {isConnected ? 'Live' : 'Disconnected'}
        </span>
      </div>

      {metrics && (
        <div className="grid grid-cols-2 gap-4">
          <MetricCard
            title="Requests/sec"
            value={metrics.requests.per_second.toFixed(2)}
          />
          <MetricCard
            title="P50 Latency"
            value={`${metrics.latency.p50_ms.toFixed(1)}ms`}
          />
          <MetricCard
            title="CPU Usage"
            value={`${metrics.resources.cpu_usage_percent.toFixed(1)}%`}
          />
          <MetricCard
            title="Memory Usage"
            value={`${metrics.resources.memory_usage_mb.toFixed(0)} MB`}
          />
        </div>
      )}
    </div>
  );
}
```

### React: Create Deployment Form

```typescript
import { useState } from 'react';
import { useSupabase } from '@/hooks/useSupabase';
import { useNavigate } from 'react-router-dom';

interface DeploymentFormData {
  repo: string;
  subdomain: string;
  package: 'nano' | 'small' | 'medium' | 'large';
  env: Record<string, string>;
  build_args: Record<string, string>;
}

function CreateDeploymentForm() {
  const { session } = useSupabase();
  const navigate = useNavigate();
  const [formData, setFormData] = useState<DeploymentFormData>({
    repo: '',
    subdomain: '',
    package: 'small',
    env: {},
    build_args: {},
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);

    try {
      const response = await fetch('http://localhost:8080/deploy', {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${session.access_token}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(formData),
      });

      if (!response.ok) {
        const errorData = await response.json();
        throw new Error(errorData.error || 'Deployment failed');
      }

      const result = await response.json();

      // Redirect to deployment details page
      navigate(`/deployments/${result.deployment_id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create deployment');
    } finally {
      setLoading(false);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div>
        <label htmlFor="repo" className="block text-sm font-medium">
          Git Repository URL
        </label>
        <input
          id="repo"
          type="url"
          required
          value={formData.repo}
          onChange={(e) => setFormData({ ...formData, repo: e.target.value })}
          className="mt-1 block w-full rounded-md border-gray-300"
          placeholder="https://github.com/username/repo"
        />
      </div>

      <div>
        <label htmlFor="subdomain" className="block text-sm font-medium">
          Subdomain
        </label>
        <div className="mt-1 flex">
          <input
            id="subdomain"
            type="text"
            required
            value={formData.subdomain}
            onChange={(e) => setFormData({ ...formData, subdomain: e.target.value })}
            className="block w-full rounded-l-md border-gray-300"
            placeholder="myapp"
          />
          <span className="inline-flex items-center px-3 rounded-r-md border border-l-0 border-gray-300 bg-gray-50 text-gray-500">
            .keshavstack.tech
          </span>
        </div>
      </div>

      <div>
        <label htmlFor="package" className="block text-sm font-medium">
          Package Size
        </label>
        <select
          id="package"
          value={formData.package}
          onChange={(e) => setFormData({ ...formData, package: e.target.value as any })}
          className="mt-1 block w-full rounded-md border-gray-300"
        >
          <option value="nano">Nano (0.25 CPU, 256MB RAM)</option>
          <option value="small">Small (0.5 CPU, 512MB RAM)</option>
          <option value="medium">Medium (1 CPU, 1GB RAM)</option>
          <option value="large">Large (2 CPU, 2GB RAM)</option>
        </select>
      </div>

      {error && (
        <div className="text-red-600 text-sm">{error}</div>
      )}

      <button
        type="submit"
        disabled={loading}
        className="w-full bg-blue-600 text-white py-2 px-4 rounded-md hover:bg-blue-700 disabled:opacity-50"
      >
        {loading ? 'Creating Deployment...' : 'Deploy'}
      </button>
    </form>
  );
}
```

---

## Error Handling

### Common Error Responses

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

### HTTP Status Codes

| Status | Meaning | Action |
|--------|---------|--------|
| 200 | Success | Process response |
| 201 | Created | Resource created successfully |
| 400 | Bad Request | Check request payload |
| 401 | Unauthorized | Refresh auth token |
| 403 | Forbidden | User doesn't own this resource |
| 404 | Not Found | Resource doesn't exist |
| 500 | Server Error | Retry or contact support |

### Frontend Error Handling Pattern

```typescript
async function apiRequest(url: string, options: RequestInit) {
  try {
    const response = await fetch(url, options);

    if (!response.ok) {
      if (response.status === 401) {
        // Redirect to login
        window.location.href = '/login';
        return;
      }

      const error = await response.json();
      throw new Error(error.error || `HTTP ${response.status}`);
    }

    return await response.json();
  } catch (error) {
    console.error('API request failed:', error);
    throw error;
  }
}
```

---

## Data Refresh Strategies

### Polling (Simple)
```typescript
useEffect(() => {
  const interval = setInterval(fetchData, 10000); // Every 10 seconds
  return () => clearInterval(interval);
}, []);
```

### SSE (Real-Time)
```typescript
const eventSource = new EventSource(url);
eventSource.onmessage = (event) => {
  const data = JSON.parse(event.data);
  updateState(data);
};
```

### Smart Polling (Conditional)
```typescript
// Only poll when deployment is building/deploying
useEffect(() => {
  if (status === 'building' || status === 'deploying') {
    const interval = setInterval(fetchData, 5000);
    return () => clearInterval(interval);
  }
}, [status]);
```

---

## UI/UX Best Practices

### 1. Loading States
- Show skeleton loaders during initial fetch
- Use spinners for actions (deploy, delete)
- Disable buttons during API calls

### 2. Status Indicators
- Color-coded badges (green=running, yellow=building, red=failed)
- Real-time status updates
- Progress bars for long operations

### 3. Data Visualization
- Line charts for request trends
- Bar charts for latency percentiles
- Pie charts for bandwidth distribution
- Gauge charts for resource utilization

### 4. Error Feedback
- Toast notifications for errors
- Inline validation for forms
- Retry buttons for failed requests

### 5. Performance
- Virtualize long lists (pod logs, deployments)
- Debounce search inputs
- Lazy load heavy components
- Cache API responses (React Query, SWR)

---

## Summary

### Key Endpoints for Frontend

| Feature | Endpoint | Method | Use Case |
|---------|----------|--------|----------|
| Dashboard | `/deployments` | GET | List all user deployments |
| Details | `/deployments/:id` | GET | Complete deployment info |
| Deploy | `/deploy` | POST | Create new deployment |
| Build Logs | `/deployments/:id/build-logs` | GET | View build progress |
| App Logs | `/deployments/:id/app-logs` | GET | Debug application |
| Analytics | `/deployments/:id/analytics` | GET | Historical metrics |
| Live Metrics | `/deployments/:id/analytics/stream` | GET (SSE) | Real-time updates |
| Platform Stats | `/platform/analytics` | GET | Admin dashboard |

### Next Steps

1. **Set up Supabase authentication** in your frontend
2. **Implement deployment list page** using `/deployments`
3. **Build deployment details page** using `/deployments/:id`
4. **Add real-time metrics** using SSE streams
5. **Create deployment form** using `/deploy`
6. **Add log viewers** for build and app logs

---

**Happy coding!** 🚀
