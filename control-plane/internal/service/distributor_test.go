package service

import (
	"context"
	"testing"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/store"
)

type testJobRepo struct {
	enqueued []domain.DeploymentJob
}

func (r *testJobRepo) Enqueue(_ context.Context, job domain.DeploymentJob) error {
	r.enqueued = append(r.enqueued, job)
	return nil
}
func (r *testJobRepo) ClaimNext(_ context.Context) (domain.DeploymentJob, error) {
	return domain.DeploymentJob{}, store.ErrNoQueuedJobs
}
func (r *testJobRepo) MarkDone(_ context.Context, _ string) error             { return nil }
func (r *testJobRepo) MarkFailed(_ context.Context, _ string, _ string) error { return nil }
func (r *testJobRepo) AssignToWorker(_ context.Context, _, _ string) error    { return nil }
func (r *testJobRepo) ReleaseFromWorker(_ context.Context, _ string) error    { return nil }
func (r *testJobRepo) ClaimForWorker(_ context.Context, _ string) (domain.DeploymentJob, error) {
	return domain.DeploymentJob{}, store.ErrNoQueuedJobs
}
func (r *testJobRepo) GetNextUnassignedJob(_ context.Context) (domain.DeploymentJob, error) {
	return domain.DeploymentJob{}, store.ErrNoQueuedJobs
}

type testWorkerRepo struct {
	workers map[string]domain.Worker
}

func (r *testWorkerRepo) Register(_ context.Context, worker domain.Worker) error {
	r.workers[worker.WorkerID] = worker
	return nil
}
func (r *testWorkerRepo) Update(_ context.Context, worker domain.Worker) error {
	r.workers[worker.WorkerID] = worker
	return nil
}
func (r *testWorkerRepo) Get(_ context.Context, workerID string) (domain.Worker, error) {
	w, ok := r.workers[workerID]
	if !ok {
		return domain.Worker{}, store.ErrNoAvailableWorkers
	}
	return w, nil
}
func (r *testWorkerRepo) List(_ context.Context) ([]domain.Worker, error) {
	out := make([]domain.Worker, 0, len(r.workers))
	for _, w := range r.workers {
		out = append(out, w)
	}
	return out, nil
}
func (r *testWorkerRepo) ListByStatus(_ context.Context, status domain.WorkerStatus) ([]domain.Worker, error) {
	out := make([]domain.Worker, 0, len(r.workers))
	for _, w := range r.workers {
		if w.Status == string(status) {
			out = append(out, w)
		}
	}
	return out, nil
}
func (r *testWorkerRepo) UpdateHeartbeat(_ context.Context, workerID string) error {
	w := r.workers[workerID]
	now := time.Now()
	w.LastHeartbeat = &now
	r.workers[workerID] = w
	return nil
}
func (r *testWorkerRepo) IncrementJobCount(_ context.Context, workerID string) error {
	w := r.workers[workerID]
	w.CurrentJobs++
	r.workers[workerID] = w
	return nil
}
func (r *testWorkerRepo) DecrementJobCount(_ context.Context, workerID string) error {
	w := r.workers[workerID]
	if w.CurrentJobs > 0 {
		w.CurrentJobs--
	}
	r.workers[workerID] = w
	return nil
}
func (r *testWorkerRepo) MarkOffline(_ context.Context, workerID string) error {
	w := r.workers[workerID]
	w.Status = string(domain.WorkerStatusOffline)
	r.workers[workerID] = w
	return nil
}
func (r *testWorkerRepo) GetAvailableWorker(_ context.Context) (domain.Worker, error) {
	for _, w := range r.workers {
		if w.Status == string(domain.WorkerStatusIdle) && w.CurrentJobs < w.MaxConcurrentJobs {
			return w, nil
		}
	}
	return domain.Worker{}, store.ErrNoAvailableWorkers
}

func TestReconcileWorkerHealthMarksStaleWorkerOffline(t *testing.T) {
	ctx := context.Background()
	stale := time.Now().Add(-2 * time.Minute)

	workerRepo := &testWorkerRepo{workers: map[string]domain.Worker{
		"remote-1": {
			WorkerID:          "remote-1",
			Status:            string(domain.WorkerStatusIdle),
			MaxConcurrentJobs: 2,
			LastHeartbeat:     &stale,
		},
	}}

	d := &JobDistributor{
		workers:          workerRepo,
		heartbeatTimeout: 90 * time.Second,
	}

	d.reconcileWorkerHealth(ctx)

	updated, _ := workerRepo.Get(ctx, "remote-1")
	if updated.Status != string(domain.WorkerStatusOffline) {
		t.Fatalf("expected worker to be offline, got %s", updated.Status)
	}
}

