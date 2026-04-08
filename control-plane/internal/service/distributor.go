package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/config"
	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/store"

	"github.com/google/uuid"
)

type JobDistributor struct {
	jobs                store.JobRepository
	workers             store.WorkerRepository
	deployments         store.DeploymentRepository
	enabled             bool
	controlPlaneWorker  string // Worker ID for control-plane if acting as worker
	placementStrategy   string // smart, local-first, remote-only
	maxJobsControlPlane int    // Max concurrent jobs for control-plane worker
	heartbeatTimeout    time.Duration
	rebalanceCooldown   time.Duration
	rebalanceMinDelta   int
	lastShiftByDeploy   map[string]time.Time
}

func NewJobDistributor(
	jobs store.JobRepository,
	workers store.WorkerRepository,
	deployments store.DeploymentRepository,
	cfg config.ControlPlaneConfig,
) *JobDistributor {
	var controlPlaneWorkerID string
	if cfg.ControlPlaneAsWorker {
		controlPlaneWorkerID = cfg.ControlPlaneWorkerID
		if controlPlaneWorkerID == "" {
			controlPlaneWorkerID = "control-plane-local"
		}
	}

	return &JobDistributor{
		jobs:                jobs,
		workers:             workers,
		deployments:         deployments,
		enabled:             cfg.EnableMultiWorker,
		controlPlaneWorker:  controlPlaneWorkerID,
		placementStrategy:   cfg.JobPlacementStrategy,
		maxJobsControlPlane: cfg.ControlPlaneMaxJobs,
		heartbeatTimeout:    cfg.WorkerHeartbeatTimeout,
		rebalanceCooldown:   10 * time.Minute,
		rebalanceMinDelta:   200,
		lastShiftByDeploy:   make(map[string]time.Time),
	}
}

func (d *JobDistributor) Start(ctx context.Context) {
	if !d.enabled {
		logs.Infof("distributor", "job distributor disabled (single-worker mode)")
		return
	}

	logs.Infof("distributor", "job distributor started (multi-worker mode, strategy=%s)", d.placementStrategy)

	// Register control-plane as worker if enabled
	if d.controlPlaneWorker != "" {
		d.registerControlPlaneAsWorker(ctx)
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.reconcileWorkerHealth(ctx)
			d.reconcileOfflineDeployments(ctx)
			d.reconcileRebalance(ctx)
			d.distributeJobs(ctx)
		}
	}
}

func (d *JobDistributor) registerControlPlaneAsWorker(ctx context.Context) {
	worker := domain.Worker{
		WorkerID:    d.controlPlaneWorker,
		Name:        "Control-Plane (Local Worker)",
		TailscaleIP: "localhost",
		Hostname:    "control-plane",
		Status:      string(domain.WorkerStatusIdle),
		Capabilities: domain.WorkerCapabilities{
			Runtime:           "kubernetes",
			CPUCores:          4, // Advertise sufficient capacity for local deployments
			MemoryGB:          8, // Adjust based on your machine's capacity
			MaxConcurrentJobs: d.maxJobsControlPlane,
			SupportedPackages: []string{"small", "medium", "large"},
		},
		MaxConcurrentJobs: d.maxJobsControlPlane,
		CurrentJobs:       0,
	}

	if err := d.workers.Register(ctx, worker); err != nil {
		logs.Errorf("distributor", "failed to register control-plane worker worker_id=%s err=%v", worker.WorkerID, err)
		return
	}
	logs.Infof("distributor", "registered control-plane as worker worker_id=%s max_jobs=%d cpu=%d memory=%dGB",
		worker.WorkerID, d.maxJobsControlPlane, worker.Capabilities.CPUCores, worker.Capabilities.MemoryGB)
}

