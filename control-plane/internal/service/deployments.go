package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/runtime"
	"MeshVPN-slef-hosting/control-plane/internal/store"
	"MeshVPN-slef-hosting/control-plane/internal/telemetry"

	"github.com/google/uuid"
)

var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type DeployRequest struct {
	Repo         string
	Port         int
	Subdomain    string
	ScalingMode  string
	MinReplicas  int
	MaxReplicas  int
	CPUTarget    int
	CPURequest   int
	CPULimit     int
	NodeSelector map[string]string
	Env          map[string]string
	BuildArgs    map[string]string
	CPUCores     float64
	MemoryMB     int
	RequestedBy  string
}

type DeploymentService struct {
	repo   store.DeploymentRepository
	jobs   store.JobRepository
	runner *runtime.Runner
}

func NewDeploymentService(repo store.DeploymentRepository, jobs store.JobRepository, runner *runtime.Runner) *DeploymentService {
	return &DeploymentService{
		repo:   repo,
		jobs:   jobs,
		runner: runner,
	}
}

func (s *DeploymentService) EnqueueDeploy(ctx context.Context, req DeployRequest) (domain.DeploymentRecord, error) {
	logs.Debugf("service", "enqueue deployment requested_by=%s repo=%s", req.RequestedBy, req.Repo)
	policy := NewCPUFirstAutoscalingPolicy()
	normalizedReq, err := policy.Normalize(req)
	if err != nil {
		return domain.DeploymentRecord{}, err
	}
	req = normalizedReq

	repoURL := strings.TrimSpace(req.Repo)
	if repoURL == "" {
		return domain.DeploymentRecord{}, fmt.Errorf("repo is required")
	}

	port := req.Port
	if port == 0 {
		port = 3000
	}

	deploymentID := strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	subdomain := strings.TrimSpace(req.Subdomain)
	if subdomain == "" {
		subdomain = "app-" + deploymentID
	}

	runtimeEnv, err := sanitizeEnvMap(req.Env)
	if err != nil {
		return domain.DeploymentRecord{}, err
	}

	buildArgs, err := sanitizeEnvMap(req.BuildArgs)
	if err != nil {
		return domain.DeploymentRecord{}, fmt.Errorf("invalid build_args: %w", err)
	}

	if req.CPUCores < 0 || req.MemoryMB < 0 {
		return domain.DeploymentRecord{}, fmt.Errorf("cpu and memory must be positive")
	}

	nodeSelector, err := sanitizeEnvMap(req.NodeSelector)
	if err != nil {
		return domain.DeploymentRecord{}, fmt.Errorf("invalid node_selector: %w", err)
	}

	start := time.Now().UTC()
	record := domain.DeploymentRecord{
		DeploymentID: deploymentID,
		RequestedBy:  strings.TrimSpace(req.RequestedBy),
		Repo:         repoURL,
		Subdomain:    subdomain,
		Port:         port,
		ScalingMode:  req.ScalingMode,
		MinReplicas:  req.MinReplicas,
		MaxReplicas:  req.MaxReplicas,
		CPUTarget:    req.CPUTarget,
		CPURequest:   req.CPURequest,
		CPULimit:     req.CPULimit,
		NodeSelector: domain.CloneStringMap(nodeSelector),
		CPUCores:     req.CPUCores,
		MemoryMB:     req.MemoryMB,
		Status:       "queued",
		Env:          domain.CloneStringMap(runtimeEnv),
		BuildArgs:    domain.CloneStringMap(buildArgs),
		StartedAt:    start,
	}
	s.repo.Start(record)

	job := domain.DeploymentJob{
		JobID:        strings.ReplaceAll(uuid.NewString(), "-", "")[:12],
		DeploymentID: deploymentID,
		Repo:         repoURL,
		Subdomain:    subdomain,
		Port:         port,
		ScalingMode:  req.ScalingMode,
		MinReplicas:  req.MinReplicas,
		MaxReplicas:  req.MaxReplicas,
		CPUTarget:    req.CPUTarget,
		CPURequest:   req.CPURequest,
		CPULimit:     req.CPULimit,
		NodeSelector: domain.CloneStringMap(nodeSelector),
		Env:          domain.CloneStringMap(runtimeEnv),
		BuildArgs:    domain.CloneStringMap(buildArgs),
		CPUCores:     req.CPUCores,
		MemoryMB:     req.MemoryMB,
		RequestedBy:  strings.TrimSpace(req.RequestedBy),
		QueuedAt:     start,
	}

	if err := s.jobs.Enqueue(ctx, job); err != nil {
		logs.Errorf("service", "enqueue failed deployment_id=%s err=%v", deploymentID, err)
		return domain.DeploymentRecord{}, fmt.Errorf("enqueue deployment: %w", err)
	}

	telemetry.ObserveDeployRequest(req.ScalingMode)
	logs.Infof("service", "enqueued deployment deployment_id=%s job_id=%s", deploymentID, job.JobID)
	return record, nil
}

func (s *DeploymentService) ListDeployments() []domain.DeploymentRecord {
	logs.Debugf("service", "listing deployments")
	return s.repo.List()
}

func (s *DeploymentService) GetDeployment(id string) (domain.DeploymentRecord, error) {
	logs.Debugf("service", "get deployment id=%s", id)
	return s.repo.Get(strings.TrimSpace(id))
}

func (s *DeploymentService) GetAppLogs(id string, tail int) (domain.DeploymentRecord, string, error) {
	logs.Debugf("service", "get app logs id=%s tail=%d", id, tail)
	rec, err := s.GetDeployment(id)
	if err != nil {
		return domain.DeploymentRecord{}, "", err
	}

	if strings.TrimSpace(rec.Container) == "" {
		return domain.DeploymentRecord{}, "", fmt.Errorf("deployment has no running container")
	}

	logs, logErr := s.runner.ContainerLogs(rec.Container, tail)
	if logErr != nil {
		return rec, logs, logErr
	}

	return rec, logs, nil
}

func sanitizeEnvMap(values map[string]string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	sanitized := make(map[string]string, len(values))
	for key, value := range values {
		trimmedKey := strings.TrimSpace(key)
		if !envKeyPattern.MatchString(trimmedKey) {
			return nil, fmt.Errorf("invalid env var name: %s", key)
		}
		sanitized[trimmedKey] = value
	}

	return sanitized, nil
}
