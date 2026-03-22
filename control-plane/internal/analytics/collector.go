package analytics

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/telemetry"
)

type AnalyticsRepository interface {
	GetAllActiveDeploymentIDs() ([]string, error)
	GetRequestCounts(deploymentID string) (int64, int64, int64, error)
	GetBandwidthStats(deploymentID string) (int64, int64, error)
	CalculatePercentiles(deploymentID string, duration time.Duration) (float64, float64, float64, error)
	UpdateMetrics(metrics domain.DeploymentMetrics) error
	CleanupOldRequests(olderThan time.Time) error
}

type MetricsCollector struct {
	analyticsRepo AnalyticsRepository
	namespace     string
	kubectl       string
}

func NewMetricsCollector(analyticsRepo AnalyticsRepository, namespace, kubectl string) *MetricsCollector {
	if namespace == "" {
		namespace = "meshvpn-apps"
	}
	if kubectl == "" {
		kubectl = "kubectl"
	}

	return &MetricsCollector{
		analyticsRepo: analyticsRepo,
		namespace:     namespace,
		kubectl:       kubectl,
	}
}

// Start begins the metrics collection loop
func (c *MetricsCollector) Start(ctx context.Context, interval time.Duration) {
	logs.Infof("analytics-collector", "starting metrics collector interval=%s", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run immediately on start
	c.Aggregate(ctx)

	for {
		select {
		case <-ctx.Done():
			logs.Infof("analytics-collector", "stopping metrics collector")
			return
		case <-ticker.C:
			c.Aggregate(ctx)
		}
	}
}

// Aggregate collects and updates metrics for all active deployments
func (c *MetricsCollector) Aggregate(ctx context.Context) {
	logs.Debugf("analytics-collector", "aggregating metrics")

	// Get all active deployments
	deploymentIDs, err := c.analyticsRepo.GetAllActiveDeploymentIDs()
	if err != nil {
		logs.Errorf("analytics-collector", "failed to get active deployments: %v", err)
		return
	}

	if len(deploymentIDs) == 0 {
		logs.Debugf("analytics-collector", "no active deployments to process")
		return
	}

	logs.Infof("analytics-collector", "processing %d active deployments", len(deploymentIDs))

	for _, deploymentID := range deploymentIDs {
		if err := c.aggregateDeployment(ctx, deploymentID); err != nil {
			logs.Errorf("analytics-collector", "failed to aggregate deployment_id=%s: %v", deploymentID, err)
		}
	}

	// Cleanup old request logs (keep last 7 days)
	sevenDaysAgo := time.Now().Add(-7 * 24 * time.Hour)
	if err := c.analyticsRepo.CleanupOldRequests(sevenDaysAgo); err != nil {
		logs.Errorf("analytics-collector", "cleanup failed: %v", err)
	}
}

// aggregateDeployment collects metrics for a single deployment
func (c *MetricsCollector) aggregateDeployment(ctx context.Context, deploymentID string) error {
	metrics := domain.DeploymentMetrics{
		DeploymentID: deploymentID,
		LastUpdated:  time.Now(),
	}

	// Get pod counts from Kubernetes
	current, desired, err := c.getPodCounts(deploymentID)
	if err != nil {
		logs.Debugf("analytics-collector", "failed to get pod counts for %s: %v", deploymentID, err)
		// Continue with zeros if K8s query fails
	}
	metrics.CurrentPods = current
	metrics.DesiredPods = desired

	// Update Prometheus metrics
	telemetry.UpdateDeploymentPods(deploymentID, current, desired)

	// Get request counts from database
	total, last1h, last24h, err := c.analyticsRepo.GetRequestCounts(deploymentID)
	if err != nil {
		logs.Debugf("analytics-collector", "failed to get request counts for %s: %v", deploymentID, err)
	}
	metrics.RequestCountTotal = total
	metrics.RequestCount1h = last1h
	metrics.RequestCount24h = last24h

	// Calculate requests per second (based on last hour)
	if last1h > 0 {
		metrics.RequestsPerSecond = float64(last1h) / 3600.0
	}

	// Get bandwidth stats
	sent, received, err := c.analyticsRepo.GetBandwidthStats(deploymentID)
	if err != nil {
		logs.Debugf("analytics-collector", "failed to get bandwidth for %s: %v", deploymentID, err)
	}
	metrics.BandwidthSentBytes = sent
	metrics.BandwidthRecvBytes = received

	// Calculate latency percentiles (last hour)
	p50, p90, p99, err := c.analyticsRepo.CalculatePercentiles(deploymentID, 1*time.Hour)
	if err != nil {
		logs.Debugf("analytics-collector", "failed to calculate percentiles for %s: %v", deploymentID, err)
	}
	metrics.LatencyP50Ms = p50
	metrics.LatencyP90Ms = p90
	metrics.LatencyP99Ms = p99

	// TODO: Get CPU/Memory usage from Kubernetes metrics-server
	// This would require querying the metrics API
	// For now, we'll leave these as zero

	// Store aggregated metrics
	if err := c.analyticsRepo.UpdateMetrics(metrics); err != nil {
		return fmt.Errorf("update metrics: %w", err)
	}

	logs.Debugf("analytics-collector", "updated metrics deployment_id=%s pods=%d/%d requests_1h=%d",
		deploymentID, current, desired, last1h)

	return nil
}

// getPodCounts queries Kubernetes for current and desired pod counts
func (c *MetricsCollector) getPodCounts(deploymentID string) (current, desired int, err error) {
	deploymentName := "app-" + deploymentID

	// Get desired replicas from deployment spec
	cmd := exec.Command(c.kubectl, "-n", c.namespace, "get", "deployment", deploymentName,
		"-o", "jsonpath={.spec.replicas}")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("get deployment replicas: %w", err)
	}

	desiredStr := strings.TrimSpace(string(output))
	if desiredStr != "" {
		desired, _ = strconv.Atoi(desiredStr)
	}

	// Get current ready replicas
	cmd = exec.Command(c.kubectl, "-n", c.namespace, "get", "deployment", deploymentName,
		"-o", "jsonpath={.status.readyReplicas}")
	output, err = cmd.Output()
	if err != nil {
		return 0, desired, fmt.Errorf("get ready replicas: %w", err)
	}

	currentStr := strings.TrimSpace(string(output))
	if currentStr != "" {
		current, _ = strconv.Atoi(currentStr)
	}

	return current, desired, nil
}