func (d *JobDistributor) distributeJobs(ctx context.Context) {
	// Get next unassigned job
	job, err := d.jobs.GetNextUnassignedJob(ctx)
	if err == store.ErrNoQueuedJobs {
		return // No jobs to distribute
	}
	if err != nil {
		logs.Errorf("distributor", "failed to get unassigned job err=%v", err)
		return
	}

	// Select worker based on strategy
	var selectedWorker domain.Worker
	var selectionErr error

	switch d.placementStrategy {
	case "smart":
		selectedWorker, selectionErr = d.smartPlacement(ctx, job)
	case "local-first":
		selectedWorker, selectionErr = d.localFirstPlacement(ctx)
	case "remote-only":
		selectedWorker, selectionErr = d.remoteOnlyPlacement(ctx)
	default:
		selectedWorker, selectionErr = d.smartPlacement(ctx, job)
	}

	if selectionErr != nil {
		logs.Debugf("distributor", "no available workers for job job_id=%s deployment_id=%s err=%v", job.JobID, job.DeploymentID, selectionErr)
		d.logPlacementDiagnostics(ctx, job, selectionErr)
		return // Job stays queued
	}

	// Assign job to worker
	if err := d.jobs.AssignToWorker(ctx, job.JobID, selectedWorker.WorkerID); err != nil {
		logs.Errorf("distributor", "failed to assign job job_id=%s worker_id=%s err=%v",
			job.JobID, selectedWorker.WorkerID, err)
		return
	}

	// Increment worker's job count
	if err := d.workers.IncrementJobCount(ctx, selectedWorker.WorkerID); err != nil {
		logs.Errorf("distributor", "failed to increment worker job count worker_id=%s job_id=%s err=%v",
			selectedWorker.WorkerID, job.JobID, err)
		if relErr := d.jobs.ReleaseFromWorker(ctx, job.JobID); relErr != nil {
			logs.Errorf("distributor", "failed to release assigned job after increment failure job_id=%s err=%v", job.JobID, relErr)
		}
		return
	}

	if d.deployments != nil {
		rec, err := d.deployments.Get(job.DeploymentID)
		if err != nil {
			logs.Errorf("distributor", "failed to update deployment assignment deployment_id=%s worker_id=%s err=%v",
				job.DeploymentID, selectedWorker.WorkerID, err)
		} else {
			next := rec
			next.OwnerWorkerID = selectedWorker.WorkerID
			if next.Status == "queued" {
				next.Status = "deploying"
			}
			next.BuildLogs = next.BuildLogs + "\n=== distributor ===\nassigned to worker " + selectedWorker.WorkerID + "\n"
			d.deployments.Update(next)
			logs.Debugf("distributor", "deployment assignment updated deployment_id=%s worker_id=%s status=%s",
				next.DeploymentID, selectedWorker.WorkerID, next.Status)
		}
	}

	logs.Infof("distributor", "assigned job job_id=%s deployment_id=%s to worker_id=%s",
		job.JobID, job.DeploymentID, selectedWorker.WorkerID)
}

func (d *JobDistributor) smartPlacement(ctx context.Context, job domain.DeploymentJob) (domain.Worker, error) {
	// For small packages, prefer control-plane if available
	if job.CPUCores <= 0.5 && d.controlPlaneWorker != "" {
		worker, err := d.workers.Get(ctx, d.controlPlaneWorker)
		if err == nil && worker.Status != string(domain.WorkerStatusOffline) && worker.CurrentJobs < worker.MaxConcurrentJobs {
			logs.Debugf("distributor", "smart placement: small package → control-plane")
			return worker, nil
		}
		if err != nil {
			logs.Warnf("distributor", "smart placement: control-plane worker lookup failed worker_id=%s err=%v", d.controlPlaneWorker, err)
		} else {
			logs.Warnf("distributor", "smart placement: control-plane unavailable worker_id=%s status=%s jobs=%d/%d",
				worker.WorkerID, worker.Status, worker.CurrentJobs, worker.MaxConcurrentJobs)
		}
	}

	// For medium/large packages, prefer remote workers
	worker, err := d.getRemoteWorker(ctx)
	if err == nil {
		logs.Debugf("distributor", "smart placement: medium/large package → remote worker")
		return worker, nil
	}

	// Fallback to control-plane if no remote workers
	if d.controlPlaneWorker != "" {
		worker, err := d.workers.Get(ctx, d.controlPlaneWorker)
		if err == nil && worker.Status != string(domain.WorkerStatusOffline) && worker.CurrentJobs < worker.MaxConcurrentJobs {
			logs.Debugf("distributor", "smart placement: fallback → control-plane")
			return worker, nil
		}
		if err != nil {
			logs.Warnf("distributor", "smart placement fallback: control-plane worker lookup failed worker_id=%s err=%v", d.controlPlaneWorker, err)
		} else {
			logs.Warnf("distributor", "smart placement fallback: control-plane unavailable worker_id=%s status=%s jobs=%d/%d",
				worker.WorkerID, worker.Status, worker.CurrentJobs, worker.MaxConcurrentJobs)
		}
	}

	return domain.Worker{}, store.ErrNoAvailableWorkers
}

