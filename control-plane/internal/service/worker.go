package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/runtime"
	"MeshVPN-slef-hosting/control-plane/internal/store"
	"MeshVPN-slef-hosting/control-plane/internal/telemetry"
)

type DeploymentWorker struct {
	repo         store.DeploymentRepository
	jobs         store.JobRepository
	workers      store.WorkerRepository
	runner       *runtime.Runner
	pollInterval time.Duration
	enableCPUHPA bool
	workerID     string
	useAssigned  bool
}

func NewDeploymentWorker(repo store.DeploymentRepository, jobs store.JobRepository, workers store.WorkerRepository, runner *runtime.Runner, pollInterval time.Duration, enableCPUHPA bool, workerID string, useAssigned bool) *DeploymentWorker {
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	if strings.TrimSpace(workerID) == "" {
		workerID = "control-plane-local"
	}
	return &DeploymentWorker{repo: repo, jobs: jobs, workers: workers, runner: runner, pollInterval: pollInterval, enableCPUHPA: enableCPUHPA, workerID: workerID, useAssigned: useAssigned}
}

func (w *DeploymentWorker) Start(ctx context.Context) {
	logs.Infof("worker", "deployment worker started poll_interval=%s", w.pollInterval)
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logs.Infof("worker", "deployment worker stopping")
			return
		case <-ticker.C:
			w.processNext(ctx)
		}
	}
}

