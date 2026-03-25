package analytics

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/store"
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
	analyticsRepo  AnalyticsRepository
	workerRepo     store.WorkerRepository
	deploymentRepo store.DeploymentRepository
	namespace      string
	kubectl        string
}

func NewMetricsCollector(analyticsRepo AnalyticsRepository, workerRepo store.WorkerRepository, deploymentRepo store.DeploymentRepository, namespace, kubectl string) *MetricsCollector {
	if namespace == "" {
		namespace = "meshvpn-apps"
	}
	if kubectl == "" {
		kubectl = "kubectl"
	}

	return &MetricsCollector{
		analyticsRepo:  analyticsRepo,
		workerRepo:     workerRepo,
		deploymentRepo: deploymentRepo,
		namespace:      namespace,
		kubectl:        kubectl,
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

	// Collect platform-level metrics first
	c.aggregatePlatformMetrics(ctx)

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

	// Get CPU/Memory usage from Kubernetes metrics-server.
	cpuPercent, memoryMB, err := c.getDeploymentResourceUsage(deploymentID)
	if err != nil {
		logs.Debugf("analytics-collector", "failed to get resource usage for %s: %v", deploymentID, err)
	}
	metrics.CPUUsagePercent = cpuPercent
	metrics.MemoryUsageMB = memoryMB
	telemetry.UpdateDeploymentResourceUsage(deploymentID, cpuPercent, memoryMB)

	// Store aggregated metrics
	if err := c.analyticsRepo.UpdateMetrics(metrics); err != nil {
		return fmt.Errorf("update metrics: %w", err)
	}

	logs.Debugf("analytics-collector", "updated metrics deployment_id=%s pods=%d/%d requests_1h=%d",
		deploymentID, current, desired, last1h)

	return nil
}

// aggregatePlatformMetrics collects and updates platform-level metrics
func (c *MetricsCollector) aggregatePlatformMetrics(ctx context.Context) {
	// Get all workers
	workers, err := c.workerRepo.List(ctx)
	if err != nil {
		logs.Errorf("analytics-collector", "failed to get workers: %v", err)
		workers = []domain.Worker{}
	}

	// Get all deployments
	deployments := c.deploymentRepo.List()

	// Count workers by status
	idleWorkers := 0
	busyWorkers := 0
	offlineWorkers := 0
	totalCapacity := 0
	usedCapacity := 0

	for _, w := range workers {
		totalCapacity += w.MaxConcurrentJobs
		usedCapacity += w.CurrentJobs

		switch w.Status {
		case "idle":
			idleWorkers++
		case "busy":
			busyWorkers++
		case "offline":
			offlineWorkers++
		}

		// Update per-worker metrics
		telemetry.UpdateWorkerMetrics(
			w.WorkerID,
			w.Name,
			0, // pods - will be calculated from deployments
			w.CurrentJobs,
			w.Capabilities.CPUCores,
			w.Capabilities.MemoryGB,
		)
	}

	// Update platform worker metrics
	telemetry.UpdatePlatformWorkers(idleWorkers, busyWorkers, offlineWorkers)
	telemetry.UpdatePlatformWorkerCapacity(totalCapacity, usedCapacity, totalCapacity-usedCapacity)

	// Count deployments by status
	runningCount := 0
	failedCount := 0
	queuedCount := 0
	totalPods := 0

	for _, d := range deployments {
		switch d.Status {
		case "running":
			runningCount++
		case "failed":
			failedCount++
		case "queued":
			queuedCount++
		}

		// Count pods for running deployments
		if d.Status == "running" {
			current, _, err := c.getPodCounts(d.DeploymentID)
			if err == nil {
				totalPods += current
			}
		}
	}

	// Update platform deployment metrics
	telemetry.UpdatePlatformDeployments(runningCount, failedCount, queuedCount)
	telemetry.UpdatePlatformPodsTotal(totalPods)

	logs.Debugf("analytics-collector", "platform metrics: workers=%d (idle=%d busy=%d offline=%d) deployments=%d (running=%d) pods=%d",
		len(workers), idleWorkers, busyWorkers, offlineWorkers, len(deployments), runningCount, totalPods)
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

// getDeploymentResourceUsage aggregates CPU and memory usage for a deployment.
// CPU is reported as percentage of requested CPU; memory is reported in MB.
func (c *MetricsCollector) getDeploymentResourceUsage(deploymentID string) (cpuPercent, memoryMB float64, err error) {
	deploymentName := "app-" + deploymentID

	output, err := runCommand(c.kubectl, "-n", c.namespace, "top", "pod", "-l", "app="+deploymentName, "--no-headers")
	if err != nil {
		return 0, 0, fmt.Errorf("kubectl top pod: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return 0, 0, nil
	}

	var totalMilliCPU int64
	var totalMemoryMB float64
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		milli, err := parseCPUToMilli(fields[1])
		if err == nil {
			totalMilliCPU += milli
		}

		mb, err := parseMemoryToMB(fields[2])
		if err == nil {
			totalMemoryMB += mb
		}
	}

	requestedMilliCPU, err := c.getRequestedCPUMilli(deploymentName)
	if err != nil {
		logs.Debugf("analytics-collector", "failed to get requested CPU for %s: %v", deploymentID, err)
	}

	if requestedMilliCPU > 0 {
		cpuPercent = (float64(totalMilliCPU) / float64(requestedMilliCPU)) * 100.0
	}

	return cpuPercent, totalMemoryMB, nil
}

func (c *MetricsCollector) getRequestedCPUMilli(deploymentName string) (int64, error) {
	output, err := runCommand(
		c.kubectl,
		"-n", c.namespace,
		"get", "deployment", deploymentName,
		"-o", "jsonpath={.spec.template.spec.containers[0].resources.requests.cpu}",
	)
	if err != nil {
		return 0, err
	}

	value := strings.TrimSpace(output)
	if value == "" {
		return 0, nil
	}

	return parseCPUToMilli(value)
}

func parseCPUToMilli(value string) (int64, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return 0, fmt.Errorf("empty cpu value")
	}

	if strings.HasSuffix(v, "m") {
		n, err := strconv.ParseInt(strings.TrimSuffix(v, "m"), 10, 64)
		if err != nil {
			return 0, err
		}
		return n, nil
	}

	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, err
	}
	return int64(f * 1000), nil
}

func parseMemoryToMB(value string) (float64, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return 0, fmt.Errorf("empty memory value")
	}

	re := regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)([A-Za-z]+)?$`)
	m := re.FindStringSubmatch(v)
	if len(m) != 3 {
		return 0, fmt.Errorf("invalid memory value: %s", value)
	}

	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, err
	}

	unit := strings.ToLower(m[2])
	switch unit {
	case "", "m":
		return n / 1_000_000, nil
	case "ki":
		return n / 1024, nil
	case "mi":
		return n, nil
	case "gi":
		return n * 1024, nil
	case "ti":
		return n * 1024 * 1024, nil
	case "k":
		return n / 1_000_000, nil
	case "g":
		return n * 1000, nil
	default:
		return 0, fmt.Errorf("unsupported memory unit: %s", unit)
	}
}

func runCommand(command string, args ...string) (string, error) {
	cmd := exec.Command(command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return string(output), nil
}