func (d *JobDistributor) localFirstPlacement(ctx context.Context) (domain.Worker, error) {
	// Try control-plane first
	if d.controlPlaneWorker != "" {
		worker, err := d.workers.Get(ctx, d.controlPlaneWorker)
		if err == nil && worker.Status != string(domain.WorkerStatusOffline) && worker.CurrentJobs < worker.MaxConcurrentJobs {
			return worker, nil
		}
		if err != nil {
			logs.Warnf("distributor", "local-first: control-plane worker lookup failed worker_id=%s err=%v", d.controlPlaneWorker, err)
		} else {
			logs.Warnf("distributor", "local-first: control-plane unavailable worker_id=%s status=%s jobs=%d/%d",
				worker.WorkerID, worker.Status, worker.CurrentJobs, worker.MaxConcurrentJobs)
		}
	} else {
		logs.Warnf("distributor", "local-first: control-plane worker id is empty")
	}

	// Fallback to remote workers
	return d.getRemoteWorker(ctx)
}

func (d *JobDistributor) remoteOnlyPlacement(ctx context.Context) (domain.Worker, error) {
	return d.getRemoteWorker(ctx)
}

func (d *JobDistributor) getRemoteWorker(ctx context.Context) (domain.Worker, error) {
	workers, err := d.workers.List(ctx)
	if err != nil {
		return domain.Worker{}, err
	}

	candidates := make([]domain.Worker, 0, len(workers))
	for _, w := range workers {
		if w.WorkerID == d.controlPlaneWorker {
			continue
		}
		if w.Status == string(domain.WorkerStatusOffline) {
			continue
		}
		if w.CurrentJobs >= w.MaxConcurrentJobs {
			continue
		}
		candidates = append(candidates, w)
	}

	if len(candidates) == 0 {
		logs.Warnf("distributor", "remote worker selection: no eligible remote candidates total_workers=%d control_plane_worker=%s", len(workers), d.controlPlaneWorker)
		return domain.Worker{}, store.ErrNoAvailableWorkers
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		freeSlotsI := candidates[i].MaxConcurrentJobs - candidates[i].CurrentJobs
		freeSlotsJ := candidates[j].MaxConcurrentJobs - candidates[j].CurrentJobs
		if freeSlotsI != freeSlotsJ {
			return freeSlotsI > freeSlotsJ
		}
		if candidates[i].Capabilities.CPUCores != candidates[j].Capabilities.CPUCores {
			return candidates[i].Capabilities.CPUCores > candidates[j].Capabilities.CPUCores
		}
		if candidates[i].Capabilities.MemoryGB != candidates[j].Capabilities.MemoryGB {
			return candidates[i].Capabilities.MemoryGB > candidates[j].Capabilities.MemoryGB
		}
		return candidates[i].WorkerID < candidates[j].WorkerID
	})

	return candidates[0], nil
}

