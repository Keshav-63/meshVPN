# Frontend API Integration Guide

This guide shows how a frontend app can call the control-plane API for deployments, build logs, and app logs.

## Base URL

Use one of these:

- Local: `http://localhost:8080`
- Public tunnel: `https://self.keshavstack.tech`

In frontend code, set:

```js
const API_BASE = "https://self.keshavstack.tech";
```

## 1) Health check

```js
export async function getHealth() {
  const res = await fetch(`${API_BASE}/health`);
  if (!res.ok) throw new Error(`Health failed: ${res.status}`);
  return res.json();
}
```

## 2) Trigger a deployment

Supports runtime env vars (`env`), Docker build args (`build_args`), and CPU-first autoscaling fields.

```js
export async function deployApp() {
  const payload = {
    repo: "https://github.com/your-org/your-app.git",
    port: 3000,
    subdomain: "projectname",
    scaling_mode: "horizontal",
    min_replicas: 2,
    max_replicas: 10,
    cpu_target_utilization: 65,
    cpu_request_milli: 500,
    cpu_limit_milli: 1000,
    node_selector: {
      "meshvpn.worker": "true"
    },
    env: {
      NODE_ENV: "production",
      API_URL: "https://api.example.com"
    },
    build_args: {
      NEXT_PUBLIC_API_BASE: "https://api.example.com"
    }
  };

  const res = await fetch(`${API_BASE}/deploy`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload)
  });

  const data = await res.json();
  if (!res.ok) {
    // On failure, API returns error and build_logs when available.
    throw new Error(data.error || `Deploy failed: ${res.status}`);
  }

  return data;
}
```

Expected successful response fields include:

- `deployment_id`
- `status` (`queued` for async flow)
- `scaling_mode`, `min_replicas`, `max_replicas`
- `cpu_target_utilization`, `cpu_request_milli`, `cpu_limit_milli`
- `node_selector`

## 3) List deployments

```js
export async function listDeployments() {
  const res = await fetch(`${API_BASE}/deployments`);
  if (!res.ok) throw new Error(`List failed: ${res.status}`);
  const data = await res.json();
  return data.deployments || [];
}
```

Deployment items include status and timestamps:

- `status`: `deploying`, `running`, or `failed`
- `started_at`, `finished_at`
- `error` (if failed)

## 4) Get build logs by deployment id

```js
export async function getBuildLogs(deploymentId) {
  const res = await fetch(`${API_BASE}/deployments/${deploymentId}/build-logs`);
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || `Build logs failed: ${res.status}`);
  return data;
}
```

Returns:

- `deployment_id`
- `status`
- `build_logs`

## 5) Get application logs by deployment id

`tail` is optional (default 200, max 5000).

```js
export async function getAppLogs(deploymentId, tail = 300) {
  const res = await fetch(
    `${API_BASE}/deployments/${deploymentId}/app-logs?tail=${encodeURIComponent(tail)}`
  );
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || `App logs failed: ${res.status}`);
  return data;
}
```

Returns:

- `deployment_id`
- `container`
- `tail`
- `application_logs`

## 6) Example frontend flow

```js
export async function deployAndTrack() {
  const deploy = await deployApp();
  const deploymentId = deploy.deployment_id;

  const build = await getBuildLogs(deploymentId);
  console.log("Build status:", build.status);
  console.log(build.build_logs);

  if (build.status === "running") {
    const appLogs = await getAppLogs(deploymentId, 200);
    console.log(appLogs.application_logs);
  }

  return deploy;
}
```

## 7) Polling helper for status

```js
export async function waitUntilFinished(deploymentId, timeoutMs = 10 * 60 * 1000) {
  const started = Date.now();

  while (Date.now() - started < timeoutMs) {
    const deployments = await listDeployments();
    const current = deployments.find((d) => d.deployment_id === deploymentId);

    if (!current) {
      throw new Error("Deployment not found while polling");
    }

    if (current.status === "running" || current.status === "failed") {
      return current;
    }

    await new Promise((r) => setTimeout(r, 3000));
  }

  throw new Error("Timed out waiting for deployment to finish");
}
```

## 8) Environment variable rules

- Env keys must match: `^[A-Za-z_][A-Za-z0-9_]*$`
- `env` applies at runtime (`docker run -e ...`)
- `build_args` applies at build time (`docker build --build-arg ...`)
- For `build_args` to have effect, Dockerfile must define matching `ARG` lines

Example Dockerfile snippet:

```dockerfile
ARG NEXT_PUBLIC_API_BASE
ENV NEXT_PUBLIC_API_BASE=$NEXT_PUBLIC_API_BASE
```

## 9) Notes for browser apps

- If frontend runs on a different origin, configure a reverse proxy or CORS middleware in the control-plane service.
- Keep API host in a frontend environment variable, for example `VITE_API_BASE` or `NEXT_PUBLIC_API_BASE`.

## 10) Prometheus metrics endpoint

The control plane now exposes a Prometheus endpoint:

- `GET /metrics`

Useful app-level metrics:

- `control_plane_deploy_requests_total{scaling_mode=...}`
- `control_plane_worker_jobs_total{status=...}`
- `control_plane_worker_job_duration_seconds`