func (w *DeploymentWorker) processNext(ctx context.Context) {
	startedAt := time.Now()
	finalStatus := "noop"
	defer func() {
		telemetry.ObserveWorkerJob(finalStatus, startedAt)
	}()

	var (
		job domain.DeploymentJob
		err error
	)

	if w.useAssigned {
		job, err = w.jobs.ClaimForWorker(ctx, w.workerID)
	} else {
		job, err = w.jobs.ClaimNext(ctx)
	}
	if err != nil {
		if err == store.ErrNoQueuedJobs {
			finalStatus = "empty"
			logs.Debugf("worker", "no queued jobs")
			return
		}
		finalStatus = "claim_failed"
		logs.Errorf("worker", "claim job failed worker_id=%s assigned_mode=%t err=%v", w.workerID, w.useAssigned, err)
		return
	}

	if w.useAssigned && w.workers != nil {
		defer func() {
			if decErr := w.workers.DecrementJobCount(ctx, w.workerID); decErr != nil {
				logs.Errorf("worker", "failed to decrement worker job count worker_id=%s err=%v", w.workerID, decErr)
				return
			}
			logs.Debugf("worker", "decremented worker job count worker_id=%s job_id=%s", w.workerID, job.JobID)
		}()
	}

	logs.Infof("worker", "processing job worker_id=%s assigned_mode=%t job_id=%s deployment_id=%s assigned_worker_id=%s",
		w.workerID, w.useAssigned, job.JobID, job.DeploymentID, job.AssignedWorkerID)
	rec, getErr := w.repo.Get(job.DeploymentID)
	if getErr != nil {
		finalStatus = "lookup_failed"
		logs.Errorf("worker", "deployment lookup failed deployment_id=%s err=%v", job.DeploymentID, getErr)
		_ = w.jobs.MarkFailed(ctx, job.JobID, getErr.Error())
		return
	}

	rec.Status = "deploying"
	rec.BuildLogs = "=== worker ===\njob claimed, deployment started\n"
	w.repo.Update(rec)

	var liveLogsMu sync.Mutex
	liveBuildLogs := rec.BuildLogs
	lastPersistedLen := len(liveBuildLogs)
	lastPersistAt := time.Now()

	persistLiveLogs := func(force bool) {
		liveLogsMu.Lock()
		currentLogs := liveBuildLogs
		shouldPersist := force || len(currentLogs)-lastPersistedLen >= 2048 || time.Since(lastPersistAt) >= time.Second
		if !shouldPersist {
			liveLogsMu.Unlock()
			return
		}
		lastPersistedLen = len(currentLogs)
		lastPersistAt = time.Now()
		liveLogsMu.Unlock()

		n := rec
		n.Status = "deploying"
		n.OwnerWorkerID = w.workerID
		n.BuildLogs = currentLogs
		w.repo.Update(n)
	}

	appendLiveLog := func(chunk string) {
		if chunk == "" {
			return
		}

		liveLogsMu.Lock()
		liveBuildLogs += chunk
		liveLogsMu.Unlock()

		persistLiveLogs(false)
	}

	result, buildLogs, deployErr := w.runner.DeployRepoWithUpdates(job.Repo, job.DeploymentID, job.Subdomain, job.Port, job.Env, job.BuildArgs, job.CPUCores, job.MemoryMB, appendLiveLog)
	persistLiveLogs(true)
	if deployErr != nil {
		finalStatus = "failed"
		if strings.TrimSpace(buildLogs) == "" {
			liveLogsMu.Lock()
			capturedLogs := liveBuildLogs
			liveLogsMu.Unlock()
			buildLogs = capturedLogs + "\n=== error ===\n" + deployErr.Error() + "\n"
		}
		finished := time.Now().UTC()
		w.repo.Update(domain.DeploymentRecord{
			DeploymentID:  rec.DeploymentID,
			OwnerWorkerID: w.workerID,
			RequestedBy:   rec.RequestedBy,
			UserID:        rec.UserID,
			Package:       rec.Package,
			Repo:          rec.Repo,
			Subdomain:     rec.Subdomain,
			Port:          rec.Port,
			ScalingMode:   rec.ScalingMode,
			MinReplicas:   rec.MinReplicas,
			MaxReplicas:   rec.MaxReplicas,
			CPUTarget:     rec.CPUTarget,
			CPURequest:    rec.CPURequest,
			CPULimit:      rec.CPULimit,
			NodeSelector:  rec.NodeSelector,
			CPUCores:      rec.CPUCores,
			MemoryMB:      rec.MemoryMB,
			Status:        "failed",
			Error:         deployErr.Error(),
			BuildLogs:     buildLogs,
			Env:           rec.Env,
			BuildArgs:     rec.BuildArgs,
			StartedAt:     rec.StartedAt,
			FinishedAt:    &finished,
		})
		_ = w.jobs.MarkFailed(ctx, job.JobID, deployErr.Error())
		logs.Errorf("worker", "deploy failed deployment_id=%s err=%v", rec.DeploymentID, deployErr)
		return
	}

	finished := time.Now().UTC()
	if strings.TrimSpace(buildLogs) == "" {
		liveLogsMu.Lock()
		capturedLogs := liveBuildLogs
		liveLogsMu.Unlock()
		buildLogs = capturedLogs + "\n=== worker ===\ndeployment completed without runtime logs\n"
	}
	hpaBuildLogs := buildLogs
	if w.enableCPUHPA && rec.ScalingMode == ScalingModeHorizontal {
		hpaOutput, hpaErr := w.runner.ApplyCPUAutoscaling(result.Container, rec.MinReplicas, rec.MaxReplicas, rec.CPUTarget)
		if hpaErr != nil {
			logs.Errorf("worker", "hpa apply failed deployment_id=%s err=%v", rec.DeploymentID, hpaErr)
			if strings.TrimSpace(hpaOutput) != "" {
				hpaBuildLogs = hpaBuildLogs + "\n=== hpa output ===\n" + hpaOutput + "\n"
			}
			hpaBuildLogs = hpaBuildLogs + "\n=== hpa warning ===\n" + hpaErr.Error() + "\n"
		} else if strings.TrimSpace(hpaOutput) != "" {
			hpaBuildLogs = hpaBuildLogs + "\n=== hpa output ===\n" + hpaOutput + "\n"
		}
	}

	w.repo.Update(domain.DeploymentRecord{
		DeploymentID:  result.DeploymentID,
		OwnerWorkerID: w.workerID,
		RequestedBy:   rec.RequestedBy,
		UserID:        rec.UserID,
		Package:       rec.Package,
		Repo:          result.Repo,
		Subdomain:     result.Subdomain,
		Port:          result.Port,
		ScalingMode:   rec.ScalingMode,
		MinReplicas:   rec.MinReplicas,
		MaxReplicas:   rec.MaxReplicas,
		CPUTarget:     rec.CPUTarget,
		CPURequest:    rec.CPURequest,
		CPULimit:      rec.CPULimit,
		NodeSelector:  rec.NodeSelector,
		CPUCores:      rec.CPUCores,
		MemoryMB:      rec.MemoryMB,
		Container:     result.Container,
		Image:         result.Image,
		URL:           result.URL,
		Status:        "running",
		BuildLogs:     hpaBuildLogs,
		Env:           rec.Env,
		BuildArgs:     rec.BuildArgs,
		StartedAt:     rec.StartedAt,
		FinishedAt:    &finished,
	})

	_ = w.jobs.MarkDone(ctx, job.JobID)
	finalStatus = "running"
	logs.Infof("worker", "deploy success deployment_id=%s", rec.DeploymentID)
}
