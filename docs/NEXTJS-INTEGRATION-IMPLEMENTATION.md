# Next.js Frontend Integration Implementation

This guide provides the complete code needed to integrate your Next.js frontend with the MeshVPN backend. Copy these files into your existing Next.js project.

## 1. Environment Setup

Update your `.env.local` to include the API base URL and Supabase credentials.

```env
# .env.local
NEXT_PUBLIC_API_URL=http://localhost:8080 # Or https://self.keshavstack.tech in production
NEXT_PUBLIC_SUPABASE_URL=your_supabase_url
NEXT_PUBLIC_SUPABASE_ANON_KEY=your_supabase_anon_key
```

## 2. API Client with Interceptor

Create a centralized API client that automatically attaches the user's Supabase JWT token.

**File:** `src/lib/api-client.ts`

```typescript
import { createClientComponentClient } from '@supabase/auth-helpers-nextjs';

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

export async function fetchWithAuth(endpoint: string, options: RequestInit = {}) {
  const supabase = createClientComponentClient();
  
  // Get current session to fetch the JWT token
  const { data: { session } } = await supabase.auth.getSession();
  
  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    ...options.headers,
  };

  // Attach token if the user is authenticated
  if (session?.access_token) {
    headers['Authorization'] = `Bearer ${session.access_token}`;
  }

  const response = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    headers,
  });

  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.error || `API Request failed: ${response.status}`);
  }

  return response.json();
}
```

## 3. Deployments Service

Functions to interact with the backend deployments endpoints.

**File:** `src/services/deployments.ts`

```typescript
import { fetchWithAuth } from '@/lib/api-client';

export interface DeploymentPayload {
  repo: string;
  port?: number;
  subdomain?: string;
  package?: 'small' | 'medium' | 'large';
  cpu_cores?: number;
  memory_mb?: number;
  scaling_mode?: string;
  min_replicas?: number;
  max_replicas?: number;
  cpu_target_utilization?: number;
  cpu_request_milli?: number;
  cpu_limit_milli?: number;
  node_selector?: Record<string, string>;
  env?: Record<string, string>;
  build_args?: Record<string, string>;
}

export interface DeployResponse {
  deployment_id: string;
  message: string;
  status: string;
  url: string;
  subdomain: string;
  repo: string;
  port: number;
  package: string;
  scaling_mode: string;
  min_replicas: number;
  max_replicas: number;
  cpu_cores: number;
  memory_mb: number;
  cpu_target_utilization: number;
  autoscaling_enabled: boolean;
}

export interface DeploymentInfo {
  deployment_id: string;
  package: string;
  port: number;
  repo: string;
  scaling_mode: string;
  status: string;
  subdomain: string;
}

export async function createDeployment(payload: DeploymentPayload): Promise<DeployResponse> {
  return fetchWithAuth('/deploy', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export async function listDeployments(): Promise<DeploymentInfo[]> {
  const data = await fetchWithAuth('/deployments');
  return data.deployments || [];
}

export async function getBuildLogs(deploymentId: string) {
  return fetchWithAuth(`/deployments/${deploymentId}/build-logs`);
}

export async function getAppLogs(deploymentId: string, tail: number = 200) {
  return fetchWithAuth(`/deployments/${deploymentId}/app-logs?tail=${tail}`);
}
```

## 4. Analytics Service

Functions to interact with snapshot analytics and real-time streaming (SSE).

**File:** `src/services/analytics.ts`

```typescript
import { fetchWithAuth } from '@/lib/api-client';
import { createClientComponentClient } from '@supabase/auth-helpers-nextjs';

export interface AnalyticsMetrics {
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

export interface AnalyticsSnapshotResponse {
  deployment_id: string;
  deployment: Record<string, any>;
  metrics: AnalyticsMetrics;
}

export async function getAnalyticsSnapshot(deploymentId: string): Promise<AnalyticsSnapshotResponse> {
  return fetchWithAuth(`/deployments/${deploymentId}/analytics`);
}

/**
 * Connects to the SSE stream for analytics.
 * Returns the EventSource instance so you can close it on unmount.
 */
export async function subscribeToAnalyticsStream(
  deploymentId: string,
  onMessage: (data: AnalyticsSnapshotResponse) => void,
  onError: (err: Event) => void
) {
  const supabase = createClientComponentClient();
  const { data: { session } } = await supabase.auth.getSession();
  
  if (!session?.access_token) {
    throw new Error('User is not authenticated');
  }

  const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';
  
  // NOTE: Standard EventSource doesn't support Authorization headers easily without a polyfill.
  // One secure trick is to pass the token as a query param if backend supports it, 
  // or use a wrapper like @microsoft/fetch-event-source.
  // Assuming backend accepts token via query string for SSE:
  const url = `${API_BASE}/deployments/${deploymentId}/analytics/stream?token=${session.access_token}`;
  
  const eventSource = new EventSource(url);

  eventSource.onmessage = (event) => {
    try {
      const parsedData = JSON.parse(event.data);
      onMessage(parsedData);
    } catch (e) {
      console.error('Failed to parse SSE data', e);
    }
  };

  eventSource.onerror = (err) => {
    console.error('SSE Error:', err);
    onError(err);
    eventSource.close();
  };

  return eventSource;
}
```

