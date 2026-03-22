# Resource Packages Documentation

This document explains the resource package system for MeshVPN deployments and how subscription status affects autoscaling behavior.

## Table of Contents

- [Overview](#overview)
- [Package Tiers](#package-tiers)
- [Subscription vs Non-Subscription](#subscription-vs-non-subscription)
- [Selecting a Package](#selecting-a-package)
- [Package Specifications](#package-specifications)
- [Autoscaling Behavior](#autoscaling-behavior)
- [Best Practices](#best-practices)
- [FAQ](#faq)

---

## Overview

MeshVPN provides three predefined resource packages for easy deployment sizing:

- **Small**: Lightweight applications, APIs, static sites
- **Medium**: Standard web applications, microservices
- **Large**: Resource-intensive applications, databases, data processing

Packages define CPU, memory, and maximum replica limits. **Subscribers** additionally get horizontal autoscaling based on CPU usage.

---

## Package Tiers

### Small Package

**Best for:**
- Static websites
- Simple APIs
- Lightweight microservices
- Development/testing environments

**Specifications:**
- CPU: 0.5 cores (500 millicores)
- Memory: 512 MB
- Max Replicas: 3 (subscribers only)

**Example use cases:**
- Next.js static site
- Express.js REST API
- Hugo/Jekyll blog
- Webhook receiver

---

### Medium Package

**Best for:**
- Standard web applications
- REST/GraphQL APIs with moderate traffic
- Background workers
- Small databases

**Specifications:**
- CPU: 1.0 core (1000 millicores)
- Memory: 1024 MB (1 GB)
- Max Replicas: 5 (subscribers only)

**Example use cases:**
- React/Vue application
- Node.js web server
- Python Flask/FastAPI app
- Redis cache
- Small PostgreSQL instance

---

### Large Package

**Best for:**
- Resource-intensive applications
- High-traffic services
- Data processing workloads
- Production databases

**Specifications:**
- CPU: 2.0 cores (2000 millicores)
- Memory: 2048 MB (2 GB)
- Max Replicas: 10 (subscribers only)

**Example use cases:**
- Large Next.js application
- Machine learning inference server
- Video processing service
- Production PostgreSQL/MySQL
- Elasticsearch node

---

## Subscription vs Non-Subscription

### Non-Subscribers (Free Tier)

- **Fixed Resources**: Package specs are applied, but scaling is disabled
- **Single Replica**: Always runs exactly 1 pod
- **No Autoscaling**: Pod count does not change based on load
- **All Packages Available**: Can still choose Small, Medium, or Large

**Example:**
```json
POST /deploy
{
  "repo": "https://github.com/user/my-app",
  "package": "medium"
}

Response:
{
  "scaling_mode": "none",
  "min_replicas": 1,
  "max_replicas": 1,
  "cpu_cores": 1.0,
  "memory_mb": 1024,
  "autoscaling_enabled": false
}
```

### Subscribers

- **Horizontal Autoscaling**: Automatically scales pods based on CPU usage
- **Dynamic Replicas**: Scales from 1 to package max_replicas
- **CPU Target**: Default 70% CPU utilization trigger
- **Customizable**: Can override scaling parameters

**Example:**
```json
POST /deploy
{
  "repo": "https://github.com/user/my-app",
  "package": "medium"
}

Response:
{
  "scaling_mode": "horizontal",
  "min_replicas": 1,
  "max_replicas": 5,
  "cpu_cores": 1.0,
  "memory_mb": 1024,
  "cpu_target_utilization": 70,
  "autoscaling_enabled": true
}
```

---

## Selecting a Package

### In API Request

Specify the package name in your deployment request:

```bash
curl -X POST http://localhost:8080/deploy \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/user/my-app",
    "package": "medium",
    "port": 3000
  }'
```

### Default Package

If you don't specify a package, **Small** is used by default:

```bash
curl -X POST http://localhost:8080/deploy \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/user/my-app",
    "port": 3000
  }'
```

### Valid Package Names

- `"small"` or `"Small"` or `"SMALL"` (case-insensitive)
- `"medium"` or `"Medium"` or `"MEDIUM"`
- `"large"` or `"Large"` or `"LARGE"`

Invalid package names will return a `400 Bad Request` error.

---

## Package Specifications

| Package | CPU Cores | CPU (millicores) | Memory (MB) | Memory (GB) | Max Replicas | Cost Indicator |
|---------|-----------|------------------|-------------|-------------|--------------|----------------|
| Small   | 0.5       | 500m             | 512         | 0.5 GB      | 3            | $            |
| Medium  | 1.0       | 1000m            | 1024        | 1.0 GB      | 5            | $$           |
| Large   | 2.0       | 2000m            | 2048        | 2.0 GB      | 10           | $$$          |

### Resource Limits

Each pod gets:
- **CPU Request**: Package CPU cores
- **CPU Limit**: 500m (safety limit to prevent runaway processes)
- **Memory Request**: Package memory MB
- **Memory Limit**: Same as request (hard limit)

**Example for Medium Package:**
```yaml
resources:
  requests:
    cpu: "1000m"
    memory: "1024Mi"
  limits:
    cpu: "500m"      # Safety limit
    memory: "1024Mi"
```

---

## Autoscaling Behavior

### For Subscribers Only

Kubernetes Horizontal Pod Autoscaler (HPA) is created with these settings:

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: app-{deployment-id}
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: app-{deployment-id}
  minReplicas: 1
  maxReplicas: {package.MaxReplicas}
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

### Scaling Triggers

- **Scale Up**: When CPU usage exceeds 70% for sustained period
- **Scale Down**: When CPU usage drops below 70% consistently
- **Max Pods**: Limited by package tier (3, 5, or 10)
- **Min Pods**: Always 1 (deployments never scale to zero)

### Scaling Timeline

- **Scale Up**: Usually 30-60 seconds after CPU threshold crossed
- **Scale Down**: 5 minutes cooldown to prevent flapping
- **Pod Startup**: Depends on application (typically 10-30 seconds)

### Custom Scaling Parameters (Subscribers Only)

Subscribers can customize scaling behavior:

```json
POST /deploy
{
  "repo": "https://github.com/user/my-app",
  "package": "large",
  "cpu_target_utilization": 80,
  "min_replicas": 2,
  "max_replicas": 8
}
```

**Constraints:**
- `cpu_target_utilization`: 1-100 (default: 70)
- `min_replicas`: Must be ≥ 1
- `max_replicas`: Cannot exceed package limit (3, 5, or 10)

---

## Best Practices

### Choosing the Right Package

1. **Start Small**: Begin with Small package and monitor performance
2. **Monitor Metrics**: Use analytics API to track CPU/memory usage
3. **Upgrade When Needed**: Move to larger package if consistently hitting limits
4. **Consider Traffic**: Estimate peak concurrent users

### CPU Guidelines

- **< 100 req/s**: Small package usually sufficient
- **100-500 req/s**: Medium package recommended
- **> 500 req/s**: Large package or multiple deployments

### Memory Guidelines

- **Static Sites**: Small (< 512 MB)
- **Node.js APIs**: Medium (512-1024 MB)
- **Python/ML Apps**: Large (1024-2048 MB)
- **Databases**: Large with persistent volumes

### Autoscaling Tips (Subscribers)

1. **Set Realistic CPU Targets**: 70-80% is optimal; too low wastes resources
2. **Warm-up Considerations**: Apps with slow startup should use higher min_replicas
3. **Traffic Patterns**: If traffic is predictable, consider fixed replicas with Large package
4. **Cost vs Performance**: More replicas = higher resource usage but better availability

### Resource Optimization

```javascript
// Example: Optimize Node.js memory
const options = {
  max_old_space_size: 896  // Leave some buffer under 1024 MB
};

// Dockerfile
ENV NODE_OPTIONS="--max-old-space-size=896"
```

---

## FAQ

### Can I change my package after deployment?

Not currently. You need to create a new deployment with the desired package. We recommend:
1. Deploy with new package
2. Test the new deployment
3. Delete old deployment

### What happens if I exceed package limits?

- **CPU**: Pod is throttled (requests slow down)
- **Memory**: Pod is killed and restarted (OOMKilled)

Always monitor your analytics to stay within limits.

### Do packages affect pricing?

Package selection does not directly affect pricing, but:
- Larger packages use more cluster resources
- Subscribers with autoscaling may use more resources during traffic spikes
- Pricing is based on total resource usage across all deployments

### Can non-subscribers use autoscaling?

No. Autoscaling is a subscriber-only feature. Non-subscribers always run 1 replica regardless of package size.

### How do I become a subscriber?

Currently, subscription management is handled through the admin interface. Contact support to upgrade your account.

### What if my app needs more than 2 CPU cores?

For applications requiring > 2 cores:
1. Consider breaking into multiple microservices
2. Use Large package with autoscaling (up to 10 pods = 20 cores total)
3. Contact support for custom enterprise packages

### Can I mix packages across deployments?

Yes! Each deployment can use a different package. Example:
- Frontend: Small package (static Next.js)
- API: Medium package (Node.js backend)
- Workers: Large package (data processing)

### How does autoscaling work with request queuing?

Kubernetes HPA only monitors CPU/memory, not request queue depth. For queue-based scaling:
1. Use custom metrics (requires KEDA)
2. Contact support for advanced scaling configuration

### What's the difference between CPU request and limit?

- **Request**: Guaranteed CPU allocation (used for scheduling)
- **Limit**: Maximum CPU the pod can use (500m safety cap)

The safety limit prevents a single deployment from consuming excessive cluster resources.

---

## Related Documentation

- [Analytics API](./ANALYTICS-API.md) - Monitor resource usage and performance
- [Setup Guide](./SETUP.md) - Initial deployment setup
- [Grafana Setup](./GRAFANA-SETUP.md) - Platform monitoring dashboards

---

## Support

For package-related questions or custom requirements, please open an issue at: https://github.com/anthropics/claude-code/issues
