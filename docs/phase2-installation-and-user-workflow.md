# Phase 2 Installation, Roles, and Complete User Workflow Testing

## 1) Do you still need Docker?

Short answer: yes.

In this Phase 2 implementation, Docker is still required even when runtime backend is k3s.

Why:

- The control-plane currently builds and pushes images using Docker CLI.
- k3s then pulls those images and runs pods.
- So Docker is no longer the final app scheduler in k3s mode, but it is still the image build and push engine.

Runtime roles by mode:

- Docker mode:
  - Build image with Docker
  - Run container directly with Docker
  - Traefik labels on container handle routing
- k3s mode:
  - Build image with Docker
  - Push image to registry
  - k3s schedules pods, service, ingress, HPA

## 2) What to install (minimum)

Install these on the control-plane laptop:

- Git
- Go
- Docker Desktop (or Docker Engine)
- kubectl
- Helm
- Tailscale
- k3s server

Install these on each worker laptop:

- Docker Desktop (or Docker Engine)
- Tailscale
- k3s agent

Install in cluster (via kubectl/Helm):

- metrics-server
- kube-prometheus-stack (Prometheus, Grafana, kube-state-metrics, node-exporter)

Optional later:

- OpenTelemetry collector
- Loki or another long-term logs backend

## 3) Required accounts/services

- Container registry where control-plane can push and k3s can pull images
  - Example: ghcr.io
- DNS/edge routing strategy for app domains (for public access)
  - Cloudflare tunnel or equivalent

## 4) Core configuration checklist

Control-plane environment variables:

- DATABASE_URL or SUPABASE_DB_URL
- SUPABASE_JWT_SECRET
- REQUIRE_AUTH=true
- RUNTIME_BACKEND=k3s
- ENABLE_CPU_HPA=true
- K8S_NAMESPACE=meshvpn-apps
- K8S_IMAGE_PREFIX=your-registry-prefix
- WORKER_POLL_INTERVAL=2s

Optional:

- KUBECTL_BIN=kubectl
- K8S_CONFIG_PATH=path-to-kubeconfig

Cluster requirements:

- metrics-server healthy
- ingress controller available
- namespace exists or can be created
- image pull permissions configured for your registry

## 5) Server and worker bootstrap sequence

1. Join all laptops to Tailscale same tailnet.
2. Verify node-to-node connectivity over Tailscale.
3. Install k3s server on control-plane laptop.
4. Join worker laptops as k3s agents.
5. Verify nodes are Ready.
6. Install metrics-server.
7. Install kube-prometheus-stack with low-footprint values.
8. Confirm Prometheus targets are up.

## 6) Complete user workflow testing (end-to-end)

Use this exact order for UAT.

### Stage A: Control-plane baseline

1. Start control-plane.
2. Verify:
   - GET /health returns 200
   - GET /metrics returns Prometheus metrics

### Stage B: Deployment submission flow

1. Submit deploy request with:
   - scaling_mode=horizontal
   - min_replicas, max_replicas
   - cpu_target_utilization
   - cpu_request_milli
2. Expect API response:
   - 202 accepted
   - status=queued

### Stage C: Worker execution flow

1. Verify deployment transitions:
   - queued -> deploying -> running
2. Verify build logs endpoint returns clone/build/apply/rollout sections.

### Stage D: Kubernetes object verification

For returned deployment id, verify in namespace:

- Deployment exists
- Service exists
- Ingress exists
- HPA exists (when enabled and horizontal mode)

### Stage E: Functional traffic flow

1. Open app URL.
2. Confirm app responds.
3. Confirm ingress routes to service and service routes to pods.

### Stage F: Autoscaling behavior

1. Generate sustained traffic for 3 to 5 minutes.
2. Watch desired/current replica count.
3. Confirm scale-up when CPU exceeds target.
4. Stop load and confirm gradual scale-down (stabilized behavior).

### Stage G: Observability validation

In Grafana, verify panels for:

- request rate and latency by route
- pod CPU and memory
- node CPU and memory
- HPA desired vs current replicas
- control-plane worker metrics

### Stage H: Multi-worker load distribution

1. Confirm pods are scheduled across control-plane and worker nodes.
2. Confirm request traffic is served while pods are on different workers.

### Stage I: Failure and recovery

1. Keep traffic running.
2. Stop one worker node.
3. Confirm traffic still served from remaining healthy pods.
4. Restart worker and verify cluster recovers cleanly.

### Stage J: Rollback test

1. Switch to docker backend mode.
2. Restart control-plane.
3. Confirm deploy still works in docker mode.

## 7) Acceptance criteria for Phase 2 completion

- k3s backend deploy path succeeds for sample app.
- CPU-first HPA scales out and scales in under load.
- p95 remains within your target band under normal load profile.
- request and resource visibility works in Grafana.
- service remains available after worker failure.
- docker fallback mode still works.

## 8) Common mistakes to avoid

- Missing K8S_IMAGE_PREFIX causes k3s deploy failures.
- Missing metrics-server causes HPA to stay inactive.
- Not setting CPU requests breaks utilization-based HPA decisions.
- Missing registry pull auth causes pod ImagePullBackOff.
- Expecting autoscaling to be instant during sudden spikes.
