# Testing Quick Start Guide

Quick reference for testing all MeshVPN Phase 2 features.

## Prerequisites

Ensure all services are running:

```bash
# 1. K3D Cluster
k3d cluster start meshvpn
kubectl get nodes  # Should show 1 node Ready

# 2. Control-Plane
cd ~/MeshVPN-slef-hosting/control-plane
export DATABASE_URL="your_postgres_url"
export REQUIRE_AUTH=false  # Simplifies testing
go run ./cmd/control-plane
# Should show: "Listening on :8080"

# 3. Cloudflare Tunnel
cd ~/MeshVPN-slef-hosting/infra
docker compose up -d
docker logs cloudflared-1  # Check connection

# 4. Observability Stack
cd ~/MeshVPN-slef-hosting/infra/observability
docker compose up -d
docker ps  # Verify prometheus and grafana running
```

## Quick Health Check

```bash
# Local health check
curl http://localhost:8080/health
# Expected: {"status":"LaptopCloud running"}

# Public health check (via Cloudflare)
curl https://self.keshavstack.tech/health

# Prometheus
curl http://localhost:9090/-/healthy

# Grafana
curl http://localhost:3001/api/health
```

## Method 1: Automated CLI Testing (Fastest)

```bash
cd ~/MeshVPN-slef-hosting
./scripts/test-e2e.sh
```

**Output:** Colorized test results with pass/fail indicators.

**Duration:** ~30 seconds

---

## Method 2: Postman Testing (Recommended)

### Setup (One-time)

1. **Import Collection**
   ```
   File: postman/MeshVPN-Phase2-E2E-Tests.postman_collection.json
   ```

2. **Import Environment**
   ```
   File: postman/MeshVPN-Local.postman_environment.json
   ```

3. **Select Environment**
   - Click dropdown (top-right)
   - Select "MeshVPN Local Development"

### Run Tests

**Option A: Run Entire Collection**
1. Right-click collection
2. Click "Run collection"
3. Click "Run"

**Option B: Run Individual Tests**
1. Expand folders
2. Click request
3. Click "Send"

### Key Tests to Run

1. **Health & Metrics** → Health Check
2. **Deployment with Packages** → Deploy Small Package
3. **Deployment with Packages** → Deploy Medium Package
4. **Analytics API** → Get Analytics Snapshot
5. **Subdomain Testing** → Both tests (conflict detection)

---

## Method 3: Manual curl Testing

### Test 1: Deploy Small Package

```bash
curl -X POST http://localhost:8080/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/vercel/next.js",
    "package": "small",
    "port": 3000
  }' | jq

# Save deployment_id from response
DEPLOYMENT_ID="<deployment_id_from_response>"
```

**Expected Response:**
```json
{
  "message": "deployment queued",
  "deployment_id": "74b295d2",
  "package": "small",
  "cpu_cores": 0.5,
  "memory_mb": 512,
  "scaling_mode": "none",
  "autoscaling_enabled": false,
  "subdomain": "next-js",
  "url": "https://next-js.keshavstack.tech"
}
```

---

### Test 2: List Deployments

```bash
curl http://localhost:8080/deployments | jq
```

---

### Test 3: Get Analytics

```bash
# Wait 60 seconds for metrics collection
sleep 60

curl http://localhost:8080/deployments/$DEPLOYMENT_ID/analytics | jq
```

**Expected Response:**
```json
{
  "deployment_id": "74b295d2",
  "deployment": { ... },
  "metrics": {
    "requests": {
      "total": 0,
      "last_hour": 0,
      "per_second": 0
    },
    "latency": {
      "p50_ms": 0,
      "p90_ms": 0,
      "p99_ms": 0
    },
    "pods": {
      "current": 1,
      "desired": 1
    }
  }
}
```

---

### Test 4: SSE Stream (Real-time Analytics)

```bash
curl -N http://localhost:8080/deployments/$DEPLOYMENT_ID/analytics/stream
```

**Expected:** Updates every 5 seconds in SSE format

Press Ctrl+C to stop.

---

### Test 5: Package Validation

```bash
# Invalid package - should fail
curl -X POST http://localhost:8080/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/test/test",
    "package": "xlarge",
    "port": 3000
  }' | jq

# Expected: {"error": "invalid package 'xlarge'. must be: small, medium, large"}
```

---

### Test 6: Subdomain Conflict

