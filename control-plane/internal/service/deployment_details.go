package service

import (
	"fmt"
	"sync"

	"MeshVPN-slef-hosting/control-plane/internal/analytics"
	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/store"
)

// AnalyticsRepository interface for analytics operations
type AnalyticsRepository interface {
	GetMetrics(deploymentID string) (domain.DeploymentMetrics, error)
	GetDeploymentSummaries(deploymentIDs []string) (map[string]domain.DeploymentMetrics, error)
}

// DeploymentDetailsService aggregates deployment information from multiple sources
type DeploymentDetailsService struct {
	deploymentRepo store.DeploymentRepository
	analyticsRepo  AnalyticsRepository
	k8sClient      *analytics.KubernetesClient
}

// NewDeploymentDetailsService creates a new deployment details service
func NewDeploymentDetailsService(
	deploymentRepo store.DeploymentRepository,
	analyticsRepo AnalyticsRepository,
	k8sClient *analytics.KubernetesClient,
) *DeploymentDetailsService {
	return &DeploymentDetailsService{
		deploymentRepo: deploymentRepo,
		analyticsRepo:  analyticsRepo,
		k8sClient:      k8sClient,
	}
}

// GetDeploymentDetails retrieves comprehensive deployment information
func (s *DeploymentDetailsService) GetDeploymentDetails(deploymentID string) (domain.DeploymentDetails, error) {
	logs.Debugf("deployment-details", "fetching comprehensive details for deployment_id=%s", deploymentID)

	var details domain.DeploymentDetails
	var deployment domain.DeploymentRecord
	var metrics domain.DeploymentMetrics
	var pods []domain.PodMetrics
	var resources domain.ResourceAllocation
	var err1, err2, err3 error

	// Use WaitGroup to fetch data in parallel
	var wg sync.WaitGroup
	wg.Add(3)

	// Fetch deployment record
	go func() {
		defer wg.Done()
		deployment, err1 = s.deploymentRepo.Get(deploymentID)
	}()

	// Fetch analytics metrics
	go func() {
		defer wg.Done()
		if s.analyticsRepo != nil {
			metrics, err2 = s.analyticsRepo.GetMetrics(deploymentID)
		}
	}()

	// Fetch Kubernetes pod metrics and resource allocation
	go func() {
		defer wg.Done()
		if s.k8sClient != nil && deployment.Status == "running" {
			pods, resources, err3 = s.k8sClient.GetPodMetricsWithResources(deploymentID)
		}
	}()

	wg.Wait()

	// Check for critical errors
	if err1 != nil {
		return domain.DeploymentDetails{}, fmt.Errorf("get deployment: %w", err1)
	}

	if err2 != nil {
		logs.Debugf("deployment-details", "failed to get metrics for %s: %v", deploymentID, err2)
		// Continue with empty metrics
	}

	if err3 != nil {
		logs.Debugf("deployment-details", "failed to get k8s metrics for %s: %v", deploymentID, err3)
		// Continue with empty k8s data
	}

	// Build deployment details
	details.Deployment = deployment
	details.Metrics = metrics
	details.Pods = pods
	details.Resources = resources

	// Build scaling info
	details.Scaling = domain.ScalingInfo{
		Mode:        deployment.ScalingMode,
		CurrentPods: metrics.CurrentPods,
		DesiredPods: metrics.DesiredPods,
		MinReplicas: deployment.MinReplicas,
		MaxReplicas: deployment.MaxReplicas,
		CPUTarget:   deployment.CPUTarget,
		HPAEnabled:  deployment.ScalingMode == "horizontal",
	}

	logs.Debugf("deployment-details", "fetched details for deployment_id=%s pods=%d metrics_count=%d",
		deploymentID, len(pods), metrics.RequestCountTotal)

	return details, nil
}

// GetDeploymentSummaries retrieves summary information for multiple deployments
func (s *DeploymentDetailsService) GetDeploymentSummaries(deploymentRecords []domain.DeploymentRecord) ([]domain.DeploymentSummary, error) {
	if len(deploymentRecords) == 0 {
		return []domain.DeploymentSummary{}, nil
	}

	logs.Debugf("deployment-details", "fetching summaries for %d deployments", len(deploymentRecords))

	// Extract deployment IDs
	deploymentIDs := make([]string, len(deploymentRecords))
	for i, dep := range deploymentRecords {
		deploymentIDs[i] = dep.DeploymentID
	}

	// Fetch metrics for all deployments in batch
	metricsMap := make(map[string]domain.DeploymentMetrics)
	if s.analyticsRepo != nil {
		var err error
		metricsMap, err = s.analyticsRepo.GetDeploymentSummaries(deploymentIDs)
		if err != nil {
			logs.Errorf("deployment-details", "failed to get batch metrics: %v", err)
			// Continue with empty metrics
		}
	}

	// Build summaries
	summaries := make([]domain.DeploymentSummary, 0, len(deploymentRecords))
	for _, dep := range deploymentRecords {
		metrics, hasMetrics := metricsMap[dep.DeploymentID]

		summary := domain.DeploymentSummary{
			DeploymentID: dep.DeploymentID,
			Subdomain:    dep.Subdomain,
			URL:          dep.URL,
			Status:       dep.Status,
			Package:      dep.Package,
			StartedAt:    dep.StartedAt,
		}

		if hasMetrics {
			summary.CurrentPods = metrics.CurrentPods
			summary.RequestCount24h = metrics.RequestCount24h
			summary.CPUUsagePercent = metrics.CPUUsagePercent
			summary.MemoryUsageMB = metrics.MemoryUsageMB
			summary.LastUpdated = metrics.LastUpdated
		}

		summaries = append(summaries, summary)
	}

	logs.Debugf("deployment-details", "built %d deployment summaries", len(summaries))
	return summaries, nil
}
