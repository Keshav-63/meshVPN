package httpapi

import (
	"net/http"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/store"

	"github.com/gin-gonic/gin"
)

type PlatformAnalyticsHandler struct {
	deploymentRepo store.DeploymentRepository
	workerRepo     store.WorkerRepository
	jobRepo        store.JobRepository
	analyticsRepo  AnalyticsRepository
}

func NewPlatformAnalyticsHandler(
	deploymentRepo store.DeploymentRepository,
	workerRepo store.WorkerRepository,
	jobRepo store.JobRepository,
	analyticsRepo AnalyticsRepository,
) *PlatformAnalyticsHandler {
	return &PlatformAnalyticsHandler{
		deploymentRepo: deploymentRepo,
		workerRepo:     workerRepo,
		jobRepo:        jobRepo,
		analyticsRepo:  analyticsRepo,
	}
}

// GET /platform/analytics
func (h *PlatformAnalyticsHandler) GetPlatformAnalytics(c *gin.Context) {
	ctx := c.Request.Context()

	// Get all workers
	workers, err := h.workerRepo.List(ctx)
	if err != nil {
		logs.Errorf("platform-analytics", "failed to get workers: %v", err)
		workers = []domain.Worker{}
	}

	// Get all deployments
	deployments := h.deploymentRepo.List()

	// Count deployments by status
	runningCount := 0
	failedCount := 0
	queuedCount := 0
	totalDeployments := len(deployments)

	for _, d := range deployments {
		switch d.Status {
		case "running":
			runningCount++
		case "failed":
			failedCount++
		case "queued":
			queuedCount++
		}
	}

	// Calculate worker stats
	totalWorkers := len(workers)
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
	}

	// Get platform-wide metrics from analytics
	var totalRequests int64
	var totalBandwidthSent int64
	var totalBandwidthReceived int64
	var avgLatencyP50 float64
	var totalPods int

	if h.analyticsRepo != nil {
		// Aggregate metrics across all deployments
		for _, d := range deployments {
			if d.Status == "running" {
				metrics, err := h.analyticsRepo.GetMetrics(d.DeploymentID)
				if err == nil {
					totalRequests += metrics.RequestCountTotal
					totalBandwidthSent += metrics.BandwidthSentBytes
					totalBandwidthReceived += metrics.BandwidthRecvBytes
					totalPods += metrics.CurrentPods

					// Average latency (simple average for now)
					if metrics.LatencyP50Ms > 0 {
						avgLatencyP50 = (avgLatencyP50 + metrics.LatencyP50Ms) / 2
					}
				}
			}
		}
	}

	// Build response
	response := gin.H{
		"platform": gin.H{
			"deployments": gin.H{
				"total":   totalDeployments,
				"running": runningCount,
				"failed":  failedCount,
				"queued":  queuedCount,
			},
			"workers": gin.H{
				"total":   totalWorkers,
				"idle":    idleWorkers,
				"busy":    busyWorkers,
				"offline": offlineWorkers,
			},
			"capacity": gin.H{
				"total":              totalCapacity,
				"used":               usedCapacity,
				"available":          totalCapacity - usedCapacity,
				"utilization_percent": float64(usedCapacity) / float64(totalCapacity) * 100,
			},
			"resources": gin.H{
				"total_pods": totalPods,
			},
			"traffic": gin.H{
				"total_requests":         totalRequests,
				"bandwidth_sent_bytes":   totalBandwidthSent,
				"bandwidth_recv_bytes":   totalBandwidthReceived,
				"avg_latency_p50_ms":     avgLatencyP50,
			},
		},
		"workers": h.getWorkerBreakdown(workers, deployments),
	}

	c.JSON(http.StatusOK, response)
}

// GET /platform/workers/:id/analytics
func (h *PlatformAnalyticsHandler) GetWorkerAnalytics(c *gin.Context) {
	ctx := c.Request.Context()
	workerID := c.Param("id")

	// Get worker
	worker, err := h.workerRepo.Get(ctx, workerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
		return
	}

	// Get deployments assigned to this worker
	deployments := h.deploymentRepo.List()
	workerDeployments := []interface{}{}
	totalPods := 0
	totalRequests := int64(0)

	for _, d := range deployments {
		// Check if this deployment was built by this worker
		// (You'd need to track this in deployment_jobs table)
		// For now, we'll use a placeholder

		if d.Status == "running" {
			if h.analyticsRepo != nil {
				metrics, err := h.analyticsRepo.GetMetrics(d.DeploymentID)
				if err == nil {
					totalPods += metrics.CurrentPods
					totalRequests += metrics.RequestCountTotal

					workerDeployments = append(workerDeployments, gin.H{
						"deployment_id":  d.DeploymentID,
						"subdomain":      d.Subdomain,
						"package":        d.Package,
						"current_pods":   metrics.CurrentPods,
						"request_count":  metrics.RequestCountTotal,
						"cpu_percent":    metrics.CPUUsagePercent,
						"memory_mb":      metrics.MemoryUsageMB,
					})
				}
			}
		}
	}

	response := gin.H{
		"worker": gin.H{
			"worker_id":          worker.WorkerID,
			"name":               worker.Name,
			"tailscale_ip":       worker.TailscaleIP,
			"status":             worker.Status,
			"current_jobs":       worker.CurrentJobs,
			"max_concurrent_jobs": worker.MaxConcurrentJobs,
			"last_heartbeat":     worker.LastHeartbeat,
		},
		"resources": gin.H{
			"total_pods":     totalPods,
			"total_requests": totalRequests,
			"cpu_cores":      worker.Capabilities.CPUCores,
			"memory_gb":      worker.Capabilities.MemoryGB,
		},
		"deployments": workerDeployments,
	}

	c.JSON(http.StatusOK, response)
}

func (h *PlatformAnalyticsHandler) getWorkerBreakdown(workers []domain.Worker, deployments []domain.DeploymentRecord) []gin.H {
	breakdown := []gin.H{}

	for _, w := range workers {
		// Count deployments for this worker
		deploymentCount := 0
		for _, d := range deployments {
			// TODO: Check if deployment was built by this worker
			// This requires querying deployment_jobs table
			_ = d
		}

		breakdown = append(breakdown, gin.H{
			"worker_id":          w.WorkerID,
			"name":               w.Name,
			"status":             w.Status,
			"current_jobs":       w.CurrentJobs,
			"max_concurrent_jobs": w.MaxConcurrentJobs,
			"deployment_count":   deploymentCount,
			"cpu_cores":          w.Capabilities.CPUCores,
			"memory_gb":          w.Capabilities.MemoryGB,
			"last_heartbeat":     w.LastHeartbeat,
		})
	}

	return breakdown
}
