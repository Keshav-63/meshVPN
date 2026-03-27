# Phase 1 Postman API Testing Guide

This guide gives a complete Postman testing flow for the Phase 1 backend:

- Supabase GitHub JWT auth
- Async deployment queue
- Deployment status and logs endpoints

## 1) Where to set environment variables

Use this file:

- `infra/.env` (copy from `infra/.env.example`)

Required keys:

- `DATABASE_URL` or `SUPABASE_DB_URL`
- `SUPABASE_JWT_SECRET`
- `REQUIRE_AUTH=true`
- `WORKER_POLL_INTERVAL=2s`
- `WORKER_BATCH_SIZE=3`
- `APP_BASE_DOMAIN=localhost` (or your domain)

Then start services:

```powershell
cd infra
docker compose --env-file .env up -d
```

If running API outside docker:

```powershell
cd control-plane
$env:DATABASE_URL="..."
$env:SUPABASE_JWT_SECRET="..."
$env:REQUIRE_AUTH="true"
$env:WORKER_POLL_INTERVAL="2s"
$env:WORKER_BATCH_SIZE="3"
go run ./cmd/control-plane
```

## 2) What I need from you for testing

Provide these values:

1. `SUPABASE_JWT_SECRET`
2. One valid user access token from Supabase login via GitHub
3. `DATABASE_URL` (or `SUPABASE_DB_URL`)
4. One deployable repo URL with root `Dockerfile`

## 3) Create Postman environment

Create an environment named `phase1-local` with:

- `baseUrl` = `http://localhost:8080`
- `authToken` = `<paste Supabase access token>`
- `deploymentId` = ``

## 4) Collection-level pre-request script

Add to collection Pre-request Script:

```javascript
const token = pm.environment.get('authToken');
if (token && token.trim() !== '') {
  pm.request.headers.upsert({ key: 'Authorization', value: `Bearer ${token}` });
}
pm.request.headers.upsert({ key: 'Content-Type', value: 'application/json' });
```

## 5) Request-by-request test plan

### A) Health (no auth required)

- `GET {{baseUrl}}/health`

Tests:

```javascript
pm.test('Health status is 200', function () {
  pm.response.to.have.status(200);
});
pm.test('Health payload has status', function () {
  const body = pm.response.json();
  pm.expect(body).to.have.property('status');
});
```

### B) WhoAmI (auth required)

- `GET {{baseUrl}}/auth/whoami`

Tests:

```javascript
pm.test('WhoAmI status is 200', function () {
  pm.response.to.have.status(200);
});
pm.test('WhoAmI returns GitHub provider identity', function () {
  const body = pm.response.json();
  pm.expect(body).to.have.property('sub');
  pm.expect(body).to.have.property('provider');
  pm.expect(String(body.provider).toLowerCase()).to.eql('github');
});
```

### C) Queue Deploy

- `POST {{baseUrl}}/deploy`

Body:

```json
{
  "repo": "https://github.com/your-org/your-app.git",
  "port": 3000,
  "subdomain": "phase1demo",
  "cpu_cores": 0.5,
  "memory_mb": 512,
  "env": {
    "NODE_ENV": "production"
  },
  "build_args": {
    "NEXT_PUBLIC_API_BASE": "https://api.example.com"
  }
}
```

Tests:

```javascript
pm.test('Deploy request is accepted', function () {
  pm.response.to.have.status(202);
});
pm.test('Deployment is queued and id is returned', function () {
  const body = pm.response.json();
  pm.expect(body).to.have.property('deployment_id');
  pm.expect(body).to.have.property('status', 'queued');
  pm.environment.set('deploymentId', body.deployment_id);
});
```

### D) List Deployments

- `GET {{baseUrl}}/deployments`

Tests:

```javascript
pm.test('List deployments status is 200', function () {
  pm.response.to.have.status(200);
});
pm.test('Current deployment is present in list', function () {
  const body = pm.response.json();
  const id = pm.environment.get('deploymentId');
  pm.expect(body).to.have.property('deployments');
  pm.expect(body.deployments.some(d => d.deployment_id === id)).to.eql(true);
});
```

### E) Build Logs by ID

- `GET {{baseUrl}}/deployments/{{deploymentId}}/build-logs`

Tests:

```javascript
pm.test('Build logs status is 200', function () {
  pm.response.to.have.status(200);
});
pm.test('Build logs payload has deployment and status', function () {
  const body = pm.response.json();
  pm.expect(body).to.have.property('deployment_id');
  pm.expect(body).to.have.property('status');
});
```

### F) App Logs by ID

- `GET {{baseUrl}}/deployments/{{deploymentId}}/app-logs?tail=200`

Tests:

```javascript
pm.test('App logs status is 200 or 400 before running', function () {
  pm.expect([200, 400]).to.include(pm.response.code);
});
pm.test('App logs response shape is valid', function () {
  const body = pm.response.json();
  if (pm.response.code === 200) {
    pm.expect(body).to.have.property('application_logs');
  } else {
    pm.expect(body).to.have.property('error');
  }
});
```

## 6) Negative test cases

1. Missing token:
- Clear `authToken` and call `/auth/whoami`.
- Expected: `401`.

2. Non-GitHub token:
- Use token with provider not github.
- Expected: `403`.

3. Invalid env var key:
- `env: { "1INVALID": "x" }` in deploy body.
- Expected: `400`.

4. Invalid tail:
- `/app-logs?tail=-1`
- Expected: `400`.

## 7) Debugging checklist

1. API startup:
- use `go run ./cmd/control-plane` (not `go run .`)

2. Logs to watch:
- `[DEBUG] [auth]`
- `[DEBUG] [http]`
- `[INFO] [service]`
- `[INFO] [worker]`
- `[INFO] [runtime]`
- `[DEBUG] [jobs-postgres]` or `[DEBUG] [jobs-memory]`

3. If deploy remains `queued`:
- verify worker started in logs
- verify DB connectivity and `deployment_jobs` rows

4. If `/auth/whoami` fails:
- verify `SUPABASE_JWT_SECRET`
- verify token is from Supabase Auth (GitHub provider)
  