```bash
# First deployment
curl -X POST http://localhost:8080/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/test/app1",
    "subdomain": "test-conflict",
    "package": "small",
    "port": 3000
  }' | jq

# Second deployment with same subdomain - should fail
curl -X POST http://localhost:8080/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/test/app2",
    "subdomain": "test-conflict",
    "package": "small",
    "port": 3000
  }' | jq

# Expected: {"error": "subdomain 'test-conflict' is already in use"}
```

---

## Database Verification

Connect to your PostgreSQL database:

```bash
psql "$DATABASE_URL"
```

### Check Tables Exist

```sql
\dt
```

**Expected Tables:**
- deployments
- deployment_jobs
- users
- deployment_metrics
- deployment_requests

---

### Check Users Table

```sql
SELECT * FROM users;
```

**Expected:** Rows created on first login (if auth enabled)

---

### Check Deployment Metrics

```sql
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

**Expected:** Recent metrics (updated every 60 seconds)

---

### Check Deployments Have Package

```sql
SELECT
  deployment_id,
  repo,
  subdomain,
  package,
  status
FROM deployments
ORDER BY created_at DESC
LIMIT 5;
```

**Expected:** All deployments have package field (small/medium/large)

---

## Grafana Dashboard Testing

### 1. Access Grafana

```
http://localhost:3001
```

**Login:** Not required (anonymous access enabled)

---

### 2. Platform Overview Dashboard

1. Click **Dashboards** → **Browse**
2. Open **MeshVPN** folder
3. Click **Platform Overview**

**Verify:**
- Total Active Deployments shows count
- Request Rate graph displays
- All panels load without errors

---

### 3. Deployment Detail Dashboard

1. Navigate to **Deployment Detail**
2. Select deployment from dropdown

**Verify:**
- Request Rate graph shows data
- Latency Percentiles display
- Pod Count gauge shows 1/1 (for non-subscribers)
- CPU/Memory graphs load

---

## Kubernetes Verification

### Check Deployment Resources

```bash
kubectl -n meshvpn-apps get deploy -o yaml | grep -A 10 resources
```

**Expected (for Small package):**
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

### Check No HPA for Non-Subscribers

```bash
kubectl -n meshvpn-apps get hpa
```

**Expected:** No resources found (autoscaling disabled)

---

### Check Pods Running

```bash
kubectl -n meshvpn-apps get pods
```

**Expected:** Pods in Running state

---

## Common Issues & Solutions

### "Connection refused" on localhost:8080

**Solution:**
```bash
# Check control-plane is running
ps aux | grep control-plane

# Restart if needed
cd ~/MeshVPN-slef-hosting/control-plane
go run ./cmd/control-plane
```

---

### "deployment not found" in analytics

**Solution:**
- Wait for deployment to reach "running" status
- Check deployment exists: `curl http://localhost:8080/deployments`
- Verify deployment_id is correct

---

### Analytics show all zeros

**Solution:**
- Wait 60 seconds for first metrics collection
- Generate traffic to the deployment
- Check analytics collector is running:
  ```bash
  curl http://localhost:8080/metrics | grep analytics
  ```

---

### Grafana dashboards empty

**Solution:**
1. Verify Prometheus datasource connected
2. Check time range (top-right in Grafana)
3. Generate some deployments to create metrics
4. Wait 60 seconds for metrics collection

---

## Success Criteria

✅ All services running (K3D, control-plane, cloudflared, prometheus, grafana)

✅ Health checks pass (local and public)

✅ Deployments accept all package sizes (small/medium/large)

✅ Auto-subdomain generates from repo name

✅ Subdomain conflicts are rejected

✅ Analytics API returns metrics structure

✅ Grafana dashboards load with data

✅ Database tables populated

✅ Kubernetes resources match package specs

✅ Non-subscribers have no HPA (autoscaling disabled)

---

## Full Test Coverage

For comprehensive testing, see:

- **[E2E Testing Guide](docs/E2E-TESTING.md)** - Complete testing procedures
- **[Postman Collection](postman/)** - Pre-built test suite
- **[Automated Script](scripts/test-e2e.sh)** - CLI testing

---

## Next Steps After Testing

1. **Review Analytics** - Check metrics in Grafana
2. **Test Real Deployment** - Deploy your own application
3. **Monitor Performance** - Watch resource usage
4. **Explore Documentation:**
   - [ANALYTICS-API.md](docs/ANALYTICS-API.md)
   - [PACKAGES.md](docs/PACKAGES.md)
   - [GRAFANA-SETUP.md](docs/GRAFANA-SETUP.md)

---

**Last Updated:** 2026-03-21
**Covers:** Phase 1 + Phase 2 (All Features)
