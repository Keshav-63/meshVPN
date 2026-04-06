# MeshVPN Dynamic Placement, Failover, and Failback Architecture

## Purpose

This document defines the target architecture and implementation plan for:

1. Dynamic best-target deployment placement
2. Automatic failover when a worker goes offline
3. Controlled failback or rebalance when workers recover
4. Registry-first image reuse for faster takeover
5. End-to-end test and dry-run strategy before production rollout

The focus is service availability first, correctness second, and performance third.

---

## Design Principles

1. Availability first: user deployment URL should recover automatically after worker loss.
2. Deterministic decisions: every placement and migration must be explainable by score and policy.
3. Idempotent operations: repeated events must converge to one valid deployment state.
4. Safe migrations: no aggressive ping-pong between workers.
5. Observable behavior: every state transition must be visible in logs and metrics.
6. Backward compatibility: existing single-worker and local-first modes must continue to work.

---

## Current Platform Reality

### Existing strengths

1. Worker registry and heartbeat endpoints are already available.
2. Distributor supports assignment of queued jobs to workers.
3. Control-plane embedded worker can run deployments.
4. Deployments and jobs persist in PostgreSQL.
5. GHCR image push path already exists in runtime workers.

### Current gaps to close

1. Running deployments are not actively reconciled if owner worker becomes offline.
2. Deployment ownership is not fully modeled for migration logic.
3. No failover controller currently re-homes running workloads.
4. No policy-based failback or rebalance.
5. No deterministic registry-first image reuse contract in shift workflows.

---

## Target Architecture Components

## 1) Placement Engine

Single policy module used by:

1. New deployment scheduling
2. Offline failover target selection
3. Online rebalance target selection

Inputs:

1. Worker health and heartbeat age
2. Worker capabilities and current load
3. Deployment resource request profile
4. Policy knobs from environment configuration

Output:

1. Ranked list of target workers with score and reason

## 2) Worker Health Controller

Responsibilities:

1. Detect stale heartbeats beyond timeout
2. Mark workers online or offline
3. Emit worker state change events

## 3) Deployment Ownership Model

Each running deployment must have an explicit owner worker ID and migration metadata.

Minimum fields:

1. owner_worker_id
2. placement_generation
3. failover_state
4. failover_count
5. last_failover_at
6. migration_cooldown_until

## 4) Failover Controller

Responsibilities:

1. Watch offline worker transitions
2. Identify impacted running deployments
3. Create takeover jobs and assign best target
4. Verify health after takeover
5. Update ownership and status

## 5) Rebalance and Failback Controller

Responsibilities:

1. Evaluate recovered workers after warmup
2. Move deployments only when benefit threshold is met
3. Respect cooldown and migration budget limits

## 6) Registry Resolver

Responsibilities:

1. Build deterministic image tag
2. Check image existence in registry
3. Reuse image when available
4. Build and push only when image missing

---

## End-to-End Deployment Flow

## A. New Deployment Request

1. API receives deploy request.
2. Deployment record created with status queued.
3. Placement engine computes best target among healthy workers.
4. If remote worker has best score and capacity, assign to that worker.
5. If none fit, fallback to control-plane worker.
6. If no capacity at all, remain queued and retry.
7. Worker claims job and executes.
8. Worker uses registry-first logic:
   1. If image tag exists, skip build and deploy.
   2. If image tag missing, build then push then deploy.
9. Post-deploy health check passes.
10. Deployment status set running and owner_worker_id set to selected worker.

## B. Worker Offline Failover

1. Health controller marks worker offline after timeout.
2. Failover controller queries running deployments owned by offline worker.
3. Each deployment enters failover_pending.
4. Placement engine ranks alternatives excluding offline worker.
5. Takeover job assigned to best candidate.
6. Candidate redeploys using registry-first image reuse.
7. Health check succeeds:
   1. owner_worker_id updated
   2. status set running
   3. failover event recorded
8. If takeover fails:
   1. Try next candidate with bounded retries
   2. If exhausted, keep failover_pending with retry backoff

## C. Worker Recovery and Controlled Failback

1. Worker heartbeat returns and remains stable for warmup duration.
2. Rebalance controller evaluates candidate deployments currently elsewhere.
3. Move occurs only when all conditions are true:
   1. Positive score delta exceeds threshold
   2. Target has required headroom
   3. Deployment is outside cooldown window
   4. Migration budget for period is available
