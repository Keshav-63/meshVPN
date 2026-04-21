# Multi-Worker Architecture with Tailscale

## Overview

A distributed deployment system where multiple worker machines can process deployment jobs in parallel, connected via Tailscale mesh network.

## Components

### 1. Control Plane (Coordinator)
- Receives deployment requests via HTTP API
- Queues jobs in database
- Distributes jobs to available workers
- Monitors worker health
- Tracks deployment status

### 2. Worker Agents (Remote)
- Runs on separate machines (laptops, servers, cloud VMs)
- Connects to control-plane via Tailscale
- Pulls jobs from assigned queue
- Executes deployments using local Kubernetes/Docker
- Reports status back to control-plane

### 3. Tailscale Mesh Network
- Secure point-to-point connectivity
- No port forwarding needed
- NAT traversal built-in
- Workers can be behind firewalls

## Data Flow

```
User → Control-Plane API → Job Queue
                              ↓
                    [Job Distribution Logic]
                              ↓
            ┌─────────────────┼─────────────────┐
            ↓                 ↓                 ↓
        Worker-1          Worker-2          Worker-N
     (Tailscale IP)    (Tailscale IP)    (Tailscale IP)
            ↓                 ↓                 ↓
      Deploy to K8s     Deploy to K8s     Deploy to K8s
```

    ## Final Runtime Logic (Source of Truth)

    This section is the authoritative runtime behavior implemented in the current code.

    ### Deployment State Machine

    Allowed deployment statuses:

    - `queued`: Request accepted and job is waiting for assignment.
    - `deploying`: Job has been assigned/claimed and deployment is in progress.
    - `running`: Deployment completed successfully.
    - `failed`: Deployment or worker execution failed.

    State transitions:

    1. API enqueue creates deployment in `queued`.
    2. Distributor assigns job to worker and marks deployment `deploying` with `owner_worker_id`.
    3. Worker success path marks deployment `running`.
    4. Worker failure path marks deployment `failed` and stores error/build logs.
    5. Failover/rebalance re-queue path moves deployment back to `queued`, clears owner, and creates a new job.

    ### Job State Machine

    Allowed job statuses in persistence:

    - `queued`: Ready for assignment/claim.
    - `running`: Claimed by a worker.
    - `done`: Completed successfully.
    - `failed`: Completed with error.

    State transitions:

    1. Enqueue inserts job as `queued`.
    2. Claim (`ClaimNext` or `ClaimForWorker`) sets job `running`.
    3. Worker completion callback sets job `done`.
    4. Worker failure callback sets job `failed`.

    ### Assignment and Capacity Rules

    1. Distributor picks only unassigned queued jobs.
    2. Distributor assigns worker only when worker is not `offline` and `current_jobs < max_concurrent_jobs`.
    3. Distributor increments worker `current_jobs` only after successful assignment.
    4. If increment fails, assignment is released immediately (job is not left stuck assigned).
    5. In multi-worker mode, embedded control-plane worker claims only jobs assigned to its own `worker_id`.
    6. Worker `current_jobs` is decremented after each claimed assigned job completes or fails.

    ### Health, Failover, and Rebalance Rules

    1. Worker heartbeat timeout marks remote worker `offline`.
    2. Running deployments owned by offline workers are re-queued for failover.
    3. Rebalance moves running deployments only when:
       - cooldown window has passed, and
       - score delta exceeds minimum threshold.
    4. Re-queued deployments receive a fresh job id and remain traceable via deployment id.

    ### Callback Guarantees

    Worker callback endpoints enforce strict updates:

    1. `job-complete` must mark job `done` successfully before decrementing worker load.
    2. `job-failed` must mark job `failed` successfully before decrementing worker load.
    3. Heartbeat updates fail fast on invalid payloads or missing workers.
    4. Database updates that affect zero rows are treated as errors (not silent success).

## Database Schema

### Workers Table
```sql
CREATE TABLE workers (
    worker_id VARCHAR(64) PRIMARY KEY,        -- UUID
    name VARCHAR(255) NOT NULL,                -- Human-readable name
    tailscale_ip VARCHAR(64) NOT NULL,         -- 100.x.x.x
    hostname VARCHAR(255),                     -- Worker machine hostname
    status VARCHAR(32) DEFAULT 'idle',         -- idle, busy, offline
    capabilities JSONB,                        -- {docker: true, k8s: true, memory_gb: 16}
    max_concurrent_jobs INT DEFAULT 1,
    current_jobs INT DEFAULT 0,
    last_heartbeat TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
```

### Jobs Table (Extended)
```sql
ALTER TABLE jobs ADD COLUMN assigned_worker_id VARCHAR(64);
ALTER TABLE jobs ADD COLUMN assigned_at TIMESTAMP;
```

## Worker Registration

1. Worker agent starts and gets Tailscale IP
2. Worker calls `/workers/register` with:
   - Worker ID (generated or from config)
   - Tailscale IP
   - Capabilities (CPU, memory, supported runtimes)
3. Control-plane stores worker in database
4. Worker starts heartbeat loop (every 30s)