func (d *JobDistributor) logPlacementDiagnostics(ctx context.Context, job domain.DeploymentJob, selectionErr error) {
	workers, err := d.workers.List(ctx)
	if err != nil {
		logs.Errorf("distributor", "placement diagnostics failed job_id=%s deployment_id=%s err=%v",
			job.JobID, job.DeploymentID, err)
		return
	}

	if len(workers) == 0 {
		logs.Warnf("distributor", "placement diagnostics: no workers registered job_id=%s deployment_id=%s strategy=%s err=%v",
			job.JobID, job.DeploymentID, d.placementStrategy, selectionErr)
		return
	}

	logs.Warnf("distributor", "placement diagnostics: job_id=%s deployment_id=%s strategy=%s err=%v workers=%d",
		job.JobID, job.DeploymentID, d.placementStrategy, selectionErr, len(workers))

	now := time.Now()
	for _, w := range workers {
		role := "remote"
		if w.WorkerID == d.controlPlaneWorker {
			role = "control-plane"
		}

		heartbeatAge := "none"
		if w.LastHeartbeat != nil {
			heartbeatAge = now.Sub(*w.LastHeartbeat).Truncate(time.Second).String()
		}

		freeSlots := w.MaxConcurrentJobs - w.CurrentJobs
		eligible := w.Status != string(domain.WorkerStatusOffline) && freeSlots > 0

		logs.Warnf("distributor", "worker snapshot worker_id=%s role=%s status=%s jobs=%d/%d free_slots=%d eligible=%t heartbeat_age=%s",
			w.WorkerID, role, w.Status, w.CurrentJobs, w.MaxConcurrentJobs, freeSlots, eligible, heartbeatAge)
	}
}

func (d *JobDistributor) reconcileWorkerHealth(ctx context.Context) {
	workers, err := d.workers.List(ctx)
	if err != nil {
		logs.Errorf("distributor", "worker health reconciliation failed err=%v", err)
		return
	}

	now := time.Now()
	for _, w := range workers {
		if w.WorkerID == d.controlPlaneWorker {
			continue
		}
		if w.Status == string(domain.WorkerStatusOffline) {
			continue
		}
		if w.LastHeartbeat == nil {
			continue
		}
		if now.Sub(*w.LastHeartbeat) <= d.heartbeatTimeout {
			continue
		}

		if err := d.workers.MarkOffline(ctx, w.WorkerID); err != nil {
			logs.Errorf("distributor", "failed to mark worker offline worker_id=%s err=%v", w.WorkerID, err)
			continue
		}
		logs.Infof("distributor", "worker marked offline worker_id=%s heartbeat_age=%s", w.WorkerID, now.Sub(*w.LastHeartbeat))
	}
}

func (d *JobDistributor) reconcileOfflineDeployments(ctx context.Context) {
	if d.deployments == nil {
		return
	}

	workers, err := d.workers.List(ctx)
	if err != nil {
		logs.Errorf("distributor", "failed to list workers for failover err=%v", err)
		return
	}

	offline := make(map[string]struct{}, len(workers))
	for _, w := range workers {
		if w.Status == string(domain.WorkerStatusOffline) {
			offline[w.WorkerID] = struct{}{}
		}
	}
	if len(offline) == 0 {
		return
	}

	for _, dep := range d.deployments.List() {
		if dep.Status != "running" {
			continue
		}
		owner := strings.TrimSpace(dep.OwnerWorkerID)
		if owner == "" {
			continue
		}
		if _, isOffline := offline[owner]; !isOffline {
			continue
		}

		jobID, err := d.requeueDeployment(ctx, dep, "owner worker went offline, deployment re-queued for failover")
		if err != nil {
			logs.Errorf("distributor", "failed to enqueue failover job deployment_id=%s err=%v", dep.DeploymentID, err)
			continue
		}

		logs.Infof("distributor", "re-queued deployment due to offline worker deployment_id=%s old_worker_id=%s job_id=%s",
			dep.DeploymentID, owner, jobID)
	}
}

