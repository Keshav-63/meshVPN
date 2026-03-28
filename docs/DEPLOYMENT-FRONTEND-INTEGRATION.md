# Deployment Flow - Frontend Integration Guide

This guide explains how to integrate your Next.js frontend with the MeshVPN deployment backend.

## Overview

Your frontend has a 4-step deployment flow:
1. **Select Project Type** - Web Service, Static Site, Background Worker, Cron Job, etc.
2. **Connect Repository** - GitHub OAuth or paste GitHub URL
3. **Configure Project** - Name, branch, build settings, environment variables
4. **Deploy** - Start the deployment

## Current Backend Support

### ✅ Currently Supported

The `/deploy` endpoint accepts:

```typescript
interface DeploymentRequest {
  repo: string;                    // Required: GitHub repository URL
  port?: number;                   // Optional: Application port (default varies by project type)
  subdomain?: string;              // Optional: Custom subdomain (auto-generated if not provided)
  package?: 'small' | 'medium' | 'large';  // Optional: Resource package (default: 'small')
  env?: Record<string, string>;    // Optional: Environment variables
  build_args?: Record<string, string>;     // Optional: Docker build arguments

  // Advanced options (for subscribers)
  scaling_mode?: string;
  min_replicas?: number;
  max_replicas?: number;
  cpu_target_utilization?: number;
  cpu_request_milli?: number;
  cpu_limit_milli?: number;
  node_selector?: Record<string, string>;
}
```

### ❌ Not Yet Supported (Needs Backend Implementation)

These fields from your UI flow are **NOT** currently supported by the backend:

- `branch` - Which git branch to deploy (currently uses default branch)
- `build_command` - Custom build command (currently uses auto-detection)
- `publish_directory` - Output directory for static sites
- `root_directory` - Monorepo subdirectory support
- `project_name` - Separate from subdomain (currently uses subdomain as identifier)

---

## API Integration Examples

### 1. Authentication Required

All deployment endpoints require authentication. Include the Supabase JWT token in requests:

```typescript
const headers = {
  'Authorization': `Bearer ${session.access_token}`,
  'Content-Type': 'application/json'
};
```

### 2. Create Deployment

**Endpoint:** `POST /deploy`

**Request Example:**

```typescript
const deploymentData = {
  repo: 'https://github.com/username/my-app',
  port: 3000,
  subdomain: 'my-awesome-app',
  package: 'small',
  env: {
    NODE_ENV: 'production',
    API_KEY: 'your-api-key'
  }
};

const response = await fetch(`${API_URL}/deploy`, {
  method: 'POST',
  headers: {
    'Authorization': `Bearer ${token}`,
    'Content-Type': 'application/json'
  },
  body: JSON.stringify(deploymentData)
});

const result = await response.json();
```

**Response (202 Accepted):**

```json
{
  "message": "deployment queued",
  "deployment_id": "uuid-here",
  "status": "queued",
  "url": "https://my-awesome-app.keshavstack.tech",
  "subdomain": "my-awesome-app",
  "repo": "https://github.com/username/my-app",
  "port": 3000,
  "package": "small",
  "cpu_cores": 0.5,
  "memory_mb": 512,
  "scaling_mode": "none",
  "min_replicas": 1,
  "max_replicas": 1,
  "cpu_target_utilization": 70,
  "autoscaling_enabled": false
}
```

### 3. List Deployments

**Endpoint:** `GET /deployments`

```typescript
const response = await fetch(`${API_URL}/deployments`, {
  method: 'GET',
  headers: {
    'Authorization': `Bearer ${token}`
  }
});

const data = await response.json();
```

**Response:**

```json
{
  "deployments": [
    {
      "deployment_id": "uuid-1",
      "status": "running",
      "subdomain": "my-app",
      "repo": "https://github.com/user/repo",
      "port": 3000,
      "package": "small",
      "scaling_mode": "none"
    }
  ]
}
```

### 4. Get Build Logs

**Endpoint:** `GET /deployments/:id/build-logs`

```typescript
const response = await fetch(`${API_URL}/deployments/${deploymentId}/build-logs`, {
  method: 'GET',
  headers: {
    'Authorization': `Bearer ${token}`
  }
});

const logs = await response.json();
```

**Response:**

```json
{
  "deployment_id": "uuid-here",
  "status": "building",
  "build_logs": "Step 1/5 : FROM node:18-alpine\n---> abc123\nStep 2/5 : WORKDIR /app\n..."
}
```

### 5. Get Application Logs

**Endpoint:** `GET /deployments/:id/app-logs?tail=200`