4. If not all conditions are met, deployment remains where it is.
5. If moved, perform safe handoff and update ownership.

---

## Placement and Shift Decision Logic

## Worker eligibility filter

A worker is eligible only if:

1. status is online-idle or online-busy with available slots
2. heartbeat age is within timeout
3. runtime and namespace compatibility matches job
4. required cpu and memory fit projected free capacity

## Score function

Base score can be computed as weighted sum:

1. Capacity score: free_cpu_ratio and free_memory_ratio
2. Load score: inverse of current_jobs and queue pressure
3. Reliability score: recent success rate and failure penalty
4. Locality score: optional preference for current owner to avoid unnecessary moves

Example conceptual score:

S = 0.40 * capacity + 0.25 * load + 0.25 * reliability + 0.10 * locality

Policy:

1. Highest score wins
2. Control-plane is always included as fallback candidate when enabled
3. Tie-break with lower current_jobs then deterministic worker ID ordering

## Rebalance guardrails

1. Minimum score improvement threshold, for example 0.20
2. Cooldown duration after any migration
3. Max migrations per window globally
4. Max migrations per deployment per hour

---

## Registry-First Image Strategy

## Image identity

Use deterministic immutable tag from source revision:

1. deployment ID
2. repo URL fingerprint
3. commit SHA

Recommended tag format:

ghcr.io/org/app-<deployment_id>:<commit_sha>

## Execution behavior

1. Resolve desired tag.
2. Check tag existence in registry.
3. If exists, deploy immediately with existing image.
4. If missing, build and push then deploy.
5. Record image_digest in deployment metadata for traceability.

Benefits:

1. Faster failover and failback
2. Reproducible runtime artifact
3. Reduced rebuild cost

---

## Data Model and API Changes

## Deployment schema additions

1. owner_worker_id TEXT
2. placement_generation INT
3. failover_state TEXT
4. failover_count INT
5. last_failover_at TIMESTAMPTZ
6. migration_cooldown_until TIMESTAMPTZ
7. image_tag TEXT
8. image_digest TEXT

## Worker schema additions

1. health_state TEXT
2. last_seen_online_at TIMESTAMPTZ
3. reliability_score DOUBLE PRECISION

## Optional events table

Deployment events for auditability:

1. placement_selected
2. failover_started
3. failover_succeeded
4. failover_failed
5. rebalance_started
6. rebalance_succeeded
7. rebalance_skipped

---

## Controllers and Intervals

## Health controller

1. Tick every 10 seconds
2. Detect heartbeat timeout and set offline
3. Detect recovery and set online

## Failover controller

1. Trigger on worker offline transition and periodic reconcile
2. Reconcile interval 15 to 30 seconds

## Rebalance controller

1. Evaluate every 60 to 180 seconds
2. Respect migration budget and cooldown

---

## Coding Principles for Implementation

1. Single responsibility services with clear boundaries.
2. Use explicit interfaces for placement engine and reconciler.
3. Make all migration operations idempotent.
4. Use transactional updates for deployment ownership changes.
5. Keep side effects after durable state transitions.
6. Add structured logs with deployment_id, worker_id, reason, score.
7. Fail closed on ambiguous state, then retry via reconcile loop.
8. Avoid hidden magic numbers; all thresholds configurable.

---

## Implementation Plan (Phased)

## Phase 0: Safety groundwork

1. Add feature flags:
   1. ENABLE_WORKER_FAILOVER
   2. ENABLE_WORKER_REBALANCE
2. Add no-op scaffolding and metrics wiring.
3. Add migration files for schema extensions.

Exit criteria:

1. Build passes
2. Existing deployment paths unchanged when flags are off

## Phase 1: Ownership and health truth

1. Persist owner_worker_id at successful deployment completion.
2. Add health controller for heartbeat timeout and recovery.
3. Expose worker health state in existing worker APIs.

Exit criteria:

1. Any running deployment has owner_worker_id set
2. Offline worker transitions are deterministic and logged

## Phase 2: Offline failover

1. Add failover controller reconcile loop.
2. On offline worker, create takeover jobs for impacted deployments.
3. Use placement engine for candidate ranking.
4. Use registry-first image path in takeover execution.

Exit criteria:

