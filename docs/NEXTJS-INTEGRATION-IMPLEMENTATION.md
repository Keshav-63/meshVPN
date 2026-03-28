# Next.js Frontend Integration Implementation

This guide provides the complete code needed to integrate your Next.js frontend with the MeshVPN backend. Copy these files into your existing Next.js project.

## Authentication Support

The backend now supports **multiple authentication methods**:
- ✅ **GitHub OAuth** - Users can sign in with their GitHub account
- ✅ **Email/Password** - Users can sign up and sign in with email and password

### How Authentication Works

**Important:** The MeshVPN backend does **NOT** have `/login` or `/signup` endpoints. Authentication is handled entirely by Supabase.

```
┌─────────────┐     1. Sign In      ┌──────────────┐
│  Frontend   │ ─────────────────> │   Supabase   │
│  (Next.js)  │ <───────────────── │              │
└─────────────┘     2. JWT Token    └──────────────┘
       │
       │ 3. API Call with
       │    Authorization: Bearer <JWT>
       ▼
┌─────────────┐
│   Backend   │ ─── 4. Validates JWT token
│ (MeshVPN)   │      (checks provider: github/email)
└─────────────┘
```

The backend automatically validates the JWT token's `provider` claim and accepts both `github` and `email` providers.

### Available Backend Endpoints

**Public Endpoints** (No authentication required):
- `GET /health` - Health check
- `GET /metrics` - Prometheus metrics

**Protected Endpoints** (Require JWT token):
- `GET /auth/whoami` - Get current user info
- `POST /deploy` - Create a new deployment
- `GET /deployments` - List all user's deployments
- `GET /deployments/:id/build-logs` - Get build logs for a deployment
- `GET /deployments/:id/app-logs` - Get application logs
- `GET /deployments/:id/analytics` - Get deployment analytics snapshot
- `GET /deployments/:id/analytics/stream` - Stream real-time analytics (SSE)
- `GET /platform/analytics` - Platform-wide analytics
- `GET /platform/workers/:id/analytics` - Worker-specific analytics

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