func (d *JobDistributor) reconcileRebalance(ctx context.Context) {
	if d.deployments == nil {
		return
	}

	workers, err := d.workers.List(ctx)
	if err != nil {
		logs.Errorf("distributor", "failed to list workers for rebalance err=%v", err)
		return
	}

	workerByID := make(map[string]domain.Worker, len(workers))
	for _, w := range workers {
		workerByID[w.WorkerID] = w
	}

	now := time.Now().UTC()
	for _, dep := range d.deployments.List() {
		if dep.Status != "running" {
			continue
		}

		ownerID := strings.TrimSpace(dep.OwnerWorkerID)
		if ownerID == "" {
			continue
		}

		if last, ok := d.lastShiftByDeploy[dep.DeploymentID]; ok {
			if now.Sub(last) < d.rebalanceCooldown {
				continue
			}
		}

		owner, ok := workerByID[ownerID]
		if !ok {
			continue
		}

		ownerScore, ownerEligible := workerPlacementScore(owner)
		if !ownerEligible {
			continue
		}

		bestWorker, bestScore, found := d.bestAvailableWorker(workers)
		if !found {
			continue
		}
		if bestWorker.WorkerID == ownerID {
			continue
		}

		if bestScore-ownerScore < d.rebalanceMinDelta {
			continue
		}

		reason := "better worker available online, deployment re-queued for rebalance"
		jobID, err := d.requeueDeployment(ctx, dep, reason)
		if err != nil {
			logs.Errorf("distributor", "failed to enqueue rebalance job deployment_id=%s err=%v", dep.DeploymentID, err)
			continue
		}

		logs.Infof("distributor", "rebalanced deployment deployment_id=%s from_worker=%s to_worker=%s old_score=%d new_score=%d job_id=%s",
			dep.DeploymentID, ownerID, bestWorker.WorkerID, ownerScore, bestScore, jobID)
	}
}

func (d *JobDistributor) bestAvailableWorker(workers []domain.Worker) (domain.Worker, int, bool) {
	bestScore := -1
	var bestWorker domain.Worker

	for _, w := range workers {
		score, ok := workerPlacementScore(w)
		if !ok {
			continue
		}
		if score > bestScore {
			bestScore = score
			bestWorker = w
		}
	}

	if bestScore < 0 {
		return domain.Worker{}, 0, false
	}

	return bestWorker, bestScore, true
}

func workerPlacementScore(w domain.Worker) (int, bool) {
	if w.Status == string(domain.WorkerStatusOffline) {
		return 0, false
	}
	if w.MaxConcurrentJobs <= 0 {
		return 0, false
	}

	freeSlots := w.MaxConcurrentJobs - w.CurrentJobs
	if freeSlots <= 0 {
		return 0, false
	}

	score := freeSlots*1000 + w.Capabilities.CPUCores*100 + w.Capabilities.MemoryGB*10
	return score, true
}

func (d *JobDistributor) requeueDeployment(ctx context.Context, dep domain.DeploymentRecord, reason string) (string, error) {
	now := time.Now().UTC()
	oldOwner := strings.TrimSpace(dep.OwnerWorkerID)

	dep.Status = "queued"
	dep.OwnerWorkerID = ""
	dep.Error = reason
	dep.BuildLogs = dep.BuildLogs + "\n=== rebalance/failover ===\n" + reason + "\n"
	dep.FinishedAt = nil
	d.deployments.Update(dep)

	job := domain.DeploymentJob{
		JobID:        strings.ReplaceAll(uuid.NewString(), "-", "")[:12],
		DeploymentID: dep.DeploymentID,
		Repo:         dep.Repo,
		Subdomain:    dep.Subdomain,
		Port:         dep.Port,
		ScalingMode:  dep.ScalingMode,
		MinReplicas:  dep.MinReplicas,
		MaxReplicas:  dep.MaxReplicas,
		CPUTarget:    dep.CPUTarget,
		CPURequest:   dep.CPURequest,
		CPULimit:     dep.CPULimit,
		NodeSelector: domain.CloneStringMap(dep.NodeSelector),
		Env:          domain.CloneStringMap(dep.Env),
		BuildArgs:    domain.CloneStringMap(dep.BuildArgs),
		CPUCores:     dep.CPUCores,
		MemoryMB:     dep.MemoryMB,
		RequestedBy:  dep.RequestedBy,
		QueuedAt:     now,
	}

	if err := d.jobs.Enqueue(ctx, job); err != nil {
		return "", err
	}

	if d.lastShiftByDeploy == nil {
		d.lastShiftByDeploy = make(map[string]time.Time)
	}
	d.lastShiftByDeploy[dep.DeploymentID] = now
	if oldOwner != "" {
		logs.Debugf("distributor", "deployment shift recorded deployment_id=%s previous_owner=%s", dep.DeploymentID, oldOwner)
	}

	return job.JobID, nil
}