1. Offline worker deployments are re-homed automatically
2. Deployment URL recovers without manual action

## Phase 3: Rebalance and failback

1. Add recovery warmup and score-delta based movement.
2. Add cooldown and migration budget guardrails.
3. Add event logs for rebalance decision traceability.

Exit criteria:

1. Recovered workers receive workloads only when beneficial
2. No migration ping-pong in soak tests

## Phase 4: Hardening and observability

1. Add Prometheus metrics for placement and migration outcomes.
2. Add debug endpoint or report for placement decisions.
3. Add runbook documentation and operator controls.

Exit criteria:

1. Operators can explain any placement or migration from logs and metrics

---

## Testing Strategy

## Unit tests

1. Placement scoring and tie-breaking determinism
2. Eligibility filtering with capacity and heartbeat constraints
3. Failover state machine transitions
4. Rebalance threshold and cooldown logic
5. Registry resolver behavior for image exists and missing

## Integration tests

1. Worker heartbeat timeout marks offline
2. Offline worker triggers takeover job creation
3. Ownership updates after successful takeover
4. Failed takeover retries next candidate
5. Registry-first path skips build when image exists

## End-to-end tests

1. New deployment routes to best worker
2. Worker crash causes automatic re-home
3. Worker recovery triggers conditional failback
4. URL remains stable and app serves traffic after migration

## Non-functional tests

1. Reconcile loop under high worker count
2. Database contention around migration locks
3. Migration storm prevention under multi-worker flaps

---

## Dry-Run Plan (Pre-Production)

## Environment

1. One control-plane worker
2. Two remote workers with different capacities
3. Shared registry credentials configured
4. Synthetic app fixture with health endpoint

## Dry-run scenarios

1. Baseline placement
   1. Submit small and large deployment requests
   2. Verify best target chosen by score policy

2. Offline failover
   1. Hard-stop owner worker process or network path
   2. Wait heartbeat timeout
   3. Verify automatic takeover to next best worker
   4. Verify status and ownership updates

3. Registry reuse
   1. Trigger takeover for already-built image
   2. Confirm build step skipped and deployment succeeds

4. Recovery rebalance
   1. Restore worker and wait warmup
   2. Verify migration occurs only when score delta passes threshold
   3. Verify no movement when delta is below threshold

5. Anti-flap protection
   1. Toggle worker online-offline repeatedly
   2. Verify cooldown and budget prevent ping-pong

## Pass criteria

1. No manual intervention required in target scenarios
2. Stable deployment URL through failover events
3. No duplicate ownership for same deployment
4. No infinite migration loops
5. Decision reason visible in logs

---

## Observability Requirements

## Logs

Mandatory structured fields:

1. deployment_id
2. old_worker_id
3. new_worker_id
4. decision_type
5. score_old
6. score_new
7. reason
8. generation

## Metrics

1. meshvpn_worker_health_state
2. meshvpn_failover_attempts_total
3. meshvpn_failover_success_total
4. meshvpn_rebalance_attempts_total
5. meshvpn_rebalance_success_total
6. meshvpn_placement_decision_duration_seconds
7. meshvpn_migration_cooldown_skips_total

## Dashboards

1. Worker health and heartbeat freshness
2. Deployment ownership map
3. Placement decision trend
4. Failover MTTR
5. Rebalance churn rate

---

## Failure Modes and Handling

1. No eligible target worker:
   1. Keep deployment failover_pending
   2. Retry with backoff
   3. Emit warning event

2. Registry unavailable:
   1. Retry pull and then optional rebuild path
   2. Mark transient error and continue reconcile

3. Split-brain ownership risk:
   1. Use transactional ownership lock and generation checks

4. Repeated worker flapping:
   1. Increase reliability penalty
   2. Exclude worker temporarily from placement

---

## Rollout Plan

1. Deploy with feature flags disabled.
2. Enable health controller first.
3. Enable failover for canary deployments only.
4. Enable rebalance in observe-only mode.
5. Enable active rebalance with conservative thresholds.
6. Gradually raise migration budget after soak period.

---

## Summary

This architecture ensures:

1. Best available dynamic placement for new deployments
2. Automatic recovery from worker outages
3. Controlled and explainable failback behavior
4. Faster migration by reusing existing registry images
5. Production-safe rollout with strong testing and dry-run validation