func TestReconcileOfflineDeploymentsRequeuesRunningOwnedDeployment(t *testing.T) {
	ctx := context.Background()

	depRepo := store.NewInMemoryDeploymentRepository()
	depRepo.Start(domain.DeploymentRecord{
		DeploymentID:  "dep-1",
		OwnerWorkerID: "remote-offline",
		Repo:          "https://github.com/example/repo.git",
		Subdomain:     "demo",
		Port:          3000,
		Status:        "running",
		CPUCores:      0.5,
		MemoryMB:      512,
	})

	workerRepo := &testWorkerRepo{workers: map[string]domain.Worker{
		"remote-offline": {
			WorkerID:          "remote-offline",
			Status:            string(domain.WorkerStatusOffline),
			MaxConcurrentJobs: 2,
		},
	}}
	jobRepo := &testJobRepo{}

	d := &JobDistributor{
		jobs:        jobRepo,
		workers:     workerRepo,
		deployments: depRepo,
	}

	d.reconcileOfflineDeployments(ctx)

	updated, err := depRepo.Get("dep-1")
	if err != nil {
		t.Fatalf("expected deployment, got err=%v", err)
	}
	if updated.Status != "queued" {
		t.Fatalf("expected deployment to be queued, got %s", updated.Status)
	}
	if updated.OwnerWorkerID != "" {
		t.Fatalf("expected owner worker cleared, got %s", updated.OwnerWorkerID)
	}
	if len(jobRepo.enqueued) != 1 {
		t.Fatalf("expected one failover job, got %d", len(jobRepo.enqueued))
	}
	if jobRepo.enqueued[0].DeploymentID != "dep-1" {
		t.Fatalf("expected failover job for dep-1, got %s", jobRepo.enqueued[0].DeploymentID)
	}
}

func TestReconcileRebalanceRequeuesWhenBetterWorkerAvailable(t *testing.T) {
	ctx := context.Background()

	depRepo := store.NewInMemoryDeploymentRepository()
	depRepo.Start(domain.DeploymentRecord{
		DeploymentID:  "dep-2",
		OwnerWorkerID: "control-plane-local",
		Repo:          "https://github.com/example/repo.git",
		Subdomain:     "demo2",
		Port:          3000,
		Status:        "running",
		CPUCores:      1,
		MemoryMB:      1024,
	})

	workerRepo := &testWorkerRepo{workers: map[string]domain.Worker{
		"control-plane-local": {
			WorkerID:          "control-plane-local",
			Status:            string(domain.WorkerStatusIdle),
			MaxConcurrentJobs: 2,
			CurrentJobs:       1,
			Capabilities: domain.WorkerCapabilities{
				CPUCores: 4,
				MemoryGB: 8,
			},
		},
		"remote-strong": {
			WorkerID:          "remote-strong",
			Status:            string(domain.WorkerStatusIdle),
			MaxConcurrentJobs: 4,
			CurrentJobs:       0,
			Capabilities: domain.WorkerCapabilities{
				CPUCores: 12,
				MemoryGB: 32,
			},
		},
	}}
	jobRepo := &testJobRepo{}

	d := &JobDistributor{
		jobs:              jobRepo,
		workers:           workerRepo,
		deployments:       depRepo,
		rebalanceCooldown: 10 * time.Minute,
		rebalanceMinDelta: 200,
		lastShiftByDeploy: make(map[string]time.Time),
	}

	d.reconcileRebalance(ctx)

	updated, err := depRepo.Get("dep-2")
	if err != nil {
		t.Fatalf("expected deployment, got err=%v", err)
	}
	if updated.Status != "queued" {
		t.Fatalf("expected deployment to be queued for rebalance, got %s", updated.Status)
	}
	if updated.OwnerWorkerID != "" {
		t.Fatalf("expected owner worker cleared after rebalance, got %s", updated.OwnerWorkerID)
	}
	if len(jobRepo.enqueued) != 1 {
		t.Fatalf("expected one rebalance job, got %d", len(jobRepo.enqueued))
	}
}

func TestReconcileRebalanceRespectsCooldown(t *testing.T) {
	ctx := context.Background()

	depRepo := store.NewInMemoryDeploymentRepository()
	depRepo.Start(domain.DeploymentRecord{
		DeploymentID:  "dep-3",
		OwnerWorkerID: "control-plane-local",
		Repo:          "https://github.com/example/repo.git",
		Subdomain:     "demo3",
		Port:          3000,
		Status:        "running",
		CPUCores:      1,
		MemoryMB:      1024,
	})

	workerRepo := &testWorkerRepo{workers: map[string]domain.Worker{
		"control-plane-local": {
			WorkerID:          "control-plane-local",
			Status:            string(domain.WorkerStatusIdle),
			MaxConcurrentJobs: 2,
			CurrentJobs:       1,
			Capabilities: domain.WorkerCapabilities{
				CPUCores: 4,
				MemoryGB: 8,
			},
		},
		"remote-strong": {
			WorkerID:          "remote-strong",
			Status:            string(domain.WorkerStatusIdle),
			MaxConcurrentJobs: 4,
			CurrentJobs:       0,
			Capabilities: domain.WorkerCapabilities{
				CPUCores: 12,
				MemoryGB: 32,
			},
		},
	}}
	jobRepo := &testJobRepo{}

	d := &JobDistributor{
		jobs:              jobRepo,
		workers:           workerRepo,
		deployments:       depRepo,
		rebalanceCooldown: 10 * time.Minute,
		rebalanceMinDelta: 200,
		lastShiftByDeploy: map[string]time.Time{"dep-3": time.Now().Add(-2 * time.Minute)},
	}

	d.reconcileRebalance(ctx)

	updated, err := depRepo.Get("dep-3")
	if err != nil {
		t.Fatalf("expected deployment, got err=%v", err)
	}
	if updated.Status != "running" {
		t.Fatalf("expected deployment to stay running due to cooldown, got %s", updated.Status)
	}
	if len(jobRepo.enqueued) != 0 {
		t.Fatalf("expected no rebalance jobs during cooldown, got %d", len(jobRepo.enqueued))
	}
}