*Tip: If the backend requires the bearer header strictly for SSE and doesn't support a `?token=` parameter, you can install and use [`@microsoft/fetch-event-source`](https://www.npmjs.com/package/@microsoft/fetch-event-source).*

## 5. React Hook for Deployment Polling

A custom hook to trigger deployments and poll for status until completion.

**File:** `src/hooks/useDeployRunner.ts`

```typescript
import { useState, useCallback } from 'react';
import { createDeployment, listDeployments, DeploymentPayload, DeployResponse, DeploymentInfo } from '@/services/deployments';

export function useDeployRunner() {
  const [isDeploying, setIsDeploying] = useState(false);
  const [status, setStatus] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [deploymentData, setDeploymentData] = useState<DeployResponse | DeploymentInfo | null>(null);

  const startDeployment = useCallback(async (payload: DeploymentPayload) => {
    setIsDeploying(true);
    setStatus('initializing');
    setError(null);

    try {
      const deployRes = await createDeployment(payload);
      const deploymentId = deployRes.deployment_id;
      setDeploymentData(deployRes);

      // Start polling
      const pollInterval = setInterval(async () => {
        try {
          const all = await listDeployments();
          const current = all.find((d) => d.deployment_id === deploymentId);

          if (current) {
            setStatus(current.status);
            
            if (current.status === 'running' || current.status === 'failed') {
              clearInterval(pollInterval);
              setIsDeploying(false);
              setDeploymentData(current);
            }
          }
        } catch (pollErr: any) {
          clearInterval(pollInterval);
          setError(pollErr.message);
          setIsDeploying(false);
        }
      }, 3000); // poll every 3 seconds

    } catch (err: any) {
      setError(err.message);
      setIsDeploying(false);
      setStatus('failed');
    }
  }, []);

  return { startDeployment, isDeploying, status, error, deploymentData };
}
```

## 6. Example Usage (Real-time Analytics Dashboard)

How to use the services and hooks inside a Next.js client component.

**File:** `src/app/deployments/[id]/page.tsx`

```tsx
'use client';

import { useEffect, useState } from 'react';
import { subscribeToAnalyticsStream, getAnalyticsSnapshot, AnalyticsMetrics } from '@/services/analytics';
import { getAppLogs } from '@/services/deployments';

export default function DeploymentDashboard({ params }: { params: { id: string } }) {
  const deploymentId = params.id;
  const [metrics, setMetrics] = useState<AnalyticsMetrics | null>(null);
  const [logs, setLogs] = useState<string>('');

  useEffect(() => {
    // 1. Initial Data Load
    getAnalyticsSnapshot(deploymentId)
      .then((data) => setMetrics(data.metrics))
      .catch(console.error);

    getAppLogs(deploymentId, 50)
      .then((data) => setLogs(data.application_logs))
      .catch(console.error);

    // 2. Setup real-time updates via SSE
    let eventSource: EventSource | null = null;
    
    subscribeToAnalyticsStream(
      deploymentId,
      (data) => {
        // Update metrics state whenever new data streams in
        setMetrics(data.metrics);
      },
      (error) => console.error('Stream dropped')
    ).then(es => { eventSource = es; });

    // Cleanup: close stream on component unmount
    return () => {
      if (eventSource) eventSource.close();
    };
  }, [deploymentId]);

  if (!metrics) return <div>Loading real-time analytics...</div>;

  return (
    <div className="p-8">
      <h1 className="text-2xl font-bold mb-4">Deployment Dashboard: {deploymentId}</h1>
      
      <div className="grid grid-cols-3 gap-4 mb-8">
        <div className="p-4 border rounded shadow-sm">
          <h3 className="font-semibold text-gray-500">CPU Usage</h3>
          <p className="text-xl">{metrics.resources.cpu_usage_percent}%</p>
        </div>
        <div className="p-4 border rounded shadow-sm">
          <h3 className="font-semibold text-gray-500">Memory</h3>
          <p className="text-xl">{metrics.resources.memory_usage_mb} MB</p>
        </div>
        <div className="p-4 border rounded shadow-sm">
          <h3 className="font-semibold text-gray-500">Active Pods</h3>
          <p className="text-xl">{metrics.pods.current} / {metrics.pods.desired}</p>
        </div>
      </div>

      <div className="bg-gray-900 text-green-400 p-4 rounded h-64 overflow-y-auto font-mono text-sm shadow-inner">
        <h3 className="text-white mb-2">Live Application Logs (Tail)</h3>
        <pre>{logs || 'Waiting for logs...'}</pre>
      </div>
    </div>
  );
}
```
