package service

import (
	"context"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/config"
	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/store"
)

type JobDistributor struct {
	jobs               store.JobRepository
	workers            store.WorkerRepository
	enabled            bool
	controlPlaneWorker string // Worker ID for control-plane if acting as worker
	placementStrategy  string // smart, local-first, remote-only
	maxJobsControlPlane int   // Max concurrent jobs for control-plane worker
}

func NewJobDistributor(
	jobs store.JobRepository,
	workers store.WorkerRepository,
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
		enabled:             cfg.EnableMultiWorker,
		controlPlaneWorker:  controlPlaneWorkerID,
		placementStrategy:   cfg.JobPlacementStrategy,
		maxJobsControlPlane: cfg.ControlPlaneMaxJobs,
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
			MaxConcurrentJobs: d.maxJobsControlPlane,
			SupportedPackages: []string{"small", "medium", "large"},
		},
		MaxConcurrentJobs: d.maxJobsControlPlane,
		CurrentJobs:       0,
	}

	d.workers.Register(ctx, worker)
	logs.Infof("distributor", "registered control-plane as worker worker_id=%s max_jobs=%d",
		worker.WorkerID, d.maxJobsControlPlane)
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
		logs.Debugf("distributor", "no available workers for job job_id=%s", job.JobID)
		return // Job stays queued
	}

	// Assign job to worker
	if err := d.jobs.AssignToWorker(ctx, job.JobID, selectedWorker.WorkerID); err != nil {
		logs.Errorf("distributor", "failed to assign job job_id=%s worker_id=%s err=%v",
			job.JobID, selectedWorker.WorkerID, err)
		return
	}

	// Increment worker's job count
	d.workers.IncrementJobCount(ctx, selectedWorker.WorkerID)

	logs.Infof("distributor", "assigned job job_id=%s deployment_id=%s to worker_id=%s",
		job.JobID, job.DeploymentID, selectedWorker.WorkerID)
}

func (d *JobDistributor) smartPlacement(ctx context.Context, job domain.DeploymentJob) (domain.Worker, error) {
	// For small packages, prefer control-plane if available
	if job.CPUCores <= 0.5 && d.controlPlaneWorker != "" {
		worker, err := d.workers.Get(ctx, d.controlPlaneWorker)
		if err == nil && worker.Status == string(domain.WorkerStatusIdle) && worker.CurrentJobs < worker.MaxConcurrentJobs {
			logs.Debugf("distributor", "smart placement: small package → control-plane")
			return worker, nil
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
		if err == nil && worker.Status == string(domain.WorkerStatusIdle) && worker.CurrentJobs < worker.MaxConcurrentJobs {
			logs.Debugf("distributor", "smart placement: fallback → control-plane")
			return worker, nil
		}
	}

	return domain.Worker{}, store.ErrNoAvailableWorkers
}

func (d *JobDistributor) localFirstPlacement(ctx context.Context) (domain.Worker, error) {
	// Try control-plane first
	if d.controlPlaneWorker != "" {
		worker, err := d.workers.Get(ctx, d.controlPlaneWorker)
		if err == nil && worker.Status == string(domain.WorkerStatusIdle) && worker.CurrentJobs < worker.MaxConcurrentJobs {
			return worker, nil
		}
	}

	// Fallback to remote workers
	return d.getRemoteWorker(ctx)
}

func (d *JobDistributor) remoteOnlyPlacement(ctx context.Context) (domain.Worker, error) {
	return d.getRemoteWorker(ctx)
}

func (d *JobDistributor) getRemoteWorker(ctx context.Context) (domain.Worker, error) {
	workers, err := d.workers.ListByStatus(ctx, domain.WorkerStatusIdle)
	if err != nil {
		return domain.Worker{}, err
	}

	// Exclude control-plane worker
	for _, w := range workers {
		if w.WorkerID != d.controlPlaneWorker && w.CurrentJobs < w.MaxConcurrentJobs {
			return w, nil
		}
	}

	return domain.Worker{}, store.ErrNoAvailableWorkers
}