## Job Assignment Strategies

### Strategy 1: Pull Model (Current)
- Workers poll `/jobs/claim` endpoint
- Control-plane marks job as assigned to worker
- Simple, but requires workers to poll

### Strategy 2: Push Model (Recommended)
- Control-plane maintains worker queue
- On new job, control-plane finds available worker
- Control-plane calls worker's `/execute-job` endpoint
- Worker processes and reports back

### Strategy 3: Hybrid
- Workers register and send capabilities
- Workers expose `/execute-job` endpoint
- Control-plane pushes jobs to workers
- Workers can also pull if idle

## Worker Capabilities

```json
{
  "runtime": "kubernetes",
  "k8s_version": "v1.31",
  "memory_gb": 16,
  "cpu_cores": 8,
  "max_concurrent_jobs": 3,
  "supported_packages": ["small", "medium", "large"]
}
```

## Communication Protocol

### Worker → Control-Plane

#### Register
```
POST http://control-plane.tailnet:8080/api/workers/register
{
  "worker_id": "worker-abc123",
  "name": "laptop-worker",
  "tailscale_ip": "100.64.1.2",
  "hostname": "keshav-laptop",
  "capabilities": {...}
}
```

#### Heartbeat
```
POST http://control-plane.tailnet:8080/api/workers/{id}/heartbeat
{
  "status": "idle",
  "current_jobs": 0,
  "memory_used_mb": 4096
}
```

#### Report Job Status
```
POST http://control-plane.tailnet:8080/api/jobs/{id}/status
{
  "status": "running|failed|completed",
  "build_logs": "...",
  "error": "..."
}
```

### Control-Plane → Worker

#### Assign Job
```
POST http://{worker.tailscale_ip}:8081/execute-job
{
  "job_id": "job-123",
  "deployment_id": "abc456",
  "repo": "https://github.com/user/repo",
  "package": "small",
  "port": 3000,
  "env": {...},
  "build_args": {...}
}
```

## Implementation Phases

### Phase 1: Worker Registry ✓
- [ ] Create worker domain models
- [ ] Create worker store/repository
- [ ] Add worker registration API endpoint
- [ ] Add worker heartbeat endpoint
- [ ] Add worker list/status endpoints

### Phase 2: Worker Agent ✓
- [ ] Create standalone worker agent binary
- [ ] Implement job executor
- [ ] Implement status reporting
- [ ] Add Tailscale IP detection
- [ ] Add configuration file support

### Phase 3: Job Distribution ✓
- [ ] Update job queue to track assigned worker
- [ ] Implement worker selection algorithm
- [ ] Update deployment service to push jobs to workers
- [ ] Handle worker failures/timeouts

### Phase 4: Monitoring ✓
- [ ] Worker health checks
- [ ] Job retry on worker failure
- [ ] Worker capacity tracking
- [ ] Metrics (worker count, job distribution, success rate)

### Phase 5: Advanced Features
- [ ] Worker tags/labels (gpu, high-memory, etc.)
- [ ] Job affinity (prefer certain workers)
- [ ] Load balancing across workers
- [ ] Worker auto-scaling integration

## Configuration

### Control-Plane .env
```env
ENABLE_MULTI_WORKER=true
WORKER_HEARTBEAT_TIMEOUT=90s
WORKER_JOB_TIMEOUT=600s
TAILSCALE_HOSTNAME=control-plane
```

### Worker agent.yaml
```yaml
worker:
  id: worker-laptop-1
  name: "Keshav's Laptop"
  control_plane_url: http://control-plane.tailnet:8080
  api_port: 8081
  max_concurrent_jobs: 2

runtime:
  type: kubernetes
  kubeconfig: /home/user/.kube/config
  namespace: worker-apps

tailscale:
  auto_detect_ip: true
```

## Security Considerations

1. **Authentication**: Workers authenticate using shared secret or JWT
2. **Tailscale ACLs**: Restrict which workers can connect
3. **Job Validation**: Workers validate jobs before execution
4. **Resource Limits**: Workers enforce resource quotas
5. **Secure Logs**: Sanitize logs before sending to control-plane

## Migration Path

1. Deploy new worker table schema
2. Keep existing single-worker mode as default
3. Add `ENABLE_MULTI_WORKER=false` flag
4. Deploy worker agent to first remote machine
5. Test job execution
6. Enable multi-worker mode
7. Add more workers
8. Deprecate embedded worker

## Testing

- Unit tests for worker store
- Integration tests for worker registration
- E2E tests for job distribution
- Load tests with multiple workers
- Failure scenarios (worker crash, network partition)

## Monitoring Queries (Prometheus)

```promql
# Total workers
meshvpn_workers_total

# Workers by status
meshvpn_workers_by_status{status="idle|busy|offline"}

# Jobs per worker
meshvpn_jobs_per_worker

# Worker job duration
histogram_quantile(0.95, meshvpn_worker_job_duration_seconds)
```

---

**Next Steps**: Implement Phase 1 - Worker Registry
