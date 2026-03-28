package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/service"

	"github.com/gin-gonic/gin"
)

type AnalyticsRepository interface {
	GetMetrics(deploymentID string) (domain.DeploymentMetrics, error)
	RecordRequest(req domain.DeploymentRequest) error
}

type AnalyticsHandler struct {
	deploymentService *service.DeploymentService
	analyticsRepo     AnalyticsRepository
}

func NewAnalyticsHandler(deploymentService *service.DeploymentService, analyticsRepo AnalyticsRepository) *AnalyticsHandler {
	return &AnalyticsHandler{
		deploymentService: deploymentService,
		analyticsRepo:     analyticsRepo,
	}
}

// GetAnalytics returns current metrics for a deployment (GET /deployments/:id/analytics)
func (h *AnalyticsHandler) GetAnalytics(c *gin.Context) {
	deploymentID := c.Param("id")

	// Verify deployment exists and user has access
	deployment, err := h.deploymentService.GetDeployment(deploymentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}

	// Check ownership if user context available
	if userVal, exists := c.Get("auth.user"); exists {
		user := userVal.(domain.User)
		if deployment.UserID != "" && deployment.UserID != user.UserID {
			logs.Errorf("analytics", "unauthorized access attempt user=%s deployment=%s owner=%s",
				user.UserID, deploymentID, deployment.UserID)
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
	}

	// Get metrics from analytics repository
	metrics, err := h.analyticsRepo.GetMetrics(deploymentID)
	if err != nil {
		logs.Errorf("analytics", "failed to get metrics deployment_id=%s: %v", deploymentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load metrics"})
		return
	}

	// Build response
	response := gin.H{
		"deployment_id": deploymentID,
		"deployment": gin.H{
			"repo":          deployment.Repo,
			"subdomain":     deployment.Subdomain,
			"url":           deployment.URL,
			"package":       deployment.Package,
			"status":        deployment.Status,
			"scaling_mode":  deployment.ScalingMode,
			"min_replicas":  deployment.MinReplicas,
			"max_replicas":  deployment.MaxReplicas,
			"started_at":    deployment.StartedAt,
		},
		"metrics": gin.H{
			"requests": gin.H{
				"total":       metrics.RequestCountTotal,
				"last_hour":   metrics.RequestCount1h,
				"last_24h":    metrics.RequestCount24h,
				"per_second":  metrics.RequestsPerSecond,
			},
			"latency": gin.H{
				"p50_ms": metrics.LatencyP50Ms,
				"p90_ms": metrics.LatencyP90Ms,
				"p99_ms": metrics.LatencyP99Ms,
			},
			"bandwidth": gin.H{
				"sent_bytes":     metrics.BandwidthSentBytes,
				"received_bytes": metrics.BandwidthRecvBytes,
			},
			"pods": gin.H{
				"current": metrics.CurrentPods,
				"desired": metrics.DesiredPods,
			},
			"resources": gin.H{
				"cpu_usage_percent": metrics.CPUUsagePercent,
				"memory_usage_mb":   metrics.MemoryUsageMB,
			},
			"last_updated": metrics.LastUpdated,
		},
	}

	c.JSON(http.StatusOK, response)
}

// StreamAnalytics provides real-time metrics via Server-Sent Events (GET /deployments/:id/analytics/stream)
func (h *AnalyticsHandler) StreamAnalytics(c *gin.Context) {
	deploymentID := c.Param("id")

	// Verify deployment exists and user has access
	deployment, err := h.deploymentService.GetDeployment(deploymentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}

	// Check ownership if user context available
	if userVal, exists := c.Get("auth.user"); exists {
		user := userVal.(domain.User)
		if deployment.UserID != "" && deployment.UserID != user.UserID {
			logs.Errorf("analytics", "unauthorized SSE access attempt user=%s deployment=%s owner=%s",
				user.UserID, deploymentID, deployment.UserID)
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
	}

	logs.Infof("analytics", "SSE stream started deployment_id=%s", deploymentID)

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")

	// Create ticker for periodic updates (every 5 seconds)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Send initial data immediately
	h.sendMetricsSSE(c, deploymentID)

	for {
		select {
		case <-c.Request.Context().Done():
			logs.Infof("analytics", "SSE stream closed deployment_id=%s", deploymentID)
			return
		case <-ticker.C:
			h.sendMetricsSSE(c, deploymentID)
		}
	}
}

// sendMetricsSSE sends a single metrics update via SSE
func (h *AnalyticsHandler) sendMetricsSSE(c *gin.Context, deploymentID string) {
	metrics, err := h.analyticsRepo.GetMetrics(deploymentID)
	if err != nil {
		logs.Errorf("analytics", "SSE metrics fetch failed deployment_id=%s: %v", deploymentID, err)
		// Send error event
		fmt.Fprintf(c.Writer, "event: error\ndata: {\"error\": \"failed to fetch metrics\"}\n\n")
		c.Writer.Flush()
		return
	}

	// Build metrics payload
	payload := gin.H{
		"deployment_id": deploymentID,
		"timestamp":     time.Now().Unix(),
		"requests": gin.H{
			"total":      metrics.RequestCountTotal,
			"last_hour":  metrics.RequestCount1h,
			"last_24h":   metrics.RequestCount24h,
			"per_second": metrics.RequestsPerSecond,
		},
		"latency": gin.H{
			"p50_ms": metrics.LatencyP50Ms,
			"p90_ms": metrics.LatencyP90Ms,
			"p99_ms": metrics.LatencyP99Ms,
		},
		"bandwidth": gin.H{
			"sent_bytes":     metrics.BandwidthSentBytes,
			"received_bytes": metrics.BandwidthRecvBytes,
		},
		"pods": gin.H{
			"current": metrics.CurrentPods,
			"desired": metrics.DesiredPods,
		},
		"resources": gin.H{
			"cpu_usage_percent": metrics.CPUUsagePercent,
			"memory_usage_mb":   metrics.MemoryUsageMB,
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(payload)
	if err != nil {
		logs.Errorf("analytics", "SSE marshal failed: %v", err)
		return
	}

	// Send SSE message
	fmt.Fprintf(c.Writer, "data: %s\n\n", string(data))
	c.Writer.Flush()

	logs.Debugf("analytics", "SSE update sent deployment_id=%s pods=%d/%d requests_1h=%d",
		deploymentID, metrics.CurrentPods, metrics.DesiredPods, metrics.RequestCount1h)
}