```typescript
const response = await fetch(`${API_URL}/deployments/${deploymentId}/app-logs?tail=500`, {
  method: 'GET',
  headers: {
    'Authorization': `Bearer ${token}`
  }
});

const logs = await response.json();
```

**Response:**

```json
{
  "deployment_id": "uuid-here",
  "container": "k8s_my-app-xyz_...",
  "tail": 500,
  "application_logs": "Server started on port 3000\nConnected to database\n..."
}
```

---

## Frontend Implementation Guide

### Step 1: Project Type Selection

Map UI project types to backend configuration:

```typescript
const projectTypeConfig = {
  'static-site': {
    defaultPort: 80,
    defaultPackage: 'small',
    detectBuildCommand: true
  },
  'web-service': {
    defaultPort: 3000,
    defaultPackage: 'small',
    detectBuildCommand: true
  },
  'background-worker': {
    defaultPort: null,
    defaultPackage: 'small',
    detectBuildCommand: false
  },
  'cron-job': {
    defaultPort: null,
    defaultPackage: 'small',
    detectBuildCommand: false
  }
};
```

### Step 2: GitHub Repository Connection

**Option A: GitHub OAuth (Recommended)**

Use Supabase to authenticate and fetch repositories:

```typescript
// Sign in with GitHub
const { data, error } = await supabase.auth.signInWithOAuth({
  provider: 'github',
  options: {
    scopes: 'repo',  // Request repo access
    redirectTo: `${window.location.origin}/auth/callback`
  }
});

// After OAuth, fetch user's repositories
const response = await fetch('https://api.github.com/user/repos', {
  headers: {
    'Authorization': `Bearer ${githubToken}`,
    'Accept': 'application/vnd.github.v3+json'
  }
});

const repos = await response.json();
```

**Option B: Paste GitHub URL**

```typescript
// Validate GitHub URL format
function validateGitHubURL(url: string): boolean {
  const pattern = /^https:\/\/github\.com\/[\w-]+\/[\w.-]+$/;
  return pattern.test(url);
}

// Extract repo info
function parseGitHubURL(url: string) {
  const match = url.match(/github\.com\/([\w-]+)\/([\w.-]+)/);
  return match ? { owner: match[1], repo: match[2] } : null;
}
```

### Step 3: Configure Project

```typescript
interface ProjectConfig {
  name: string;           // Used as subdomain
  repo: string;          // GitHub URL
  branch: string;        // ⚠️ Not yet supported by backend
  buildCommand: string;  // ⚠️ Not yet supported by backend
  rootDirectory: string; // ⚠️ Not yet supported by backend
  publishDirectory: string; // ⚠️ Not yet supported by backend
  port: number;
  env: Record<string, string>;
}

// Validation
function validateConfig(config: ProjectConfig): string[] {
  const errors: string[] = [];

  if (!config.name || config.name.length < 3) {
    errors.push('Project name must be at least 3 characters');
  }

  if (!/^[a-z0-9-]+$/.test(config.name)) {
    errors.push('Project name can only contain lowercase letters, numbers, and hyphens');
  }

  if (!config.repo) {
    errors.push('Repository URL is required');
  }

  return errors;
}
```

### Step 4: Deploy

```typescript
async function createDeployment(config: ProjectConfig) {
  const payload = {
    repo: config.repo,
    subdomain: config.name,
    port: config.port,
    package: 'small',  // or let user select
    env: config.env
    // Note: branch, buildCommand, rootDirectory, publishDirectory
    // are not yet supported by backend
  };

  const response = await fetch(`${API_URL}/deploy`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${supabaseToken}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(payload)
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error);
  }

  return await response.json();
}
```

### Deployment Status Polling

```typescript
async function pollDeploymentStatus(deploymentId: string) {
  const checkStatus = async () => {
    const deployments = await fetch(`${API_URL}/deployments`, {
      headers: { 'Authorization': `Bearer ${token}` }
    }).then(r => r.json());

    const deployment = deployments.deployments.find(
      d => d.deployment_id === deploymentId
    );

    return deployment?.status;
  };

  // Poll every 3 seconds
  return new Promise((resolve, reject) => {
    const interval = setInterval(async () => {
      const status = await checkStatus();

      if (status === 'running') {
        clearInterval(interval);
        resolve(status);
      } else if (status === 'failed') {
        clearInterval(interval);
        reject(new Error('Deployment failed'));
      }
    }, 3000);
  });
}
```

---

## Resource Packages

The backend supports three resource tiers:

| Package | CPU Cores | Memory | Max Replicas | Use Case |
|---------|-----------|--------|--------------|----------|
| `small` | 0.5 | 512 MB | 3 | Small apps, APIs |
| `medium` | 1.0 | 1024 MB | 5 | Production apps |
| `large` | 2.0 | 2048 MB | 10 | High-traffic apps |

**Free tier users:** Get fixed resources (no autoscaling)
**Subscribers:** Get autoscaling enabled automatically

---

## Error Handling

```typescript
try {
  const deployment = await createDeployment(config);
  console.log('Deployment created:', deployment.deployment_id);
} catch (error) {
  if (error.message.includes('subdomain')) {
    // Subdomain already in use
    showError('This project name is already taken. Please choose another.');
  } else if (error.message.includes('repo')) {
    // Invalid repository
    showError('Invalid repository URL. Please check and try again.');
  } else {
    // Generic error
    showError('Deployment failed. Please try again.');
  }
}
```

---

## Complete React Component Example

```tsx
'use client';

import { useState } from 'react';
import { createClientComponentClient } from '@supabase/auth-helpers-nextjs';

export default function DeploymentFlow() {
  const [step, setStep] = useState(1);
  const [config, setConfig] = useState({
    type: 'web-service',
    repo: '',
    name: '',
    branch: 'main',
    port: 3000,
    env: {}
  });
  const [isDeploying, setIsDeploying] = useState(false);
  const supabase = createClientComponentClient();

  const handleDeploy = async () => {
    setIsDeploying(true);

    try {
      // Get Supabase token
      const { data: { session } } = await supabase.auth.getSession();

      if (!session) {
        throw new Error('Not authenticated');
      }

      // Create deployment
      const response = await fetch('http://localhost:8080/deploy', {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${session.access_token}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          repo: config.repo,
          subdomain: config.name,
          port: config.port,
          package: 'small',
          env: config.env
        })
      });

      if (!response.ok) {
        const error = await response.json();
        throw new Error(error.error);
      }

      const deployment = await response.json();

      // Redirect to deployment page
      window.location.href = `/deployments/${deployment.deployment_id}`;

    } catch (error) {
      console.error('Deployment failed:', error);
      alert(error.message);
    } finally {
      setIsDeploying(false);
    }
  };

  return (
    <div className="deployment-flow">
      {step === 1 && <ProjectTypeSelector onChange={(type) => setConfig({...config, type})} />}
      {step === 2 && <RepositorySelector onChange={(repo) => setConfig({...config, repo})} />}
      {step === 3 && <ProjectConfiguration config={config} onChange={setConfig} />}
      {step === 4 && (
        <button
          onClick={handleDeploy}
          disabled={isDeploying}
        >
          {isDeploying ? 'Deploying...' : 'Deploy Now'}
        </button>
      )}
    </div>
  );
}
```

---

## What Needs to Be Added to Backend

To fully support your UI flow, these features need backend implementation:

### Priority 1 (Required)
- [ ] **Branch selection** - Deploy specific git branches
- [ ] **Build command** - Custom build commands (npm run build, etc.)
- [ ] **Root directory** - Monorepo support

### Priority 2 (Nice to have)
- [ ] **Publish directory** - For static sites (dist/, build/, etc.)
- [ ] **Project name separate from subdomain** - Allow renaming
- [ ] **Auto-detection** - Detect framework and suggest build settings

### Implementation Needed

Update `DeployRequestPayload` in router.go:

```go
type DeployRequestPayload struct {
    Repo             string            `json:"repo" binding:"required"`
    Branch           string            `json:"branch"`           // NEW
    BuildCommand     string            `json:"build_command"`    // NEW
    RootDirectory    string            `json:"root_directory"`   // NEW
    PublishDirectory string            `json:"publish_directory"` // NEW
    Port             int               `json:"port"`
    Subdomain        string            `json:"subdomain"`
    Package          string            `json:"package"`
    Env              map[string]string `json:"env"`
    BuildArgs        map[string]string `json:"build_args"`
}
```

---

## Testing Checklist

- [ ] User can authenticate with GitHub OAuth
- [ ] User can select project type
- [ ] User can paste GitHub repository URL
- [ ] User can configure project name and port
- [ ] Deployment creates successfully
- [ ] Deployment status updates correctly
- [ ] Build logs stream in real-time
- [ ] Application logs are accessible
- [ ] Error messages are user-friendly
- [ ] Subdomain validation works
- [ ] Environment variables are passed correctly

---

## Support

For questions or issues, check:
- Backend API: `http://localhost:8080/swagger/index.html`
- Authentication docs: `docs/NEXTJS-INTEGRATION-IMPLEMENTATION.md`
- GitHub OAuth setup: See GitHub provider configuration section above