export async function getCurrentUser() {
  return fetchWithAuth('/auth/whoami');
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

## 6. Authentication Setup

The backend supports both GitHub OAuth and email-based authentication. Here's how to implement both in your frontend:

**File:** `src/components/AuthForm.tsx`

```tsx
'use client';

import { useState } from 'react';
import { createClientComponentClient } from '@supabase/auth-helpers-nextjs';

export default function AuthForm() {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [isSignUp, setIsSignUp] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const supabase = createClientComponentClient();

  // GitHub OAuth Sign In
  const handleGitHubSignIn = async () => {
    const { error } = await supabase.auth.signInWithOAuth({
      provider: 'github',
      options: {
        redirectTo: `${window.location.origin}/auth/callback`,
      },
    });
    if (error) setError(error.message);
  };

  // Email/Password Sign Up
  const handleEmailSignUp = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    const { error } = await supabase.auth.signUp({
      email,
      password,
      options: {
        emailRedirectTo: `${window.location.origin}/auth/callback`,
      },
    });

    if (error) {
      setError(error.message);
    } else {
      alert('Check your email for the confirmation link!');
    }
  };

  // Email/Password Sign In
  const handleEmailSignIn = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    const { error } = await supabase.auth.signInWithPassword({
      email,
      password,
    });

    if (error) setError(error.message);
  };

  return (
    <div className="max-w-md mx-auto p-6 bg-white rounded-lg shadow">
      <h2 className="text-2xl font-bold mb-6">
        {isSignUp ? 'Sign Up' : 'Sign In'}
      </h2>

      {error && (
        <div className="mb-4 p-3 bg-red-100 text-red-700 rounded">
          {error}
        </div>
      )}

      {/* GitHub OAuth Button */}
      <button
        onClick={handleGitHubSignIn}
        className="w-full mb-4 p-3 bg-gray-800 text-white rounded hover:bg-gray-700 flex items-center justify-center gap-2"
      >
        <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
          <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
        </svg>
        Continue with GitHub
      </button>

      <div className="relative mb-4">
        <div className="absolute inset-0 flex items-center">
          <div className="w-full border-t border-gray-300"></div>
        </div>
        <div className="relative flex justify-center text-sm">
          <span className="px-2 bg-white text-gray-500">Or continue with email</span>
        </div>
      </div>

      {/* Email/Password Form */}
      <form onSubmit={isSignUp ? handleEmailSignUp : handleEmailSignIn}>
        <input
          type="email"
          placeholder="Email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          className="w-full mb-3 p-3 border rounded"
          required
        />
        <input
          type="password"
          placeholder="Password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="w-full mb-4 p-3 border rounded"
          required
        />
        <button
          type="submit"
          className="w-full p-3 bg-blue-600 text-white rounded hover:bg-blue-700"
        >
          {isSignUp ? 'Sign Up' : 'Sign In'}
        </button>
      </form>

      <p className="mt-4 text-center text-sm text-gray-600">
        {isSignUp ? 'Already have an account?' : "Don't have an account?"}{' '}
        <button
          onClick={() => setIsSignUp(!isSignUp)}
          className="text-blue-600 hover:underline"
        >
          {isSignUp ? 'Sign In' : 'Sign Up'}
        </button>
      </p>
    </div>
  );
}
```

**File:** `src/app/auth/callback/route.ts`

```typescript
import { createRouteHandlerClient } from '@supabase/auth-helpers-nextjs';
import { cookies } from 'next/headers';
import { NextResponse } from 'next/server';

export async function GET(request: Request) {
  const requestUrl = new URL(request.url);
  const code = requestUrl.searchParams.get('code');

  if (code) {
    const supabase = createRouteHandlerClient({ cookies });
    await supabase.auth.exchangeCodeForSession(code);
  }

  // Redirect to dashboard or home page after authentication
  return NextResponse.redirect(new URL('/dashboard', request.url));
}
```

## 7. Example Usage (Real-time Analytics Dashboard)

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

## 8. Important Notes

### Authentication Flow
- Both GitHub OAuth and email/password authentication use the same Supabase JWT token
- The backend validates the `provider` claim in the JWT (accepts `github` or `email`)
- No changes needed in your API calls - the `fetchWithAuth` function handles both providers automatically

### Backend Integration
- All authenticated API endpoints require the `Authorization: Bearer <token>` header
- The token is automatically attached by the `fetchWithAuth` function
- If authentication fails, the backend returns a 401 Unauthorized response

### Supabase Configuration
Make sure your Supabase project has:
1. **GitHub OAuth enabled** (if using GitHub sign-in)
   - Configure GitHub OAuth app in Supabase Dashboard > Authentication > Providers
2. **Email authentication enabled** (if using email/password)
   - Enable in Supabase Dashboard > Authentication > Providers > Email
3. **JWT expiry** configured appropriately for your use case

### Backend Configuration (CRITICAL)

**Understanding Supabase JWT Signing Methods:**

Supabase supports two JWT signing methods:
- **HS256 (Legacy)**: Symmetric signing using a shared JWT secret (most common)
- **ES256 (New)**: Asymmetric signing using public/private key pairs via JWKS

Most Supabase projects use **HS256** by default. The backend automatically detects which method your tokens use.

**Required Configuration:**

1. **SUPABASE_JWT_SECRET** - Your JWT secret (REQUIRED)
   - Find it in: Supabase Dashboard → **Project Settings** → **API** → **JWT Settings** → **JWT Secret**
   - This is a **base64-encoded string** (NOT a JWT token, NOT the anon key, NOT the service_role key)
   - Example: `ez3/ahIG...` (base64 string)

2. **SUPABASE_URL** - Your Supabase project URL (Optional, for ES256 projects only)
   - Find it in: Supabase Dashboard → **Project Settings** → **API** → **Project URL**
   - Format: `https://your-project-ref.supabase.co`
   - Only needed if your project uses ES256 signing keys

3. **SUPABASE_ANON_KEY** - Your Supabase anon key (Optional, for ES256 projects only)
   - Find it in: Supabase Dashboard → **Project Settings** → **API** → **Project API keys** → **anon public**
   - Starts with `eyJ...`
   - Only needed if your project uses ES256 signing keys

**Minimal `.env` configuration (HS256 - most common):**
```env
SUPABASE_JWT_SECRET=ez3/ahIGWvDHUQgzJuzLja//Nd7624q6ZymGQow65StHJfrxeFzRyKu0yKwEclaBTFSeyGbmD9hmPawV9dorYQ==
```

**Full `.env` configuration (ES256 projects):**
```env
SUPABASE_URL=https://rpqlrujltxsaqixzefgb.supabase.co
SUPABASE_ANON_KEY=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...your-anon-key
SUPABASE_JWT_SECRET=your-base64-encoded-jwt-secret-here
```

**How it works:**
- The backend automatically detects the signing algorithm from the JWT token header
- For **HS256** tokens (most projects): Uses the base64-decoded JWT secret directly
- For **ES256** tokens (newer projects): Fetches the public key from JWKS endpoint
- No manual configuration needed - it just works!

**Common Errors:**
- ❌ Using the `service_role` key instead of JWT secret
- ❌ Using the `anon` key instead of JWT secret
- ❌ Getting 404 on JWKS endpoint → Your project uses HS256, not ES256 (this is normal!)
- ✅ Using the correct JWT Secret from Project Settings → API → JWT Settings

The backend supports both **HS256** (HMAC) and **ES256** (Elliptic Curve) signing methods automatically!

### CORS Configuration
Ensure your backend allows requests from your frontend domain. The control plane should have CORS configured to accept requests from your Next.js app's origin.

## 9. Quick Start Checklist

- [ ] Set up environment variables in `.env.local`
- [ ] Install required dependencies: `@supabase/auth-helpers-nextjs`, `@supabase/supabase-js`
- [ ] Create the API client (`src/lib/api-client.ts`)
- [ ] Create service files for deployments and analytics
- [ ] Implement authentication UI with both GitHub and email options
- [ ] Set up the auth callback route handler
- [ ] Test authentication with both GitHub OAuth and email/password
- [ ] Test API calls to backend endpoints (deploy, list deployments, analytics)
- [ ] Verify real-time analytics streaming (SSE) works correctly

Your frontend is now ready to integrate with the MeshVPN backend! 🚀
