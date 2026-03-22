# End-to-End Testing Guide - Phase 2

Complete testing guide for all MeshVPN features including resource packages, analytics, user management, and autoscaling.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Setup & Installation](#setup--installation)
3. [Database Verification](#database-verification)
4. [Postman Setup](#postman-setup)
5. [Test Scenarios](#test-scenarios)
6. [Analytics Testing](#analytics-testing)
7. [Grafana Dashboard Testing](#grafana-dashboard-testing)
8. [Troubleshooting](#troubleshooting)

---

## Prerequisites

### Required Services Running

```bash
# Verify all containers are running
docker ps

# Expected containers:
# - k3d-meshvpn-cluster-server-0 (K3D)
# - k3d-meshvpn-cluster-serverlb (K3D LoadBalancer)
# - k3d-meshvpn-cluster-tools (K3D Tools)
# - cloudflared-1 (Cloudflare Tunnel)
# - observability_prometheus
# - observability_grafana
```

### Required Tools

- Postman Desktop or Postman Web
- PostgreSQL client (optional, for database verification)
- curl (for CLI testing)
- Browser (for Grafana testing)

---

## Setup & Installation

### 1. Start All Services

```bash
# Start K3D cluster (if not running)
k3d cluster start meshvpn

# Verify cluster is ready
kubectl get nodes

# Start Cloudflare Tunnel
cd ~/MeshVPN-slef-hosting/infra
docker compose up -d

# Start Observability Stack
cd ~/MeshVPN-slef-hosting/infra/observability
docker compose up -d

# Verify all services
docker ps
```

### 2. Run Database Migrations

```bash
cd ~/MeshVPN-slef-hosting/control-plane

# Set environment variables
export DATABASE_URL="your_postgres_connection_string"
export SUPABASE_JWT_SECRET="your_jwt_secret"
export REQUIRE_AUTH=false  # Disable auth for easier testing
export RUNTIME_BACKEND=k3s
export ENABLE_CPU_HPA=true
export K8S_NAMESPACE=meshvpn-apps
export K8S_IMAGE_PREFIX=ghcr.io/your-github-username
export K8S_CONFIG_PATH=/home/your-username/k3d-kubeconfig.yaml

# Run control-plane (migrations run automatically on startup)
go run ./cmd/control-plane
```

Expected output:
```
Control plane starting...
Running database migrations...
Migration 001_initial.sql: OK
Migration 002_add_deployment_fields.sql: OK
Migration 003_users_and_analytics.sql: OK
Analytics collector started interval=1m
Listening on :8080
Worker started (poll interval: 2s)
```

### 3. Verify Services are Accessible

```bash
# Health check
curl http://localhost:8080/health
# Expected: {"status":"LaptopCloud running"}

# Metrics check
curl http://localhost:8080/metrics
# Expected: Prometheus format metrics

# Prometheus
curl http://localhost:9090/-/healthy
# Expected: Prometheus is Healthy.

# Grafana
curl http://localhost:3001/api/health
# Expected: {"commit":"...","database":"ok","version":"..."}
```

---

## Database Verification

### Connect to Database

```bash
# Using psql (adjust connection string)
psql "postgresql://user:password@host:5432/dbname"

# Or for Supabase
psql "postgresql://postgres:password@db.project.supabase.co:5432/postgres?sslmode=require"
```

### Verify Tables Exist

```sql
-- List all tables
\dt

-- Expected tables:
-- deployments
-- deployment_jobs
-- users
-- deployment_metrics
-- deployment_requests
```

### Verify Table Structure

```sql
-- Check users table
\d users

-- Expected columns:
-- user_id, email, provider, is_subscriber, subscription_tier, created_at, updated_at

-- Check deployment_metrics table
\d deployment_metrics

-- Expected columns:
-- deployment_id, request_count_total, request_count_1h, request_count_24h,
-- requests_per_second, bandwidth_sent_bytes, bandwidth_received_bytes,
-- latency_p50_ms, latency_p90_ms, latency_p99_ms, current_pods, desired_pods,
-- cpu_usage_percent, memory_usage_mb, last_updated

-- Check deployments table for new columns
\d deployments

-- Should include: package, user_id
```

---

## Postman Setup

### 1. Create New Collection

1. Open Postman
2. Click **New** → **Collection**
3. Name: "MeshVPN Phase 2 - Complete E2E Tests"
4. Description: "End-to-end testing for MeshVPN with packages, analytics, and autoscaling"

### 2. Setup Environment Variables

Create a Postman environment with these variables:

```json
{
  "base_url": "http://localhost:8080",
  "public_url": "https://self.keshavstack.tech",
  "domain": "keshavstack.tech",
  "auth_token": "your_jwt_token_if_auth_enabled",
  "deployment_id": "",
  "test_repo": "https://github.com/vercel/next.js",
  "test_subdomain": "test-app"
}
```

**Note**: If `REQUIRE_AUTH=false`, you can skip the `auth_token`.

---

## Test Scenarios

### Scenario 1: Health & Metrics

#### Test 1.1: Health Check

**Request:**
```
GET {{base_url}}/health
```

**Expected Response:**
```json
{
  "status": "LaptopCloud running"
}
```

**Status Code:** 200 OK

---

#### Test 1.2: Prometheus Metrics

**Request:**
```
GET {{base_url}}/metrics
```

**Expected Response:**
- Prometheus format metrics
- Should include:
  - `deployment_queue_length`
  - `deployment_worker_runs_total`
  - `deployment_requests_total`
  - `deployment_pods`

**Status Code:** 200 OK

**Postman Test Script:**
```javascript
pm.test("Status code is 200", function () {
    pm.response.to.have.status(200);
});

pm.test("Response contains Prometheus metrics", function () {
    pm.expect(pm.response.text()).to.include("deployment_queue_length");
    pm.expect(pm.response.text()).to.include("deployment_requests_total");
});
```

---

### Scenario 2: Authentication (If Enabled)

#### Test 2.1: Whoami

**Request:**
```
GET {{base_url}}/auth/whoami
Headers:
  Authorization: Bearer {{auth_token}}
```

**Expected Response:**
```json
{
  "sub": "user-id-from-jwt",
  "email": "user@example.com",
  "provider": "github"
}
```

**Status Code:** 200 OK

---

### Scenario 3: Deployment with Packages

#### Test 3.1: Deploy with Small Package (Auto-Subdomain)

**Request:**
```
POST {{base_url}}/deploy
Headers:
  Content-Type: application/json
  Authorization: Bearer {{auth_token}} (if auth enabled)

Body:
{
  "repo": "https://github.com/vercel/next.js",
  "package": "small",
  "port": 3000
}
```

**Expected Response:**
```json
{
  "message": "deployment queued",
  "deployment_id": "74b295d2",
  "status": "queued",
  "repo": "https://github.com/vercel/next.js",
  "subdomain": "next-js",
  "url": "https://next-js.keshavstack.tech",
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

**Status Code:** 202 Accepted

**Postman Test Script:**
```javascript
pm.test("Status code is 202", function () {
    pm.response.to.have.status(202);
});

pm.test("Response contains deployment_id", function () {
    var jsonData = pm.response.json();
    pm.expect(jsonData).to.have.property('deployment_id');
    pm.environment.set("deployment_id", jsonData.deployment_id);
});

pm.test("Package is small", function () {
    var jsonData = pm.response.json();
    pm.expect(jsonData.package).to.eql('small');
    pm.expect(jsonData.cpu_cores).to.eql(0.5);
    pm.expect(jsonData.memory_mb).to.eql(512);
});

pm.test("Subdomain auto-generated", function () {
    var jsonData = pm.response.json();
    pm.expect(jsonData.subdomain).to.exist;
    pm.expect(jsonData.url).to.include(jsonData.subdomain);
});

pm.test("Autoscaling disabled for non-subscriber", function () {
    var jsonData = pm.response.json();
    pm.expect(jsonData.autoscaling_enabled).to.be.false;
    pm.expect(jsonData.scaling_mode).to.eql('none');
    pm.expect(jsonData.max_replicas).to.eql(1);
});
```

---

#### Test 3.2: Deploy with Medium Package (Custom Subdomain)

**Request:**
```
POST {{base_url}}/deploy
Headers:
  Content-Type: application/json

Body:
{
  "repo": "https://github.com/nodejs/node",
  "package": "medium",
  "port": 8080,
  "subdomain": "node-app",
  "env": {
    "NODE_ENV": "production"
  }
}
```

**Expected Response:**
```json
{
  "message": "deployment queued",
  "deployment_id": "a1b2c3d4",
  "status": "queued",
  "repo": "https://github.com/nodejs/node",
  "subdomain": "node-app",
  "url": "https://node-app.keshavstack.tech",
  "port": 8080,
  "package": "medium",
  "cpu_cores": 1.0,
  "memory_mb": 1024,
  "scaling_mode": "none",
  "min_replicas": 1,
  "max_replicas": 1
}
```

**Postman Test Script:**
```javascript
pm.test("Package is medium", function () {
    var jsonData = pm.response.json();
    pm.expect(jsonData.package).to.eql('medium');
    pm.expect(jsonData.cpu_cores).to.eql(1.0);
    pm.expect(jsonData.memory_mb).to.eql(1024);
});

pm.test("Custom subdomain used", function () {
    var jsonData = pm.response.json();
    pm.expect(jsonData.subdomain).to.eql('node-app');
});
```

---

#### Test 3.3: Deploy with Large Package

**Request:**
```
POST {{base_url}}/deploy
Headers:
  Content-Type: application/json

Body:
{
  "repo": "https://github.com/python/cpython",
  "package": "large",
  "port": 5000
}
```

**Expected Response:**
```json
{
  "package": "large",
  "cpu_cores": 2.0,
  "memory_mb": 2048,
  "max_replicas": 1
}
```

**Postman Test Script:**
```javascript
pm.test("Package is large", function () {
    var jsonData = pm.response.json();
    pm.expect(jsonData.package).to.eql('large');
    pm.expect(jsonData.cpu_cores).to.eql(2.0);
    pm.expect(jsonData.memory_mb).to.eql(2048);
});
```

---

#### Test 3.4: Deploy with Invalid Package

**Request:**
```
POST {{base_url}}/deploy
Headers:
  Content-Type: application/json

Body:
{
  "repo": "https://github.com/test/test",
  "package": "xlarge",
  "port": 3000
}
```

**Expected Response:**
```json
{
  "error": "invalid package 'xlarge'. must be: small, medium, large"
}
```

**Status Code:** 400 Bad Request

**Postman Test Script:**
```javascript
pm.test("Status code is 400", function () {
    pm.response.to.have.status(400);
});

pm.test("Error message for invalid package", function () {
    var jsonData = pm.response.json();
    pm.expect(jsonData.error).to.include('invalid package');
});
```

---

#### Test 3.5: Deploy without Package (Default to Small)

**Request:**
```
POST {{base_url}}/deploy
Headers:
  Content-Type: application/json

Body:
{
  "repo": "https://github.com/test/default-package",
  "port": 3000
}
```

**Expected Response:**
```json
{
  "package": "small",
  "cpu_cores": 0.5,
  "memory_mb": 512
}
```

**Postman Test Script:**
```javascript
pm.test("Defaults to small package", function () {
    var jsonData = pm.response.json();
    pm.expect(jsonData.package).to.eql('small');
});
```

---

#### Test 3.6: Subdomain Conflict Detection

**Request 1:**
```
POST {{base_url}}/deploy
Body:
{
  "repo": "https://github.com/test/app1",
  "subdomain": "my-app",
  "package": "small",
  "port": 3000
}
```

**Request 2 (Same subdomain):**
```
POST {{base_url}}/deploy
Body:
{
  "repo": "https://github.com/test/app2",
  "subdomain": "my-app",
  "package": "small",
  "port": 3000
}
```

**Expected Response for Request 2:**
```json
{
  "error": "subdomain 'my-app' is already in use"
}
```

**Status Code:** 400 Bad Request

---

### Scenario 4: List Deployments

#### Test 4.1: List All Deployments

**Request:**
```
GET {{base_url}}/deployments
```

**Expected Response:**
```json
{
  "deployments": [
    {
      "deployment_id": "74b295d2",
      "repo": "https://github.com/vercel/next.js",
      "subdomain": "next-js",
      "url": "https://next-js.keshavstack.tech",
      "status": "queued",
      "package": "small",
      "created_at": "2026-03-21T20:00:00Z"
    }
  ]
}
```

**Postman Test Script:**
```javascript
pm.test("Status code is 200", function () {
    pm.response.to.have.status(200);
});

pm.test("Response contains deployments array", function () {
    var jsonData = pm.response.json();
    pm.expect(jsonData).to.have.property('deployments');
    pm.expect(jsonData.deployments).to.be.an('array');
});

pm.test("Deployments include package field", function () {
    var jsonData = pm.response.json();
    if (jsonData.deployments.length > 0) {
        pm.expect(jsonData.deployments[0]).to.have.property('package');
    }
});
```

---

### Scenario 5: Build Logs

#### Test 5.1: Get Build Logs

**Request:**
```
GET {{base_url}}/deployments/{{deployment_id}}/build-logs
```

**Expected Response:**
```json
{
  "deployment_id": "74b295d2",
  "status": "building",
  "build_logs": "Step 1/5 : FROM node:18-alpine\n ---> Pulling image...\n..."
}
```

**Postman Test Script:**
```javascript
pm.test("Status code is 200", function () {
    pm.response.to.have.status(200);
});

pm.test("Response contains build_logs", function () {
    var jsonData = pm.response.json();
    pm.expect(jsonData).to.have.property('build_logs');
});
```

---

### Scenario 6: Application Logs

#### Test 6.1: Get App Logs (Default Tail)

**Request:**
```
GET {{base_url}}/deployments/{{deployment_id}}/app-logs
```

**Expected Response:**
```json
{
  "deployment_id": "74b295d2",
  "container": "app-74b295d2-abc123",
  "tail": 200,
  "application_logs": "Server listening on port 3000\n..."
}
```

---

#### Test 6.2: Get App Logs (Custom Tail)

**Request:**
```
GET {{base_url}}/deployments/{{deployment_id}}/app-logs?tail=50
```

**Expected Response:**
```json
{
  "tail": 50,
  "application_logs": "..."
}
```

**Postman Test Script:**
```javascript
pm.test("Tail parameter is respected", function () {
    var jsonData = pm.response.json();
    pm.expect(jsonData.tail).to.eql(50);
});
```

---

## Analytics Testing

### Scenario 7: Analytics API

#### Test 7.1: Get Analytics Snapshot

**Request:**
```
GET {{base_url}}/deployments/{{deployment_id}}/analytics
Headers:
  Authorization: Bearer {{auth_token}} (if auth enabled)
```

**Expected Response:**
```json
{
  "deployment_id": "74b295d2",
  "deployment": {
    "repo": "https://github.com/vercel/next.js",
    "subdomain": "next-js",
    "url": "https://next-js.keshavstack.tech",
    "package": "small",
    "status": "running",
    "scaling_mode": "none",
    "min_replicas": 1,
    "max_replicas": 1,
    "started_at": "2026-03-21T20:00:00Z"
  },
  "metrics": {
    "requests": {
      "total": 1523,
      "last_hour": 145,
      "last_24h": 1200,
      "per_second": 0.04
    },
    "latency": {
      "p50_ms": 125.4,
      "p90_ms": 280.6,
      "p99_ms": 520.3
    },
    "bandwidth": {
      "sent_bytes": 5242880,
      "received_bytes": 1048576
    },
    "pods": {
      "current": 1,
      "desired": 1
    },
    "resources": {
      "cpu_usage_percent": 35.2,
      "memory_usage_mb": 256.5
    },
    "last_updated": "2026-03-21T21:00:00Z"
  }
}
```

**Postman Test Script:**
```javascript
pm.test("Status code is 200", function () {
    pm.response.to.have.status(200);
});

pm.test("Response contains deployment and metrics", function () {
    var jsonData = pm.response.json();
    pm.expect(jsonData).to.have.property('deployment');
    pm.expect(jsonData).to.have.property('metrics');
});

pm.test("Metrics include all categories", function () {
    var jsonData = pm.response.json();
    var metrics = jsonData.metrics;
    pm.expect(metrics).to.have.property('requests');
    pm.expect(metrics).to.have.property('latency');
    pm.expect(metrics).to.have.property('bandwidth');
    pm.expect(metrics).to.have.property('pods');
    pm.expect(metrics).to.have.property('resources');
});

pm.test("Request metrics structure", function () {
    var jsonData = pm.response.json();
    var requests = jsonData.metrics.requests;
    pm.expect(requests).to.have.property('total');
    pm.expect(requests).to.have.property('last_hour');
    pm.expect(requests).to.have.property('last_24h');
    pm.expect(requests).to.have.property('per_second');
});

pm.test("Latency percentiles exist", function () {
    var jsonData = pm.response.json();
    var latency = jsonData.metrics.latency;
    pm.expect(latency).to.have.property('p50_ms');
    pm.expect(latency).to.have.property('p90_ms');
    pm.expect(latency).to.have.property('p99_ms');
});

pm.test("Pod status matches package", function () {
    var jsonData = pm.response.json();
    var pods = jsonData.metrics.pods;
    pm.expect(pods.current).to.be.a('number');
    pm.expect(pods.desired).to.be.a('number');

    // Non-subscribers should have desired = 1
    if (jsonData.deployment.scaling_mode === 'none') {
        pm.expect(pods.desired).to.eql(1);
    }
});
```

---

#### Test 7.2: Analytics for Non-Existent Deployment

**Request:**
```
GET {{base_url}}/deployments/nonexistent123/analytics
```

**Expected Response:**
```json
{
  "error": "deployment not found"
}
```

**Status Code:** 404 Not Found

---

#### Test 7.3: Analytics Stream (SSE)

**Note**: Postman doesn't handle SSE well. Use curl or browser for this test.

**CLI Test:**
```bash
curl -N https://self.keshavstack.tech/deployments/74b295d2/analytics/stream \
  -H "Authorization: Bearer $TOKEN"
```

**Expected Output:**
```
data: {"deployment_id":"74b295d2","timestamp":1711028580,"requests":{"total":1523,"last_hour":145,"per_second":0.04},"latency":{"p50_ms":125.4,"p90_ms":280.6,"p99_ms":520.3},"bandwidth":{"sent_bytes":5242880,"received_bytes":1048576},"pods":{"current":1,"desired":1},"resources":{"cpu_usage_percent":35.2,"memory_usage_mb":256.5}}

data: {"deployment_id":"74b295d2","timestamp":1711028585,"requests":{"total":1525,"last_hour":146,"per_second":0.041},...}
```

Updates should arrive every 5 seconds.

---

### Scenario 8: User Management (Database)

#### Test 8.1: Verify User Created on First Login

After deploying with auth enabled, check the database:

```sql
SELECT * FROM users;
```

**Expected Result:**
```
user_id | email | provider | is_subscriber | subscription_tier | created_at | updated_at
--------|-------|----------|---------------|-------------------|------------|------------
abc123  | user@example.com | github | false | NULL | 2026-03-21... | 2026-03-21...
```

---

#### Test 8.2: Verify Deployment Has User ID

```sql
SELECT deployment_id, repo, package, user_id FROM deployments LIMIT 5;
```

**Expected Result:**
```
deployment_id | repo | package | user_id
--------------|------|---------|--------
74b295d2 | https://github.com/vercel/next.js | small | abc123
```

---

### Scenario 9: Metrics Collection

#### Test 9.1: Wait for Metrics Collection (1 Minute)

The analytics collector runs every 60 seconds. Wait 1 minute after deployment is running.

```sql
-- Check if metrics are being collected
SELECT
  deployment_id,
  request_count_total,
  current_pods,
  desired_pods,
  last_updated
FROM deployment_metrics
ORDER BY last_updated DESC
LIMIT 5;
```

**Expected Result:**
Rows should exist with recent `last_updated` timestamps.

---

#### Test 9.2: Verify Prometheus Metrics

```bash
curl http://localhost:8080/metrics | grep deployment_pods
```

**Expected Output:**
```
deployment_pods{deployment_id="74b295d2",type="current"} 1
deployment_pods{deployment_id="74b295d2",type="desired"} 1
```

---

## Grafana Dashboard Testing

### Test 10.1: Access Grafana

1. Open http://localhost:3001
2. Should auto-login (anonymous access enabled)
3. No credentials required

**Expected:** Grafana home page loads

---

### Test 10.2: Verify Prometheus Datasource

1. Go to **Configuration** → **Data Sources**
2. Click **Prometheus**
3. Click **Test** button

**Expected:** "Data source is working"

---

### Test 10.3: Platform Overview Dashboard

1. Click **Dashboards** → **Browse**
2. Open **MeshVPN** folder
3. Click **Platform Overview**

**Expected Panels:**
- Total Active Deployments (should show count)
- Platform Request Rate (time series)
- Platform Bandwidth Usage
- Platform-wide Latency Percentiles
- Top 10 Deployments by Request Count
- Top 10 Deployments by Bandwidth
- Average CPU/Memory Usage
- Platform Pod Distribution

**Verify:**
- All panels load without errors
- Metrics show data (may be zeros if no traffic yet)
- Refresh rate is 5 seconds

---

### Test 10.4: Deployment Detail Dashboard

1. Navigate to **Deployment Detail** dashboard
2. Select a deployment ID from dropdown

**Expected Panels:**
- Request Rate Over Time
- Latency Percentiles
- Status Code Distribution
- Error Rate
- Pod Count (Gauge)
- Bandwidth Over Time
- CPU Usage
- Memory Usage

**Verify:**
- All panels load
- Deployment selector works
- Data updates every 5 seconds

---

### Test 10.5: Custom PromQL Query

1. Open **Explore** view
2. Select **Prometheus** datasource
3. Run query:

```promql
sum(rate(deployment_requests_total[5m])) by (deployment_id)
```

**Expected:** Graph showing request rate per deployment

---

## Kubernetes Verification

### Test 11.1: Verify Deployment Resources

```bash
kubectl -n meshvpn-apps get deploy -o yaml | grep -A 10 resources
```

**Expected Output (for Small package):**
```yaml
resources:
  limits:
    cpu: 500m
    memory: 512Mi
  requests:
    cpu: 500m
    memory: 512Mi
```

---

### Test 11.2: Verify HPA (Non-Subscribers)

```bash
kubectl -n meshvpn-apps get hpa
```

**Expected:** No HPA resources (autoscaling disabled for non-subscribers)

---

### Test 11.3: Verify Package Labels

```bash
kubectl -n meshvpn-apps get deploy --show-labels
```

**Expected Labels:**
```
app=app-74b295d2,package=small
```

---

## End-to-End Workflow Test

### Complete User Journey

#### Step 1: Deploy Application

```bash
curl -X POST http://localhost:8080/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/vercel/next.js",
    "package": "medium",
    "port": 3000
  }'
```

Save the `deployment_id` from response.

---

#### Step 2: Monitor Build Progress

```bash
# Watch build logs
while true; do
  curl -s http://localhost:8080/deployments/{deployment_id}/build-logs | jq -r '.build_logs' | tail -20
  sleep 5
done
```

Wait for status to become `running`.

---

#### Step 3: Check Kubernetes Resources

```bash
kubectl -n meshvpn-apps get deploy,svc,ing,pods
```

**Expected:**
- Deployment: `app-{deployment_id}`
- Service: `app-{deployment_id}-service`
- Ingress: `app-{deployment_id}-ingress`
- Pod(s): Running

---

#### Step 4: Access Application

```bash
curl https://{subdomain}.keshavstack.tech
```

**Expected:** Application response

---

#### Step 5: Generate Traffic for Analytics

```bash
# Generate 100 requests
for i in {1..100}; do
  curl -s https://{subdomain}.keshavstack.tech > /dev/null
  echo "Request $i sent"
done
```

---

#### Step 6: Wait for Metrics Collection

Wait 60 seconds for analytics collector to run.

---

#### Step 7: Check Analytics

```bash
curl http://localhost:8080/deployments/{deployment_id}/analytics | jq
```

**Verify:**
- `requests.total` > 0
- `requests.last_hour` > 0
- `latency.p50_ms` has value
- `pods.current` = 1
- `pods.desired` = 1 (non-subscriber)

---

#### Step 8: View in Grafana

1. Open http://localhost:3001
2. Go to **Deployment Detail** dashboard
3. Select your deployment ID
4. Verify metrics match API response

---

## Troubleshooting

### Analytics Show Zero

**Symptom:** All analytics metrics are 0

**Solutions:**
1. Wait 60 seconds for first collection cycle
2. Check collector is running:
   ```bash
   curl http://localhost:8080/metrics | grep analytics_collector
   ```
3. Check database connection:
   ```sql
   SELECT COUNT(*) FROM deployment_metrics;
   ```
4. Generate traffic to deployment
5. Check control-plane logs for errors

---

### SSE Stream Not Working

**Symptom:** SSE endpoint returns error or no data

**Solutions:**
1. Verify deployment exists and is running
2. Check auth token if authentication enabled
3. Use curl with `-N` flag:
   ```bash
   curl -N http://localhost:8080/deployments/{id}/analytics/stream
   ```
4. Check browser console for CORS errors

---

### Grafana Dashboards Empty

**Symptom:** Dashboards load but show "No data"

**Solutions:**
1. Verify Prometheus datasource is connected
2. Check Prometheus is scraping control-plane:
   ```bash
   curl http://localhost:9090/api/v1/targets
   ```
3. Verify metrics exist:
   ```bash
   curl http://localhost:8080/metrics | grep deployment_
   ```
4. Check time range in dashboard (top-right)
5. Generate some traffic to create metrics

---

### Package Not Applied

**Symptom:** Deployment doesn't have expected CPU/memory

**Solutions:**
1. Check deployment YAML:
   ```bash
   kubectl -n meshvpn-apps get deploy app-{id} -o yaml
   ```
2. Verify package field in database:
   ```sql
   SELECT deployment_id, package FROM deployments WHERE deployment_id = '{id}';
   ```
3. Check control-plane logs for package validation

---

### Subdomain Conflict Not Detected

**Symptom:** Can create two deployments with same subdomain

**Solutions:**
1. Check database constraint:
   ```sql
   \d deployments
   ```
   Should have unique constraint on subdomain
2. Verify both deployments are in database:
   ```sql
   SELECT deployment_id, subdomain FROM deployments;
   ```

---

## Success Criteria

✅ **All tests pass:**
- Health check returns 200
- Deployments accept all three package sizes
- Auto-subdomain generates correctly
- Subdomain conflicts are rejected
- Analytics API returns metrics
- SSE stream sends updates every 5 seconds
- Grafana dashboards load with data
- Kubernetes resources match package specs
- Non-subscribers have no HPA
- Database tables populated correctly

✅ **Performance:**
- Deployment queue processes within 2s
- Analytics collection completes within 60s
- SSE updates arrive within 5s
- API responses < 500ms

✅ **End-to-End:**
- Complete user journey from deploy to analytics works
- Metrics appear in database, Prometheus, and Grafana
- All documentation is accurate

---

## Postman Collection Export

After creating all tests, export the collection:

1. Click **...** next to collection name
2. Click **Export**
3. Choose **Collection v2.1**
4. Save as `MeshVPN-Phase2-E2E-Tests.postman_collection.json`

Share this file with team members for consistent testing.

---

**Last Updated:** 2026-03-21
**Test Coverage:** Phase 1 + Phase 2 (Packages, Analytics, Autoscaling)
