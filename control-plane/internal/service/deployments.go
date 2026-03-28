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
	Package      string
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
	UserID       string
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

	// Generate subdomain (auto-extract from repo or use user-provided)
	subdomain, subErr := s.GenerateSubdomain(repoURL, req.Subdomain)
	if subErr != nil {
		return domain.DeploymentRecord{}, subErr
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
		UserID:       strings.TrimSpace(req.UserID),
		Package:      strings.TrimSpace(req.Package),
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

func (s *DeploymentService) ListDeploymentsByUser(userID string) []domain.DeploymentRecord {
	logs.Debugf("service", "listing deployments for user=%s", userID)
	return s.repo.ListByUserID(userID)
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

// extractRepoName extracts the repository name from a GitHub URL
// Examples:
//   - https://github.com/user/my-app.git -> my-app
//   - git@github.com:user/my-app.git -> my-app
//   - https://github.com/user/my-app -> my-app
func extractRepoName(repoURL string) string {
	repoURL = strings.TrimSpace(repoURL)

	// Remove .git suffix
	repoURL = strings.TrimSuffix(repoURL, ".git")

	// Extract last part of path
	parts := strings.Split(repoURL, "/")
	if len(parts) == 0 {
		return ""
	}

	repoName := parts[len(parts)-1]

	// For SSH URLs like git@github.com:user/repo
	if strings.Contains(repoName, ":") {
		colonParts := strings.Split(repoName, ":")
		if len(colonParts) > 1 {
			repoName = colonParts[len(colonParts)-1]
		}
	}

	return sanitizeSubdomain(repoName)
}

// sanitizeSubdomain converts a string to valid subdomain format
// - Lowercase
// - Replace underscores and spaces with hyphens
// - Remove invalid characters
// - Ensure starts with alphanumeric
func sanitizeSubdomain(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))

	// Replace underscores and spaces with hyphens
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, " ", "-")

	// Remove non-alphanumeric characters except hyphens
	var result strings.Builder
	for _, char := range name {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			result.WriteRune(char)
		}
	}

	subdomain := result.String()

	// Remove leading/trailing hyphens
	subdomain = strings.Trim(subdomain, "-")

	// Ensure starts with alphanumeric
	if len(subdomain) > 0 && subdomain[0] == '-' {
		subdomain = subdomain[1:]
	}

	// Limit length to 63 characters (DNS subdomain limit)
	if len(subdomain) > 63 {
		subdomain = subdomain[:63]
	}

	return subdomain
}

// generateShortID generates a short random ID for subdomain suffix
func generateShortID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")[:6]
}

// isSubdomainUnique checks if subdomain is already in use
func (s *DeploymentService) isSubdomainUnique(subdomain string) bool {
	deployments := s.repo.List()
	for _, d := range deployments {
		if d.Subdomain == subdomain && d.Status != "failed" {
			return false
		}
	}
	return true
}

// GenerateSubdomain generates a unique subdomain from repo URL or uses user-provided
func (s *DeploymentService) GenerateSubdomain(repoURL, userProvided string) (string, error) {
	userProvided = strings.TrimSpace(userProvided)

	// If user explicitly provided subdomain, validate and use it
	if userProvided != "" {
		subdomain := sanitizeSubdomain(userProvided)
		if subdomain == "" {
			return "", fmt.Errorf("invalid subdomain format: %s", userProvided)
		}

		if !s.isSubdomainUnique(subdomain) {
			return "", fmt.Errorf("subdomain '%s' is already in use", subdomain)
		}

		return subdomain, nil
	}

	// Auto-generate from repo name
	subdomain := extractRepoName(repoURL)
	if subdomain == "" {
		return "", fmt.Errorf("could not extract subdomain from repo URL")
	}

	// Check if unique, append random suffix if conflict
	if !s.isSubdomainUnique(subdomain) {
		originalSubdomain := subdomain
		subdomain = fmt.Sprintf("%s-%s", subdomain, generateShortID())
		logs.Infof("service", "subdomain conflict, using %s instead of %s", subdomain, originalSubdomain)
	}

	return subdomain, nil
}
