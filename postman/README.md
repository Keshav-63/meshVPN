# Postman Collection for MeshVPN Testing

This folder contains Postman collections and environments for comprehensive end-to-end testing of the MeshVPN platform.

## Files

- `MeshVPN-Phase2-E2E-Tests.postman_collection.json` - Complete test suite for all Phase 2 features
- `MeshVPN-Local.postman_environment.json` - Local development environment variables

## Quick Start

### 1. Import Collection

1. Open Postman Desktop or Postman Web
2. Click **Import** button (top-left)
3. Drag and drop `MeshVPN-Phase2-E2E-Tests.postman_collection.json`
4. Click **Import**

### 2. Import Environment

1. Click **Import** button
2. Drag and drop `MeshVPN-Local.postman_environment.json`
3. Click **Import**

### 3. Select Environment

1. Click environment dropdown (top-right)
2. Select **MeshVPN Local Development**

### 4. Configure Environment Variables

Click the eye icon next to the environment dropdown to edit variables:

| Variable | Description | Example Value |
|----------|-------------|---------------|
| `base_url` | Local control-plane API | `http://localhost:8080` |
| `public_url` | Public Cloudflare URL | `https://self.keshavstack.tech` |
| `domain` | Your base domain | `keshavstack.tech` |
| `auth_token` | JWT token (if auth enabled) | Leave empty if `REQUIRE_AUTH=false` |
| `deployment_id` | Auto-set by tests | Leave empty |
| `test_repo` | Test repository URL | `https://github.com/vercel/next.js` |
| `test_subdomain` | Test subdomain name | `test-app` |

### 5. Run Tests

#### Option A: Run Entire Collection

1. Right-click collection name
2. Click **Run collection**
3. Click **Run MeshVPN Phase 2 - Complete E2E Tests**

**Note:** Some tests depend on previous tests (e.g., analytics tests need a deployment to exist). Run in order for best results.

#### Option B: Run Individual Tests

1. Expand collection folders
2. Click on individual request
3. Click **Send**

## Test Organization

### 1. Health & Metrics
- Health Check
- Prometheus Metrics

### 2. Deployment with Packages
- Deploy Small Package (Auto-Subdomain)
- Deploy Medium Package (Custom Subdomain)
- Deploy Large Package
- Deploy Invalid Package (Should Fail)
- Deploy Default Package (No Package Specified)

### 3. Deployment Management
- List All Deployments
- Get Build Logs
- Get App Logs (Default Tail)
- Get App Logs (Custom Tail)

### 4. Analytics API
- Get Analytics Snapshot
- Analytics for Non-Existent Deployment

### 5. Subdomain Testing
- Deploy with Subdomain A
- Deploy with Same Subdomain (Should Fail)

## Test Scripts

Each test includes assertions to verify:
- HTTP status codes
- Response structure
- Field values
- Error messages
- Package specifications
- Analytics metrics

## Environment Variables Auto-Set

Some variables are automatically set by test scripts:

- `deployment_id` - Set after successful deployment for use in subsequent tests

## Prerequisites

Before running tests, ensure:

1. **Control-plane is running:**
   ```bash
   cd ~/MeshVPN-slef-hosting/control-plane
   go run ./cmd/control-plane
   ```

2. **K3D cluster is running:**
   ```bash
   k3d cluster list
   ```

3. **Database is accessible:**
   ```bash
   echo $DATABASE_URL
   ```

4. **Observability stack is running:**
   ```bash
   cd ~/MeshVPN-slef-hosting/infra/observability
   docker compose ps
   ```

## Common Issues

### "Connection refused" errors

- Verify control-plane is running on port 8080
- Check `base_url` environment variable

### "deployment not found" in analytics tests

- Ensure you've run deployment tests first
- Check `deployment_id` variable is set
- Wait for deployment to reach "running" status

### Authentication errors

- If `REQUIRE_AUTH=true`, set `auth_token` variable
- If `REQUIRE_AUTH=false`, leave `auth_token` empty

## Advanced Usage

### Running with Newman (CLI)

Install Newman:
```bash
npm install -g newman
```

Run collection:
```bash
newman run MeshVPN-Phase2-E2E-Tests.postman_collection.json \
  -e MeshVPN-Local.postman_environment.json
```

### Generating HTML Report

```bash
newman run MeshVPN-Phase2-E2E-Tests.postman_collection.json \
  -e MeshVPN-Local.postman_environment.json \
  -r htmlextra \
  --reporter-htmlextra-export test-results.html
```

### CI/CD Integration

Add to GitHub Actions:

```yaml
- name: Run Postman Tests
  run: |
    npm install -g newman
    newman run postman/MeshVPN-Phase2-E2E-Tests.postman_collection.json \
      -e postman/MeshVPN-Local.postman_environment.json \
      --bail
```

## Test Coverage

This collection covers:

- ✅ Resource packages (Small/Medium/Large)
- ✅ Auto-subdomain generation
- ✅ Subdomain conflict detection
- ✅ Package validation
- ✅ Deployment queue
- ✅ Build logs
- ✅ Application logs
- ✅ Analytics API
- ✅ Metrics structure
- ✅ Error handling

## Related Documentation

- [E2E Testing Guide](../docs/E2E-TESTING.md) - Complete testing procedures
- [Analytics API](../docs/ANALYTICS-API.md) - Analytics endpoint documentation
- [Packages](../docs/PACKAGES.md) - Resource package specifications
- [Setup Guide](../docs/SETUP.md) - Platform setup instructions

## Support

For issues or questions about testing:
- Check the [E2E Testing Guide](../docs/E2E-TESTING.md)
- Review [Troubleshooting](../docs/SETUP.md#troubleshooting)
- Open an issue at https://github.com/anthropics/claude-code/issues
