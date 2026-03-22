package service

import (
	"context"
	"strings"
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
	runner       *runtime.Runner
	pollInterval time.Duration
	enableCPUHPA bool
}

func NewDeploymentWorker(repo store.DeploymentRepository, jobs store.JobRepository, runner *runtime.Runner, pollInterval time.Duration, enableCPUHPA bool) *DeploymentWorker {
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	return &DeploymentWorker{repo: repo, jobs: jobs, runner: runner, pollInterval: pollInterval, enableCPUHPA: enableCPUHPA}
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

	job, err := w.jobs.ClaimNext(ctx)
	if err != nil {
		if err == store.ErrNoQueuedJobs {
			finalStatus = "empty"
			logs.Debugf("worker", "no queued jobs")
			return
		}
		finalStatus = "claim_failed"
		logs.Errorf("worker", "claim next job failed err=%v", err)
		return
	}

	logs.Infof("worker", "processing job job_id=%s deployment_id=%s", job.JobID, job.DeploymentID)
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

	result, buildLogs, deployErr := w.runner.DeployRepo(job.Repo, job.DeploymentID, job.Subdomain, job.Port, job.Env, job.BuildArgs, job.CPUCores, job.MemoryMB)
	if deployErr != nil {
		finalStatus = "failed"
		if strings.TrimSpace(buildLogs) == "" {
			buildLogs = rec.BuildLogs + "\n=== error ===\n" + deployErr.Error() + "\n"
		}
		finished := time.Now().UTC()
		w.repo.Update(domain.DeploymentRecord{
			DeploymentID: rec.DeploymentID,
			RequestedBy:  rec.RequestedBy,
			Repo:         rec.Repo,
			Subdomain:    rec.Subdomain,
			Port:         rec.Port,
			ScalingMode:  rec.ScalingMode,
			MinReplicas:  rec.MinReplicas,
			MaxReplicas:  rec.MaxReplicas,
			CPUTarget:    rec.CPUTarget,
			CPURequest:   rec.CPURequest,
			CPULimit:     rec.CPULimit,
			NodeSelector: rec.NodeSelector,
			CPUCores:     rec.CPUCores,
			MemoryMB:     rec.MemoryMB,
			Status:       "failed",
			Error:        deployErr.Error(),
			BuildLogs:    buildLogs,
			Env:          rec.Env,
			BuildArgs:    rec.BuildArgs,
			StartedAt:    rec.StartedAt,
			FinishedAt:   &finished,
		})
		_ = w.jobs.MarkFailed(ctx, job.JobID, deployErr.Error())
		logs.Errorf("worker", "deploy failed deployment_id=%s err=%v", rec.DeploymentID, deployErr)
		return
	}

	finished := time.Now().UTC()
	if strings.TrimSpace(buildLogs) == "" {
		buildLogs = rec.BuildLogs + "\n=== worker ===\ndeployment completed without runtime logs\n"
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
		DeploymentID: result.DeploymentID,
		RequestedBy:  rec.RequestedBy,
		Repo:         result.Repo,
		Subdomain:    result.Subdomain,
		Port:         result.Port,
		ScalingMode:  rec.ScalingMode,
		MinReplicas:  rec.MinReplicas,
		MaxReplicas:  rec.MaxReplicas,
		CPUTarget:    rec.CPUTarget,
		CPURequest:   rec.CPURequest,
		CPULimit:     rec.CPULimit,
		NodeSelector: rec.NodeSelector,
		CPUCores:     rec.CPUCores,
		MemoryMB:     rec.MemoryMB,
		Container:    result.Container,
		Image:        result.Image,
		URL:          result.URL,
		Status:       "running",
		BuildLogs:    hpaBuildLogs,
		Env:          rec.Env,
		BuildArgs:    rec.BuildArgs,
		StartedAt:    rec.StartedAt,
		FinishedAt:   &finished,
	})

	_ = w.jobs.MarkDone(ctx, job.JobID)
	finalStatus = "running"
	logs.Infof("worker", "deploy success deployment_id=%s", rec.DeploymentID)
}
